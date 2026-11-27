package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/soheilhy/cmux"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"

	"pulse/internal/agentproto"
	"pulse/internal/checker"
	"pulse/internal/config"
	"pulse/internal/notify"
	"pulse/internal/store"
	"pulse/internal/xray"
	"pulse/web"
)

type Config struct {
	Addr             string
	DBPath           string
	Version          string
	EnvTelegramToken string
	EnvTelegramChat  string
	EnvCFToken       string
	EnvWebhookURL    string
	EnvPublicBaseURL string
	EnvXrayBin       string
}

type App struct {
	cfg    Config
	db     *store.Store
	logger *slog.Logger
	auth   *authManager
	hub    *Hub
	probes *checker.Manager
	xray   *xray.Engine
	links  *agentLinkHub
	logs   *logSink
	grpc   *grpc.Server

	confMu sync.RWMutex
	conf   *config.Settings

	start time.Time

	runMu   sync.Mutex
	running map[int64]bool
	slots   chan struct{}

	notifyMu   sync.Mutex
	notifyLast map[string]time.Time

	presenceMu sync.Mutex
	linkSeen   map[int64]bool
	restSeen   map[int64]int64
}

func New(cfg Config, logger *slog.Logger) (*App, error) {
	if logger == nil {
		logger = slog.Default()
	}
	db, err := store.Open(cfg.DBPath)
	if err != nil {
		return nil, err
	}
	conf, err := config.Load(db, db)
	if err != nil {
		db.Close()
		return nil, err
	}
	conf.ApplyEnv(cfg.EnvTelegramToken, cfg.EnvTelegramChat, cfg.EnvCFToken, cfg.EnvWebhookURL)
	if v := strings.TrimSpace(cfg.EnvPublicBaseURL); v != "" && strings.TrimSpace(conf.PublicBaseURL) == "" {
		conf.PublicBaseURL = strings.TrimRight(v, "/")
	}
	if v := strings.TrimSpace(cfg.EnvXrayBin); v != "" {
		conf.XrayBinary = v
	}
	if conf.VAPIDPublic == "" || conf.VAPIDPrivate == "" {
		priv, pub, err := webpush.GenerateVAPIDKeys()
		if err != nil {
			db.Close()
			return nil, fmt.Errorf("generate VAPID keys: %w", err)
		}
		conf.VAPIDPrivate, conf.VAPIDPublic = priv, pub
		if err := db.SetSettings(map[string]string{"vapid_private": priv, "vapid_public": pub}); err != nil {
			db.Close()
			return nil, err
		}
	}
	probes := checker.New(conf.DNSResolver)
	xrayEngine := xray.NewEngine(conf.XrayBinary)
	detectCtx, detectCancel := context.WithTimeout(context.Background(), 6*time.Second)
	xrayStatus := xrayEngine.Detect(detectCtx)
	detectCancel()
	probes.Xray = xrayEngine
	probes.SetProxyTestURL(conf.XrayTestURL)
	logSink := newLogSink(db)
	logger = slog.New(newDualHandler(logger.Handler(), logSink))
	a := &App{
		cfg:        cfg,
		db:         db,
		logger:     logger,
		auth:       newAuth(conf.SessionSecret, db),
		hub:        newHub(),
		probes:     probes,
		xray:       xrayEngine,
		links:      newAgentLinkHub(),
		logs:       logSink,
		conf:       conf,
		start:      time.Now(),
		running:    map[int64]bool{},
		slots:      make(chan struct{}, 20),
		notifyLast: map[string]time.Time{},
		linkSeen:   map[int64]bool{},
		restSeen:   map[int64]int64{},
	}
	a.grpc = grpc.NewServer(
		grpc.MaxRecvMsgSize(16<<20),
		grpc.MaxSendMsgSize(16<<20),
		grpc.KeepaliveParams(keepalive.ServerParameters{Time: 30 * time.Second, Timeout: 10 * time.Second}),
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{MinTime: 5 * time.Second, PermitWithoutStream: true}),
	)
	agentproto.RegisterServer(a.grpc, a)
	a.migrateCFAccounts(conf)
	a.logger.Info("panel initialized", "addr", cfg.Addr, "db", cfg.DBPath)
	a.logFor("xray").Info("xray status", "available", xrayStatus.Available,
		"version", xrayStatus.Version, "path", xrayStatus.Path, "error", xrayStatus.Error)
	return a, nil
}

func (a *App) Close() error {
	if a.logs != nil {
		a.logs.close()
	}
	return a.db.Close()
}

func (a *App) config() *config.Settings {
	a.confMu.RLock()
	defer a.confMu.RUnlock()
	return a.conf
}

func (a *App) setConfig(c *config.Settings) {
	a.confMu.Lock()
	a.conf = c
	a.confMu.Unlock()
}

func (a *App) pushSender() *notify.PushSender {
	c := a.config()
	subject := strings.TrimSpace(c.PublicBaseURL)
	if !strings.HasPrefix(subject, "https://") {
		subject = "mailto:admin@example.com"
	}
	return &notify.PushSender{PublicKey: c.VAPIDPublic, PrivateKey: c.VAPIDPrivate, Subject: subject}
}

func (a *App) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/health", a.handleHealth)
	mux.HandleFunc("GET /api/auth/state", a.handleAuthState)
	mux.HandleFunc("POST /api/auth/setup", a.handleSetup)
	mux.HandleFunc("POST /api/auth/login", a.handleLogin)
	mux.HandleFunc("POST /api/auth/logout", a.handleLogout)
	mux.HandleFunc("GET /api/agent/hello", a.handleAgentHello)
	mux.HandleFunc("POST /api/agent/report", a.handleAgentReport)
	mux.HandleFunc("GET /api/agent/link", a.handleAgentLinkWS)

	mux.HandleFunc("GET /api/stream", a.requireAuth(a.handleStream))
	mux.HandleFunc("GET /api/summary", a.requireAuth(a.handleSummary))

	mux.HandleFunc("GET /api/monitors", a.requireAuth(a.handleListMonitors))
	mux.HandleFunc("POST /api/monitors", a.requireAuth(a.handleCreateMonitor))
	mux.HandleFunc("GET /api/monitors/{id}", a.requireAuth(a.handleGetMonitor))
	mux.HandleFunc("PUT /api/monitors/{id}", a.requireAuth(a.handleUpdateMonitor))
	mux.HandleFunc("DELETE /api/monitors/{id}", a.requireAuth(a.handleDeleteMonitor))
	mux.HandleFunc("POST /api/monitors/{id}/toggle", a.requireAuth(a.handleToggleMonitor))
	mux.HandleFunc("POST /api/monitors/{id}/check", a.requireAuth(a.handleCheckNow))
	mux.HandleFunc("GET /api/monitors/{id}/checks", a.requireAuth(a.handleMonitorChecks))

	mux.HandleFunc("GET /api/incidents", a.requireAuth(a.handleIncidents))
	mux.HandleFunc("POST /api/incidents/{id}/resolve", a.requireAuth(a.handleResolveIncident))

	mux.HandleFunc("GET /api/rules", a.requireAuth(a.handleListRules))
	mux.HandleFunc("POST /api/rules", a.requireAuth(a.handleCreateRule))
	mux.HandleFunc("PUT /api/rules/{id}", a.requireAuth(a.handleUpdateRule))
	mux.HandleFunc("DELETE /api/rules/{id}", a.requireAuth(a.handleDeleteRule))

	mux.HandleFunc("GET /api/agents", a.requireAuth(a.handleListAgents))
	mux.HandleFunc("POST /api/agents", a.requireAuth(a.handleCreateAgent))
	mux.HandleFunc("GET /api/agents/{id}/metrics", a.requireAuth(a.handleAgentMetrics))
	mux.HandleFunc("POST /api/agents/{id}/rotate", a.requireAuth(a.handleRotateAgent))
	mux.HandleFunc("POST /api/agents/{id}/command", a.requireAuth(a.handleAgentCommand))
	mux.HandleFunc("GET /api/geo", a.requireAuth(a.handleGeoLookup))
	mux.HandleFunc("PUT /api/agents/{id}", a.requireAuth(a.handleUpdateAgent))
	mux.HandleFunc("DELETE /api/agents/{id}", a.requireAuth(a.handleDeleteAgent))

	mux.HandleFunc("GET /api/cloudflare/status", a.requireAuth(a.handleCFStatus))
	mux.HandleFunc("GET /api/cloudflare/accounts", a.requireAuth(a.handleCFAccounts))
	mux.HandleFunc("POST /api/cloudflare/accounts", a.requireAuth(a.handleCFCreateAccount))
	mux.HandleFunc("PUT /api/cloudflare/accounts/{id}", a.requireAuth(a.handleCFUpdateAccount))
	mux.HandleFunc("DELETE /api/cloudflare/accounts/{id}", a.requireAuth(a.handleCFDeleteAccount))
	mux.HandleFunc("GET /api/cloudflare/zones", a.requireAuth(a.handleCFZones))
	mux.HandleFunc("GET /api/cloudflare/records", a.requireAuth(a.handleCFRecords))
	mux.HandleFunc("POST /api/cloudflare/records", a.requireAuth(a.handleCFCreateRecord))
	mux.HandleFunc("PUT /api/cloudflare/records/{id}", a.requireAuth(a.handleCFUpdateRecord))
	mux.HandleFunc("DELETE /api/cloudflare/records/{id}", a.requireAuth(a.handleCFDeleteRecord))
	mux.HandleFunc("POST /api/cloudflare/update", a.requireAuth(a.handleCFUpdate))

	mux.HandleFunc("GET /api/settings", a.requireAuth(a.handleGetSettings))
	mux.HandleFunc("PUT /api/settings", a.requireAuth(a.handleUpdateSettings))
	mux.HandleFunc("POST /api/settings/test/telegram", a.requireAuth(a.handleTestTelegram))
	mux.HandleFunc("POST /api/settings/test/webhook", a.requireAuth(a.handleTestWebhook))
	mux.HandleFunc("GET /api/xray/status", a.requireAuth(a.handleXrayStatus))
	mux.HandleFunc("POST /api/xray/detect", a.requireAuth(a.handleXrayDetect))

	mux.HandleFunc("GET /api/push/config", a.requireAuth(a.handlePushConfig))
	mux.HandleFunc("POST /api/push/subscribe", a.requireAuth(a.handlePushSubscribe))
	mux.HandleFunc("POST /api/push/unsubscribe", a.requireAuth(a.handlePushUnsubscribe))
	mux.HandleFunc("POST /api/push/test", a.requireAuth(a.handlePushTest))

	mux.HandleFunc("GET /api/notifications", a.requireAuth(a.handleNotifications))
	mux.HandleFunc("GET /api/logs", a.requireAuth(a.handleLogs))
	mux.HandleFunc("GET /api/audit", a.requireAuth(a.handleAudit))
	mux.HandleFunc("GET /api/backup", a.requireAuth(a.handleBackup))
	mux.HandleFunc("POST /api/auth/password", a.requireAuth(a.handleChangePassword))

	mux.HandleFunc("/", a.handleStatic)

	return a.recoverer(a.securityHeaders(a.csrfGuard(a.apiLogger(mux))))
}

func (a *App) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := a.auth.verify(r); !ok {
			writeErr(w, http.StatusUnauthorized, "برای دسترسی باید وارد شوید")
			return
		}
		next(w, r)
	}
}

func (a *App) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeData(w, map[string]any{
		"ok":         true,
		"version":    a.cfg.Version,
		"uptime_sec": int64(time.Since(a.start).Seconds()),
		"time":       time.Now().Unix(),
	})
}

func (a *App) handleAuthState(w http.ResponseWriter, r *http.Request) {
	users, _ := a.db.CountUsers()
	authenticated := false
	username := ""
	if u, ok := a.auth.verify(r); ok {
		authenticated = true
		username = u
	}
	writeData(w, map[string]any{
		"needs_setup":   users == 0,
		"authenticated": authenticated,
		"username":      username,
		"site_title":    a.config().SiteTitle,
		"version":       a.cfg.Version,
	})
}

func (a *App) handleSetup(w http.ResponseWriter, r *http.Request) {
	users, err := a.db.CountUsers()
	if err != nil {
		writeErr(w, 500, "خطا در بررسی وضعیت حساب")
		return
	}
	if users > 0 {
		writeErr(w, http.StatusConflict, "حساب مدیریت از قبل ساخته شده است")
		return
	}
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	body.Username = strings.TrimSpace(body.Username)
	if len(body.Username) < 3 || len(body.Username) > 32 {
		writeErr(w, 400, "نام کاربری باید بین 3 تا 32 کاراکتر باشد")
		return
	}
	if len(body.Password) < 8 {
		writeErr(w, 400, "رمز عبور باید حداقل 8 کاراکتر باشد")
		return
	}
	hash, err := a.auth.hashPassword(body.Password)
	if err != nil {
		writeErr(w, 500, "خطا در ساخت رمز عبور")
		return
	}
	if err := a.db.CreateUser(body.Username, hash); err != nil {
		writeErr(w, 500, "ساخت حساب ناموفق بود")
		return
	}
	a.db.AddAudit(body.Username, "setup", "حساب مدیریت ساخته شد")
	a.logFor("security").Info("admin account created", "username", body.Username, "ip", clientIP(r))
	a.auth.issue(w, r, body.Username)
	writeData(w, map[string]any{"username": body.Username})
}

func (a *App) handleLogin(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	if !a.auth.allow(ip) {
		a.logFor("security").Warn("login rate limited", "ip", ip)
		writeErr(w, http.StatusTooManyRequests, "تلاش بیش از حد؛ چند دقیقه دیگر امتحان کنید")
		return
	}
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	body.Username = strings.TrimSpace(body.Username)
	user, err := a.db.GetUser(body.Username)
	if err != nil {
		writeErr(w, 500, "خطا در ورود")
		return
	}
	if user == nil {
		users, _ := a.db.CountUsers()
		if users == 0 {
			writeErr(w, 400, "ابتدا حساب مدیریت را بسازید")
			return
		}
		a.auth.fail(ip)
		a.logFor("security").Warn("login failed", "username", body.Username, "ip", ip)
		writeErr(w, http.StatusUnauthorized, "نام کاربری یا رمز عبور نادرست است")
		return
	}
	if !a.auth.checkPassword(user.PasswordHash, body.Password) {
		a.auth.fail(ip)
		a.logFor("security").Warn("login failed", "username", body.Username, "ip", ip)
		writeErr(w, http.StatusUnauthorized, "نام کاربری یا رمز عبور نادرست است")
		return
	}
	a.auth.reset(ip)
	a.auth.issue(w, r, user.Username)
	a.logFor("security").Info("login succeeded", "username", user.Username, "ip", ip)
	a.db.AddAudit(user.Username, "login", "ورود موفق از "+ip)
	writeData(w, map[string]any{"username": user.Username})
}

func (a *App) handleLogout(w http.ResponseWriter, r *http.Request) {
	a.auth.clear(w)
	writeData(w, map[string]any{"ok": true})
}

func (a *App) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	username, _ := a.auth.verify(r)
	var body struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if len(body.NewPassword) < 8 {
		writeErr(w, 400, "رمز عبور جدید باید حداقل 8 کاراکتر باشد")
		return
	}
	user, err := a.db.GetUser(username)
	if err != nil || user == nil {
		writeErr(w, 401, "حساب پیدا نشد")
		return
	}
	if !a.auth.checkPassword(user.PasswordHash, body.CurrentPassword) {
		writeErr(w, 403, "رمز عبور فعلی نادرست است")
		return
	}
	hash, err := a.auth.hashPassword(body.NewPassword)
	if err != nil {
		writeErr(w, 500, "خطا در رمزگذاری")
		return
	}
	if err := a.db.UpdatePassword(username, hash); err != nil {
		writeErr(w, 500, "تغییر رمز ناموفق بود")
		return
	}
	a.db.AddAudit(username, "password", "رمز عبور تغییر کرد")
	writeData(w, map[string]any{"ok": true})
}

func (a *App) handleStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, 500, "SSE پشتیبانی نمی‌شود")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	ch := a.hub.subscribe()
	defer a.hub.unsubscribe(ch)
	fmt.Fprintf(w, "retry: 3000\nevent: ready\ndata: {\"type\":\"ready\"}\n\n")
	flusher.Flush()
	heartbeat := time.NewTicker(20 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-heartbeat.C:
			fmt.Fprint(w, ": keepalive\n\n")
			flusher.Flush()
		case msg, ok := <-ch:
			if !ok {
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", strings.TrimSpace(string(msg)))
			flusher.Flush()
		}
	}
}

func (a *App) handleStatic(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		writeErr(w, http.StatusNotFound, "مسیر پیدا نشد")
		return
	}
	name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if name == "" || name == "." {
		name = "index.html"
	}
	data, err := web.Files.ReadFile(name)
	if err != nil {
		if strings.Contains(path.Base(name), ".") {
			http.NotFound(w, r)
			return
		}
		name = "index.html"
		data, err = web.Files.ReadFile(name)
		if err != nil {
			http.Error(w, "UI not embedded", http.StatusInternalServerError)
			return
		}
	}
	ext := strings.ToLower(filepath.Ext(name))
	ctype := mime.TypeByExtension(ext)
	if ctype == "" {
		switch ext {
		case ".woff2":
			ctype = "font/woff2"
		case ".webmanifest":
			ctype = "application/manifest+json"
		case ".svg":
			ctype = "image/svg+xml"
		}
	}
	if ctype == "" {
		ctype = "application/octet-stream"
	}
	w.Header().Set("Content-Type", ctype)
	switch {
	case name == "sw.js" || name == "index.html":
		w.Header().Set("Cache-Control", "no-cache")
	case strings.HasPrefix(name, "assets/"):
		w.Header().Set("Cache-Control", "no-cache")
	case strings.HasPrefix(name, "fonts/") || strings.HasPrefix(name, "icons/"):
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	default:
		w.Header().Set("Cache-Control", "public, max-age=3600")
	}
	if name == "sw.js" {
		w.Header().Set("Service-Worker-Allowed", "/")
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (a *App) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; script-src 'self'; connect-src 'self'; font-src 'self'; manifest-src 'self'; worker-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
		if requestIsHTTPS(r) {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}

func (a *App) csrfGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" {
			u, err := url.Parse(origin)
			if err != nil || !sameHost(u.Host, r) {
				a.logFor("security").Warn("CSRF origin rejected", "origin", origin, "host", r.Host, "path", r.URL.Path)
				writeErr(w, http.StatusForbidden, "درخواست از منبع نامعتبر رد شد")
				return
			}
		}
		if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch {
			if r.ContentLength > 0 && !strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "application/json") {
				a.logFor("security").Warn("non-JSON API body rejected", "path", r.URL.Path, "content_type", r.Header.Get("Content-Type"))
				writeErr(w, http.StatusUnsupportedMediaType, "بدنه درخواست باید JSON باشد")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func sameHost(originHost string, r *http.Request) bool {
	originHost = strings.ToLower(strings.TrimSpace(originHost))
	if originHost == strings.ToLower(r.Host) {
		return true
	}
	if fh := r.Header.Get("X-Forwarded-Host"); fh != "" {
		return originHost == strings.ToLower(strings.TrimSpace(strings.Split(fh, ",")[0]))
	}
	return false
}

func (a *App) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				a.logger.Error("panic recovered", "error", rec, "path", r.URL.Path)
				writeErr(w, http.StatusInternalServerError, "خطای داخلی سرور")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func (a *App) Run(ctx context.Context) error {
	go a.scheduler(ctx)
	listener, err := net.Listen("tcp", a.cfg.Addr)
	if err != nil {
		return err
	}
	m := cmux.New(listener)
	grpcL := m.MatchWithWriters(cmux.HTTP2MatchHeaderFieldPrefixSendSettings("content-type", "application/grpc"))
	httpL := m.Match(cmux.Any())
	srv := &http.Server{
		Addr:              a.cfg.Addr,
		Handler:           a.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      0,
		IdleTimeout:       90 * time.Second,
	}
	errCh := make(chan error, 3)
	go func() {
		if err := a.grpc.Serve(grpcL); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			errCh <- fmt.Errorf("grpc: %w", err)
		}
	}()
	go func() {
		if err := srv.Serve(httpL); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("http: %w", err)
		}
	}()
	go func() {
		if err := m.Serve(); err != nil && !errors.Is(err, net.ErrClosed) {
			errCh <- fmt.Errorf("cmux: %w", err)
		}
	}()
	a.logger.Info("pulse is listening", "addr", a.cfg.Addr, "version", a.cfg.Version, "agent_link", "gRPC + WebSocket")

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
		stopped := make(chan struct{})
		go func() {
			a.grpc.GracefulStop()
			close(stopped)
		}()
		select {
		case <-stopped:
		case <-time.After(3 * time.Second):
			a.grpc.Stop()
		}
		_ = listener.Close()
		return nil
	case err := <-errCh:
		_ = listener.Close()
		a.grpc.Stop()
		return err
	}
}

func nowUnix() int64 { return time.Now().Unix() }

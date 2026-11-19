package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
	_ "time/tzdata"

	"pulse/internal/agent"
	"pulse/internal/app"
)

var version = "1.4.0"

func main() {
	if handleManagementCLI(os.Args[1:]) {
		return
	}
	var (
		addr      = flag.String("addr", env("PULSE_ADDR", ":8080"), "HTTP listen address")
		dbPath    = flag.String("db", env("PULSE_DB_PATH", "./data/pulse.db"), "SQLite database path")
		debug     = flag.Bool("debug", false, "enable debug logging")
		showVer   = flag.Bool("version", false, "print version")
		agentMode = flag.Bool("agent", false, "run as a server monitoring agent")
		serverURL = flag.String("server", env("PULSE_SERVER", ""), "master base URL (agent mode)")
		token     = flag.String("token", env("PULSE_TOKEN", ""), "agent token (agent mode)")
		agentName = flag.String("name", env("PULSE_AGENT_NAME", ""), "agent display name")
		interval  = flag.Duration("interval", 5*time.Second, "agent report interval")
		services  = flag.String("services", env("PULSE_AGENT_SERVICES", ""), "comma separated systemd services to monitor")
		collectWG = flag.Bool("wg", envBool("PULSE_AGENT_WG", true), "collect WireGuard handshake data")
		once      = flag.Bool("once", false, "send a single report and exit (agent mode)")
	)
	flag.Parse()

	if *showVer {
		os.Stdout.WriteString("pulse " + version + "\n")
		return
	}
	level := slog.LevelInfo
	if *debug {
		level = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level}))

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if *agentMode {
		if *agentName == "" {
			*agentName, _ = os.Hostname()
		}
		serviceList := []string{}
		for _, s := range strings.Split(*services, ",") {
			if s = strings.TrimSpace(s); s != "" {
				serviceList = append(serviceList, s)
			}
		}
		err := agent.Run(ctx, agent.Options{
			Server:    *serverURL,
			Token:     *token,
			Name:      *agentName,
			Interval:  *interval,
			Services:  serviceList,
			CollectWG: *collectWG,
			Once:      *once,
			Version:   version,
		}, logger)
		if err != nil {
			logger.Error("agent stopped", "error", err)
			os.Exit(1)
		}
		return
	}

	a, err := app.New(app.Config{
		Addr:             *addr,
		DBPath:           *dbPath,
		Version:          version,
		EnvTelegramToken: os.Getenv("PULSE_TELEGRAM_TOKEN"),
		EnvTelegramChat:  os.Getenv("PULSE_TELEGRAM_CHAT"),
		EnvCFToken:       os.Getenv("PULSE_CF_TOKEN"),
		EnvWebhookURL:    os.Getenv("PULSE_WEBHOOK_URL"),
		EnvPublicBaseURL: os.Getenv("PULSE_PUBLIC_BASE_URL"),
		EnvXrayBin:       os.Getenv("PULSE_XRAY_BIN"),
	}, logger)
	if err != nil {
		logger.Error("startup failed", "error", err)
		os.Exit(1)
	}
	defer a.Close()
	logger.Info("starting Pulse", "version", version, "addr", *addr, "db", *dbPath)
	if err := a.Run(ctx); err != nil {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envBool(key string, def bool) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return def
	}
}

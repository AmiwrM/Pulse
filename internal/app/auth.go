package app

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"

	"pulse/internal/store"
)

const sessionCookie = "pulse_session"

type sessionPayload struct {
	User string `json:"u"`
	Exp  int64  `json:"e"`
}

type authManager struct {
	secret []byte
	ttl    time.Duration
	store  *store.Store

	mu       sync.Mutex
	attempts map[string]*loginAttempt
}

type loginAttempt struct {
	count int
	first time.Time
}

func newAuth(secret string, st *store.Store) *authManager {
	return &authManager{secret: []byte(secret), ttl: 14 * 24 * time.Hour, store: st, attempts: map[string]*loginAttempt{}}
}

func (a *authManager) issue(w http.ResponseWriter, r *http.Request, username string) {
	payload, _ := json.Marshal(sessionPayload{User: username, Exp: time.Now().Add(a.ttl).Unix()})
	body := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, a.secret)
	mac.Write([]byte(body))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    body + "." + sig,
		Path:     "/",
		MaxAge:   int(a.ttl.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   requestIsHTTPS(r),
	})
}

func (a *authManager) clear(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func (a *authManager) verify(r *http.Request) (string, bool) {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return "", false
	}
	parts := strings.Split(c.Value, ".")
	if len(parts) != 2 {
		return "", false
	}
	mac := hmac.New(sha256.New, a.secret)
	mac.Write([]byte(parts[0]))
	expected := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(parts[1]), []byte(expected)) {
		return "", false
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", false
	}
	var p sessionPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return "", false
	}
	if p.Exp < time.Now().Unix() || p.User == "" {
		return "", false
	}
	return p.User, true
}

func (a *authManager) allow(ip string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	now := time.Now()
	for k, v := range a.attempts {
		if now.Sub(v.first) > 15*time.Minute {
			delete(a.attempts, k)
		}
	}
	at := a.attempts[ip]
	if at == nil || now.Sub(at.first) > 10*time.Minute {
		return true
	}
	return at.count < 8
}

func (a *authManager) fail(ip string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	now := time.Now()
	at := a.attempts[ip]
	if at == nil || now.Sub(at.first) > 10*time.Minute {
		a.attempts[ip] = &loginAttempt{count: 1, first: now}
		return
	}
	at.count++
}

func (a *authManager) reset(ip string) {
	a.mu.Lock()
	delete(a.attempts, ip)
	a.mu.Unlock()
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if first := strings.TrimSpace(strings.Split(xff, ",")[0]); first != "" {
			return first
		}
	}
	if xr := r.Header.Get("X-Real-IP"); xr != "" {
		return xr
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

func requestIsHTTPS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func (a *authManager) hashPassword(password string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	return string(b), err
}

func (a *authManager) checkPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

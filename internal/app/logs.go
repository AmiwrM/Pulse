package app

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"pulse/internal/model"
	"pulse/internal/store"
)

// logSink decouples database writes from the request/check hot path. A full
// queue drops the entry instead of blocking a monitor or an agent.
type logSink struct {
	db      *store.Store
	ch      chan model.LogEntry
	done    chan struct{}
	once    sync.Once
	mu      sync.RWMutex
	closed  bool
	dropped atomic.Uint64
}

func newLogSink(db *store.Store) *logSink {
	s := &logSink{db: db, ch: make(chan model.LogEntry, 512), done: make(chan struct{})}
	go s.run()
	return s
}

func (s *logSink) run() {
	defer close(s.done)
	for entry := range s.ch {
		if err := s.db.AddLog(entry); err != nil {
			// Never route this through slog: it would recurse into this sink.
			fmt.Fprintf(os.Stderr, "pulse log write failed: %v\n", err)
		}
	}
}

func (s *logSink) enqueue(entry model.LogEntry) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return
	}
	select {
	case s.ch <- entry:
	default:
		s.dropped.Add(1)
	}
}

func (s *logSink) close() {
	s.once.Do(func() {
		s.mu.Lock()
		s.closed = true
		close(s.ch)
		s.mu.Unlock()
		<-s.done
	})
}

type dualHandler struct {
	base   slog.Handler
	sink   *logSink
	attrs  []slog.Attr
	groups []string
}

func newDualHandler(base slog.Handler, sink *logSink) slog.Handler {
	return &dualHandler{base: base, sink: sink}
}

func (h *dualHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.base.Enabled(ctx, level)
}

func (h *dualHandler) Handle(ctx context.Context, r slog.Record) error {
	err := h.base.Handle(ctx, r)
	entry := h.entry(r)
	h.sink.enqueue(entry)
	return err
}

func (h *dualHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := &dualHandler{
		base:   h.base.WithAttrs(attrs),
		sink:   h.sink,
		attrs:  append(append([]slog.Attr(nil), h.attrs...), attrs...),
		groups: append([]string(nil), h.groups...),
	}
	return next
}

func (h *dualHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	return &dualHandler{
		base:   h.base.WithGroup(name),
		sink:   h.sink,
		attrs:  append([]slog.Attr(nil), h.attrs...),
		groups: append(append([]string(nil), h.groups...), name),
	}
}

func (h *dualHandler) entry(r slog.Record) model.LogEntry {
	all := append([]slog.Attr(nil), h.attrs...)
	r.Attrs(func(a slog.Attr) bool {
		all = append(all, a)
		return true
	})
	fields := map[string]any{}
	flattenAttrs(fields, h.groups, all)
	category := "panel"
	if v, ok := fields["category"].(string); ok && strings.TrimSpace(v) != "" {
		category = strings.TrimSpace(v)
	}
	delete(fields, "category")
	detail := ""
	if len(fields) > 0 {
		if raw, err := json.Marshal(fields); err == nil {
			detail = string(raw)
		} else {
			detail = fmt.Sprint(fields)
		}
	}
	at := r.Time.Unix()
	if at <= 0 {
		at = time.Now().Unix()
	}
	return model.LogEntry{
		At: at, Level: strings.ToLower(r.Level.String()), Category: category,
		Message: r.Message, Detail: detail,
	}
}

func flattenAttrs(out map[string]any, prefix []string, attrs []slog.Attr) {
	for _, attr := range attrs {
		key := attr.Key
		if len(prefix) > 0 {
			key = strings.Join(append(append([]string(nil), prefix...), key), ".")
		}
		value := attr.Value.Resolve()
		if value.Kind() == slog.KindGroup {
			flattenAttrs(out, append(append([]string(nil), prefix...), attr.Key), value.Group())
			continue
		}
		out[key] = slogValue(value)
	}
}

func slogValue(v slog.Value) any {
	switch v.Kind() {
	case slog.KindString:
		return v.String()
	case slog.KindInt64:
		return v.Int64()
	case slog.KindUint64:
		return v.Uint64()
	case slog.KindFloat64:
		return v.Float64()
	case slog.KindBool:
		return v.Bool()
	case slog.KindDuration:
		return v.Duration().String()
	case slog.KindTime:
		return v.Time().Format(time.RFC3339)
	case slog.KindAny:
		if err, ok := v.Any().(error); ok {
			return err.Error()
		}
		return v.Any()
	default:
		return v.String()
	}
}

func (a *App) logFor(category string) *slog.Logger {
	if a == nil || a.logger == nil {
		return slog.Default()
	}
	return a.logger.With("category", category)
}

// statusRecorder preserves the interfaces used by SSE and WebSocket handlers.
type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (r *statusRecorder) WriteHeader(code int) {
	if r.status == 0 {
		r.status = code
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	n, err := r.ResponseWriter.Write(b)
	r.bytes += n
	return n, err
}

func (r *statusRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (r *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := r.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("response writer does not support hijacking")
	}
	conn, rw, err := h.Hijack()
	if r.status == 0 {
		r.status = http.StatusSwitchingProtocols
	}
	return conn, rw, err
}

// apiLogger records API requests. It is intentionally quiet for successful
// GETs (which would otherwise be dominated by dashboard polling) and captures
// mutations, client errors and server errors.
func (a *App) apiLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(rec, r)
		status := rec.status
		if status == 0 {
			status = http.StatusOK
		}
		level := slog.LevelDebug
		if r.Method != http.MethodGet || status >= 400 {
			level = slog.LevelInfo
		}
		if status >= 500 {
			level = slog.LevelError
		}
		if r.URL.Path == "/api/stream" || r.URL.Path == "/api/agent/link" {
			return
		}
		a.logFor("api").Log(r.Context(), level, r.Method+" "+r.URL.Path,
			"status", status, "duration_ms", time.Since(start).Milliseconds(), "ip", clientIP(r))
	})
}

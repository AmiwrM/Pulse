package xray

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/proxy"
)

const DefaultTestURL = "https://www.gstatic.com/generate_204"

type Status struct {
	Available bool   `json:"available"`
	Path      string `json:"path"`
	Version   string `json:"version"`
	Error     string `json:"error"`
	CheckedAt int64  `json:"checked_at"`
}

type Result struct {
	OK         bool
	LatencyMS  int64
	Error      string
	InfraError bool
}

type Engine struct {
	mu        sync.RWMutex
	binary    string
	path      string
	version   string
	available bool
	errText   string
	checkedAt int64
	procSem   chan struct{}
}

func NewEngine(binary string) *Engine {
	binary = strings.TrimSpace(binary)
	if binary == "" {
		binary = "xray"
	}
	return &Engine{binary: binary, procSem: make(chan struct{}, 6)}
}

func (e *Engine) Binary() string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.binary
}

func (e *Engine) SetBinary(binary string) {
	binary = strings.TrimSpace(binary)
	if binary == "" {
		binary = "xray"
	}
	e.mu.Lock()
	if e.binary != binary {
		e.binary = binary
		e.available = false
		e.version = ""
		e.path = ""
		e.errText = "هنوز بررسی نشده است"
	}
	e.mu.Unlock()
}

func (e *Engine) Status() Status {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return Status{
		Available: e.available,
		Path:      e.path,
		Version:   e.version,
		Error:     e.errText,
		CheckedAt: e.checkedAt,
	}
}

func (e *Engine) Available() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.available
}

var versionRE = regexp.MustCompile(`(?i)xray\s+([0-9][^\s]*)`)

// Detect locates the binary and verifies that it can run.
func (e *Engine) Detect(ctx context.Context) Status {
	e.mu.RLock()
	binary := e.binary
	e.mu.RUnlock()

	path, err := exec.LookPath(binary)
	if err != nil {
		return e.setStatus(false, "", "", fmt.Sprintf("باینری Xray پیدا نشد (%s): %v", binary, err))
	}
	cctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	out, err := exec.CommandContext(cctx, path, "version").CombinedOutput()
	text := strings.TrimSpace(string(out))
	if err != nil {
		msg := text
		if msg == "" {
			msg = err.Error()
		}
		return e.setStatus(false, path, "", msg)
	}
	m := versionRE.FindStringSubmatch(text)
	if len(m) < 2 {
		return e.setStatus(false, path, "", "نسخهٔ Xray قابل تشخیص نبود: "+truncate(text, 200))
	}
	return e.setStatus(true, path, m[1], "")
}

func (e *Engine) setStatus(available bool, path, version, errText string) Status {
	e.mu.Lock()
	e.available = available
	if path != "" {
		e.path = path
	}
	e.version = version
	e.errText = errText
	e.checkedAt = time.Now().Unix()
	status := Status{Available: available, Path: e.path, Version: version, Error: errText, CheckedAt: e.checkedAt}
	e.mu.Unlock()
	return status
}

// TestNodes starts one temporary Xray process per node, each with a local SOCKS
// inbound, then performs a real HTTP request through that proxy. This validates
// the protocol handshake, authentication and actual egress instead of a simple
// TCP connect.
func (e *Engine) TestNodes(ctx context.Context, nodes []Node, testURL string, concurrency int) ([]Result, error) {
	status := e.Status()
	if !status.Available {
		if status.Error == "" {
			status.Error = "هستهٔ Xray آماده نیست"
		}
		return nil, fmt.Errorf("%s", status.Error)
	}
	testURL = strings.TrimSpace(testURL)
	if testURL == "" {
		testURL = DefaultTestURL
	}
	u, err := url.Parse(testURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("آدرس تست اتصال معتبر نیست")
	}
	if concurrency <= 0 {
		concurrency = 4
	}
	if concurrency > 8 {
		concurrency = 8
	}

	results := make([]Result, len(nodes))
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	for i := range nodes {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				results[i] = Result{Error: "اتمام مهلت تست", InfraError: true}
				return
			}
			defer func() { <-sem }()
			results[i] = e.testNode(ctx, nodes[i], testURL)
		}(i)
	}
	wg.Wait()
	return results, nil
}

func (e *Engine) testNode(parent context.Context, node Node, testURL string) Result {
	status := e.Status()
	if !status.Available {
		return Result{Error: status.Error, InfraError: true}
	}
	ctx, cancel := context.WithTimeout(parent, 14*time.Second)
	defer cancel()

	// Reserve a local port for the SOCKS inbound.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return Result{Error: "تخصیص پورت محلی ناموفق بود", InfraError: true}
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	config, err := buildConfig(node, port)
	if err != nil {
		return Result{Error: err.Error()}
	}

	dir, err := os.MkdirTemp("", "pulse-xray-*")
	if err != nil {
		return Result{Error: "ساخت پوشهٔ موقت ناموفق بود", InfraError: true}
	}
	defer os.RemoveAll(dir)
	configPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(configPath, config, 0o600); err != nil {
		return Result{Error: "نوشتن کانفیگ موقت ناموفق بود", InfraError: true}
	}

	// Cap the total number of temporary Xray processes across all monitors.
	select {
	case e.procSem <- struct{}{}:
		defer func() { <-e.procSem }()
	case <-ctx.Done():
		return Result{Error: "اتمام مهلت تست واقعی", InfraError: true}
	}

	var logs safeBuffer
	logs.limit = 4096
	cmd := exec.CommandContext(ctx, status.Path, "run", "-c", configPath)
	cmd.Stdout = &logs
	cmd.Stderr = &logs
	if err := cmd.Start(); err != nil {
		return Result{Error: "اجرای Xray ناموفق بود: " + err.Error(), InfraError: true}
	}
	waitCh := make(chan error, 1)
	go func() { waitCh <- cmd.Wait() }()
	cleanup := func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		select {
		case <-waitCh:
		case <-time.After(2 * time.Second):
		}
	}
	defer cleanup()

	socksAddr := net.JoinHostPort("127.0.0.1", fmt.Sprint(port))
	if err := waitPort(ctx, socksAddr, waitCh, 5*time.Second); err != nil {
		return Result{Error: "Xray شروع نشد: " + err.Error() + logsSuffix(logs.String()), InfraError: true}
	}

	start := time.Now()
	err = httpThroughSocks(ctx, socksAddr, testURL)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		return Result{OK: false, LatencyMS: latency, Error: err.Error()}
	}
	return Result{OK: true, LatencyMS: latency}
}

func waitPort(ctx context.Context, addr string, waitCh <-chan error, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-waitCh:
			if err == nil {
				err = fmt.Errorf("process exited")
			}
			return err
		default:
			conn, err := net.DialTimeout("tcp", addr, 250*time.Millisecond)
			if err == nil {
				conn.Close()
				return nil
			}
			if time.Now().After(deadline) {
				return fmt.Errorf("timeout waiting for SOCKS on %s", addr)
			}
			time.Sleep(80 * time.Millisecond)
		}
	}
}

func httpThroughSocks(ctx context.Context, socksAddr, testURL string) error {
	d, err := proxy.SOCKS5("tcp", socksAddr, nil, proxy.Direct)
	if err != nil {
		return fmt.Errorf("SOCKS5: %w", err)
	}
	dial := func(ctx context.Context, network, address string) (net.Conn, error) {
		type outcome struct {
			conn net.Conn
			err  error
		}
		if cd, ok := d.(proxy.ContextDialer); ok {
			return cd.DialContext(ctx, network, address)
		}
		ch := make(chan outcome, 1)
		go func() {
			conn, err := d.Dial(network, address)
			ch <- outcome{conn, err}
		}()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case out := <-ch:
			return out.conn, out.err
		}
	}
	tr := &http.Transport{
		DialContext:           dial,
		DisableKeepAlives:     true,
		TLSHandshakeTimeout:   8 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
		ForceAttemptHTTP2:     true,
	}
	defer tr.CloseIdleConnections()
	client := &http.Client{
		Transport: tr,
		Timeout:   12 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, testURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Pulse-Xray-Test/1.0")
	req.Header.Set("Accept", "*/*")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("درخواست واقعی ناموفق: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return fmt.Errorf("آدرس تست کد %d برگرداند", resp.StatusCode)
	}
	return nil
}

type safeBuffer struct {
	mu    sync.Mutex
	sb    strings.Builder
	limit int
}

func (b *safeBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.limit <= 0 {
		b.limit = 4096
	}
	remain := b.limit - b.sb.Len()
	if remain > 0 {
		if len(p) > remain {
			b.sb.Write(p[:remain])
		} else {
			b.sb.Write(p)
		}
	}
	return len(p), nil
}

func (b *safeBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.sb.String()
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func logsSuffix(logs string) string {
	logs = strings.TrimSpace(logs)
	if logs == "" {
		return ""
	}
	return " — " + truncate(logs, 500)
}

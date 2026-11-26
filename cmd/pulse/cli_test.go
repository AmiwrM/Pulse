package main

import (
	"strings"
	"testing"
)

func TestChecksumFor(t *testing.T) {
	sums := `aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa  pulse-linux-amd64
bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb  pulse-linux-arm64
`
	if got := checksumFor(sums, "pulse-linux-amd64"); got != "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Fatalf("checksumFor(amd64) = %q", got)
	}
	if got := checksumFor(sums, "missing"); got != "" {
		t.Fatalf("checksumFor(missing) = %q, want empty", got)
	}
}

func TestPortHelpers(t *testing.T) {
	if got := withPort("127.0.0.1:8080", 9090); got != "127.0.0.1:9090" {
		t.Fatalf("withPort host = %q", got)
	}
	if got := withPort(":8080", 9090); got != ":9090" {
		t.Fatalf("withPort empty host = %q", got)
	}
	if got := addrPort("127.0.0.1:8080"); got != "8080" {
		t.Fatalf("addrPort = %q", got)
	}
	unit := "[Service]\nExecStart=/usr/local/bin/pulse --addr 127.0.0.1:8080 --db /var/lib/pulse/pulse.db\n"
	if got := unitAddr(unit); got != "127.0.0.1:8080" {
		t.Fatalf("unitAddr = %q", got)
	}
	rewritten := rewriteUnitAddr(unit, "127.0.0.1:9090")
	if !strings.Contains(rewritten, "--addr 127.0.0.1:9090") || strings.Contains(rewritten, ":8080") {
		t.Fatalf("rewriteUnitAddr = %q", rewritten)
	}
	caddy := "reverse_proxy 127.0.0.1:8080 {\n transport http { versions h2c 2 }\n}\n"
	if got := rewriteCaddyPort(caddy, "8080", "9090"); !strings.Contains(got, "127.0.0.1:9090") || strings.Contains(got, ":8080") {
		t.Fatalf("rewriteCaddyPort = %q", got)
	}
	env := "PULSE_PUBLIC_BASE_URL=http://example.com:8080\nPULSE_TOKEN=x\n"
	if got := rewritePublicPort(env, "8080", "9090"); !strings.Contains(got, "example.com:9090") || !strings.Contains(got, "PULSE_TOKEN=x") {
		t.Fatalf("rewritePublicPort = %q", got)
	}
}

func TestIsAgentApplet(t *testing.T) {
	cases := map[string]bool{
		"pulse":           false,
		"pulse.exe":       false,
		"pulse-agent":     true,
		"pulse-agent.exe": true,
		"pulse-a":         true,
	}
	for name, want := range cases {
		if got := isAgentApplet(name); got != want {
			t.Fatalf("isAgentApplet(%q) = %v, want %v", name, got, want)
		}
	}
}

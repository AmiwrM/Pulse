package main

import "testing"

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

package main

import "testing"

func TestEnv(t *testing.T) {
	t.Setenv("PULSE_TEST_ENV", "value")
	if got := env("PULSE_TEST_ENV", "fallback"); got != "value" {
		t.Fatalf("env() = %q, want value", got)
	}
	t.Setenv("PULSE_TEST_ENV", "")
	if got := env("PULSE_TEST_ENV", "fallback"); got != "fallback" {
		t.Fatalf("env() = %q, want fallback", got)
	}
}

func TestEnvBool(t *testing.T) {
	cases := []struct {
		value string
		def   bool
		want  bool
	}{
		{"1", false, true},
		{"true", false, true},
		{"YES", false, true},
		{"on", false, true},
		{"0", true, false},
		{"false", true, false},
		{"no", true, false},
		{"off", true, false},
		{"", true, true},
		{"", false, false},
		{"invalid", true, true},
	}
	for _, c := range cases {
		t.Setenv("PULSE_TEST_BOOL", c.value)
		if got := envBool("PULSE_TEST_BOOL", c.def); got != c.want {
			t.Fatalf("envBool(%q, %v) = %v, want %v", c.value, c.def, got, c.want)
		}
	}
}

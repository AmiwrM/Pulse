package checker

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"pulse/internal/model"
)

func TestHTTPKeywordScope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Profile", "Pulse")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("body-only"))
	}))
	defer srv.Close()

	m := New("")
	m.UserAgent = "Pulse-Test"
	run := func(cfg string) Result {
		var c map[string]any
		if err := json.Unmarshal([]byte(cfg), &c); err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(c)
		return m.checkHTTP(context.Background(), model.Monitor{
			Type: "http", Target: srv.URL, Timeout: 5000, Config: raw,
		})
	}

	bodyOnly := run(`{"keyword":"Pulse","keyword_scope":"body"}`)
	if bodyOnly.OK {
		t.Fatalf("body scope should not match a header keyword: %+v", bodyOnly)
	}
	headersOnly := run(`{"keyword":"Pulse","keyword_scope":"headers"}`)
	if !headersOnly.OK {
		t.Fatalf("header scope should match X-Profile: %+v", headersOnly)
	}
	both := run(`{"keyword":"body-only","keyword_scope":"both"}`)
	if !both.OK {
		t.Fatalf("both scope should match the body: %+v", both)
	}
	notContains := run(`{"keyword":"Pulse","keyword_scope":"headers","keyword_mode":"not_contains"}`)
	if notContains.OK {
		t.Fatalf("not_contains should fail when the header contains the keyword: %+v", notContains)
	}
}

func TestHTTPKeywordScopeValidation(t *testing.T) {
	err := Validate("http", "https://example.com", json.RawMessage(`{"keyword":"x","keyword_scope":"cookies"}`))
	if err == nil {
		t.Fatal("expected an invalid keyword scope to be rejected")
	}
	if err := Validate("http", "https://example.com", json.RawMessage(`{"keyword":"x","keyword_scope":"headers"}`)); err != nil {
		t.Fatalf("headers scope should be valid: %v", err)
	}
}

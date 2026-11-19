package cloudflare

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClientFlow(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/user/tokens/verify", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("missing bearer token")
		}
		w.Write([]byte(`{"success":true,"result":{"id":"token","status":"active"}}`))
	})
	mux.HandleFunc("/zones", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"success":true,"result":[{"id":"z1","name":"example.com","status":"active"}]}`))
	})
	mux.HandleFunc("/zones/z1/dns_records", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("type") != "A" {
			t.Errorf("expected type filter, got %q", r.URL.Query().Get("type"))
		}
		w.Write([]byte(`{"success":true,"result":[{"id":"r1","zone_id":"z1","type":"A","name":"example.com","content":"1.2.3.4","ttl":1,"proxied":true}]}`))
	})
	mux.HandleFunc("/zones/z1/dns_records/r1", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch {
			var body map[string]string
			b, _ := io.ReadAll(r.Body)
			json.Unmarshal(b, &body)
			if body["content"] != "5.6.7.8" {
				t.Errorf("unexpected PATCH body: %s", b)
			}
		}
		w.Write([]byte(`{"success":true,"result":{"id":"r1","zone_id":"z1","type":"A","name":"example.com","content":"5.6.7.8","ttl":1,"proxied":true}}`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	c := New("test-token")
	c.BaseURL = server.URL
	ctx := context.Background()
	v, err := c.Verify(ctx)
	if err != nil || v.Status != "active" {
		t.Fatalf("verify failed: %v %+v", err, v)
	}
	zones, err := c.ListZones(ctx)
	if err != nil || len(zones) != 1 || zones[0].Name != "example.com" {
		t.Fatalf("zones failed: %v %+v", err, zones)
	}
	records, err := c.ListRecords(ctx, "z1", "A", "example")
	if err != nil || len(records) != 1 || records[0].Content != "1.2.3.4" {
		t.Fatalf("records failed: %v %+v", err, records)
	}
	updated, err := c.UpdateRecordContent(ctx, "z1", "r1", "5.6.7.8")
	if err != nil || updated.Content != "5.6.7.8" {
		t.Fatalf("update failed: %v %+v", err, updated)
	}
}

func TestClientErrorParsing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		w.Write([]byte(`{"success":false,"errors":[{"code":9109,"message":"Unauthorized"}]}`))
	}))
	defer server.Close()
	c := New("bad")
	c.BaseURL = server.URL
	_, err := c.Verify(context.Background())
	if err == nil || !strings.Contains(err.Error(), "Unauthorized") {
		t.Fatalf("expected Cloudflare error, got %v", err)
	}
}

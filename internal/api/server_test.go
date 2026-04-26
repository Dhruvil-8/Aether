package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"aether/internal/node"
)

func newTestServer(t *testing.T, cfg node.Config) *Server {
	t.Helper()
	if cfg.DataDir == "" {
		cfg.DataDir = t.TempDir()
	}
	store, err := node.OpenStore(cfg.DataDir)
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	peers, err := node.OpenPeerBook(cfg.DataDir)
	if err != nil {
		t.Fatalf("OpenPeerBook: %v", err)
	}
	return NewServer(cfg, store, peers, nil)
}

func TestCORSRejectsUntrustedCrossOriginPreflight(t *testing.T) {
	s := newTestServer(t, node.DefaultConfig())
	req := httptest.NewRequest(http.MethodOptions, "http://127.0.0.1:8080/api/post", nil)
	req.Host = "127.0.0.1:8080"
	req.Header.Set("Origin", "https://example.invalid")

	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want empty", got)
	}
}

func TestCORSAllowsSameOriginPreflight(t *testing.T) {
	s := newTestServer(t, node.DefaultConfig())
	req := httptest.NewRequest(http.MethodOptions, "http://127.0.0.1:8080/api/post", nil)
	req.Host = "127.0.0.1:8080"
	req.Header.Set("Origin", "http://127.0.0.1:8080")

	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://127.0.0.1:8080" {
		t.Fatalf("Access-Control-Allow-Origin = %q", got)
	}
}

func TestCORSAllowsConfiguredOrigin(t *testing.T) {
	cfg := node.DefaultConfig()
	cfg.UIAllowedOrigins = []string{"https://ui.example.invalid"}
	s := newTestServer(t, cfg)

	req := httptest.NewRequest(http.MethodOptions, "http://127.0.0.1:8080/api/post", nil)
	req.Host = "127.0.0.1:8080"
	req.Header.Set("Origin", "https://ui.example.invalid")

	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://ui.example.invalid" {
		t.Fatalf("Access-Control-Allow-Origin = %q", got)
	}
}

func TestPostRejectsOversizedJSONBody(t *testing.T) {
	cfg := node.DefaultConfig()
	cfg.MaxJSONPayloadBytes = 16
	s := newTestServer(t, cfg)

	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8080/api/post", strings.NewReader(`{"content":"this body is too large"}`))
	req.Host = "127.0.0.1:8080"
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusRequestEntityTooLarge)
	}
}

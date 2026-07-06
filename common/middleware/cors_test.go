package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fx0x55/micro-go-lab/common/config"
)

func TestNewCorsMiddleware_Wildcard(t *testing.T) {
	mw := NewCorsMiddleware(config.CORSConfig{AllowedOrigins: []string{"*"}})
	next := func(w http.ResponseWriter, r *http.Request) {}
	handler := mw(next)

	r := httptest.NewRequest("GET", "/", http.NoBody)
	r.Header.Set("Origin", "https://example.com")
	w := httptest.NewRecorder()
	handler(w, r)

	if w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Errorf("expected *, got %q", w.Header().Get("Access-Control-Allow-Origin"))
	}
}

func TestNewCorsMiddleware_AllowedOrigin(t *testing.T) {
	mw := NewCorsMiddleware(config.CORSConfig{AllowedOrigins: []string{"https://example.com"}})
	next := func(w http.ResponseWriter, r *http.Request) {}
	handler := mw(next)

	r := httptest.NewRequest("GET", "/", http.NoBody)
	r.Header.Set("Origin", "https://example.com")
	w := httptest.NewRecorder()
	handler(w, r)

	if w.Header().Get("Access-Control-Allow-Origin") != "https://example.com" {
		t.Errorf("expected origin echoed, got %q", w.Header().Get("Access-Control-Allow-Origin"))
	}
	if w.Header().Get("Vary") != "Origin" {
		t.Errorf("expected Vary: Origin, got %q", w.Header().Get("Vary"))
	}
}

func TestNewCorsMiddleware_DisallowedOrigin(t *testing.T) {
	mw := NewCorsMiddleware(config.CORSConfig{AllowedOrigins: []string{"https://example.com"}})
	next := func(w http.ResponseWriter, r *http.Request) {}
	handler := mw(next)

	r := httptest.NewRequest("GET", "/", http.NoBody)
	r.Header.Set("Origin", "https://evil.com")
	w := httptest.NewRecorder()
	handler(w, r)

	if w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Errorf("expected no ACAO header, got %q", w.Header().Get("Access-Control-Allow-Origin"))
	}
}

func TestNewCorsMiddleware_OptionsPreflight(t *testing.T) {
	mw := NewCorsMiddleware(config.CORSConfig{AllowedOrigins: []string{"*"}})
	called := false
	next := func(w http.ResponseWriter, r *http.Request) { called = true }
	handler := mw(next)

	r := httptest.NewRequest("OPTIONS", "/", http.NoBody)
	w := httptest.NewRecorder()
	handler(w, r)

	if w.Code != http.StatusNoContent {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNoContent)
	}
	if called {
		t.Error("next handler should not be called for OPTIONS")
	}
}

func TestCorsMiddleware_DefaultWildcard(t *testing.T) {
	next := func(w http.ResponseWriter, r *http.Request) {}
	handler := CorsMiddleware(next)

	r := httptest.NewRequest("GET", "/", http.NoBody)
	r.Header.Set("Origin", "https://anything.com")
	w := httptest.NewRecorder()
	handler(w, r)

	if w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Errorf("expected *, got %q", w.Header().Get("Access-Control-Allow-Origin"))
	}
}

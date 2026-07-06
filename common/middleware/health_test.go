package middleware

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthHandler_AllHealthy(t *testing.T) {
	h := HealthHandler("test-svc", func() error { return nil })
	r := httptest.NewRequest("GET", "/healthz", http.NoBody)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["status"] != "ok" {
		t.Errorf("status = %q, want %q", resp["status"], "ok")
	}
	if resp["service"] != "test-svc" {
		t.Errorf("service = %q, want %q", resp["service"], "test-svc")
	}
}

func TestHealthHandler_CheckFails(t *testing.T) {
	h := HealthHandler("test-svc",
		func() error { return nil },
		func() error { return errors.New("db down") },
	)
	r := httptest.NewRequest("GET", "/healthz", http.NoBody)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["status"] != "unhealthy" {
		t.Errorf("status = %q, want %q", resp["status"], "unhealthy")
	}
	if resp["error"] != "db down" {
		t.Errorf("error = %q, want %q", resp["error"], "db down")
	}
}

func TestHealthHandler_NoChecks(t *testing.T) {
	h := HealthHandler("test-svc")
	r := httptest.NewRequest("GET", "/healthz", http.NoBody)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

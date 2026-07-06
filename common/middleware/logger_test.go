package middleware

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

const testRemoteAddr = "10.0.0.1:1234"

func TestRequestLogger_CapturesStatus(t *testing.T) {
	next := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		w.Write([]byte("ok"))
	}
	handler := RequestLogger(next)

	r := httptest.NewRequest("GET", "/test", http.NoBody)
	r.RemoteAddr = testRemoteAddr
	w := httptest.NewRecorder()
	handler(w, r)

	if w.Code != http.StatusAccepted {
		t.Errorf("status = %d, want %d", w.Code, http.StatusAccepted)
	}
	if w.Body.String() != "ok" {
		t.Errorf("body = %q, want %q", w.Body.String(), "ok")
	}
}

func TestRequestLogger_ReadsBody(t *testing.T) {
	var receivedBody string
	next := func(w http.ResponseWriter, r *http.Request) {
		buf := new(bytes.Buffer)
		buf.ReadFrom(r.Body)
		receivedBody = buf.String()
	}
	handler := RequestLogger(next)

	body := bytes.NewBufferString(`{"name":"test"}`)
	r := httptest.NewRequest("POST", "/submit", body)
	r.RemoteAddr = testRemoteAddr
	w := httptest.NewRecorder()
	handler(w, r)

	if receivedBody != `{"name":"test"}` {
		t.Errorf("body = %q, want %q", receivedBody, `{"name":"test"}`)
	}
}

func TestRequestLogger_PreservesDefaultStatus(t *testing.T) {
	next := func(w http.ResponseWriter, r *http.Request) {
		// 不调用 WriteHeader，测试默认 200
	}
	handler := RequestLogger(next)

	r := httptest.NewRequest("GET", "/health", http.NoBody)
	w := httptest.NewRecorder()
	handler(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

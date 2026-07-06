package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOkJson(t *testing.T) {
	w := httptest.NewRecorder()
	OkJson(w, map[string]string{"key": "value"})

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp Response
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Code != 0 {
		t.Errorf("code = %d, want 0", resp.Code)
	}
	if resp.Message != "ok" {
		t.Errorf("message = %q, want %q", resp.Message, "ok")
	}
}

func TestCreatedJson(t *testing.T) {
	w := httptest.NewRecorder()
	CreatedJson(w, "data")

	if w.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d", w.Code, http.StatusCreated)
	}
}

func TestErrorJson(t *testing.T) {
	w := httptest.NewRecorder()
	ErrorJson(w, http.StatusTeapot, "short and stout")

	if w.Code != http.StatusTeapot {
		t.Errorf("status = %d, want %d", w.Code, http.StatusTeapot)
	}
	var resp Response
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Code != -1 {
		t.Errorf("code = %d, want -1", resp.Code)
	}
	if resp.Message != "short and stout" {
		t.Errorf("message = %q, want %q", resp.Message, "short and stout")
	}
}

func TestBadRequest(t *testing.T) {
	w := httptest.NewRecorder()
	BadRequest(w, "bad input")
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestUnauthorized(t *testing.T) {
	w := httptest.NewRecorder()
	Unauthorized(w, "no token")
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestNotFound(t *testing.T) {
	w := httptest.NewRecorder()
	NotFound(w, "gone")
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestInternalError(t *testing.T) {
	w := httptest.NewRecorder()
	InternalError(w, "oops")
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestNotAllowHandler(t *testing.T) {
	h := NotAllowHandler()
	r := httptest.NewRequest("GET", "/", http.NoBody)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestResponseOmitemptyData(t *testing.T) {
	w := httptest.NewRecorder()
	ErrorJson(w, http.StatusBadRequest, "err")

	var raw map[string]any
	json.NewDecoder(w.Body).Decode(&raw)
	if _, exists := raw["data"]; exists {
		t.Error("expected 'data' to be omitted when nil")
	}
}

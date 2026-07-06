package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

func TestGetUserID(t *testing.T) {
	tests := []struct {
		name   string
		setVal any
		want   uint
	}{
		{"nil context", nil, 0},
		{"json.Number valid", json.Number("42"), 42},
		{"json.Number negative", json.Number("-1"), 0},
		{"json.Number invalid", json.Number("abc"), 0},
		{"float64 valid", float64(100), 100},
		{"float64 negative", float64(-1), 0},
		{"uint", uint(7), 7},
		{"int", int(3), 3},
		{"string valid", "55", 55},
		{"string invalid", "not-a-number", 0},
		{"unsupported type", true, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, _ := http.NewRequest("GET", "/", http.NoBody)
			if tt.setVal != nil {
				r = r.WithContext(context.WithValue(r.Context(), CtxKeyUserID, tt.setVal))
			}
			got := GetUserID(r)
			if got != tt.want {
				t.Errorf("GetUserID() = %d, want %d", got, tt.want)
			}
		})
	}
}

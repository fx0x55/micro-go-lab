package middleware

import (
	"net/http"
	"testing"
)

func TestClientIP(t *testing.T) {
	const testIP = "1.2.3.4"
	tests := []struct {
		name string
		xff  string
		addr string
		want string
	}{
		{"X-Forwarded-For single", testIP, testRemoteAddr, testIP},
		{"X-Forwarded-For multiple", testIP + ", 5.6.7.8, 9.10.11.12", testRemoteAddr, testIP},
		{"X-Forwarded-For with spaces", "  " + testIP + "  , 5.6.7.8", testRemoteAddr, testIP},
		{"no X-Forwarded-For", "", testRemoteAddr, "10.0.0.1"},
		{"no X-Forwarded-For no port", "", "10.0.0.1", "10.0.0.1"},
		{"empty X-Forwarded-For", "", "192.168.1.1:8080", "192.168.1.1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, _ := http.NewRequest("GET", "/", http.NoBody)
			if tt.xff != "" {
				r.Header.Set("X-Forwarded-For", tt.xff)
			}
			r.RemoteAddr = tt.addr
			got := clientIP(r)
			if got != tt.want {
				t.Errorf("clientIP() = %q, want %q", got, tt.want)
			}
		})
	}
}

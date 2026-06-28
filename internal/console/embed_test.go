package console

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

// portOf parses the numeric port from an httptest server URL so it can be
// handed to Handler, which builds backend URLs as localhost:<port>.
func portOf(t *testing.T, serverURL string) int {
	t.Helper()
	u, err := url.Parse(serverURL)
	if err != nil {
		t.Fatalf("parse server url %q: %v", serverURL, err)
	}
	p, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatalf("parse port from %q: %v", serverURL, err)
	}
	return p
}

func TestHandler_servesStaticAssets(t *testing.T) {
	h := Handler(4566, 4568)

	tests := []struct {
		name        string
		path        string
		wantStatus  int
		wantSnippet string
	}{
		{"root serves index", "/", http.StatusOK, "<!doctype html>"},
		// http.FileServer canonicalizes /index.html to / with a 301.
		{"index.html redirects to root", "/index.html", http.StatusMovedPermanently, ""},
		{"favicon served", "/favicon.svg", http.StatusOK, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Fatalf("got status %d; want %d", w.Code, tt.wantStatus)
			}
			if tt.wantSnippet != "" &&
				!strings.Contains(strings.ToLower(w.Body.String()), tt.wantSnippet) {
				t.Errorf("body missing %q:\n%s", tt.wantSnippet, w.Body.String())
			}
		})
	}
}

func TestHandler_spaFallback(t *testing.T) {
	h := Handler(4566, 4568)

	tests := []struct {
		name       string
		path       string
		accept     string
		wantStatus int
	}{
		{"navigation route falls back to index", "/explorer", "text/html", http.StatusOK},
		{"deep route falls back to index", "/resource-groups/rg-x", "text/html", http.StatusOK},
		{"missing asset without html accept is 404", "/assets/missing.js", "*/*", http.StatusNotFound},
		{"missing asset with empty accept is 404", "/assets/missing.css", "", http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			if tt.accept != "" {
				req.Header.Set("Accept", tt.accept)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Fatalf("path %q: got status %d; want %d", tt.path, w.Code, tt.wantStatus)
			}
			if tt.wantStatus == http.StatusOK &&
				!strings.Contains(strings.ToLower(w.Body.String()), "<!doctype html>") {
				t.Errorf("path %q: expected index.html fallback, got:\n%s", tt.path, w.Body.String())
			}
		})
	}
}

func TestHandler_proxiesAPIAndHealth(t *testing.T) {
	// ARM backend is HTTPS (self-signed) in production; the proxy skips TLS
	// verification, so a TLS test server stands in for it.
	arm := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("arm:" + r.URL.Path))
	}))
	defer arm.Close()

	// Health backend is plain HTTP.
	health := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("health:" + r.URL.Path))
	}))
	defer health.Close()

	h := Handler(portOf(t, arm.URL), portOf(t, health.URL))

	tests := []struct {
		name     string
		path     string
		wantBody string
	}{
		{"api stream proxied to arm", "/api/requests/stream", "arm:/api/requests/stream"},
		{"api state proxied to arm", "/api/state/export", "arm:/api/state/export"},
		{"health proxied to health server", "/health", "health:/health"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("path %q: got status %d; want 200", tt.path, w.Code)
			}
			if got := w.Body.String(); got != tt.wantBody {
				t.Errorf("path %q: got body %q; want %q", tt.path, got, tt.wantBody)
			}
		})
	}
}

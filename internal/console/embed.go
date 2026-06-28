package console

import (
	"crypto/tls"
	"crypto/x509"
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
)

//go:embed dist/*
var distFS embed.FS

// Handler returns an http.Handler that serves the console SPA.
//
// apiPort is the HTTPS ARM port (e.g. 4566) and healthPort is the plain-HTTP
// health port (e.g. 4568). The handler proxies:
//
//   - /api/*        → https://localhost:<apiPort>/api/*
//   - /health       → http://localhost:<healthPort>/health
//
// so the browser can reach both endpoints same-origin without CORS headers.
// All other paths either serve embedded static assets or fall back to
// index.html for client-side routing (only for navigation requests that
// Accept text/html; asset 404s become plain 404 responses).
//
// armCertPool is the trust anchor for the ARM server's self-signed cert. When
// non-nil the proxy verifies the loopback TLS connection against it instead of
// skipping verification; the ARM cert carries a `localhost` SAN so the proxy's
// `localhost` dial target validates. A nil pool falls back to skipping
// verification (the loopback-only connection still never leaves the host).
func Handler(apiPort, healthPort int, armCertPool *x509.CertPool) http.Handler {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic("console: embedded dist not found: " + err.Error())
	}
	fileServer := http.FileServer(http.FS(sub))

	// Proxy for ARM API calls. The ARM server uses a self-signed TLS cert; trust
	// it via the supplied pool rather than disabling verification.
	armTLS := &tls.Config{MinVersion: tls.VersionTLS12}
	if armCertPool != nil {
		armTLS.RootCAs = armCertPool
	} else {
		armTLS.InsecureSkipVerify = true //nolint:gosec // loopback only; no cert pool supplied
	}
	armTarget, _ := url.Parse(fmt.Sprintf("https://localhost:%d", apiPort))
	armProxy := httputil.NewSingleHostReverseProxy(armTarget)
	armProxy.Transport = &http.Transport{TLSClientConfig: armTLS}

	// Proxy for the plain-HTTP health server.
	healthTarget, _ := url.Parse(fmt.Sprintf("http://localhost:%d", healthPort))
	healthProxy := httputil.NewSingleHostReverseProxy(healthTarget)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// Proxy /api/* and /health to the appropriate backend servers so the
		// browser can reach them same-origin (no CORS required).
		if strings.HasPrefix(path, "/api/") || path == "/api" {
			armProxy.ServeHTTP(w, r)
			return
		}
		if path == "/health" {
			healthProxy.ServeHTTP(w, r)
			return
		}

		if path == "/" {
			fileServer.ServeHTTP(w, r)
			return
		}

		// Try serving the file directly.
		clean := strings.TrimPrefix(path, "/")
		if f, err := sub.Open(clean); err == nil {
			f.Close()
			fileServer.ServeHTTP(w, r)
			return
		}

		// SPA fallback: only serve index.html for navigation requests
		// (those that Accept text/html). Asset requests that 404 get a plain
		// 404 so the browser doesn't try to parse HTML as JavaScript/CSS.
		if strings.Contains(r.Header.Get("Accept"), "text/html") {
			r.URL.Path = "/"
			fileServer.ServeHTTP(w, r)
			return
		}

		http.NotFound(w, r)
	})
}

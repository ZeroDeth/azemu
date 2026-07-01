package main

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zerodeth/azemu/internal/arm"
	"github.com/zerodeth/azemu/internal/auth"
	"github.com/zerodeth/azemu/internal/store"
)

// TestCDNHostMux_routing locks down the real entrypoint: a
// {endpoint}.azureedge.net host must be dispatched to the classic CDN data
// plane, a {endpoint}.azurefd.net host must be dispatched to the Front Door
// data plane, and every other host falls through to ARM. All three bypass
// the ARM router when matched.
func TestCDNHostMux_routing(t *testing.T) {
	ar := arm.NewRouter(store.NewMemoryStore(), "http://azurite:10000", "https://kv", "redis://r:6379")

	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusTeapot)
	})
	mux := cdnHostMux(ar, next)

	// CDN host: handled by ServeCDNContent, so the ARM next handler is bypassed.
	// No endpoint is seeded, so it returns 404, but the point is that the ARM
	// path was not taken.
	req := httptest.NewRequest(http.MethodGet, "http://otacdn.azureedge.net/c/blob", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if nextCalled {
		t.Fatal("CDN host was routed to the ARM handler instead of ServeCDNContent")
	}

	// Front Door host: handled by ServeAFDContent, so the ARM next handler is
	// bypassed. No endpoint is seeded, so it returns 404, but the point is that
	// the ARM path was not taken.
	nextCalled = false
	req = httptest.NewRequest(http.MethodGet, "http://fdedge.azurefd.net/c/blob", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if nextCalled {
		t.Fatal("Front Door host was routed to the ARM handler instead of ServeAFDContent")
	}
	if rec.Result().StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404 from the unresolved AFD proxy", rec.Result().StatusCode)
	}

	// Non-CDN host: falls through to the ARM handler.
	nextCalled = false
	req = httptest.NewRequest(http.MethodGet, "http://localhost:4566/subscriptions", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if !nextCalled {
		t.Fatal("non-CDN host did not fall through to the ARM handler")
	}
}

// TestArmCertPoolFromTLS verifies the console proxy's trust anchor is built
// from a real cert and degrades to nil (skip-verify fallback) when the cert is
// missing or unparseable.
func TestArmCertPoolFromTLS(t *testing.T) {
	t.Run("valid cert yields a pool that trusts the leaf", func(t *testing.T) {
		cert, _, err := auth.LoadOrGenerateSelfSignedTLS("", "localhost", "127.0.0.1")
		if err != nil {
			t.Fatalf("generate cert: %v", err)
		}
		pool := armCertPoolFromTLS(cert)
		if pool == nil {
			t.Fatal("expected non-nil pool for a valid cert")
		}
		// The pool must verify a TLS server presenting this exact cert under the
		// localhost SAN, which is how the console proxy reaches the ARM port.
		srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		srv.TLS = &tls.Config{Certificates: []tls.Certificate{cert}}
		srv.StartTLS()
		defer srv.Close()

		client := &http.Client{Transport: &http.Transport{
			TLSClientConfig: &tls.Config{RootCAs: pool, ServerName: "localhost", MinVersion: tls.VersionTLS12},
		}}
		resp, err := client.Get(srv.URL)
		if err != nil {
			t.Fatalf("expected pool to trust the server cert: %v", err)
		}
		resp.Body.Close()
	})

	t.Run("empty cert yields nil", func(t *testing.T) {
		if pool := armCertPoolFromTLS(tls.Certificate{}); pool != nil {
			t.Fatal("expected nil pool for an empty certificate")
		}
	})

	t.Run("unparseable cert yields nil", func(t *testing.T) {
		bad := tls.Certificate{Certificate: [][]byte{{0x00, 0x01, 0x02}}}
		if pool := armCertPoolFromTLS(bad); pool != nil {
			t.Fatal("expected nil pool for an unparseable certificate")
		}
	})
}

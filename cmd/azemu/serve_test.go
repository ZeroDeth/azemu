package main

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/zerodeth/azemu/internal/arm"
	"github.com/zerodeth/azemu/internal/auth"
	mw "github.com/zerodeth/azemu/internal/middleware"
	"github.com/zerodeth/azemu/internal/store"
)

// TestCDNHostMux_routing locks down the real entrypoint: a
// {endpoint}.azureedge.net host must be dispatched to the CDN data plane and
// bypass the ARM router, while every other host falls through to ARM.
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

// TestTLSKeyAlgorithm covers the key types tls.X509KeyPair accepts, because
// AZEMU_CERT_PATH lets a user supply any of them and /health must describe the
// certificate actually in use rather than the one azemu would have generated.
func TestTLSKeyAlgorithm(t *testing.T) {
	t.Parallel()

	newCert := func(t *testing.T, pub, priv any) tls.Certificate {
		t.Helper()
		tmpl := &x509.Certificate{
			SerialNumber: big.NewInt(1),
			Subject:      pkix.Name{CommonName: "azemu test"},
			NotBefore:    time.Now().Add(-time.Hour),
			NotAfter:     time.Now().Add(time.Hour),
		}
		der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, pub, priv)
		if err != nil {
			t.Fatalf("create certificate: %v", err)
		}
		return tls.Certificate{Certificate: [][]byte{der}}
	}

	ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate ecdsa key: %v", err)
	}
	ecP384, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		t.Fatalf("generate ecdsa p384 key: %v", err)
	}
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}
	edPub, edPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate ed25519 key: %v", err)
	}

	tests := []struct {
		name string
		cert tls.Certificate
		want string
	}{
		{"ecdsa p256", newCert(t, &ecKey.PublicKey, ecKey), "ECDSA P-256"},
		{"ecdsa p384", newCert(t, &ecP384.PublicKey, ecP384), "ECDSA P-384"},
		{"rsa 2048", newCert(t, &rsaKey.PublicKey, rsaKey), "RSA 2048"},
		{"ed25519", newCert(t, edPub, edPriv), "Ed25519"},
		{"empty", tls.Certificate{}, "unknown"},
		{"undecodable", tls.Certificate{Certificate: [][]byte{{0x00}}}, "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tlsKeyAlgorithm(tt.cert); got != tt.want {
				t.Errorf("tlsKeyAlgorithm() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestMethodNotAllowed_throughRouter drives a real router rather than calling
// the handler directly, so removing the r.MethodNotAllowed registration fails
// this test instead of passing silently.
func TestMethodNotAllowed_throughRouter(t *testing.T) {
	t.Parallel()

	tracker := mw.NewUnhandledTracker()
	r := chi.NewRouter()
	r.MethodNotAllowed(methodNotAllowed(r, tracker))
	r.Get("/subscriptions/{id}/resourcegroups/{rg}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	r.Delete("/subscriptions/{id}/resourcegroups/{rg}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPatch, "/subscriptions/s/resourcegroups/rg", nil))

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	// RFC 9110 15.5.6: the 405 must advertise what this server does support.
	allow := rec.Header().Get("Allow")
	for _, want := range []string{http.MethodGet, http.MethodDelete} {
		if !strings.Contains(allow, want) {
			t.Errorf("Allow = %q, missing registered method %s", allow, want)
		}
	}
	if strings.Contains(allow, http.MethodPatch) {
		t.Errorf("Allow = %q, must not advertise the rejected method", allow)
	}
	if strings.Contains(allow, http.MethodPut) {
		t.Errorf("Allow = %q, advertises a method that is not registered", allow)
	}

	var body struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("response is not the Azure error envelope: %v", err)
	}
	if body.Error.Code != "MethodNotAllowed" {
		t.Errorf("code = %q, want MethodNotAllowed", body.Error.Code)
	}
	if !strings.Contains(body.Error.Message, "PATCH") {
		t.Errorf("message %q does not name the rejected method", body.Error.Message)
	}

	// The verb gap must be discoverable via /api/unhandled, like a missing route.
	if got := tracker.List(); len(got) != 1 {
		t.Errorf("tracker recorded %d entries, want 1: %v", len(got), got)
	}
}

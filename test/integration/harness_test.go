//go:build integration

package integration

import (
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/zerodeth/azemu/internal/arm"
	"github.com/zerodeth/azemu/internal/auth"
	"github.com/zerodeth/azemu/internal/metadata"
	mw "github.com/zerodeth/azemu/internal/middleware"
	"github.com/zerodeth/azemu/internal/store"
	"github.com/zerodeth/azemu/pkg/config"
)

// testTenantID is the fake tenant the auth/metadata suites use. Kept as a
// package-level constant so that every integration test talks to the same
// issuer; a stable tenant id is what lets a single OIDC discovery lookup be
// reused to verify a token minted by a separate call.
const testTenantID = "00000000-0000-0000-0000-000000000001"

// buildProductionLikeServer wires a chi router that mirrors cmd/azemu/main.go
// as closely as possible while staying inside httptest. Every middleware,
// every route group, every mount point is present so these tests catch
// contract bugs at the seams between packages — exactly the class of
// failure that TODO.md M1-M5 documents.
//
// The server is HTTPS (httptest.NewTLSServer) because OIDC discovery and
// the metadata service both hardcode `https://` when building advertised
// URLs in their response bodies — that mirrors cmd/azemu/main.go which
// always uses ListenAndServeTLS. Callers must use `srv.Client()` (not
// http.DefaultClient) so the server's self-signed cert is trusted.
//
// What is intentionally left out:
//
//   - Signal handling, graceful shutdown, cert persistence: these are
//     cmd/azemu/main.go concerns, not handler concerns.
//
// Each call produces a fresh MemoryStore and a fresh TokenService so state
// never leaks between tests.
func buildProductionLikeServer(t *testing.T) *httptest.Server {
	t.Helper()

	cfg := &config.Config{
		HTTPPort:         4566,
		HTTPSPort:        4567,
		SubscriptionID:   "00000000-0000-0000-0000-000000000000",
		TenantID:         testTenantID,
		MetadataHost:     "localhost:4567",
		AzuriteEndpoint:  "http://azurite-test:10000",
		KeyVaultEndpoint: "https://kv-test",
		RedisEndpoint:    "redis://redis-test:6379",
	}

	s := store.NewMemoryStore()
	tokenSvc, err := auth.NewTokenService(cfg.TenantID, s)
	if err != nil {
		t.Fatalf("NewTokenService: %v", err)
	}
	metaSvc := metadata.NewService(cfg)
	armRouter := arm.NewRouter(s, cfg.AzuriteEndpoint, cfg.KeyVaultEndpoint, cfg.RedisEndpoint, tokenSvc)

	r := chi.NewRouter()
	// Mirror the production middleware order from cmd/azemu/main.go.
	r.Use(mw.NormalizePath)
	r.Use(mw.AzureHeaders)
	r.Use(mw.RequireAPIVersion)

	r.Route("/metadata", metaSvc.Routes)
	r.Route("/{tenantID}", tokenSvc.TenantRoutes)
	r.Route("/subscriptions", armRouter.Routes)
	r.Route("/keyvault", armRouter.KeyVaultDataPlaneRoutes)

	srv := httptest.NewTLSServer(r)
	t.Cleanup(srv.Close)
	return srv
}

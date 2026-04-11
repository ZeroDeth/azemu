package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/zerodeth/azemu/internal/arm"
	"github.com/zerodeth/azemu/internal/auth"
	"github.com/zerodeth/azemu/internal/metadata"
	mw "github.com/zerodeth/azemu/internal/middleware"
	"github.com/zerodeth/azemu/internal/store"
	"github.com/zerodeth/azemu/pkg/config"
)

func main() {
	log.Logger = zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339}).
		With().Timestamp().Caller().Logger()

	cfg := config.Load()

	state := store.NewMemoryStore()
	tokenSvc := auth.NewTokenService(cfg.TenantID)
	metaSvc := metadata.NewService(cfg)
	armRouter := arm.NewRouter(state)

	r := chi.NewRouter()
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	// NormalizePath must run BEFORE RequireAPIVersion (which prefix-checks
	// the lowercase form of the path) and BEFORE chi route matching, so
	// camelCase ARM literals from real Azure clients are rewritten to the
	// lowercase form chi expects, and double-slash artefacts from client-
	// side URL concatenation are collapsed.
	r.Use(mw.NormalizePath)
	r.Use(mw.AzureHeaders)
	r.Use(mw.RequireAPIVersion)
	r.Use(chimw.Recoverer)

	// Debug API: list unhandled routes seen this session
	r.NotFound(mw.LogUnhandledRequests())
	r.HandleFunc("/api/unhandled", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"unhandled_routes": mw.Unhandled.List(),
		})
	})

	// Metadata endpoints (HTTPS, used by azurerm provider)
	r.Route("/metadata", metaSvc.Routes)

	// OAuth2 / token endpoints
	r.Route("/{tenantID}/oauth2", tokenSvc.Routes)
	r.Route("/{tenantID}/oauth2/v2.0", tokenSvc.RoutesV2)

	// OIDC discovery
	r.Get("/{tenantID}/.well-known/openid-configuration", tokenSvc.OpenIDConfig)
	r.Get("/{tenantID}/discovery/v2.0/keys", tokenSvc.JWKS)

	// ARM endpoints
	r.Route("/subscriptions", armRouter.Routes)

	// Load existing TLS cert from AZEMU_CERT_PATH if set, otherwise generate
	// fresh. Persistence eliminates the per-restart keychain trust friction
	// that blocks Phase 1 debugging cycles — trust once, restart freely.
	tlsCfg, generated, err := auth.LoadOrGenerateSelfSignedTLS(cfg.CertPath, "localhost", "127.0.0.1")
	if err != nil {
		// LoadOrGenerateSelfSignedTLS may return a usable cert with a
		// non-nil err if persistence failed; only fatal if there is no cert.
		if len(tlsCfg.Certificate) == 0 {
			log.Fatal().Err(err).Msg("failed to load/generate TLS cert")
		}
		log.Warn().Err(err).Str("path", cfg.CertPath).Msg("could not persist TLS cert; continuing with in-memory cert")
	}
	sharedTLS := &tls.Config{Certificates: []tls.Certificate{tlsCfg}}

	// Surface the cert path so the user can trust it. When AZEMU_CERT_PATH
	// is set, the bundle file already contains both cert and key — point
	// SSL_CERT_FILE at it (or the cert-only export below). When unset,
	// fall back to writing a cert-only file to OS temp dir for legacy UX.
	switch {
	case cfg.CertPath != "" && generated:
		log.Info().Str("path", cfg.CertPath).Msg("TLS cert generated and persisted; trust once, restart freely")
	case cfg.CertPath != "" && !generated:
		log.Info().Str("path", cfg.CertPath).Msg("TLS cert loaded from existing bundle; trust unchanged")
	default:
		certPath, err := auth.WriteCertToTemp(tlsCfg)
		if err != nil {
			log.Warn().Err(err).Msg("could not write cert file")
		} else {
			log.Info().Str("path", certPath).Msg("TLS cert written, export SSL_CERT_FILE to trust it")
		}
	}

	log.Info().
		Str("arm", fmt.Sprintf("https://localhost:%d", cfg.HTTPPort)).
		Str("metadata", fmt.Sprintf("https://localhost:%d", cfg.HTTPSPort)).
		Msg("azemu starting")

	// ARM server (HTTPS, required because metadata declares https:// URLs)
	httpSrv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.HTTPPort),
		Handler:      r,
		TLSConfig:    sharedTLS,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	// Metadata/auth server (HTTPS)
	httpsSrv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.HTTPSPort),
		Handler:      r,
		TLSConfig:    sharedTLS,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	go func() {
		if err := httpSrv.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("arm server failed")
		}
	}()
	go func() {
		if err := httpsSrv.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("metadata server failed")
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info().Msg("shutting down")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	httpSrv.Shutdown(ctx)
	httpsSrv.Shutdown(ctx)
}

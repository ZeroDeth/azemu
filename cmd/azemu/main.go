package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"flag"
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

// Version is overridden at build time via -ldflags "-X main.Version=...".
var Version = "dev"

func main() {
	// --- flags (stdlib only, per .claude/rules/go-style.md) ---
	fs := flag.NewFlagSet("azemu", flag.ExitOnError)
	showVersion := fs.Bool("version", false, "Print version and exit")
	fs.Usage = func() { printUsage(os.Stderr) }
	_ = fs.Parse(os.Args[1:])

	if *showVersion {
		fmt.Fprintf(os.Stdout, "azemu %s\n", Version)
		os.Exit(0)
	}

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
	// that blocks Phase 1 debugging cycles -- trust once, restart freely.
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

	// Resolve cert path for the banner.
	certDisplay := cfg.CertPath
	switch {
	case cfg.CertPath != "" && generated:
		log.Info().Str("path", cfg.CertPath).Msg("TLS cert generated and persisted; trust once, restart freely")
	case cfg.CertPath != "" && !generated:
		log.Info().Str("path", cfg.CertPath).Msg("TLS cert loaded from existing bundle; trust unchanged")
	default:
		certPath, werr := auth.WriteCertToTemp(tlsCfg)
		if werr != nil {
			log.Warn().Err(werr).Msg("could not write cert file")
			certDisplay = "(in-memory only)"
		} else {
			log.Info().Str("path", certPath).Msg("TLS cert written, export SSL_CERT_FILE to trust it")
			certDisplay = certPath
		}
	}

	// --- startup banner ---
	printBanner(os.Stderr, cfg, certDisplay)

	startTime := time.Now()

	// --- Health server (plain HTTP, no TLS, no middleware) ---
	healthMux := http.NewServeMux()
	healthMux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status":         "ok",
			"version":        Version,
			"uptime_seconds": int(time.Since(startTime).Seconds()),
		})
	})
	healthSrv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.HealthPort),
		Handler:      healthMux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}

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
		if err := healthSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("health server failed")
		}
	}()
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
	_ = healthSrv.Shutdown(ctx)
	_ = httpSrv.Shutdown(ctx)
	_ = httpsSrv.Shutdown(ctx)
}

func printBanner(w *os.File, cfg *config.Config, certPath string) {
	fmt.Fprintf(w, "\nazemu %s\n", Version)
	fmt.Fprintf(w, "  ARM (HTTPS)        https://localhost:%d\n", cfg.HTTPPort)
	fmt.Fprintf(w, "  metadata (HTTPS)   https://localhost:%d\n", cfg.HTTPSPort)
	fmt.Fprintf(w, "  health (HTTP)      http://localhost:%d/health\n", cfg.HealthPort)
	fmt.Fprintf(w, "  cert bundle        %s\n", certPath)
	fmt.Fprintf(w, "  docs               https://github.com/zerodeth/azemu\n\n")
}

func printUsage(w *os.File) {
	fmt.Fprintf(w, "azemu %s -- local Azure emulator for Terraform\n\n", Version)
	fmt.Fprintf(w, "Usage: azemu [flags]\n\n")
	fmt.Fprintf(w, "Flags:\n")
	fmt.Fprintf(w, "  -version    Print version and exit\n")
	fmt.Fprintf(w, "  -h, -help   Print this help and exit\n\n")
	fmt.Fprintf(w, "Environment variables:\n")
	fmt.Fprintf(w, "  AZEMU_CERT_PATH         Persistent TLS cert+key PEM bundle path\n")
	fmt.Fprintf(w, "  AZEMU_METADATA_HOST     Host for URLs in /metadata/endpoints (default localhost:4567)\n")
	fmt.Fprintf(w, "  AZEMU_SUBSCRIPTION_ID   Mock subscription ID (default 00000000-...)\n")
	fmt.Fprintf(w, "  AZEMU_TENANT_ID         Mock tenant ID (default 00000000-...)\n\n")
	fmt.Fprintf(w, "Ports:\n")
	fmt.Fprintf(w, "  :4566   ARM API (HTTPS)\n")
	fmt.Fprintf(w, "  :4567   Metadata / OAuth2 / OIDC (HTTPS)\n")
	fmt.Fprintf(w, "  :4568   Health check (plain HTTP)\n\n")
}

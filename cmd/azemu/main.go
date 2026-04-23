package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
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
	persistPath := fs.String("persist", "", "File path for write-through state persistence")
	importPath := fs.String("import", "", "Load state from file on startup")
	exportPath := fs.String("export", "", "Dump state to file and exit")
	fs.Usage = func() { printUsage(os.Stderr) }
	_ = fs.Parse(os.Args[1:])

	if *showVersion {
		fmt.Fprintf(os.Stdout, "azemu %s\n", Version)
		os.Exit(0)
	}

	log.Logger = zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339}).
		With().Timestamp().Caller().Logger()

	cfg := config.Load()

	// CLI flags override env vars for persist path.
	if *persistPath != "" {
		cfg.PersistPath = *persistPath
	}
	cfg.ImportPath = *importPath
	cfg.ExportPath = *exportPath

	// --- store selection ---
	var state store.Store
	if cfg.PersistPath != "" {
		fs, err := store.NewFileStore(cfg.PersistPath)
		if err != nil {
			log.Fatal().Err(err).Str("path", cfg.PersistPath).Msg("failed to open persist store")
		}
		state = fs
		log.Info().Str("path", cfg.PersistPath).Msg("file-backed store enabled")
	} else {
		state = store.NewMemoryStore()
	}

	// --import: load state from file, then continue serving.
	if cfg.ImportPath != "" {
		data, err := os.ReadFile(cfg.ImportPath)
		if err != nil {
			log.Fatal().Err(err).Str("path", cfg.ImportPath).Msg("failed to read import file")
		}
		if err := state.Import(data); err != nil {
			log.Fatal().Err(err).Str("path", cfg.ImportPath).Msg("failed to import state")
		}
		log.Info().Str("path", cfg.ImportPath).Msg("state imported from file")
	}

	// --export: dump state to file and exit immediately.
	if cfg.ExportPath != "" {
		data, err := state.Export()
		if err != nil {
			log.Fatal().Err(err).Msg("failed to export state")
		}
		if err := os.WriteFile(cfg.ExportPath, data, 0600); err != nil {
			log.Fatal().Err(err).Str("path", cfg.ExportPath).Msg("failed to write export file")
		}
		log.Info().Str("path", cfg.ExportPath).Msg("state exported")
		os.Exit(0)
	}

	tokenSvc, err := auth.NewTokenService(cfg.TenantID)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create token service")
	}
	metaSvc := metadata.NewService(cfg)
	armRouter := arm.NewRouter(state, cfg.AzuriteEndpoint, cfg.KeyVaultEndpoint)

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
	unhandled := mw.NewUnhandledTracker()
	r.NotFound(mw.LogUnhandledRequests(unhandled))
	r.HandleFunc("/api/unhandled", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"unhandled_routes": unhandled.List(),
		})
	})

	// State management API (no api-version required; azemu admin endpoints)
	r.Get("/api/state/export", func(w http.ResponseWriter, r *http.Request) {
		data, err := state.Export()
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"export failed: %s"}`, err), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(data)
	})
	r.Post("/api/state/import", func(w http.ResponseWriter, r *http.Request) {
		data, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"read body: %s"}`, err), http.StatusBadRequest)
			return
		}
		if err := state.Import(data); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"import failed: %s"}`, err), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"imported"}`))
	})
	r.Post("/api/state/reset", func(w http.ResponseWriter, r *http.Request) {
		state.Reset()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"reset"}`))
	})

	// Metadata endpoints (HTTPS, used by azurerm provider)
	r.Route("/metadata", metaSvc.Routes)

	// Auth surface: OAuth2 token, OIDC discovery, JWKS (all tenant-scoped)
	r.Route("/{tenantID}", tokenSvc.TenantRoutes)

	// ARM endpoints
	r.Route("/subscriptions", armRouter.Routes)

	// Key Vault data-plane endpoints (secrets). The vaultUri returned by the
	// management plane GET/PUT is rewritten to point here so the azurerm provider
	// can create and read azurerm_key_vault_secret resources against azemu.
	r.Route("/keyvault", armRouter.KeyVaultDataPlaneRoutes)

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

	errCh := make(chan error, 3)
	go func() {
		if err := healthSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("health server: %w", err)
		}
	}()
	go func() {
		if err := httpSrv.ListenAndServeTLS("", ""); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("arm server: %w", err)
		}
	}()
	go func() {
		if err := httpsSrv.ListenAndServeTLS("", ""); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("metadata server: %w", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	select {
	case sig := <-quit:
		log.Info().Str("signal", sig.String()).Msg("received signal")
	case err := <-errCh:
		log.Fatal().Err(err).Msg("server failed to start")
	}

	log.Info().Msg("shutting down")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := healthSrv.Shutdown(ctx); err != nil {
		log.Warn().Err(err).Msg("health server shutdown")
	}
	if err := httpSrv.Shutdown(ctx); err != nil {
		log.Warn().Err(err).Msg("arm server shutdown")
	}
	if err := httpsSrv.Shutdown(ctx); err != nil {
		log.Warn().Err(err).Msg("metadata server shutdown")
	}
}

func printBanner(w *os.File, cfg *config.Config, certPath string) {
	fmt.Fprintf(w, "\nazemu %s\n", Version)
	fmt.Fprintf(w, "  ARM (HTTPS)        https://localhost:%d\n", cfg.HTTPPort)
	fmt.Fprintf(w, "  metadata (HTTPS)   https://localhost:%d\n", cfg.HTTPSPort)
	fmt.Fprintf(w, "  health (HTTP)      http://localhost:%d/health\n", cfg.HealthPort)
	fmt.Fprintf(w, "  cert bundle        %s\n", certPath)
	if cfg.PersistPath != "" {
		fmt.Fprintf(w, "  persist            %s\n", cfg.PersistPath)
	}
	fmt.Fprintf(w, "  docs               https://github.com/zerodeth/azemu\n\n")
}

func printUsage(w *os.File) {
	fmt.Fprintf(w, "azemu %s -- local Azure emulator for Terraform\n\n", Version)
	fmt.Fprintf(w, "Usage: azemu [flags]\n\n")
	fmt.Fprintf(w, "Flags:\n")
	fmt.Fprintf(w, "  -version          Print version and exit\n")
	fmt.Fprintf(w, "  -persist <path>   Write-through state persistence to JSON file\n")
	fmt.Fprintf(w, "  -import <path>    Load state from file on startup\n")
	fmt.Fprintf(w, "  -export <path>    Dump state to file and exit\n")
	fmt.Fprintf(w, "  -h, -help         Print this help and exit\n\n")
	fmt.Fprintf(w, "Environment variables:\n")
	fmt.Fprintf(w, "  AZEMU_CERT_PATH           Persistent TLS cert+key PEM bundle path\n")
	fmt.Fprintf(w, "  AZEMU_PERSIST_PATH        Write-through state persistence path (same as -persist)\n")
	fmt.Fprintf(w, "  AZEMU_METADATA_HOST       Host for URLs in /metadata/endpoints (default localhost:4567)\n")
	fmt.Fprintf(w, "  AZEMU_SUBSCRIPTION_ID     Mock subscription ID (default 00000000-...)\n")
	fmt.Fprintf(w, "  AZEMU_TENANT_ID           Mock tenant ID (default 00000000-...)\n")
	fmt.Fprintf(w, "  AZEMU_AZURITE_ENDPOINT    Azurite blob base URL for storage endpoints (default http://azurite:10000)\n")
	fmt.Fprintf(w, "  AZEMU_KV_ENDPOINT         Key Vault data-plane base URL embedded in vaultUri (default https://localhost:4566)\n\n")
	fmt.Fprintf(w, "Ports:\n")
	fmt.Fprintf(w, "  :4566   ARM API (HTTPS)\n")
	fmt.Fprintf(w, "  :4567   Metadata / OAuth2 / OIDC (HTTPS)\n")
	fmt.Fprintf(w, "  :4568   Health check (plain HTTP)\n\n")
}

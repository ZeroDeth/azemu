package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
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
	"github.com/rs/zerolog/log"

	"github.com/zerodeth/azemu/internal/ado"
	"github.com/zerodeth/azemu/internal/arm"
	"github.com/zerodeth/azemu/internal/auth"
	"github.com/zerodeth/azemu/internal/console"
	"github.com/zerodeth/azemu/internal/metadata"
	mw "github.com/zerodeth/azemu/internal/middleware"
	"github.com/zerodeth/azemu/internal/store"
	"github.com/zerodeth/azemu/pkg/config"
)

func runServe(args []string) error {
	fs := flag.NewFlagSet("azemu serve", flag.ExitOnError)
	persistPath := fs.String("persist", "", "File path for write-through state persistence")
	importPath := fs.String("import", "", "Load state from file on startup")
	exportPath := fs.String("export", "", "Dump state to file and exit")
	fs.Usage = func() { printServeUsage(os.Stderr) }
	_ = fs.Parse(args)

	cfg := config.Load()

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

	tokenSvc, err := auth.NewTokenService(cfg.TenantID, newFederatedIdentityResolver(state))
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create token service")
	}
	metaSvc := metadata.NewService(cfg)
	armRouter := arm.NewRouter(state, cfg.AzuriteEndpoint, cfg.KeyVaultEndpoint, cfg.RedisEndpoint, tokenSvc)
	imdsSvc := auth.NewIMDSService(tokenSvc)
	adoOIDC, err := ado.NewOIDCService()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create ADO OIDC service")
	}
	adoSC := ado.NewServiceConnectionService()

	reqLog := mw.NewRequestRecorder(500)

	r := chi.NewRouter()
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(mw.NormalizePath)
	r.Use(mw.AzureHeaders)
	r.Use(mw.RequireAPIVersion)
	r.Use(chimw.Recoverer)

	unhandled := mw.NewUnhandledTracker()
	r.NotFound(mw.LogUnhandledRequests(unhandled))
	r.HandleFunc("/api/unhandled", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"unhandled_routes": unhandled.List(),
		})
	})

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
	r.Get("/api/requests/stream", reqLog.SSEHandler)

	r.Route("/metadata/identity", imdsSvc.Routes)
	r.Route("/metadata", metaSvc.Routes)
	r.Route("/{tenantID}", tokenSvc.TenantRoutes)

	// ARM routes are wrapped with the request recorder so only ARM traffic
	// appears in the request log – not health, metadata, state API, or ADO.
	r.Group(func(r chi.Router) {
		r.Use(reqLog.Middleware)
		r.Route("/subscriptions", armRouter.Routes)
		r.Route("/keyvault", armRouter.KeyVaultDataPlaneRoutes)
		armRouter.KeyVaultNestedItemRoutes(r)
	})

	r.Route("/ado", func(r chi.Router) {
		adoOIDC.Routes(r)
		adoSC.ServiceConnectionRoutes(r)
	})

	// *.vault.localhost serves the per-vault Key Vault data-plane hosts
	// ({vaultName}.vault.localhost) that the azurerm provider requires in
	// vaultUri. *.azureedge.net serves the CDN endpoint content hosts
	// ({endpoint}.azureedge.net) that the CDN data-plane proxy answers. Bundles
	// generated before either SAN existed are regenerated automatically; the new
	// cert must be trusted again.
	tlsCfg, generated, err := auth.LoadOrGenerateSelfSignedTLS(cfg.CertPath, "localhost", "127.0.0.1", "*.vault.localhost", "*.azureedge.net")
	if err != nil {
		if len(tlsCfg.Certificate) == 0 {
			log.Fatal().Err(err).Msg("failed to load/generate TLS cert")
		}
		log.Warn().Err(err).Str("path", cfg.CertPath).Msg("could not persist TLS cert; continuing with in-memory cert")
	}
	sharedTLS := &tls.Config{Certificates: []tls.Certificate{tlsCfg}}

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

	printBanner(os.Stderr, cfg, certDisplay)

	startTime := time.Now()

	healthMux := http.NewServeMux()
	healthMux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
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

	// On the ARM port, multiplex the CDN content data plane: a request to a
	// {endpoint}.azureedge.net host is served by the CDN proxy, everything else
	// by the ARM control plane. Real Azure serves CDN content from a distinct
	// host; azemu colocates both on the ARM port so one trusted cert and port
	// cover the read path. Mirrors the Key Vault {vault}.vault.localhost split.
	armAndCDN := cdnHostMux(armRouter, r)

	httpSrv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.HTTPPort),
		Handler:      armAndCDN,
		TLSConfig:    sharedTLS,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	httpsSrv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.HTTPSPort),
		Handler:      r,
		TLSConfig:    sharedTLS,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	adoSrv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.ADOPort),
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	// Trust anchor for the console's ARM reverse proxy: the server's own
	// self-signed leaf, so the loopback proxy verifies TLS instead of skipping
	// it. nil if the cert cannot be parsed; Handler then falls back to skipping
	// verification on the loopback-only hop.
	armCertPool := armCertPoolFromTLS(tlsCfg)
	consoleSrv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.ConsolePort),
		Handler:      console.Handler(cfg.HTTPPort, cfg.HealthPort, armCertPool),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 5)
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
	go func() {
		if err := adoSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("ado server: %w", err)
		}
	}()
	go func() {
		if err := consoleSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("console server: %w", err)
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
	if err := adoSrv.Shutdown(ctx); err != nil {
		log.Warn().Err(err).Msg("ado server shutdown")
	}
	if err := consoleSrv.Shutdown(ctx); err != nil {
		log.Warn().Err(err).Msg("console server shutdown")
	}

	return nil
}

// armCertPoolFromTLS builds a cert pool containing the server's own leaf so the
// console's loopback ARM reverse proxy can verify TLS instead of skipping it.
// Returns nil if the certificate cannot be parsed, in which case the console
// handler falls back to skipping verification on the loopback-only hop.
func armCertPoolFromTLS(cert tls.Certificate) *x509.CertPool {
	if len(cert.Certificate) == 0 {
		return nil
	}
	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		return nil
	}
	pool := x509.NewCertPool()
	pool.AddCert(leaf)
	return pool
}

// cdnHostMux dispatches CDN content hosts ({endpoint}.azureedge.net) to the CDN
// data plane and every other host to the ARM control plane. Extracted from the
// server wiring so the routing decision is unit-testable.
func cdnHostMux(armRouter *arm.Router, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if arm.IsCDNContentHost(req.Host) {
			armRouter.ServeCDNContent(w, req)
			return
		}
		next.ServeHTTP(w, req)
	})
}

func printBanner(w *os.File, cfg *config.Config, certPath string) {
	fmt.Fprintf(w, "\nazemu %s\n", Version)
	fmt.Fprintf(w, "  ARM (HTTPS)        https://localhost:%d\n", cfg.HTTPPort)
	fmt.Fprintf(w, "  metadata (HTTPS)   https://localhost:%d\n", cfg.HTTPSPort)
	fmt.Fprintf(w, "  health (HTTP)      http://localhost:%d/health\n", cfg.HealthPort)
	fmt.Fprintf(w, "  ADO OIDC (HTTP)    http://localhost:%d/ado\n", cfg.ADOPort)
	fmt.Fprintf(w, "  console (HTTP)     http://localhost:%d\n", cfg.ConsolePort)
	fmt.Fprintf(w, "  cert bundle        %s\n", certPath)
	if cfg.PersistPath != "" {
		fmt.Fprintf(w, "  persist            %s\n", cfg.PersistPath)
	}
	fmt.Fprintf(w, "  docs               https://github.com/zerodeth/azemu\n\n")
}

func printServeUsage(w *os.File) {
	fmt.Fprintf(w, "Usage: azemu serve [flags]\n\n")
	fmt.Fprintf(w, "Start the azemu emulator server.\n\n")
	fmt.Fprintf(w, "Flags:\n")
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
	fmt.Fprintf(w, "  AZEMU_KV_ENDPOINT         Key Vault data-plane base URL embedded in vaultUri (default https://localhost:4566)\n")
	fmt.Fprintf(w, "  AZEMU_ADO_PORT            Plain-HTTP port for ADO OIDC and service connection emulation (default 4569)\n")
	fmt.Fprintf(w, "  AZEMU_CONSOLE_PORT        Plain-HTTP port for the web console SPA (default 4570)\n\n")
	fmt.Fprintf(w, "Ports:\n")
	fmt.Fprintf(w, "  :4566   ARM API (HTTPS)\n")
	fmt.Fprintf(w, "  :4567   Metadata / OAuth2 / OIDC (HTTPS)\n")
	fmt.Fprintf(w, "  :4568   Health check (plain HTTP)\n")
	fmt.Fprintf(w, "  :4569   ADO OIDC / service connections (plain HTTP)\n")
	fmt.Fprintf(w, "  :4570   Web console (plain HTTP)\n\n")
}

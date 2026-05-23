package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/zerodeth/azemu/pkg/config"
)

// runTF is the `azemu tf` subcommand. It ensures azemu is running, injects
// the env vars the azurerm provider expects, and exec's `terraform <args>`.
//
// Auto-start behaviour:
//  1. Probe the health endpoint. If it responds, azemu is already running.
//  2. Otherwise, start azemu serve as a background child process.
//  3. Wait for the health endpoint to become ready (up to 30 s).
//  4. Set SSL_CERT_FILE, ARM_METADATA_HOSTNAME, ARM_SUBSCRIPTION_ID,
//     ARM_TENANT_ID, ARM_CLIENT_ID, ARM_CLIENT_SECRET.
//  5. Exec terraform with the caller's args.
func runTF(args []string) error {
	cfg := config.Load()

	healthURL := fmt.Sprintf("http://localhost:%d/health", cfg.HealthPort)

	// Check if azemu is already running.
	running := probeHealth(healthURL)

	// Auto-start if not running.
	var childProc *os.Process
	if !running {
		log.Info().Msg("azemu is not running; starting in background")
		proc, err := startAzemuBackground()
		if err != nil {
			return fmt.Errorf("auto-start azemu: %w", err)
		}
		childProc = proc

		if err := waitForHealth(healthURL, 30*time.Second); err != nil {
			// Kill the child if it never became healthy.
			_ = childProc.Kill()
			return fmt.Errorf("azemu did not become healthy: %w", err)
		}
		log.Info().Int("pid", childProc.Pid).Msg("azemu is healthy")
	} else {
		log.Info().Msg("azemu is already running")
	}

	// Resolve the cert bundle path. The order:
	//   1. AZEMU_CERT_PATH env var (user-configured persistent cert)
	//   2. Default location at .azemu/cert-bundle.pem (docker-compose layout)
	//   3. /tmp/azemu-cert.pem (legacy fallback)
	certFile := resolveCertFile(cfg.CertPath)

	// Inject env vars the azurerm provider expects. These are set only if
	// not already present so the caller can override.
	tfEnv := map[string]string{
		"SSL_CERT_FILE":         certFile,
		"ARM_METADATA_HOSTNAME": fmt.Sprintf("127.0.0.1:%d", cfg.HTTPSPort),
		"ARM_SUBSCRIPTION_ID":   cfg.SubscriptionID,
		"ARM_TENANT_ID":         cfg.TenantID,
		"ARM_CLIENT_ID":         "00000000-0000-0000-0000-000000000002",
		"ARM_CLIENT_SECRET":     "azemu-mock-secret",
	}
	for k, v := range tfEnv {
		if os.Getenv(k) == "" {
			os.Setenv(k, v)
		}
	}

	// Find terraform binary.
	tfBin, err := exec.LookPath("terraform")
	if err != nil {
		return fmt.Errorf("terraform not found in PATH: %w", err)
	}

	// If we started a child, set up signal forwarding so ctrl-c kills
	// terraform first and the child process stays alive for the next run.
	// The child will be orphaned (reparented to init/launchd) which is
	// intentional: azemu serves until the user explicitly stops it.
	if childProc != nil {
		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer stop()
		_ = ctx // signals forwarded to terraform via exec
	}

	log.Info().Str("terraform", tfBin).Strs("args", args).Msg("exec terraform")

	// Build argv: terraform <args>
	argv := append([]string{"terraform"}, args...)

	// Exec replaces the current process with terraform.
	return syscall.Exec(tfBin, argv, os.Environ())
}

// probeHealth sends a single GET to the health endpoint and returns true if
// it gets a 200 response.
func probeHealth(url string) bool {
	client := &http.Client{
		Timeout: 2 * time.Second,
		Transport: &http.Transport{
			DialContext: (&net.Dialer{Timeout: 1 * time.Second}).DialContext,
		},
	}
	resp, err := client.Get(url)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// waitForHealth polls the health endpoint until it responds with 200 or the
// timeout expires.
func waitForHealth(url string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if probeHealth(url) {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("timeout after %s waiting for %s", timeout, url)
}

// startAzemuBackground launches `azemu serve` as a detached child process.
// The child inherits the parent's env (including AZEMU_CERT_PATH etc.) and
// writes stdout/stderr to azemu.log in the working directory.
func startAzemuBackground() (*os.Process, error) {
	self, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("resolve own binary path: %w", err)
	}

	// Open a log file for the child's output.
	logPath := "azemu.log"
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("open log file %s: %w", logPath, err)
	}

	proc, err := os.StartProcess(self, []string{"azemu", "serve"}, &os.ProcAttr{
		Env:   os.Environ(),
		Files: []*os.File{os.Stdin, logFile, logFile},
		Sys: &syscall.SysProcAttr{
			Setsid: true, // detach from parent's session
		},
	})
	// Close our handle to the log file; the child has its own fd.
	logFile.Close()
	if err != nil {
		return nil, fmt.Errorf("start azemu serve: %w", err)
	}

	// Release the child so it is not waited on by this process.
	if err := proc.Release(); err != nil {
		log.Warn().Err(err).Msg("could not release child process")
	}

	log.Info().Int("pid", proc.Pid).Str("log", logPath).Msg("started azemu in background")
	return proc, nil
}

// resolveCertFile returns the first cert bundle path that exists on disk.
func resolveCertFile(configPath string) string {
	candidates := []string{configPath}

	// Check for .azemu/cert-bundle.pem relative to cwd (docker-compose layout).
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(cwd, ".azemu", "cert-bundle.pem"))
	}

	// Legacy fallback.
	candidates = append(candidates, "/tmp/azemu-cert.pem")

	for _, p := range candidates {
		if p == "" {
			continue
		}
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}

	// Return the config path even if it does not exist yet; the serve
	// subcommand will create it.
	if configPath != "" {
		return configPath
	}
	return "/tmp/azemu-cert.pem"
}

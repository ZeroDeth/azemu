package main

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/zerodeth/azemu/pkg/config"
)

// ensureAzemuRunning checks the health endpoint and auto-starts azemu serve
// in the background if it is not reachable. Returns nil on success.
func ensureAzemuRunning(cfg *config.Config) error {
	healthURL := fmt.Sprintf("http://localhost:%d/health", cfg.HealthPort)

	if probeHealth(healthURL) {
		log.Info().Msg("azemu is already running")
		return nil
	}

	log.Info().Msg("azemu is not running; starting in background")
	proc, err := startAzemuBackground()
	if err != nil {
		return fmt.Errorf("auto-start azemu: %w", err)
	}

	if err := waitForHealth(healthURL, 30*time.Second); err != nil {
		_ = proc.Kill()
		return fmt.Errorf("azemu did not become healthy: %w", err)
	}
	log.Info().Int("pid", proc.Pid).Msg("azemu is healthy")
	return nil
}

// setEnvDefaults sets environment variables from the given map, but only if
// they are not already set. This allows the caller to override any value.
func setEnvDefaults(env map[string]string) {
	for k, v := range env {
		if os.Getenv(k) == "" {
			os.Setenv(k, v)
		}
	}
}

// execBinary finds the named binary in PATH and exec's it with the given
// args. This replaces the current process.
func execBinary(name string, args []string) error {
	bin, err := exec.LookPath(name)
	if err != nil {
		return fmt.Errorf("%s not found in PATH: %w", name, err)
	}

	log.Info().Str("binary", bin).Strs("args", args).Msg("exec")

	argv := append([]string{name}, args...)
	return execProcess(bin, argv, os.Environ())
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

	logPath := "azemu.log"
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("open log file %s: %w", logPath, err)
	}

	proc, err := os.StartProcess(self, []string{"azemu", "serve"}, &os.ProcAttr{
		Env:   os.Environ(),
		Files: []*os.File{os.Stdin, logFile, logFile},
		Sys:   detachSysProcAttr(),
	})
	logFile.Close()
	if err != nil {
		return nil, fmt.Errorf("start azemu serve: %w", err)
	}

	if err := proc.Release(); err != nil {
		log.Warn().Err(err).Msg("could not release child process")
	}

	log.Info().Int("pid", proc.Pid).Str("log", logPath).Msg("started azemu in background")
	return proc, nil
}

// resolveCertFile returns the first cert bundle path that exists on disk.
func resolveCertFile(configPath string) string {
	candidates := []string{configPath}

	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(cwd, ".azemu", "cert-bundle.pem"))
	}

	candidates = append(candidates, "/tmp/azemu-cert.pem")

	for _, p := range candidates {
		if p == "" {
			continue
		}
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}

	if configPath != "" {
		return configPath
	}
	return "/tmp/azemu-cert.pem"
}

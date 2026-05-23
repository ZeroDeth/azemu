package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/zerodeth/azemu/pkg/config"
)

// runStatus checks whether azemu is running and prints its health info.
func runStatus(args []string) error {
	for _, a := range args {
		switch a {
		case "--help", "-help", "-h":
			printStatusUsage(os.Stderr)
			return nil
		default:
			fmt.Fprintf(os.Stderr, "azemu status: unknown flag %q\n\n", a)
			printStatusUsage(os.Stderr)
			os.Exit(1)
		}
	}

	cfg := config.Load()
	healthURL := fmt.Sprintf("http://localhost:%d/health", cfg.HealthPort)

	client := &http.Client{
		Timeout: 3 * time.Second,
		Transport: &http.Transport{
			DialContext: (&net.Dialer{Timeout: 2 * time.Second}).DialContext,
		},
	}

	resp, err := client.Get(healthURL)
	if err != nil {
		fmt.Fprintf(os.Stdout, "azemu is not running (health endpoint unreachable at %s)\n", healthURL)
		os.Exit(1)
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stdout, "azemu health endpoint returned %d\n", resp.StatusCode)
		os.Exit(1)
		return nil
	}

	var health struct {
		Status        string `json:"status"`
		Version       string `json:"version"`
		UptimeSeconds int    `json:"uptime_seconds"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
		return fmt.Errorf("decode health response: %w", err)
	}

	uptime := formatUptime(health.UptimeSeconds)

	fmt.Fprintf(os.Stdout, "azemu is running\n")
	fmt.Fprintf(os.Stdout, "  version    %s\n", health.Version)
	fmt.Fprintf(os.Stdout, "  status     %s\n", health.Status)
	fmt.Fprintf(os.Stdout, "  uptime     %s\n", uptime)
	fmt.Fprintf(os.Stdout, "  health     %s\n", healthURL)
	return nil
}

// formatUptime converts seconds into a human-readable duration string.
func formatUptime(seconds int) string {
	if seconds < 60 {
		return fmt.Sprintf("%ds", seconds)
	}
	if seconds < 3600 {
		return fmt.Sprintf("%dm%ds", seconds/60, seconds%60)
	}
	h := seconds / 3600
	m := (seconds % 3600) / 60
	return fmt.Sprintf("%dh%dm", h, m)
}

func printStatusUsage(w *os.File) {
	fmt.Fprintf(w, "Usage: azemu status\n\n")
	fmt.Fprintf(w, "Check whether azemu is running and display its health info.\n\n")
	fmt.Fprintf(w, "Exits 0 if azemu is healthy, 1 if unreachable or unhealthy.\n\n")
}

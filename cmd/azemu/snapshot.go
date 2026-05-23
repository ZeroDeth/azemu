package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/zerodeth/azemu/pkg/config"
)

// runSnapshot dispatches to save, load, list, or reset.
func runSnapshot(args []string) error {
	if len(args) == 0 {
		printSnapshotUsage(os.Stderr)
		os.Exit(1)
	}

	switch args[0] {
	case "save":
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "usage: azemu snapshot save <name>\n")
			os.Exit(1)
		}
		return snapshotSave(args[1])
	case "load":
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "usage: azemu snapshot load <name>\n")
			os.Exit(1)
		}
		return snapshotLoad(args[1])
	case "list":
		return snapshotList()
	case "reset":
		return snapshotReset()
	case "--help", "-help", "-h", "help":
		printSnapshotUsage(os.Stderr)
		return nil
	default:
		fmt.Fprintf(os.Stderr, "azemu snapshot: unknown action %q\n\n", args[0])
		printSnapshotUsage(os.Stderr)
		os.Exit(1)
	}
	return nil
}

// snapshotSave exports the current state from a running azemu and writes it
// to ~/.azemu/snapshots/<name>.json.
func snapshotSave(name string) error {
	cfg := config.Load()
	exportURL := fmt.Sprintf("https://localhost:%d/api/state/export", cfg.HTTPPort)

	client := insecureHTTPClient()
	resp, err := client.Get(exportURL)
	if err != nil {
		return fmt.Errorf("export state (is azemu running?): %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("export failed: %d %s", resp.StatusCode, string(body))
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read export response: %w", err)
	}

	dir, err := snapshotDir()
	if err != nil {
		return err
	}

	path := filepath.Join(dir, name+".json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("write snapshot %s: %w", path, err)
	}

	log.Info().Str("path", path).Int("bytes", len(data)).Msg("snapshot saved")
	fmt.Fprintf(os.Stdout, "saved snapshot %q (%d bytes) to %s\n", name, len(data), path)
	return nil
}

// snapshotLoad reads a snapshot file and imports it into a running azemu.
func snapshotLoad(name string) error {
	dir, err := snapshotDir()
	if err != nil {
		return err
	}

	path := filepath.Join(dir, name+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read snapshot %s: %w", path, err)
	}

	cfg := config.Load()
	importURL := fmt.Sprintf("https://localhost:%d/api/state/import", cfg.HTTPPort)

	client := insecureHTTPClient()
	resp, err := client.Post(importURL, "application/json", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("import state (is azemu running?): %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("import failed: %d %s", resp.StatusCode, string(body))
	}

	fmt.Fprintf(os.Stdout, "loaded snapshot %q (%d bytes) from %s\n", name, len(data), path)
	return nil
}

// snapshotList prints all saved snapshots.
func snapshotList() error {
	dir, err := snapshotDir()
	if err != nil {
		return err
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Fprintln(os.Stdout, "no snapshots saved yet")
			return nil
		}
		return fmt.Errorf("list snapshots: %w", err)
	}

	found := false
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".json")
		fmt.Fprintf(os.Stdout, "  %-30s  %6d bytes  %s\n",
			name, info.Size(), info.ModTime().Format(time.RFC3339))
		found = true
	}

	if !found {
		fmt.Fprintln(os.Stdout, "no snapshots saved yet")
	}
	return nil
}

// snapshotReset calls POST /api/state/reset on the running azemu.
func snapshotReset() error {
	cfg := config.Load()
	resetURL := fmt.Sprintf("https://localhost:%d/api/state/reset", cfg.HTTPPort)

	client := insecureHTTPClient()
	resp, err := client.Post(resetURL, "application/json", nil)
	if err != nil {
		return fmt.Errorf("reset state (is azemu running?): %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("reset failed: %d %s", resp.StatusCode, string(body))
	}

	fmt.Fprintln(os.Stdout, "state reset to empty")
	return nil
}

// snapshotDir returns ~/.azemu/snapshots, creating it if needed.
func snapshotDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	dir := filepath.Join(home, ".azemu", "snapshots")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("create snapshot directory %s: %w", dir, err)
	}
	return dir, nil
}

// insecureHTTPClient returns an HTTP client that skips TLS verification.
// azemu uses a self-signed cert, so we must skip verification when talking
// to the ARM port from the CLI.
func insecureHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: tlsInsecureConfig(),
		},
	}
}

func printSnapshotUsage(w *os.File) {
	fmt.Fprintf(w, "Usage: azemu snapshot <action> [name]\n\n")
	fmt.Fprintf(w, "Manage named snapshots of azemu's in-memory state.\n\n")
	fmt.Fprintf(w, "Actions:\n")
	fmt.Fprintf(w, "  save <name>   Export current state to ~/.azemu/snapshots/<name>.json\n")
	fmt.Fprintf(w, "  load <name>   Import state from a saved snapshot\n")
	fmt.Fprintf(w, "  list          List all saved snapshots\n")
	fmt.Fprintf(w, "  reset         Clear all resources (no snapshot needed)\n\n")
	fmt.Fprintf(w, "Requires a running azemu server.\n\n")
}

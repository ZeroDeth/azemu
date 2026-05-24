package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// stringSlice
// ---------------------------------------------------------------------------

func TestStringSlice_passthrough(t *testing.T) {
	in := []string{"a", "b"}
	got := stringSlice(in)
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("want [a b], got %v", got)
	}
}

func TestStringSlice_interfaceSlice(t *testing.T) {
	in := []any{"x", "y", "z"}
	got := stringSlice(in)
	if len(got) != 3 || got[0] != "x" {
		t.Fatalf("want [x y z], got %v", got)
	}
}

func TestStringSlice_mixedTypes_nonStringDropped(t *testing.T) {
	in := []any{"keep", 42, "also-keep", nil}
	got := stringSlice(in)
	if len(got) != 2 || got[0] != "keep" || got[1] != "also-keep" {
		t.Fatalf("want [keep also-keep], got %v", got)
	}
}

func TestStringSlice_nilInput(t *testing.T) {
	got := stringSlice(nil)
	if got != nil {
		t.Fatalf("want nil, got %v", got)
	}
}

func TestStringSlice_unknownType(t *testing.T) {
	got := stringSlice(123)
	if got != nil {
		t.Fatalf("want nil for unknown type, got %v", got)
	}
}

// ---------------------------------------------------------------------------
// credentialMatches
// ---------------------------------------------------------------------------

func TestCredentialMatches_exactMatch(t *testing.T) {
	props := map[string]any{
		"issuer":    "https://token.actions.githubusercontent.com",
		"subject":   "repo:org/repo:ref:refs/heads/main",
		"audiences": []any{"api://AzureADTokenExchange"},
	}
	if !credentialMatches(props, "https://token.actions.githubusercontent.com", "repo:org/repo:ref:refs/heads/main", []string{"api://AzureADTokenExchange"}) {
		t.Fatal("want match, got no match")
	}
}

func TestCredentialMatches_issuerMismatch(t *testing.T) {
	props := map[string]any{
		"issuer":    "https://other.issuer.com",
		"subject":   "repo:org/repo:ref:refs/heads/main",
		"audiences": []any{"api://AzureADTokenExchange"},
	}
	if credentialMatches(props, "https://token.actions.githubusercontent.com", "repo:org/repo:ref:refs/heads/main", []string{"api://AzureADTokenExchange"}) {
		t.Fatal("want no match on issuer mismatch, got match")
	}
}

func TestCredentialMatches_subjectMismatch(t *testing.T) {
	props := map[string]any{
		"issuer":    "https://token.actions.githubusercontent.com",
		"subject":   "repo:org/other:ref:refs/heads/main",
		"audiences": []any{"api://AzureADTokenExchange"},
	}
	if credentialMatches(props, "https://token.actions.githubusercontent.com", "repo:org/repo:ref:refs/heads/main", []string{"api://AzureADTokenExchange"}) {
		t.Fatal("want no match on subject mismatch, got match")
	}
}

func TestCredentialMatches_audienceMismatch(t *testing.T) {
	props := map[string]any{
		"issuer":    "https://token.actions.githubusercontent.com",
		"subject":   "repo:org/repo:ref:refs/heads/main",
		"audiences": []any{"some-other-audience"},
	}
	if credentialMatches(props, "https://token.actions.githubusercontent.com", "repo:org/repo:ref:refs/heads/main", []string{"api://AzureADTokenExchange"}) {
		t.Fatal("want no match on audience mismatch, got match")
	}
}

func TestCredentialMatches_multipleAudiences_oneMatches(t *testing.T) {
	props := map[string]any{
		"issuer":    "https://token.actions.githubusercontent.com",
		"subject":   "repo:org/repo:ref:refs/heads/main",
		"audiences": []any{"audience-a", "api://AzureADTokenExchange", "audience-c"},
	}
	if !credentialMatches(props, "https://token.actions.githubusercontent.com", "repo:org/repo:ref:refs/heads/main", []string{"api://AzureADTokenExchange"}) {
		t.Fatal("want match when one of multiple audiences matches, got no match")
	}
}

func TestCredentialMatches_emptyAudiences(t *testing.T) {
	props := map[string]any{
		"issuer":    "https://token.actions.githubusercontent.com",
		"subject":   "repo:org/repo:ref:refs/heads/main",
		"audiences": []any{},
	}
	if credentialMatches(props, "https://token.actions.githubusercontent.com", "repo:org/repo:ref:refs/heads/main", []string{"api://AzureADTokenExchange"}) {
		t.Fatal("want no match when allowed audiences is empty, got match")
	}
}

// ---------------------------------------------------------------------------
// formatUptime
// ---------------------------------------------------------------------------

func TestFormatUptime_seconds(t *testing.T) {
	cases := []struct {
		secs int
		want string
	}{
		{0, "0s"},
		{1, "1s"},
		{59, "59s"},
	}
	for _, tc := range cases {
		got := formatUptime(tc.secs)
		if got != tc.want {
			t.Errorf("formatUptime(%d) = %q, want %q", tc.secs, got, tc.want)
		}
	}
}

func TestFormatUptime_minutes(t *testing.T) {
	cases := []struct {
		secs int
		want string
	}{
		{60, "1m0s"},
		{90, "1m30s"},
		{3599, "59m59s"},
	}
	for _, tc := range cases {
		got := formatUptime(tc.secs)
		if got != tc.want {
			t.Errorf("formatUptime(%d) = %q, want %q", tc.secs, got, tc.want)
		}
	}
}

func TestFormatUptime_hours(t *testing.T) {
	cases := []struct {
		secs int
		want string
	}{
		{3600, "1h0m"},
		{3661, "1h1m"},
		{7323, "2h2m"},
	}
	for _, tc := range cases {
		got := formatUptime(tc.secs)
		if got != tc.want {
			t.Errorf("formatUptime(%d) = %q, want %q", tc.secs, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// statusIcon
// ---------------------------------------------------------------------------

func TestStatusIcon_knownStatuses(t *testing.T) {
	cases := []struct{ input, want string }{
		{"full", "Full"},
		{"Full", "Full"},
		{"FULL", "Full"},
		{"stub", "Stub"},
		{"Stub", "Stub"},
		{"none", "None"},
		{"None", "None"},
	}
	for _, tc := range cases {
		got := statusIcon(tc.input)
		if got != tc.want {
			t.Errorf("statusIcon(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestStatusIcon_unknown(t *testing.T) {
	got := statusIcon("partial")
	if got != "partial" {
		t.Errorf("statusIcon(unknown) = %q, want passthrough %q", got, "partial")
	}
}

// ---------------------------------------------------------------------------
// setEnvDefaults
// ---------------------------------------------------------------------------

func TestSetEnvDefaults_setsWhenUnset(t *testing.T) {
	key := "AZEMU_TEST_SETENV_UNSET_KEY"
	os.Unsetenv(key)
	t.Cleanup(func() { os.Unsetenv(key) })

	setEnvDefaults(map[string]string{key: "test-value"})
	if got := os.Getenv(key); got != "test-value" {
		t.Errorf("want %q, got %q", "test-value", got)
	}
}

func TestSetEnvDefaults_doesNotOverride(t *testing.T) {
	key := "AZEMU_TEST_SETENV_EXISTING_KEY"
	t.Setenv(key, "original")

	setEnvDefaults(map[string]string{key: "should-not-override"})
	if got := os.Getenv(key); got != "original" {
		t.Errorf("want %q (existing), got %q", "original", got)
	}
}

func TestSetEnvDefaults_multipleKeys(t *testing.T) {
	existingKey := "AZEMU_TEST_MULTI_EXISTING"
	newKey := "AZEMU_TEST_MULTI_NEW"
	t.Setenv(existingKey, "keep-me")
	os.Unsetenv(newKey)

	setEnvDefaults(map[string]string{
		existingKey: "should-not-override",
		newKey:      "new-value",
	})

	if got := os.Getenv(existingKey); got != "keep-me" {
		t.Errorf("existing key: want %q, got %q", "keep-me", got)
	}
	if got := os.Getenv(newKey); got != "new-value" {
		t.Errorf("new key: want %q, got %q", "new-value", got)
	}
	os.Unsetenv(newKey)
}

// ---------------------------------------------------------------------------
// resolveCertFile
// ---------------------------------------------------------------------------

func TestResolveCertFile_configPathExists(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "my-cert.pem")
	if err := os.WriteFile(certPath, []byte("cert"), 0644); err != nil {
		t.Fatal(err)
	}
	got := resolveCertFile(certPath)
	if got != certPath {
		t.Errorf("want %q, got %q", certPath, got)
	}
}

func TestResolveCertFile_configPathAbsent_fallsThrough(t *testing.T) {
	// configPath does not exist; no cwd/.azemu/cert-bundle.pem either.
	// resolveCertFile returns configPath as the fallback when it is non-empty.
	// Isolate cwd so .azemu/cert-bundle.pem in the project root cannot interfere.
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(orig) }) //nolint:errcheck
	nonExistent := "/tmp/azemu-does-not-exist-12345.pem"
	os.Remove(nonExistent) // ensure not present

	got := resolveCertFile(nonExistent)
	if got != nonExistent {
		t.Errorf("want configPath fallback %q, got %q", nonExistent, got)
	}
}

func TestResolveCertFile_emptyConfigPath_noFile(t *testing.T) {
	// With empty configPath and no files present, falls back to /tmp/azemu-cert.pem.
	os.Remove("/tmp/azemu-cert.pem") // ensure not present for this test
	got := resolveCertFile("")
	if got != "/tmp/azemu-cert.pem" {
		t.Errorf("want /tmp/azemu-cert.pem fallback, got %q", got)
	}
}

func TestResolveCertFile_tmpFileExists(t *testing.T) {
	// If /tmp/azemu-cert.pem exists, it should be returned when configPath is empty.
	if err := os.WriteFile("/tmp/azemu-cert.pem", []byte("cert"), 0644); err != nil {
		t.Skip("cannot write to /tmp:", err)
	}
	t.Cleanup(func() { os.Remove("/tmp/azemu-cert.pem") })

	got := resolveCertFile("")
	if got != "/tmp/azemu-cert.pem" {
		t.Errorf("want /tmp/azemu-cert.pem, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// tlsInsecureConfig
// ---------------------------------------------------------------------------

func TestTLSInsecureConfig_returnsInsecureConfig(t *testing.T) {
	cfg := tlsInsecureConfig()
	if cfg == nil {
		t.Fatal("want non-nil TLS config, got nil")
	}
	if !cfg.InsecureSkipVerify {
		t.Error("want InsecureSkipVerify=true, got false")
	}
}

// ---------------------------------------------------------------------------
// insecureHTTPClient
// ---------------------------------------------------------------------------

func TestInsecureHTTPClient_returnsNonNilClient(t *testing.T) {
	client := insecureHTTPClient()
	if client == nil {
		t.Fatal("want non-nil HTTP client, got nil")
	}
	if client.Timeout == 0 {
		t.Error("want non-zero timeout on insecure HTTP client")
	}
}

// ---------------------------------------------------------------------------
// snapshotDir
// ---------------------------------------------------------------------------

func TestSnapshotDir_returnsValidPath(t *testing.T) {
	// Redirect HOME to a temp dir so snapshotDir does not create
	// ~/.azemu/snapshots on the developer's real machine or CI agent.
	t.Setenv("HOME", t.TempDir())

	dir, err := snapshotDir()
	if err != nil {
		t.Fatalf("snapshotDir() error: %v", err)
	}
	if dir == "" {
		t.Fatal("want non-empty snapshot dir, got empty")
	}
	if !strings.HasSuffix(dir, filepath.Join(".azemu", "snapshots")) {
		t.Errorf("want path ending in .azemu/snapshots, got %q", dir)
	}
	// Directory must exist after call.
	if _, err := os.Stat(dir); err != nil {
		t.Errorf("snapshot dir %q should exist after call, got: %v", dir, err)
	}
}

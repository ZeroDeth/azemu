package auth

import (
	"crypto/x509"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadOrGenerate_FreshGeneration(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "azemu.pem")

	cert, generated, err := LoadOrGenerateSelfSignedTLS(path, "localhost", "127.0.0.1")
	if err != nil {
		t.Fatalf("LoadOrGenerate: %v", err)
	}
	if !generated {
		t.Errorf("generated = false on first call, want true")
	}
	if len(cert.Certificate) == 0 {
		t.Fatalf("returned cert has no certificate bytes")
	}

	// File must exist and have mode 0600 because it contains a private key.
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat persisted bundle: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0600 {
		t.Errorf("bundle mode = %o, want 0600 (private key must not be world-readable)", mode)
	}
}

func TestLoadOrGenerate_LoadExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "azemu.pem")

	// First call: generate and persist.
	cert1, gen1, err := LoadOrGenerateSelfSignedTLS(path, "localhost", "127.0.0.1")
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	if !gen1 {
		t.Errorf("first call generated = false")
	}

	// Second call: must load the same cert without regenerating.
	cert2, gen2, err := LoadOrGenerateSelfSignedTLS(path, "localhost", "127.0.0.1")
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if gen2 {
		t.Errorf("second call generated = true; expected to load existing")
	}

	// Compare the leaf certificate DER bytes — they must be identical.
	if string(cert1.Certificate[0]) != string(cert2.Certificate[0]) {
		t.Errorf("loaded cert differs from persisted cert; load path is broken")
	}
}

func TestLoadOrGenerate_RegeneratesOnCorruptBundle(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "azemu.pem")
	// Write garbage that is not a valid PEM bundle.
	if err := os.WriteFile(path, []byte("not a pem file at all"), 0600); err != nil {
		t.Fatalf("seed garbage file: %v", err)
	}

	cert, generated, err := LoadOrGenerateSelfSignedTLS(path, "localhost", "127.0.0.1")
	if err != nil {
		t.Fatalf("LoadOrGenerate: %v", err)
	}
	if !generated {
		t.Errorf("generated = false on corrupt bundle; expected fallback to generation")
	}
	if len(cert.Certificate) == 0 {
		t.Fatalf("returned cert is empty")
	}

	// And the file should now contain a valid bundle that loads cleanly.
	cert2, gen2, err := LoadOrGenerateSelfSignedTLS(path, "localhost", "127.0.0.1")
	if err != nil {
		t.Fatalf("re-load after regeneration: %v", err)
	}
	if gen2 {
		t.Errorf("post-regeneration load generated again; persistence is broken")
	}
	if string(cert.Certificate[0]) != string(cert2.Certificate[0]) {
		t.Errorf("regenerated cert was not persisted correctly")
	}
}

func TestLoadOrGenerate_EmptyPathDoesNotPersist(t *testing.T) {
	cert, generated, err := LoadOrGenerateSelfSignedTLS("", "localhost", "127.0.0.1")
	if err != nil {
		t.Fatalf("LoadOrGenerate with empty path: %v", err)
	}
	if !generated {
		t.Errorf("generated = false with empty path; nothing should be loaded")
	}
	if len(cert.Certificate) == 0 {
		t.Fatalf("returned cert is empty")
	}
}

func TestLoadOrGenerate_HostsAreInSAN(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "azemu.pem")

	cert, _, err := LoadOrGenerateSelfSignedTLS(path, "localhost", "127.0.0.1", "example.test")
	if err != nil {
		t.Fatalf("LoadOrGenerate: %v", err)
	}
	parsed, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		t.Fatalf("parse cert: %v", err)
	}

	wantDNS := map[string]bool{"localhost": false, "example.test": false}
	for _, name := range parsed.DNSNames {
		if _, ok := wantDNS[name]; ok {
			wantDNS[name] = true
		}
	}
	for name, found := range wantDNS {
		if !found {
			t.Errorf("DNS SAN %q missing from cert; got %v", name, parsed.DNSNames)
		}
	}

	wantIP := false
	for _, ip := range parsed.IPAddresses {
		if ip.String() == "127.0.0.1" {
			wantIP = true
		}
	}
	if !wantIP {
		t.Errorf("IP SAN 127.0.0.1 missing from cert; got %v", parsed.IPAddresses)
	}
}

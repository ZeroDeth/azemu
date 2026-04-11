package config

import "os"

type Config struct {
	HTTPPort       int
	HTTPSPort      int
	SubscriptionID string
	TenantID       string
	MetadataHost   string // what to return in /metadata/endpoints
	// CertPath is an optional PEM bundle path for TLS cert+key persistence.
	// When set, azemu loads the cert+key from this file on startup if it
	// exists, or generates a fresh pair and writes it there. This makes the
	// cert stable across restarts, so a user only needs to trust it in
	// their system keychain once. When empty, azemu falls back to the
	// legacy "fresh cert per startup, write cert-only to /tmp/azemu-cert.pem"
	// behaviour. Source: AZEMU_CERT_PATH env var.
	CertPath string
}

func Load() *Config {
	cfg := &Config{
		HTTPPort:       4566,
		HTTPSPort:      4567,
		SubscriptionID: envOr("AZEMU_SUBSCRIPTION_ID", "00000000-0000-0000-0000-000000000000"),
		TenantID:       envOr("AZEMU_TENANT_ID", "00000000-0000-0000-0000-000000000001"),
		CertPath:       envOr("AZEMU_CERT_PATH", ""),
	}
	cfg.MetadataHost = envOr("AZEMU_METADATA_HOST", "localhost:4567")
	return cfg
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

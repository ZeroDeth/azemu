package config

import (
	"os"
	"strconv"
)

type Config struct {
	HTTPPort       int
	HTTPSPort      int
	HealthPort     int
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
	// PersistPath is an optional file path for write-through state
	// persistence. When set, azemu uses a FileStore that writes the full
	// state to this JSON file after every Put/Delete. On restart, the file
	// is loaded automatically. Source: --persist flag or AZEMU_PERSIST_PATH
	// env var.
	PersistPath string
	// ImportPath loads state from this file on startup, then continues
	// serving. CLI-only (--import flag), no env var.
	ImportPath string
	// ExportPath dumps the current state to this file, then exits
	// immediately. CLI-only (--export flag), no env var.
	ExportPath string
	// AzuriteEndpoint is the base URL of the Azurite blob service sidecar.
	// ARM storage account responses use this value to populate
	// primaryEndpoints.blob (and derive queue/table ports) so SDK clients
	// can authenticate against real Azurite data-plane endpoints.
	// Source: AZEMU_AZURITE_ENDPOINT env var. Default: http://azurite:10000.
	AzuriteEndpoint string
	// KeyVaultEndpoint is the base HTTPS URL azemu advertises for Key Vault
	// data-plane operations. The vaultUri field in azurerm_key_vault responses
	// is rewritten to "{KeyVaultEndpoint}/keyvault/{name}/" so the azurerm
	// provider's subsequent secrets requests land on azemu's own handler.
	// In Docker compose, set to https://azemu:4566. Source: AZEMU_KV_ENDPOINT
	// env var. Default: https://localhost:4566.
	KeyVaultEndpoint string
	// RedisEndpoint is the connection URL for the Redis sidecar that backs the
	// Microsoft.Cache/Redis data plane. ARM responses derive hostName from this
	// value so SDK clients connect to the sidecar (under docker compose) or the
	// host instance (when running azemu directly). Source: AZEMU_REDIS_ENDPOINT
	// env var. Default: redis://azemu-redis:6379.
	RedisEndpoint string
	// ADOPort is the HTTP port for the Azure DevOps OIDC and service-endpoint
	// emulation surface. Plain HTTP (no TLS) because SYSTEM_OIDCREQUESTURI in
	// a real ADO agent is served over plain HTTP on the local pipeline worker.
	// Source: AZEMU_ADO_PORT env var. Default: 4569.
	ADOPort int
}

func Load() *Config {
	cfg := &Config{
		HTTPPort:       4566,
		HTTPSPort:      4567,
		HealthPort:     4568,
		SubscriptionID: envOr("AZEMU_SUBSCRIPTION_ID", "00000000-0000-0000-0000-000000000000"),
		TenantID:       envOr("AZEMU_TENANT_ID", "00000000-0000-0000-0000-000000000001"),
		CertPath:       envOr("AZEMU_CERT_PATH", ""),
	}
	cfg.MetadataHost = envOr("AZEMU_METADATA_HOST", "localhost:4567")
	cfg.PersistPath = envOr("AZEMU_PERSIST_PATH", "")
	cfg.AzuriteEndpoint = envOr("AZEMU_AZURITE_ENDPOINT", "http://azurite:10000")
	cfg.KeyVaultEndpoint = envOr("AZEMU_KV_ENDPOINT", "https://localhost:4566")
	cfg.RedisEndpoint = envOr("AZEMU_REDIS_ENDPOINT", "redis://azemu-redis:6379")
	cfg.ADOPort = envIntOr("AZEMU_ADO_PORT", 4569)
	return cfg
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envIntOr(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

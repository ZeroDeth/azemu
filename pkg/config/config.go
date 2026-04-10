package config

import "os"

type Config struct {
	HTTPPort       int
	HTTPSPort      int
	SubscriptionID string
	TenantID       string
	MetadataHost   string // what to return in /metadata/endpoints
}

func Load() *Config {
	cfg := &Config{
		HTTPPort:       4566,
		HTTPSPort:      4567,
		SubscriptionID: envOr("AZEMU_SUBSCRIPTION_ID", "00000000-0000-0000-0000-000000000000"),
		TenantID:       envOr("AZEMU_TENANT_ID", "00000000-0000-0000-0000-000000000001"),
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

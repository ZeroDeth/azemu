package main

import (
	"fmt"

	"github.com/zerodeth/azemu/pkg/config"
)

// runTF is the `azemu tf` subcommand. It ensures azemu is running, injects
// the env vars the azurerm provider expects, and exec's `terraform <args>`.
func runTF(args []string) error {
	cfg := config.Load()

	if err := ensureAzemuRunning(cfg); err != nil {
		return err
	}

	certFile := resolveCertFile(cfg.CertPath)

	setEnvDefaults(map[string]string{
		"SSL_CERT_FILE":         certFile,
		"ARM_METADATA_HOSTNAME": fmt.Sprintf("127.0.0.1:%d", cfg.HTTPSPort),
		"ARM_SUBSCRIPTION_ID":   cfg.SubscriptionID,
		"ARM_TENANT_ID":         cfg.TenantID,
		"ARM_CLIENT_ID":         "00000000-0000-0000-0000-000000000002",
		"ARM_CLIENT_SECRET":     "azemu-mock-secret",
	})

	return execBinary("terraform", args)
}

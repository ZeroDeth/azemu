package main

import (
	"fmt"

	"github.com/zerodeth/azemu/pkg/config"
)

// runPulumi is the `azemu pulumi` subcommand. It ensures azemu is running,
// injects the env vars the Pulumi Azure Native provider expects, and exec's
// `pulumi <args>`.
//
// The Pulumi Azure Native provider reads the same ARM_* env vars as the
// azurerm Terraform provider. Additionally, it uses ARM_ENVIRONMENT (or
// AZURE_ENVIRONMENT) and ARM_ENDPOINT (or AZURE_ENDPOINT) for custom
// cloud support.
func runPulumi(args []string) error {
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
		"ARM_ENDPOINT":          fmt.Sprintf("https://localhost:%d", cfg.HTTPPort),
	})

	return execBinary("pulumi", args)
}

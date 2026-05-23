package main

import (
	"fmt"

	"github.com/zerodeth/azemu/pkg/config"
)

// runPython is the `azemu python` subcommand. It ensures azemu is running,
// injects the AZURE_* env vars that azure-identity's DefaultAzureCredential
// reads, and exec's `python <args>`.
//
// The Azure SDK for Python uses AZURE_CLIENT_ID, AZURE_CLIENT_SECRET,
// AZURE_TENANT_ID, and AZURE_SUBSCRIPTION_ID for service-principal auth.
// AZURE_AUTHORITY_HOST overrides the login endpoint, and
// AZURE_ARM_URL overrides the ARM base URL.
func runPython(args []string) error {
	cfg := config.Load()

	if err := ensureAzemuRunning(cfg); err != nil {
		return err
	}

	certFile := resolveCertFile(cfg.CertPath)

	setEnvDefaults(map[string]string{
		"SSL_CERT_FILE":         certFile,
		"AZURE_SUBSCRIPTION_ID": cfg.SubscriptionID,
		"AZURE_TENANT_ID":       cfg.TenantID,
		"AZURE_CLIENT_ID":       "00000000-0000-0000-0000-000000000002",
		"AZURE_CLIENT_SECRET":   "azemu-mock-secret",
		"AZURE_AUTHORITY_HOST":  fmt.Sprintf("https://localhost:%d", cfg.HTTPSPort),
		"AZURE_ARM_URL":         fmt.Sprintf("https://localhost:%d", cfg.HTTPPort),
		"REQUESTS_CA_BUNDLE":    certFile,
		"CURL_CA_BUNDLE":        certFile,
	})

	return execBinary("python", args)
}

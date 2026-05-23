package main

import (
	"github.com/zerodeth/azemu/pkg/config"
)

// runKubectl is the `azemu kubectl` subcommand. It ensures azemu is running,
// injects KUBECONFIG and Azure identity env vars, and exec's `kubectl <args>`.
//
// The AKS stub in Phase 8.4 is management-plane only; it does not run a real
// Kubernetes API server. This adapter sets AZURE_* env vars so tools using
// azure-identity's DefaultAzureCredential can authenticate, and a future
// kubeconfig stub can point at a local kind/k3d cluster.
func runKubectl(args []string) error {
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
	})

	return execBinary("kubectl", args)
}

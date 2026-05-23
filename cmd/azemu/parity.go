package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
)

// parityEntry describes a single resource or capability in the parity matrix.
type parityEntry struct {
	Resource  string `json:"resource"`
	Status    string `json:"status"`
	Terraform string `json:"terraform,omitempty"`
	Notes     string `json:"notes,omitempty"`
}

// parityData is the embedded parity matrix. Keep it in sync with
// docs/PARITY.md. Each entry maps to an ARM resource or infrastructure
// capability that azemu implements.
//
// Ordering: infrastructure services first, then ARM resources (alphabetical
// by ARM type), then identity, then developer tooling.
var parityData = []parityEntry{
	// Infrastructure services
	{Resource: "Metadata service", Status: "Full", Notes: "canonical Azure schema on /metadata/endpoints"},
	{Resource: "OAuth2 token endpoint", Status: "Full", Notes: "RS256 JWT, mock credentials accepted"},
	{Resource: "OIDC discovery", Status: "Full", Notes: "/.well-known/openid-configuration"},
	{Resource: "JWKS", Status: "Full", Notes: "/discovery/v2.0/keys"},
	{Resource: "Self-signed TLS", Status: "Full", Notes: "ECDSA P-256; persistent via AZEMU_CERT_PATH"},
	{Resource: "Health check", Status: "Full", Notes: "GET /health on :4568"},

	// ARM resources
	{Resource: "Application Gateways", Status: "Full", Terraform: "azurerm_application_gateway"},
	{Resource: "AKS Managed Cluster", Status: "Full", Terraform: "azurerm_kubernetes_cluster, azurerm_kubernetes_cluster_node_pool", Notes: "management plane only"},
	{Resource: "Azure Cache for Redis", Status: "Full", Terraform: "azurerm_redis_cache", Notes: "Standard tier; sidecar for data plane"},
	{Resource: "CDN", Status: "Full", Terraform: "azurerm_cdn_profile, azurerm_cdn_endpoint"},
	{Resource: "DNS Zones", Status: "Full", Terraform: "azurerm_dns_zone, azurerm_dns_*_record", Notes: "A, AAAA, CNAME, TXT, MX, SRV, NS, SOA"},
	{Resource: "Federated Identity Credential", Status: "Full", Terraform: "azurerm_federated_identity_credential"},
	{Resource: "Key Vault", Status: "Full", Terraform: "azurerm_key_vault, azurerm_key_vault_secret", Notes: "management + data plane"},
	{Resource: "Load Balancers", Status: "Full", Terraform: "azurerm_lb, azurerm_lb_backend_address_pool, azurerm_lb_rule, azurerm_lb_probe"},
	{Resource: "Network Security Groups", Status: "Full", Terraform: "azurerm_network_security_group"},
	{Resource: "Public IP Addresses", Status: "Full", Terraform: "azurerm_public_ip"},
	{Resource: "Resource Groups", Status: "Full", Terraform: "azurerm_resource_group"},
	{Resource: "Storage Accounts", Status: "Full", Terraform: "azurerm_storage_account, azurerm_storage_container", Notes: "Azurite for data plane"},
	{Resource: "Subnets", Status: "Full", Terraform: "azurerm_subnet"},
	{Resource: "User Assigned Identity", Status: "Full", Terraform: "azurerm_user_assigned_identity"},
	{Resource: "Virtual Networks", Status: "Full", Terraform: "azurerm_virtual_network"},

	// Identity
	{Resource: "Service principal auth", Status: "Full", Notes: "accepts any client_id/secret"},
	{Resource: "Managed identity (IMDS)", Status: "Full", Notes: "IMDS token endpoint on /metadata/identity"},
	{Resource: "Workload identity (OIDC)", Status: "Full", Notes: "FIC-based token exchange"},
	{Resource: "ADO OIDC issuer", Status: "Full", Notes: "plain HTTP on :4569"},
	{Resource: "ADO Service Connections", Status: "Full", Notes: "CRUD on :4569"},
}

// runParity prints the parity matrix to stdout.
func runParity(args []string) error {
	// Simple flag check for JSON output.
	jsonOutput := false
	for _, a := range args {
		switch a {
		case "--json", "-json":
			jsonOutput = true
		case "--help", "-help", "-h":
			printParityUsage(os.Stderr)
			return nil
		default:
			fmt.Fprintf(os.Stderr, "azemu parity: unknown flag %q\n\n", a)
			printParityUsage(os.Stderr)
			os.Exit(1)
		}
	}

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(parityData)
	}

	// Count stats.
	total := len(parityData)
	full := 0
	for _, e := range parityData {
		if e.Status == "Full" {
			full++
		}
	}

	fmt.Fprintf(os.Stdout, "azemu %s -- parity matrix (%d/%d Full)\n\n", Version, full, total)

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "RESOURCE\tSTATUS\tTERRAFORM\tNOTES")
	fmt.Fprintln(w, "--------\t------\t---------\t-----")
	for _, e := range parityData {
		tf := e.Terraform
		if tf == "" {
			tf = "-"
		}
		notes := e.Notes
		if notes == "" {
			notes = "-"
		}
		// Truncate long terraform fields for readability.
		if len(tf) > 50 {
			tf = tf[:47] + "..."
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", e.Resource, statusIcon(e.Status), tf, notes)
	}
	w.Flush()
	fmt.Println()
	return nil
}

func statusIcon(status string) string {
	switch strings.ToLower(status) {
	case "full":
		return "Full"
	case "stub":
		return "Stub"
	case "none":
		return "None"
	default:
		return status
	}
}

func printParityUsage(w *os.File) {
	fmt.Fprintf(w, "Usage: azemu parity [flags]\n\n")
	fmt.Fprintf(w, "Show the supported Azure resources and their implementation status.\n\n")
	fmt.Fprintf(w, "Flags:\n")
	fmt.Fprintf(w, "  -json         Output as JSON\n")
	fmt.Fprintf(w, "  -h, -help     Print this help and exit\n\n")
}

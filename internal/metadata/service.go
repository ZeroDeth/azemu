package metadata

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/zerodeth/azemu/pkg/config"
)

// Service implements the /metadata/endpoints response that the azurerm
// provider fetches from https://{metadata_host}. This is the root of
// trust for redirecting all Terraform calls to the local emulator.
type Service struct {
	cfg *config.Config
}

func NewService(cfg *config.Config) *Service {
	return &Service{cfg: cfg}
}

func (s *Service) Routes(r chi.Router) {
	r.Get("/endpoints", s.endpoints)
}

// endpoints returns the Azure cloud environment metadata that azurerm
// uses to discover ARM, auth, and data plane URLs. By pointing everything
// at localhost, Terraform talks exclusively to the emulator.
//
// Supports api-version 2022-09-01 (modern Azure CLI) and earlier.
func (s *Service) endpoints(w http.ResponseWriter, r *http.Request) {
	base := fmt.Sprintf("https://%s", s.cfg.MetadataHost)
	httpBase := fmt.Sprintf("http://localhost:%d", s.cfg.HTTPPort)

	resp := map[string]interface{}{
		"galleryEndpoint":                       "",
		"graphEndpoint":                         base,
		"portalEndpoint":                        base,
		"authentication": map[string]interface{}{
			"loginEndpoint": base,
			"audiences":     []string{base + "/", "https://management.core.windows.net/"},
			"tenant":        s.cfg.TenantID,
			"identityProvider": "AAD",
		},
		"media":                                 "",
		"graphAudience":                         base,
		"activeDirectoryDataLake":               "",
		"batch":                                 httpBase,
		"resourceManager":                       httpBase,
		"vmImageAliasDoc":                       "",
		"activeDirectoryResourceId":             base + "/",
		"sqlManagement":                         httpBase,
		"microsoftGraphResourceId":              base + "/",
		"appInsights":                           "",
		"appInsightsTelemetryChannelResourceId": "",
		"attestation":                           "",
		"synapse":                               "",
		"logAnalytics":                          "",
		"ossrdbms":                              "",
		"suffixes": map[string]interface{}{
			"acrLoginServer":      "azurecr.io",
			"azureDatalakeAnalyticsCatalogAndJobEndpoint": "",
			"azureDatalakeStoreFileSystemEndpoint":        "",
			"keyvaultDns":        ".vault.azure.net",
			"sqlServerHostname":  ".database.windows.net",
			"storageEndpoint":    "core.windows.net",
			"storageSyncEndpoint": "",
			"mhsmDns":            "",
			"mysqlServerEndpoint": "",
			"postgresqlServerEndpoint": "",
			"mariadbServerEndpoint": "",
			"synapseAnalytics":   "",
		},
		"name": "AzEmuCloud",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

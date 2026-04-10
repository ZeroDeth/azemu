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
	armBase := fmt.Sprintf("https://localhost:%d", s.cfg.HTTPPort)
	// Data plane shares port 4566 with ARM and is served over TLS (see
	// cmd/azemu/main.go). Earlier versions declared this as http:// which
	// caused the azurerm provider's cloud classifier to mark every field
	// assigned from dataPlane as Azure Stack and reject the environment.
	// The fix for resourceManager in docs/TROUBLESHOOTING.md was incomplete;
	// batch and sqlManagement also had to switch to https.
	dataPlane := fmt.Sprintf("https://localhost:%d", s.cfg.HTTPPort)

	resp := map[string]interface{}{
		"galleryEndpoint": "https://gallery.azure.com/",
		"graphEndpoint":   base,
		"portalEndpoint":  base,
		"authentication": map[string]interface{}{
			"loginEndpoint": base,
			"audiences":     []string{base + "/", "https://management.core.windows.net/"},
			// tenant in the metadata response is a *cloud-environment* identifier,
			// not the user's tenant. The azurerm provider's IsAzureStack() classifier
			// in go-azure-sdk/sdk/environments/azure_stack.go returns true (reject)
			// when this value is anything other than "common" (case-insensitive).
			// Real Azure public cloud always returns "common" here. The user's
			// actual tenant is supplied separately via ARM_TENANT_ID / provider
			// config and used at token-request time.
			"tenant":           "common",
			"identityProvider": "AAD",
		},
		"media":                                 "https://media.azure.com/",
		"graphAudience":                         base,
		"activeDirectoryEndpoint":               base,
		"activeDirectoryDataLake":               "https://datalake.azure.net/",
		"batch":                                 dataPlane,
		"resourceManager":                       armBase,
		"vmImageAliasDoc":                       "https://gallery.azure.com/",
		"activeDirectoryResourceId":             base + "/",
		"sqlManagement":                         dataPlane,
		"microsoftGraphResourceId":              base + "/",
		"appInsights":                           "https://api.applicationinsights.io/",
		"appInsightsTelemetryChannelResourceId": "https://dc.applicationinsights.azure.com/v2/track",
		"attestation":                           "https://attest.azure.net/",
		"synapse":                               "https://dev.azuresynapse.net/",
		"logAnalytics":                          "https://api.loganalytics.io/",
		"ossrdbms":                              "https://ossrdbms-aad.database.windows.net/",
		"suffixes": map[string]interface{}{
			"acrLoginServer": ".azurecr.io",
			"azureDatalakeAnalyticsCatalogAndJobEndpoint": ".azuredatalakeanalytics.net/",
			"azureDatalakeStoreFileSystemEndpoint":        ".azuredatalakestore.net/",
			"keyvaultDns":                                 ".vault.azure.net",
			"sqlServerHostname":                           ".database.windows.net",
			"storageEndpoint":                             "core.windows.net",
			"storageSyncEndpoint":                         ".afs.azure.net",
			"mhsmDns":                                     ".managedhsm.azure.net",
			"mysqlServerEndpoint":                         ".mysql.database.azure.com",
			"postgresqlServerEndpoint":                    ".postgres.database.azure.com",
			"mariadbServerEndpoint":                       ".mariadb.database.azure.com",
			"synapseAnalytics":                            ".dev.azuresynapse.net",
		},
		"name": "AzureCloud",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

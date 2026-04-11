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
// uses to discover ARM, auth, and data plane URLs. The response shape
// MUST match the canonical Azure public cloud response from
// https://management.azure.com/metadata/endpoints?api-version=2022-09-01
// verbatim, except for fields that need to be redirected to azemu.
//
// History note: earlier versions of this handler used hand-rolled field
// names like `portalEndpoint`, `graphEndpoint`, `appInsights`, `attestation`,
// `synapse`, `logAnalytics`, `ossrdbms` and a `suffixes` block with
// `storageEndpoint`, `keyvaultDns`, etc. None of these match the real
// Azure response — the real names are `portal`, `graph`, `appInsightsResourceId`,
// `attestationResourceId`, `synapseAnalyticsResourceId`,
// `logAnalyticsResourceId`, `ossrDbmsResourceId`, `suffixes.storage`,
// `suffixes.keyVaultDns`. The result was that go-azure-sdk's environment
// loader silently saw missing fields, which surfaced later as opaque
// "endpoint X is not supported in this Azure Environment" errors when
// the provider tried to construct authorizers for individual services.
// This handler is now ground-truth-aligned to prevent that whole class
// of bug; the regression test in service_test.go pins every field
// against the canonical schema.
//
// Substitution rules:
//   - resourceManager + activeDirectoryResourceId + microsoftGraphResourceId
//   - portal + graph + media + sqlManagement + batch + appServiceResourceId
//   - appInsightsResourceId + activeDirectoryDataLake + attestationResourceId
//   - logAnalyticsResourceId + synapseAnalyticsResourceId + ossrDbmsResourceId
//     -> azemu localhost (so the provider talks to us, not real Azure)
//   - authentication.loginEndpoint -> azemu localhost (token requests stay local)
//   - authentication.tenant -> "common" (cloud-environment identifier, not user tenant)
//   - authentication.audiences -> include both localhost and the real
//     "https://management.core.windows.net/" so token verification has both
//   - vmImageAliasDoc -> leave as the real GitHub URL (it's a documentation
//     reference, not an API call; the provider doesn't redirect it)
//   - suffixes.* -> leave as the real Azure suffix values; these are domain
//     suffix strings (e.g. "core.windows.net") used by the SDK to construct
//     resource URLs, NOT endpoints that need redirecting. Renaming
//     `storageEndpoint` -> `storage` is what unblocks the Storage authorizer.
//
// Supports api-version 2022-09-01 (modern Azure CLI) and earlier.
func (s *Service) endpoints(w http.ResponseWriter, r *http.Request) {
	armBase := fmt.Sprintf("https://localhost:%d", s.cfg.HTTPPort)
	authBase := fmt.Sprintf("https://%s", s.cfg.MetadataHost)

	resp := map[string]interface{}{
		// === Top-level identification ===
		"name": "AzureCloud",

		// === Authentication block (classifier-sensitive — see Service docstring) ===
		"authentication": map[string]interface{}{
			"loginEndpoint": authBase,
			"audiences": []string{
				authBase + "/",
				"https://management.core.windows.net/",
			},
			"tenant":           "common",
			"identityProvider": "AAD",
		},

		// === Resource manager and identity endpoints (redirected to azemu) ===
		"resourceManager":            armBase + "/",
		"activeDirectoryDataLake":    armBase + "/",
		"microsoftGraphResourceId":   authBase + "/",
		"appServiceResourceId":       armBase,
		"appInsightsResourceId":      armBase,
		"attestationResourceId":      armBase,
		"synapseAnalyticsResourceId": armBase,
		"logAnalyticsResourceId":     armBase,
		"ossrDbmsResourceId":         armBase,

		// === Data plane / portal / management URLs (redirected) ===
		"portal":                                armBase,
		"graph":                                 authBase + "/",
		"graphAudience":                         authBase + "/",
		"media":                                 armBase + "/",
		"batch":                                 armBase + "/",
		"sqlManagement":                         armBase + "/",
		"appInsightsTelemetryChannelResourceId": "https://dc.applicationinsights.azure.com/v2/track",

		// === Reference data (left as real public-cloud URLs) ===
		"vmImageAliasDoc": "https://raw.githubusercontent.com/Azure/azure-rest-api-specs/master/arm-compute/quickstart-templates/aliases.json",

		// === Domain suffixes (NOT endpoints — used by SDK to construct
		// resource URLs like {acct}.blob.{storage}). These match real Azure
		// public cloud verbatim. Renaming/typo here is what previously
		// broke the Storage authorizer. ===
		"suffixes": map[string]interface{}{
			"acrLoginServer":                      "azurecr.io",
			"attestationEndpoint":                 "attest.azure.net",
			"azureDataLakeAnalyticsCatalogAndJob": "azuredatalakeanalytics.net",
			"azureDataLakeStoreFileSystem":        "azuredatalakestore.net",
			"azureFrontDoorEndpointSuffix":        "azurefd.net",
			"keyVaultDns":                         "vault.azure.net",
			"mariadbServerEndpoint":               "mariadb.database.azure.com",
			"mhsmDns":                             "managedhsm.azure.net",
			"mysqlServerEndpoint":                 "mysql.database.azure.com",
			"postgresqlServerEndpoint":            "postgres.database.azure.com",
			"sqlServerHostname":                   "database.windows.net",
			"storage":                             "core.windows.net",
			"storageSyncEndpointSuffix":           "afs.azure.net",
			"synapseAnalytics":                    "dev.azuresynapse.net",
		},
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

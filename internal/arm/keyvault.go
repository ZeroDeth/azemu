package arm

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"

	"github.com/zerodeth/azemu/internal/store"
)

const keyVaultTypeString = "Microsoft.KeyVault/vaults"

// vaultBaseURL builds the per-vault data-plane base URL. The azurerm
// provider requires the vaultUri host to look like {name}.vault.{suffix}
// (its KeyVaultIDFromBaseUrl extracts the vault name from the first host
// label and rejects anything else with "expected a URI in the format
// `the-keyvault-name.vault.**`"). {name}.vault.localhost resolves to
// loopback on macOS and on Linux with systemd-resolved, and the TLS cert
// carries a *.vault.localhost SAN. The scheme and port come from
// AZEMU_KV_ENDPOINT.
func (a *Router) vaultBaseURL(vaultName string) string {
	scheme := "https"
	port := ""
	if u, err := url.Parse(a.kvEndpoint); err == nil {
		if u.Scheme != "" {
			scheme = u.Scheme
		}
		if p := u.Port(); p != "" {
			port = ":" + p
		}
	}
	return fmt.Sprintf("%s://%s.vault.localhost%s", scheme, vaultName, port)
}

func keyVaultID(subID, rgName, name string) string {
	return fmt.Sprintf(
		"/subscriptions/%s/resourceGroups/%s/providers/Microsoft.KeyVault/vaults/%s",
		subID, rgName, name,
	)
}

// keyVaultBody is the subset of the azurerm_key_vault PUT payload that
// azemu understands. All vault configuration lives inside properties (unlike
// storage accounts, where sku is a top-level field).
type keyVaultBody struct {
	Location   string                 `json:"location"`
	Tags       map[string]string      `json:"tags"`
	Properties map[string]interface{} `json:"properties"`
}

func (a *Router) putKeyVault(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	name := chi.URLParam(r, "vaultName")

	var body keyVaultBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeAzureError(w, http.StatusBadRequest, "InvalidRequestContent", err.Error())
		return
	}
	if strings.TrimSpace(body.Location) == "" {
		writeAzureError(w, http.StatusBadRequest, "InvalidRequestContent",
			"location is required")
		return
	}

	if body.Properties == nil {
		body.Properties = map[string]interface{}{}
	}
	body.Properties["provisioningState"] = "Succeeded"
	// vaultUri points at azemu's own HTTPS endpoint (host-based per-vault
	// URL) so the azurerm provider's nested-item requests land on the
	// root-level data-plane routes (KeyVaultNestedItemRoutes), which resolve
	// the vault from the Host header.
	body.Properties["vaultUri"] = a.vaultBaseURL(name) + "/"

	// Default SKU to standard if not supplied.
	if _, ok := body.Properties["sku"]; !ok {
		body.Properties["sku"] = map[string]interface{}{"family": "A", "name": "standard"}
	}
	// Default accessPolicies to empty slice if absent.
	if _, ok := body.Properties["accessPolicies"]; !ok {
		body.Properties["accessPolicies"] = []interface{}{}
	}
	// Default softDelete settings that real Azure enforces.
	if _, ok := body.Properties["enableSoftDelete"]; !ok {
		body.Properties["enableSoftDelete"] = true
	}
	if _, ok := body.Properties["softDeleteRetentionInDays"]; !ok {
		body.Properties["softDeleteRetentionInDays"] = float64(90)
	}

	id := keyVaultID(subID, rgName, name)
	res := &store.Resource{
		ID:         id,
		Name:       name,
		Type:       keyVaultTypeString,
		Location:   strings.ToLower(body.Location),
		Tags:       normaliseTags(body.Tags),
		Properties: body.Properties,
	}

	_, exists := a.store.Get(id)
	if err := a.store.Put(id, res); err != nil {
		writeAzureError(w, http.StatusInternalServerError, "InternalServerError",
			fmt.Sprintf("put key vault %q: %s", name, err))
		return
	}

	status := http.StatusCreated
	if exists {
		status = http.StatusOK
	}
	log.Info().Str("resource_id", id).Bool("existed", exists).Msg("key vault upsert")
	writeJSON(w, status, keyVaultResponse(res))
}

func (a *Router) getKeyVault(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	name := chi.URLParam(r, "vaultName")
	id := keyVaultID(subID, rgName, name)

	res, ok := a.store.Get(id)
	if !ok {
		writeAzureError(w, http.StatusNotFound, "ResourceNotFound",
			fmt.Sprintf("The Resource 'Microsoft.KeyVault/vaults/%s' under resource group '%s' was not found.", name, rgName))
		return
	}
	writeJSON(w, http.StatusOK, keyVaultResponse(res))
}

func (a *Router) headKeyVault(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	name := chi.URLParam(r, "vaultName")
	id := keyVaultID(subID, rgName, name)

	if _, ok := a.store.Get(id); !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *Router) deleteKeyVault(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	name := chi.URLParam(r, "vaultName")
	id := keyVaultID(subID, rgName, name)

	if !a.store.Delete(id) {
		writeAzureError(w, http.StatusNotFound, "ResourceNotFound",
			fmt.Sprintf("The Resource 'Microsoft.KeyVault/vaults/%s' under resource group '%s' was not found.", name, rgName))
		return
	}

	// Cascade-delete all Key Vault data-plane entries stored under this vault.
	// Secrets live at /keyvault/{name}/secrets/... and keys at
	// /keyvault/{name}/keys/...; both must be cleaned up independently because
	// they use a different key prefix than the ARM resource.
	dataPlanePrefix := fmt.Sprintf("/keyvault/%s/", name)
	for _, res := range a.store.List(dataPlanePrefix) {
		a.store.Delete(res.ID)
	}

	log.Info().Str("resource_id", id).Msg("key vault deleted")
	// azurerm's vaults.VaultsClient#Delete expects 200 OK when soft-delete is
	// disabled. azemu does not implement soft-delete; returning 200 OK keeps the
	// autorest poller from trying to parse an empty 202 body as JSON.
	w.WriteHeader(http.StatusOK)
}

func (a *Router) listKeyVaultsByRG(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	prefix := fmt.Sprintf(
		"/subscriptions/%s/resourceGroups/%s/providers/Microsoft.KeyVault/vaults/",
		subID, rgName,
	)
	a.writeKeyVaultList(w, prefix)
}

func (a *Router) listKeyVaultsBySub(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	prefix := fmt.Sprintf("/subscriptions/%s/resourceGroups/", subID)
	a.writeKeyVaultList(w, prefix)
}

func (a *Router) writeKeyVaultList(w http.ResponseWriter, prefix string) {
	items := []map[string]interface{}{}
	for _, res := range a.store.List(prefix) {
		if res.Type != keyVaultTypeString {
			continue
		}
		items = append(items, keyVaultResponse(res))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"value": items})
}

// getDeletedKeyVault stubs the soft-delete check that azurerm v4 performs before
// creating any Key Vault. azemu does not implement soft-delete, so this endpoint
// always returns 404 (no soft-deleted vault found), which allows the provider to
// proceed with the create.
func (a *Router) getDeletedKeyVault(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "vaultName")
	location := chi.URLParam(r, "location")
	writeAzureError(w, http.StatusNotFound, "ResourceNotFound",
		fmt.Sprintf("The deleted vault '%s' in location '%s' was not found.", name, location))
}

// getKeyVaultDataPlaneRoot answers GET {vaultUri}. The azurerm provider polls
// the vault root as a data-plane availability check before nested-item
// operations; a 404 makes it retry for minutes. 200 with an empty object
// signals the vault is reachable. Unknown vaults still return 404.
func (a *Router) getKeyVaultDataPlaneRoot(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "vaultName")
	if !a.vaultExists(name) {
		writeAzureError(w, http.StatusNotFound, "VaultNotFound",
			fmt.Sprintf("Vault '%s' was not found.", name))
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{})
}

// vaultExists reports whether a Key Vault ARM resource with this name exists
// in any resource group.
func (a *Router) vaultExists(name string) bool {
	for _, res := range a.store.List("/subscriptions/") {
		if res.Type == "Microsoft.KeyVault/vaults" && res.Name == name {
			return true
		}
	}
	return false
}

// purgeDeletedKeyVault stubs the purge-deleted-vault endpoint that azurerm v4
// calls after deleting a Key Vault when purge_protection_enabled = false.
// azemu does not implement soft-delete; returning 200 OK signals that the purge
// is complete and allows the provider to continue.
func (a *Router) purgeDeletedKeyVault(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "vaultName")
	log.Info().Str("vault_name", name).Msg("key vault purge (no-op)")
	w.WriteHeader(http.StatusOK)
}

// getKeyVaultCertificateContacts handles the data-plane GET
// /{vaultName}/certificates/contacts endpoint. azurerm v4 calls this on every
// plan/refresh to detect drift in certificate contact configuration. azemu does
// not manage certificates; returning an empty contact list allows the provider
// to proceed without error.
func (a *Router) getKeyVaultCertificateContacts(w http.ResponseWriter, r *http.Request) {
	vaultName := chi.URLParam(r, "vaultName")
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id":    "https://" + vaultName + ".vault.azure.net/certificates/contacts",
		"value": []interface{}{},
		"attributes": map[string]interface{}{
			"enabled": true,
		},
	})
}

// keyVaultResponse builds the canonical ARM response for a key vault.
// provisioningState is always "Succeeded" regardless of what was stored.
func keyVaultResponse(v *store.Resource) map[string]interface{} {
	props := map[string]interface{}{
		"provisioningState": "Succeeded",
	}
	for k, val := range v.Properties {
		if k == "provisioningState" {
			continue
		}
		props[k] = val
	}
	return map[string]interface{}{
		"id":         v.ID,
		"name":       v.Name,
		"type":       v.Type,
		"location":   v.Location,
		"tags":       v.Tags,
		"properties": props,
	}
}

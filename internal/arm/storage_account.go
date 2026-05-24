package arm

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/zerodeth/azemu/internal/store"
)

const storageAccountTypeString = "Microsoft.Storage/storageAccounts"

func storageAccountID(subID, rgName, name string) string {
	return fmt.Sprintf(
		"/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Storage/storageAccounts/%s",
		subID, rgName, name,
	)
}

// storageAccountBody is the subset of the azurerm_storage_account PUT payload
// that azemu understands. SKU and Kind live at the top level of the ARM
// document (not inside properties); they are stashed in Properties["_sku"] and
// Properties["_kind"] so store.Resource stays unchanged, then promoted back on
// response.
type storageAccountBody struct {
	Location   string                 `json:"location"`
	Tags       map[string]string      `json:"tags"`
	Sku        map[string]interface{} `json:"sku"`
	Kind       string                 `json:"kind"`
	Properties map[string]interface{} `json:"properties"`
}

// storageSKUTier derives the tier string from a SKU name.
// Standard_* -> "Standard"; Premium_* -> "Premium".
func storageSKUTier(skuName string) string {
	if strings.HasPrefix(strings.ToLower(skuName), "premium") {
		return "Premium"
	}
	return "Standard"
}

// azuriteDevKey is the well-known Azurite development storage account key.
// The azurerm provider calls listKeys before any data-plane operation; returning
// this key lets SDK clients authenticate against the Azurite sidecar without
// additional configuration. Source: https://github.com/Azure/Azurite#default-storage-account
const azuriteDevKey = "Eby8vdM02xNOcqFlqUwJPLlmEtlCDXJ1OUzFT50uSRZ6IFsuFq2UVErCz4I6tq/K1SZFPTOtr/KBHBeksoGMGw=="

// storagePrimaryEndpoints returns path-style Azurite endpoint URLs for a
// storage account. azuriteBase is the blob service base URL (e.g.
// "http://azurite:10000"). Queue and table ports follow Azurite's convention:
// blob=10000, queue=10001, table=10002. Path-style URLs avoid any /etc/hosts
// wildcard requirement.
func storagePrimaryEndpoints(azuriteBase, accountName string) map[string]interface{} {
	blobBase := azuriteBase
	queueBase := azuriteBase
	tableBase := azuriteBase

	if u, err := url.Parse(azuriteBase); err == nil {
		host := u.Hostname()
		scheme := u.Scheme
		blobBase = fmt.Sprintf("%s://%s:10000", scheme, host)
		queueBase = fmt.Sprintf("%s://%s:10001", scheme, host)
		tableBase = fmt.Sprintf("%s://%s:10002", scheme, host)
	}

	return map[string]interface{}{
		"blob":  fmt.Sprintf("%s/%s/", blobBase, accountName),
		"queue": fmt.Sprintf("%s/%s/", queueBase, accountName),
		"table": fmt.Sprintf("%s/%s/", tableBase, accountName),
		// Azurite does not emulate the file service; use the blob base so the
		// field is non-empty and parseable by the provider.
		"file": fmt.Sprintf("%s/%s/", blobBase, accountName),
		"web":  fmt.Sprintf("%s/%s/", blobBase, accountName),
		"dfs":  fmt.Sprintf("%s/%s/", blobBase, accountName),
	}
}

// storageAccountNameUnique returns true when no other storage account with the
// given name exists in the store. skipID is the ID of the account being
// updated (so an idempotent PUT does not conflict with itself).
func (a *Router) storageAccountNameUnique(name, skipID string) bool {
	for _, res := range a.store.List("/subscriptions/") {
		if res.Type == storageAccountTypeString && res.Name == name && res.ID != skipID {
			return false
		}
	}
	return true
}

func (a *Router) putStorageAccount(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	name := chi.URLParam(r, "accountName")

	var body storageAccountBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeAzureError(w, http.StatusBadRequest, "InvalidRequestContent", err.Error())
		return
	}
	if strings.TrimSpace(body.Location) == "" {
		writeAzureError(w, http.StatusBadRequest, "InvalidRequestContent",
			"location is required")
		return
	}

	id := storageAccountID(subID, rgName, name)

	// Name uniqueness check: storage account names must be unique across the
	// emulator (mirrors Azure's global uniqueness requirement).
	if !a.storageAccountNameUnique(name, id) {
		writeAzureError(w, http.StatusConflict, "StorageAccountAlreadyTaken",
			fmt.Sprintf("The storage account named '%s' is already taken.", name))
		return
	}

	if body.Properties == nil {
		body.Properties = map[string]interface{}{}
	}
	body.Properties["provisioningState"] = "Succeeded"
	body.Properties["primaryLocation"] = strings.ToLower(body.Location)
	body.Properties["statusOfPrimary"] = "available"
	body.Properties["primaryEndpoints"] = storagePrimaryEndpoints(a.azuriteEndpoint, name)

	// Default accessTier to Hot if not supplied.
	if _, ok := body.Properties["accessTier"]; !ok {
		body.Properties["accessTier"] = "Hot"
	}

	// Store SKU and Kind under private keys so store.Resource stays unchanged.
	if body.Sku != nil {
		body.Properties["_sku"] = body.Sku
	} else if _, has := body.Properties["_sku"]; !has {
		body.Properties["_sku"] = map[string]interface{}{"name": "Standard_LRS"}
	}

	kind := body.Kind
	if kind == "" {
		kind = "StorageV2"
	}
	body.Properties["_kind"] = kind

	res := &store.Resource{
		ID:         id,
		Name:       name,
		Type:       storageAccountTypeString,
		Location:   strings.ToLower(body.Location),
		Tags:       normaliseTags(body.Tags),
		Properties: body.Properties,
	}

	_, exists := a.store.Get(id)
	if err := a.store.Put(id, res); err != nil {
		writeAzureError(w, http.StatusInternalServerError, "InternalServerError",
			fmt.Sprintf("put storage account %q: %s", name, err))
		return
	}

	// azurerm's storageaccounts.Create accepts 200 or 202 only — not 201.
	// Return 200 OK for both create and update (storage accounts are idempotent).
	log.Info().Str("resource_id", id).Bool("existed", exists).Msg("storage account upsert")
	writeJSON(w, http.StatusOK, storageAccountResponse(res))
}

func (a *Router) getStorageAccount(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	name := chi.URLParam(r, "accountName")
	id := storageAccountID(subID, rgName, name)

	res, ok := a.store.Get(id)
	if !ok {
		writeAzureError(w, http.StatusNotFound, "ResourceNotFound",
			fmt.Sprintf("The Resource 'Microsoft.Storage/storageAccounts/%s' under resource group '%s' was not found.", name, rgName))
		return
	}
	writeJSON(w, http.StatusOK, storageAccountResponse(res))
}

func (a *Router) headStorageAccount(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	name := chi.URLParam(r, "accountName")
	id := storageAccountID(subID, rgName, name)

	if _, ok := a.store.Get(id); !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *Router) deleteStorageAccount(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	name := chi.URLParam(r, "accountName")
	id := storageAccountID(subID, rgName, name)

	if !a.store.Delete(id) {
		writeAzureError(w, http.StatusNotFound, "ResourceNotFound",
			fmt.Sprintf("The Resource 'Microsoft.Storage/storageAccounts/%s' under resource group '%s' was not found.", name, rgName))
		return
	}

	// Cascade delete: remove all child blob containers stored under this account's id prefix.
	for _, child := range a.store.List(id + "/") {
		a.store.Delete(child.ID)
	}

	log.Info().Str("resource_id", id).Msg("storage account deleted")
	w.Header().Set("Location",
		fmt.Sprintf("/subscriptions/%s/operationresults/%s", subID, uuid.New().String()))
	w.WriteHeader(http.StatusAccepted)
}

func (a *Router) listStorageAccountsByRG(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	prefix := fmt.Sprintf(
		"/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Storage/storageAccounts/",
		subID, rgName,
	)
	a.writeStorageAccountList(w, prefix)
}

func (a *Router) listStorageAccountsBySub(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	prefix := fmt.Sprintf("/subscriptions/%s/resourceGroups/", subID)
	a.writeStorageAccountList(w, prefix)
}

// listStorageAccountKeys handles POST .../listkeys. The azurerm provider calls
// this endpoint to retrieve storage account credentials before writing any
// data-plane resource. Returning Azurite's well-known development key means SDK
// clients that point at the Azurite sidecar can authenticate without manual
// key management.
func (a *Router) listStorageAccountKeys(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	name := chi.URLParam(r, "accountName")
	id := storageAccountID(subID, rgName, name)

	if _, ok := a.store.Get(id); !ok {
		writeAzureError(w, http.StatusNotFound, "ResourceNotFound",
			fmt.Sprintf("The Resource 'Microsoft.Storage/storageAccounts/%s' under resource group '%s' was not found.", name, rgName))
		return
	}

	log.Info().Str("resource_id", id).Msg("storage account listKeys")
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"keys": []map[string]interface{}{
			{
				"keyName":      "key1",
				"value":        azuriteDevKey,
				"permissions":  "FULL",
				"creationTime": "2026-01-01T00:00:00Z",
			},
			{
				"keyName":      "key2",
				"value":        azuriteDevKey,
				"permissions":  "FULL",
				"creationTime": "2026-01-01T00:00:00Z",
			},
		},
	})
}

func (a *Router) writeStorageAccountList(w http.ResponseWriter, prefix string) {
	items := []map[string]interface{}{}
	for _, res := range a.store.List(prefix) {
		if res.Type != storageAccountTypeString {
			continue
		}
		items = append(items, storageAccountResponse(res))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"value": items})
}

// storageAccountResponse builds the canonical ARM response for a storage account.
// The SKU and Kind are stored in Properties["_sku"] and Properties["_kind"] and
// are promoted to top-level fields, matching the real Azure REST API shape.
func storageAccountResponse(s *store.Resource) map[string]interface{} {
	props := map[string]interface{}{
		"provisioningState": "Succeeded",
	}
	var sku map[string]interface{}
	kind := "StorageV2"

	for k, v := range s.Properties {
		switch k {
		case "_sku":
			if m, ok := v.(map[string]interface{}); ok {
				sku = m
			}
		case "_kind":
			if str, ok := v.(string); ok {
				kind = str
			}
		case "provisioningState":
			// always overwritten above
		default:
			props[k] = v
		}
	}

	if sku == nil {
		sku = map[string]interface{}{"name": "Standard_LRS", "tier": "Standard"}
	} else if _, hasTier := sku["tier"]; !hasTier {
		// Derive tier from name if not already set.
		if n, ok := sku["name"].(string); ok {
			sku["tier"] = storageSKUTier(n)
		}
	}

	return map[string]interface{}{
		"id":         s.ID,
		"name":       s.Name,
		"type":       s.Type,
		"location":   s.Location,
		"tags":       s.Tags,
		"sku":        sku,
		"kind":       kind,
		"properties": props,
	}
}

// getStorageFileService stubs the file service endpoint that azurerm v4 polls
// after creating a storage account to wait for the service to become available.
// azemu does not implement file services; returning a Succeeded response lets
// the provider continue without waiting.
func (a *Router) getStorageFileService(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	accountName := chi.URLParam(r, "accountName")

	id := fmt.Sprintf(
		"/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Storage/storageAccounts/%s/fileServices/default",
		subID, rgName, accountName,
	)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id":   id,
		"name": "default",
		"type": "Microsoft.Storage/storageAccounts/fileServices",
		"properties": map[string]interface{}{
			"provisioningState": "Succeeded",
		},
	})
}

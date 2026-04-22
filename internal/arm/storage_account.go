package arm

import (
	"encoding/json"
	"fmt"
	"net/http"
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

// storagePrimaryEndpoints returns the fake primary endpoint URLs for a storage
// account. The account name is used as the subdomain, matching real Azure's
// naming scheme so Terraform state fields like primary_blob_endpoint are
// populated with plausible values.
func storagePrimaryEndpoints(name string) map[string]interface{} {
	return map[string]interface{}{
		"blob":  fmt.Sprintf("https://%s.blob.core.windows.net/", name),
		"queue": fmt.Sprintf("https://%s.queue.core.windows.net/", name),
		"table": fmt.Sprintf("https://%s.table.core.windows.net/", name),
		"file":  fmt.Sprintf("https://%s.file.core.windows.net/", name),
		"web":   fmt.Sprintf("https://%s.z6.web.core.windows.net/", name),
		"dfs":   fmt.Sprintf("https://%s.dfs.core.windows.net/", name),
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
	body.Properties["primaryEndpoints"] = storagePrimaryEndpoints(name)

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

	status := http.StatusCreated
	if exists {
		status = http.StatusOK
	}
	log.Info().Str("resource_id", id).Bool("existed", exists).Msg("storage account upsert")
	writeJSON(w, status, storageAccountResponse(res))
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

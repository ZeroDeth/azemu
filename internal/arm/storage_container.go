package arm

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"

	"github.com/zerodeth/azemu/internal/store"
)

const storageContainerTypeString = "Microsoft.Storage/storageAccounts/blobServices/containers"

func storageContainerID(subID, rgName, accountName, containerName string) string {
	return fmt.Sprintf(
		"/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Storage/storageAccounts/%s/blobServices/default/containers/%s",
		subID, rgName, accountName, containerName,
	)
}

// storageContainerBody is the PUT payload for azurerm_storage_container.
// Containers have no location or SKU; only properties matter.
type storageContainerBody struct {
	Properties map[string]interface{} `json:"properties"`
}

func (a *Router) putStorageContainer(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	accountName := chi.URLParam(r, "accountName")
	containerName := chi.URLParam(r, "containerName")

	// Parent existence check: 404 if the storage account does not exist.
	parentID := storageAccountID(subID, rgName, accountName)
	if _, ok := a.store.Get(parentID); !ok {
		writeAzureError(w, http.StatusNotFound, "ParentResourceNotFound",
			fmt.Sprintf("Storage account '%s' could not be found.", accountName))
		return
	}

	var body storageContainerBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeAzureError(w, http.StatusBadRequest, "InvalidRequestContent", err.Error())
		return
	}
	if body.Properties == nil {
		body.Properties = map[string]interface{}{}
	}
	body.Properties["provisioningState"] = "Succeeded"
	body.Properties["leaseStatus"] = "Unlocked"
	body.Properties["leaseState"] = "Available"

	// Default publicAccess to None when not supplied.
	if _, ok := body.Properties["publicAccess"]; !ok {
		body.Properties["publicAccess"] = "None"
	}

	id := storageContainerID(subID, rgName, accountName, containerName)
	res := &store.Resource{
		ID:         id,
		Name:       containerName,
		Type:       storageContainerTypeString,
		Location:   "", // containers inherit location from account; not stored separately
		Tags:       normaliseTags(nil),
		Properties: body.Properties,
	}

	_, exists := a.store.Get(id)
	if err := a.store.Put(id, res); err != nil {
		writeAzureError(w, http.StatusInternalServerError, "InternalServerError",
			fmt.Sprintf("put storage container %q: %s", containerName, err))
		return
	}

	status := http.StatusCreated
	if exists {
		status = http.StatusOK
	}
	log.Info().Str("resource_id", id).Bool("existed", exists).Msg("storage container upsert")
	writeJSON(w, status, storageContainerResponse(res))
}

func (a *Router) getStorageContainer(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	accountName := chi.URLParam(r, "accountName")
	containerName := chi.URLParam(r, "containerName")
	id := storageContainerID(subID, rgName, accountName, containerName)

	res, ok := a.store.Get(id)
	if !ok {
		writeAzureError(w, http.StatusNotFound, "ResourceNotFound",
			fmt.Sprintf("Container '%s' could not be found.", containerName))
		return
	}
	writeJSON(w, http.StatusOK, storageContainerResponse(res))
}

func (a *Router) headStorageContainer(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	accountName := chi.URLParam(r, "accountName")
	containerName := chi.URLParam(r, "containerName")
	id := storageContainerID(subID, rgName, accountName, containerName)

	if _, ok := a.store.Get(id); !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *Router) deleteStorageContainer(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	accountName := chi.URLParam(r, "accountName")
	containerName := chi.URLParam(r, "containerName")
	id := storageContainerID(subID, rgName, accountName, containerName)

	if !a.store.Delete(id) {
		writeAzureError(w, http.StatusNotFound, "ResourceNotFound",
			fmt.Sprintf("Container '%s' could not be found.", containerName))
		return
	}

	log.Info().Str("resource_id", id).Msg("storage container deleted")
	// azurerm's containers.Client#Delete expects 200 OK (synchronous delete).
	// Blob containers are child resources; deletion is immediate, not async.
	w.WriteHeader(http.StatusOK)
}

func (a *Router) listStorageContainers(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	accountName := chi.URLParam(r, "accountName")
	prefix := fmt.Sprintf(
		"/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Storage/storageAccounts/%s/blobServices/default/containers/",
		subID, rgName, accountName,
	)
	items := []map[string]interface{}{}
	for _, res := range a.store.List(prefix) {
		if res.Type != storageContainerTypeString {
			continue
		}
		items = append(items, storageContainerResponse(res))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"value": items})
}

// storageContainerResponse builds the canonical ARM response for a blob container.
// Containers do not have a top-level location or SKU field.
func storageContainerResponse(c *store.Resource) map[string]interface{} {
	props := map[string]interface{}{
		"provisioningState": "Succeeded",
		"leaseStatus":       "Unlocked",
		"leaseState":        "Available",
	}
	for k, v := range c.Properties {
		switch k {
		case "provisioningState", "leaseStatus", "leaseState":
			// always overwritten above
		default:
			props[k] = v
		}
	}

	// Containers have no location; suppress empty string in response.
	loc := c.Location
	if strings.TrimSpace(loc) == "" {
		return map[string]interface{}{
			"id":         c.ID,
			"name":       c.Name,
			"type":       c.Type,
			"tags":       c.Tags,
			"properties": props,
		}
	}
	return map[string]interface{}{
		"id":         c.ID,
		"name":       c.Name,
		"type":       c.Type,
		"location":   loc,
		"tags":       c.Tags,
		"properties": props,
	}
}

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

const userAssignedIdentityTypeString = "Microsoft.ManagedIdentity/userAssignedIdentities"

func userAssignedIdentityID(subID, rgName, name string) string {
	return fmt.Sprintf(
		"/subscriptions/%s/resourceGroups/%s/providers/Microsoft.ManagedIdentity/userAssignedIdentities/%s",
		subID, rgName, name,
	)
}

type userAssignedIdentityBody struct {
	Location string            `json:"location"`
	Tags     map[string]string `json:"tags"`
}

func (a *Router) putUserAssignedIdentity(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	name := chi.URLParam(r, "identityName")

	var body userAssignedIdentityBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeAzureError(w, http.StatusBadRequest, "InvalidRequestContent", err.Error())
		return
	}
	if strings.TrimSpace(body.Location) == "" {
		writeAzureError(w, http.StatusBadRequest, "InvalidRequestContent",
			"location is required")
		return
	}

	id := userAssignedIdentityID(subID, rgName, name)
	// principalId and clientId are derived deterministically so that
	// Terraform round-trips return consistent values across plan/apply/refresh.
	ns := uuid.MustParse("6ba7b810-9dad-11d1-80b4-00c04fd430c8")
	principalID := uuid.NewSHA1(ns, []byte(id+"/principal")).String()
	clientID := uuid.NewSHA1(ns, []byte(id+"/client")).String()

	props := map[string]interface{}{
		"provisioningState": "Succeeded",
		"principalId":       principalID,
		"clientId":          clientID,
		"tenantId":          "00000000-0000-0000-0000-000000000001",
	}

	res := &store.Resource{
		ID:         id,
		Name:       name,
		Type:       userAssignedIdentityTypeString,
		Location:   strings.ToLower(body.Location),
		Tags:       normaliseTags(body.Tags),
		Properties: props,
	}

	_, exists := a.store.Get(id)
	if err := a.store.Put(id, res); err != nil {
		writeAzureError(w, http.StatusInternalServerError, "InternalServerError",
			fmt.Sprintf("put user assigned identity %q: %s", name, err))
		return
	}

	status := http.StatusCreated
	if exists {
		status = http.StatusOK
	}
	log.Info().Str("resource_id", id).Bool("existed", exists).Msg("user assigned identity upsert")
	writeJSON(w, status, userAssignedIdentityResponse(res))
}

func (a *Router) getUserAssignedIdentity(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	name := chi.URLParam(r, "identityName")
	id := userAssignedIdentityID(subID, rgName, name)

	res, ok := a.store.Get(id)
	if !ok {
		writeAzureError(w, http.StatusNotFound, "ResourceNotFound",
			fmt.Sprintf("The Resource 'Microsoft.ManagedIdentity/userAssignedIdentities/%s' under resource group '%s' was not found.", name, rgName))
		return
	}
	writeJSON(w, http.StatusOK, userAssignedIdentityResponse(res))
}

func (a *Router) headUserAssignedIdentity(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	name := chi.URLParam(r, "identityName")
	id := userAssignedIdentityID(subID, rgName, name)

	if _, ok := a.store.Get(id); !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *Router) deleteUserAssignedIdentity(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	name := chi.URLParam(r, "identityName")
	id := userAssignedIdentityID(subID, rgName, name)

	if !a.store.Delete(id) {
		writeAzureError(w, http.StatusNotFound, "ResourceNotFound",
			fmt.Sprintf("The Resource 'Microsoft.ManagedIdentity/userAssignedIdentities/%s' under resource group '%s' was not found.", name, rgName))
		return
	}

	log.Info().Str("resource_id", id).Msg("user assigned identity deleted")
	// azurerm's userassignedidentities.Client#Delete expects 200 OK or 204.
	// UAI is a leaf resource; deletion is synchronous.
	w.WriteHeader(http.StatusOK)
}

func (a *Router) listUserAssignedIdentitiesByRG(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	prefix := fmt.Sprintf(
		"/subscriptions/%s/resourceGroups/%s/providers/Microsoft.ManagedIdentity/userAssignedIdentities/",
		subID, rgName,
	)
	a.writeUserAssignedIdentityList(w, prefix)
}

func (a *Router) listUserAssignedIdentitiesBySub(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	prefix := fmt.Sprintf("/subscriptions/%s/resourceGroups/", subID)
	a.writeUserAssignedIdentityList(w, prefix)
}

func (a *Router) writeUserAssignedIdentityList(w http.ResponseWriter, prefix string) {
	items := []map[string]interface{}{}
	for _, res := range a.store.List(prefix) {
		if res.Type != userAssignedIdentityTypeString {
			continue
		}
		items = append(items, userAssignedIdentityResponse(res))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"value": items})
}

func userAssignedIdentityResponse(v *store.Resource) map[string]interface{} {
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

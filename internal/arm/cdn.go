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

const (
	cdnProfileTypeString  = "Microsoft.Cdn/profiles"
	cdnEndpointTypeString = "Microsoft.Cdn/profiles/endpoints"
)

func cdnProfileID(subID, rgName, name string) string {
	return fmt.Sprintf(
		"/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Cdn/profiles/%s",
		subID, rgName, name,
	)
}

func cdnEndpointID(subID, rgName, profileName, endpointName string) string {
	return fmt.Sprintf(
		"/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Cdn/profiles/%s/endpoints/%s",
		subID, rgName, profileName, endpointName,
	)
}

// cdnProfileBody is the subset of the azurerm_cdn_profile PUT payload that
// azemu understands. The SKU lives at the top level (name only, unlike LB/AppGW
// which have tier and capacity), stashed in Properties["_sku"] for storage.
type cdnProfileBody struct {
	Location   string                 `json:"location"`
	Tags       map[string]string      `json:"tags"`
	Sku        map[string]interface{} `json:"sku"`
	Properties map[string]interface{} `json:"properties"`
}

func (a *Router) putCDNProfile(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	name := chi.URLParam(r, "profileName")

	var body cdnProfileBody
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
	body.Properties["resourceState"] = "Active"

	// Store the SKU under a private key so it can be reconstructed on response.
	if body.Sku != nil {
		body.Properties["_sku"] = body.Sku
	} else if _, has := body.Properties["_sku"]; !has {
		body.Properties["_sku"] = map[string]interface{}{"name": "Standard_Microsoft"}
	}

	id := cdnProfileID(subID, rgName, name)
	res := &store.Resource{
		ID:         id,
		Name:       name,
		Type:       cdnProfileTypeString,
		Location:   strings.ToLower(body.Location),
		Tags:       normaliseTags(body.Tags),
		Properties: body.Properties,
	}

	_, exists := a.store.Get(id)
	if err := a.store.Put(id, res); err != nil {
		writeAzureError(w, http.StatusInternalServerError, "InternalServerError",
			fmt.Sprintf("put CDN profile %q: %s", name, err))
		return
	}

	status := http.StatusCreated
	if exists {
		status = http.StatusOK
	}
	log.Info().Str("resource_id", id).Bool("existed", exists).Msg("CDN profile upsert")
	writeJSON(w, status, cdnProfileResponse(res))
}

func (a *Router) getCDNProfile(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	name := chi.URLParam(r, "profileName")
	id := cdnProfileID(subID, rgName, name)

	res, ok := a.store.Get(id)
	if !ok {
		writeAzureError(w, http.StatusNotFound, "ResourceNotFound",
			fmt.Sprintf("The Resource 'Microsoft.Cdn/profiles/%s' under resource group '%s' was not found.", name, rgName))
		return
	}
	writeJSON(w, http.StatusOK, cdnProfileResponse(res))
}

func (a *Router) headCDNProfile(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	name := chi.URLParam(r, "profileName")
	id := cdnProfileID(subID, rgName, name)

	if _, ok := a.store.Get(id); !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *Router) deleteCDNProfile(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	name := chi.URLParam(r, "profileName")
	id := cdnProfileID(subID, rgName, name)

	if !a.store.Delete(id) {
		writeAzureError(w, http.StatusNotFound, "ResourceNotFound",
			fmt.Sprintf("The Resource 'Microsoft.Cdn/profiles/%s' under resource group '%s' was not found.", name, rgName))
		return
	}

	log.Info().Str("resource_id", id).Msg("CDN profile deleted")
	w.Header().Set("Location",
		operationResultLocation(r, subID))
	w.WriteHeader(http.StatusAccepted)
}

func (a *Router) listCDNProfilesByRG(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	prefix := fmt.Sprintf(
		"/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Cdn/profiles/",
		subID, rgName,
	)
	a.writeCDNProfileList(w, prefix)
}

func (a *Router) listCDNProfilesBySub(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	prefix := fmt.Sprintf("/subscriptions/%s/resourceGroups/", subID)
	a.writeCDNProfileList(w, prefix)
}

func (a *Router) writeCDNProfileList(w http.ResponseWriter, prefix string) {
	items := []map[string]interface{}{}
	for _, res := range a.store.List(prefix) {
		if res.Type != cdnProfileTypeString {
			continue
		}
		items = append(items, cdnProfileResponse(res))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"value": items})
}

func cdnProfileResponse(p *store.Resource) map[string]interface{} {
	props := map[string]interface{}{
		"provisioningState": "Succeeded",
		"resourceState":     "Active",
	}
	for k, val := range p.Properties {
		if k == "provisioningState" || k == "resourceState" {
			continue
		}
		props[k] = val
	}
	// Promote the stored SKU back to the top-level field.
	sku := map[string]interface{}{"name": "Standard_Microsoft"}
	if s, ok := props["_sku"]; ok {
		if sm, ok := s.(map[string]interface{}); ok {
			sku = sm
		}
		delete(props, "_sku")
	}
	return map[string]interface{}{
		"id":         p.ID,
		"name":       p.Name,
		"type":       p.Type,
		"location":   p.Location,
		"tags":       p.Tags,
		"sku":        sku,
		"properties": props,
	}
}

// --- CDN Endpoints ---

// cdnEndpointPutBody is the PUT payload for azurerm_cdn_endpoint. The origin
// configuration (hostname, HTTP/HTTPS port, protocol) is stored inside
// properties verbatim so Terraform round-trips are satisfied.
type cdnEndpointPutBody struct {
	Location   string                 `json:"location"`
	Tags       map[string]string      `json:"tags"`
	Properties map[string]interface{} `json:"properties"`
}

func (a *Router) putCDNEndpoint(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	profileName := chi.URLParam(r, "profileName")
	endpointName := chi.URLParam(r, "endpointName")

	// Verify parent profile exists.
	profileID := cdnProfileID(subID, rgName, profileName)
	if _, ok := a.store.Get(profileID); !ok {
		writeAzureError(w, http.StatusNotFound, "ParentResourceNotFound",
			fmt.Sprintf("The parent resource 'Microsoft.Cdn/profiles/%s' was not found.", profileName))
		return
	}

	var body cdnEndpointPutBody
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
	body.Properties["resourceState"] = "Running"
	// Compute the hostname from the endpoint name (matches real Azure convention).
	body.Properties["hostName"] = fmt.Sprintf("%s.azureedge.net", endpointName)

	id := cdnEndpointID(subID, rgName, profileName, endpointName)
	res := &store.Resource{
		ID:         id,
		Name:       endpointName,
		Type:       cdnEndpointTypeString,
		Location:   strings.ToLower(body.Location),
		Tags:       normaliseTags(body.Tags),
		Properties: body.Properties,
	}

	_, exists := a.store.Get(id)
	if err := a.store.Put(id, res); err != nil {
		writeAzureError(w, http.StatusInternalServerError, "InternalServerError",
			fmt.Sprintf("put CDN endpoint %q: %s", endpointName, err))
		return
	}

	status := http.StatusCreated
	if exists {
		status = http.StatusOK
	}
	log.Info().Str("resource_id", id).Bool("existed", exists).Msg("CDN endpoint upsert")
	writeJSON(w, status, cdnEndpointResponse(res))
}

func (a *Router) getCDNEndpoint(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	profileName := chi.URLParam(r, "profileName")
	endpointName := chi.URLParam(r, "endpointName")
	id := cdnEndpointID(subID, rgName, profileName, endpointName)

	res, ok := a.store.Get(id)
	if !ok {
		writeAzureError(w, http.StatusNotFound, "ResourceNotFound",
			fmt.Sprintf("The Resource 'Microsoft.Cdn/profiles/%s/endpoints/%s' was not found.", profileName, endpointName))
		return
	}
	writeJSON(w, http.StatusOK, cdnEndpointResponse(res))
}

func (a *Router) headCDNEndpoint(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	profileName := chi.URLParam(r, "profileName")
	endpointName := chi.URLParam(r, "endpointName")
	id := cdnEndpointID(subID, rgName, profileName, endpointName)

	if _, ok := a.store.Get(id); !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *Router) deleteCDNEndpoint(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	profileName := chi.URLParam(r, "profileName")
	endpointName := chi.URLParam(r, "endpointName")
	id := cdnEndpointID(subID, rgName, profileName, endpointName)

	if !a.store.Delete(id) {
		writeAzureError(w, http.StatusNotFound, "ResourceNotFound",
			fmt.Sprintf("The Resource 'Microsoft.Cdn/profiles/%s/endpoints/%s' was not found.", profileName, endpointName))
		return
	}

	log.Info().Str("resource_id", id).Msg("CDN endpoint deleted")
	w.Header().Set("Location",
		operationResultLocation(r, subID))
	w.WriteHeader(http.StatusAccepted)
}

func (a *Router) listCDNEndpoints(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	profileName := chi.URLParam(r, "profileName")
	prefix := fmt.Sprintf(
		"/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Cdn/profiles/%s/endpoints/",
		subID, rgName, profileName,
	)
	items := []map[string]interface{}{}
	for _, res := range a.store.List(prefix) {
		if res.Type != cdnEndpointTypeString {
			continue
		}
		items = append(items, cdnEndpointResponse(res))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"value": items})
}

func cdnEndpointResponse(e *store.Resource) map[string]interface{} {
	props := map[string]interface{}{
		"provisioningState": "Succeeded",
		"resourceState":     "Running",
	}
	for k, val := range e.Properties {
		if k == "provisioningState" || k == "resourceState" {
			continue
		}
		props[k] = val
	}
	return map[string]interface{}{
		"id":         e.ID,
		"name":       e.Name,
		"type":       e.Type,
		"location":   e.Location,
		"tags":       e.Tags,
		"properties": props,
	}
}

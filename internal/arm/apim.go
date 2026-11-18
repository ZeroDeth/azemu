package arm

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"

	"github.com/zerodeth/azemu/internal/store"
)

const apimServiceType = "Microsoft.ApiManagement/service"

func (a *Router) apimRoutes(r chi.Router) {
	base := "/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.apimanagement/service"
	r.Get(base, a.listAPIMServices)
	r.Get("/{subscriptionID}/providers/microsoft.apimanagement/service", a.listAPIMServices)
	r.Route(base+"/{serviceName}", func(r chi.Router) {
		r.Put("/", a.putAPIMService)
		r.Get("/", a.getAPIMService)
		r.Head("/", a.getAPIMService)
		r.Patch("/", a.patchAPIMService)
		r.Delete("/", a.deleteAPIMService)
		r.Get("/products", a.listAPIMInitialChildren)
		r.Get("/policies/policy", a.getAPIMServicePolicy)
		r.Get("/portalsettings/{setting:signin|signup|delegation}", a.apimSettings)
		r.Put("/portalsettings/{setting:signin|signup|delegation}", a.apimSettings)
		r.Post("/portalsettings/{setting:delegation}/listsecrets", a.apimSettings)
		r.Post("/tenant/{setting:access}/listsecrets", a.apimSettings)
		r.Patch("/tenant/{setting:access}", a.apimSettings)
	})
	deleted := "/{subscriptionID}/providers/microsoft.apimanagement/locations/{location}/deletedservices/{serviceName}"
	r.Get(deleted, a.getDeletedAPIMService)
	r.Delete(deleted, a.purgeAPIMService)
}

type apimServiceBody struct {
	Location   string            `json:"location"`
	Tags       map[string]string `json:"tags"`
	SKU        map[string]any    `json:"sku"`
	Properties map[string]any    `json:"properties"`
	Identity   json.RawMessage   `json:"identity"`
	Zones      []string          `json:"zones"`
}

func apimServiceID(sub, rg, name string) string {
	return fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.ApiManagement/service/%s", sub, rg, name)
}

func apimRequestID(r *http.Request) string {
	return apimServiceID(chi.URLParam(r, "subscriptionID"), chi.URLParam(r, "resourceGroupName"), chi.URLParam(r, "serviceName"))
}

func apimDeletedID(sub, location, name string) string {
	return fmt.Sprintf("/subscriptions/%s/providers/Microsoft.ApiManagement/locations/%s/deletedservices/%s", sub, location, name)
}

func apimResponse(res *store.Resource) map[string]any {
	props := make(map[string]any, len(res.Properties))
	for key, value := range res.Properties {
		if !strings.HasPrefix(key, "_") {
			props[key] = value
		}
	}
	result := map[string]any{
		"id": res.ID, "name": res.Name, "type": res.Type,
		"location": res.Location, "tags": normaliseTags(res.Tags),
		"sku": res.Properties["_sku"], "properties": props,
	}
	if identity := res.Properties["_identity"]; identity != nil {
		result["identity"] = identity
	}
	return result
}

func (a *Router) putAPIMService(w http.ResponseWriter, r *http.Request) {
	a.writeAPIMService(w, r, false)
}

func (a *Router) patchAPIMService(w http.ResponseWriter, r *http.Request) {
	a.writeAPIMService(w, r, true)
}

func (a *Router) writeAPIMService(w http.ResponseWriter, r *http.Request, patch bool) {
	id := apimRequestID(r)
	existing, exists := a.store.Get(id)
	if patch && !exists {
		writeAzureError(w, http.StatusNotFound, "ResourceNotFound", "API Management service was not found.")
		return
	}
	parentID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s", chi.URLParam(r, "subscriptionID"), chi.URLParam(r, "resourceGroupName"))
	if _, ok := a.store.Get(parentID); !ok {
		writeAzureError(w, http.StatusNotFound, "ParentResourceNotFound", "Resource group was not found.")
		return
	}
	var body apimServiceBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeAzureError(w, http.StatusBadRequest, "InvalidRequestContent", err.Error())
		return
	}
	if body.Properties == nil {
		body.Properties = map[string]any{}
	}
	if restore, _ := body.Properties["restore"].(bool); restore {
		writeAzureError(w, http.StatusNotImplemented, "NotImplemented", "API Management restore is not implemented; purge the deleted service before recreating it.")
		return
	}
	if len(body.Zones) != 0 {
		writeAzureError(w, http.StatusNotImplemented, "NotImplemented", "API Management availability zones are not implemented.")
		return
	}
	for _, key := range []string{"additionalLocations", "certificates"} {
		if values, ok := body.Properties[key].([]any); ok && len(values) != 0 {
			writeAzureError(w, http.StatusNotImplemented, "NotImplemented", "API Management "+key+" is not implemented.")
			return
		}
	}
	// Reserve internal storage keys; these are not part of the ARM contract.
	for key := range body.Properties {
		if strings.HasPrefix(key, "_") {
			writeAzureError(w, http.StatusBadRequest, "InvalidRequestContent", "Private property names are not accepted.")
			return
		}
	}
	if patch {
		props := make(map[string]any, len(existing.Properties))
		for key, value := range existing.Properties {
			props[key] = value
		}
		for key, value := range body.Properties {
			props[key] = value
		}
		body.Properties = props
		body.Location = existing.Location
		if body.Tags == nil {
			body.Tags = existing.Tags
		}
		if body.SKU == nil {
			body.SKU, _ = existing.Properties["_sku"].(map[string]any)
		}
	}
	identity, code, err := a.apimIdentity(body.Identity, existing)
	if err != nil {
		writeAzureError(w, code, "InvalidManagedServiceIdentity", err.Error())
		return
	}
	body.Properties["_identity"] = identity
	publisherName, _ := body.Properties["publisherName"].(string)
	publisherEmail, _ := body.Properties["publisherEmail"].(string)
	if strings.TrimSpace(body.Location) == "" || strings.TrimSpace(publisherName) == "" || strings.TrimSpace(publisherEmail) == "" {
		writeAzureError(w, http.StatusBadRequest, "InvalidRequestContent", "location, properties.publisherName and properties.publisherEmail are required.")
		return
	}
	if body.SKU["name"] != "Developer" || body.SKU["capacity"] != float64(1) {
		writeAzureError(w, http.StatusBadRequest, "InvalidSku", "azemu currently supports the Developer SKU with capacity 1 only.")
		return
	}
	if network, _ := body.Properties["virtualNetworkType"].(string); network != "" && network != "None" {
		writeAzureError(w, http.StatusNotImplemented, "NotImplemented", "API Management virtual network integration is not implemented.")
		return
	}
	location := strings.ToLower(strings.ReplaceAll(body.Location, " ", ""))
	deletedID := apimDeletedID(chi.URLParam(r, "subscriptionID"), location, chi.URLParam(r, "serviceName"))
	if _, deleted := a.store.Get(deletedID); deleted {
		writeAzureError(w, http.StatusConflict, "ServiceSoftDeleted", "Purge the soft-deleted API Management service before recreating it.")
		return
	}
	body.Properties["_sku"] = body.SKU
	body.Properties["provisioningState"] = "Succeeded"
	setDefault := func(key string, value any) {
		if _, ok := body.Properties[key]; !ok {
			body.Properties[key] = value
		}
	}
	name := chi.URLParam(r, "serviceName")
	hostnames, code, err := a.apimHostnames(body.Properties["hostnameConfigurations"], identity, name)
	if err != nil {
		azureCode := "InvalidHostnameConfiguration"
		if code == http.StatusForbidden {
			azureCode = "Forbidden"
		}
		writeAzureError(w, code, azureCode, err.Error())
		return
	}
	body.Properties["hostnameConfigurations"] = hostnames
	setDefault("customProperties", map[string]any{})
	setDefault("publicNetworkAccess", "Enabled")
	setDefault("virtualNetworkType", "None")
	setDefault("notificationSenderEmail", "apimgmt-noreply@mail.windowsazure.com")
	setDefault("gatewayUrl", "https://"+name+".azure-api.net")
	setDefault("gatewayRegionalUrl", "https://"+name+"-"+location+".regional.azure-api.net")
	setDefault("portalUrl", "https://"+name+".portal.azure-api.net")
	setDefault("developerPortalUrl", "https://"+name+".developer.azure-api.net")
	setDefault("managementApiUrl", "https://"+name+".management.azure-api.net")
	setDefault("scmUrl", "https://"+name+".scm.azure-api.net")
	res := &store.Resource{
		ID: id, Name: name, Type: apimServiceType, Location: location,
		Tags: normaliseTags(body.Tags), Properties: body.Properties,
	}
	if err := a.store.Put(id, res); err != nil {
		log.Error().Err(err).Str("resource_id", id).Msg("failed to store API Management service")
		writeAzureError(w, http.StatusInternalServerError, "InternalServerError", "Failed to store API Management service.")
		return
	}
	status := http.StatusCreated
	if exists {
		status = http.StatusOK
	}
	log.Info().Str("resource_id", id).Bool("existed", exists).Msg("API Management service upsert")
	writeJSON(w, status, apimResponse(res))
}

func (a *Router) getAPIMService(w http.ResponseWriter, r *http.Request) {
	res, ok := a.store.Get(apimRequestID(r))
	if !ok {
		writeAzureError(w, http.StatusNotFound, "ResourceNotFound", "API Management service was not found.")
		return
	}
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	writeJSON(w, http.StatusOK, apimResponse(res))
}

func (a *Router) listAPIMServices(w http.ResponseWriter, r *http.Request) {
	prefix := "/subscriptions/" + chi.URLParam(r, "subscriptionID") + "/resourceGroups/"
	if rg := chi.URLParam(r, "resourceGroupName"); rg != "" {
		prefix += rg + "/providers/Microsoft.ApiManagement/service/"
	}
	items := []map[string]any{}
	for _, res := range a.store.List(prefix) {
		if res.Type == apimServiceType {
			items = append(items, apimResponse(res))
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"value": items})
}

func (a *Router) deleteAPIMService(w http.ResponseWriter, r *http.Request) {
	id := apimRequestID(r)
	res, exists := a.store.Get(id)
	if !exists {
		writeAzureError(w, http.StatusNotFound, "ResourceNotFound", "API Management service was not found.")
		return
	}
	deletedID := apimDeletedID(chi.URLParam(r, "subscriptionID"), res.Location, res.Name)
	now := time.Now().UTC()
	deleted := &store.Resource{
		ID: deletedID, Name: res.Name, Type: "Microsoft.ApiManagement/deletedservices", Location: res.Location,
		Properties: map[string]any{
			"serviceId": id, "deletionDate": now.Format(time.RFC3339),
			"scheduledPurgeDate": now.Add(30 * 24 * time.Hour).Format(time.RFC3339),
		},
	}
	if err := a.store.Put(deletedID, deleted); err != nil {
		log.Error().Err(err).Str("resource_id", id).Msg("failed to soft-delete API Management service")
		writeAzureError(w, http.StatusInternalServerError, "InternalServerError", "Failed to retain deleted API Management service.")
		return
	}
	a.store.Delete(id)
	log.Info().Str("resource_id", id).Msg("API Management service soft-deleted")
	a.acceptAsyncDelete(w, r, chi.URLParam(r, "subscriptionID"))
}

func (a *Router) getDeletedAPIMService(w http.ResponseWriter, r *http.Request) {
	id := apimDeletedID(chi.URLParam(r, "subscriptionID"), chi.URLParam(r, "location"), chi.URLParam(r, "serviceName"))
	res, exists := a.store.Get(id)
	if !exists {
		writeAzureError(w, http.StatusNotFound, "ResourceNotFound", "Deleted API Management service was not found.")
		return
	}
	writeJSON(w, http.StatusOK, apimResponse(res))
}

func (a *Router) purgeAPIMService(w http.ResponseWriter, r *http.Request) {
	id := apimDeletedID(chi.URLParam(r, "subscriptionID"), chi.URLParam(r, "location"), chi.URLParam(r, "serviceName"))
	if !a.store.Delete(id) {
		writeAzureError(w, http.StatusNotFound, "ResourceNotFound", "Deleted API Management service was not found.")
		return
	}
	log.Info().Str("resource_id", id).Msg("API Management service purged")
	w.WriteHeader(http.StatusNoContent)
}

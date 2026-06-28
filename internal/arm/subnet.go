package arm

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"

	"github.com/zerodeth/azemu/internal/store"
)

// subnetTypeString is the canonical ARM type for subnets. Subnets are
// modelled as child resources of a virtual network, stored at a hierarchical
// id that is a path extension of the parent vnet id — this is what lets
// store.Delete cascade subnets when the parent vnet is deleted.
const subnetTypeString = "Microsoft.Network/virtualNetworks/subnets"

func subnetID(subID, rgName, vnetName, subnetName string) string {
	return fmt.Sprintf(
		"/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/virtualNetworks/%s/subnets/%s",
		subID, rgName, vnetName, subnetName,
	)
}

// subnetBody matches the azurerm_subnet PUT payload. Subnets inherit location
// from their parent vnet so location is intentionally absent from the body
// struct — ARM rejects subnet bodies that try to set their own location.
type subnetBody struct {
	Properties map[string]interface{} `json:"properties"`
}

func (a *Router) putSubnet(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	vnetName := chi.URLParam(r, "vnetName")
	name := chi.URLParam(r, "subnetName")

	// Parent-exists check: real ARM returns 404 ParentResourceNotFound when
	// a subnet is PUT against a non-existent vnet. azemu mirrors this for
	// fidelity so broken Terraform graphs surface the correct error.
	parentID := vnetID(subID, rgName, vnetName)
	parent, ok := a.store.Get(parentID)
	if parent == nil || !ok {
		writeAzureError(w, http.StatusNotFound, "ParentResourceNotFound",
			fmt.Sprintf("Virtual network '%s' could not be found.", vnetName))
		return
	}

	var body subnetBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeAzureError(w, http.StatusBadRequest, "InvalidRequestContent", err.Error())
		return
	}
	if body.Properties == nil {
		body.Properties = map[string]interface{}{}
	}
	body.Properties["provisioningState"] = "Succeeded"

	id := subnetID(subID, rgName, vnetName, name)
	res := &store.Resource{
		ID:   id,
		Name: name,
		Type: subnetTypeString,
		// Subnets inherit location from their parent vnet; storing it here
		// keeps list responses consistent even if a future handler surfaces it.
		Location:   parent.Location,
		Properties: body.Properties,
	}

	_, exists := a.store.Get(id)
	if err := a.store.Put(id, res); err != nil {
		writeAzureError(w, http.StatusInternalServerError, "InternalServerError",
			fmt.Sprintf("put subnet %q: %s", name, err))
		return
	}

	status := http.StatusCreated
	if exists {
		status = http.StatusOK
	}
	log.Info().Str("resource_id", id).Bool("existed", exists).Msg("subnet upsert")
	writeJSON(w, status, subnetResponse(res))
}

func (a *Router) getSubnet(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	vnetName := chi.URLParam(r, "vnetName")
	name := chi.URLParam(r, "subnetName")
	id := subnetID(subID, rgName, vnetName, name)

	res, ok := a.store.Get(id)
	if !ok {
		writeAzureError(w, http.StatusNotFound, "ResourceNotFound",
			fmt.Sprintf("Subnet '%s' could not be found.", name))
		return
	}
	writeJSON(w, http.StatusOK, subnetResponse(res))
}

func (a *Router) headSubnet(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	vnetName := chi.URLParam(r, "vnetName")
	name := chi.URLParam(r, "subnetName")
	id := subnetID(subID, rgName, vnetName, name)

	if _, ok := a.store.Get(id); !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *Router) deleteSubnet(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	vnetName := chi.URLParam(r, "vnetName")
	name := chi.URLParam(r, "subnetName")
	id := subnetID(subID, rgName, vnetName, name)

	if !a.store.Delete(id) {
		writeAzureError(w, http.StatusNotFound, "ResourceNotFound",
			fmt.Sprintf("Subnet '%s' could not be found.", name))
		return
	}

	log.Info().Str("resource_id", id).Msg("subnet deleted")
	a.acceptAsyncDelete(w, r, subID)
}

func (a *Router) listSubnets(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	vnetName := chi.URLParam(r, "vnetName")

	prefix := vnetID(subID, rgName, vnetName) + "/subnets/"
	resources := a.store.List(prefix)

	items := []map[string]interface{}{}
	for _, res := range resources {
		if res.Type != subnetTypeString {
			continue
		}
		items = append(items, subnetResponse(res))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"value": items})
}

// subnetResponse is the full ARM response shape returned by GET / PUT / LIST
// endpoints. It is a standalone resource document.
func subnetResponse(s *store.Resource) map[string]interface{} {
	props := map[string]interface{}{
		"provisioningState": "Succeeded",
	}
	for k, v := range s.Properties {
		if k == "provisioningState" {
			continue
		}
		props[k] = v
	}
	return map[string]interface{}{
		"id":         s.ID,
		"name":       s.Name,
		"type":       s.Type,
		"properties": props,
	}
}

// subnetEmbedded is the compact shape used inside a parent vnet's
// properties.subnets array. Azure's vnet document embeds subnets without
// the top-level "type" and without the outer "properties" wrapper doubled.
// Keeping this separate from subnetResponse lets the two shapes evolve
// independently if we ever need to diverge further.
func subnetEmbedded(s *store.Resource) map[string]interface{} {
	props := map[string]interface{}{
		"provisioningState": "Succeeded",
	}
	for k, v := range s.Properties {
		if k == "provisioningState" {
			continue
		}
		props[k] = v
	}
	return map[string]interface{}{
		"id":         s.ID,
		"name":       s.Name,
		"properties": props,
	}
}

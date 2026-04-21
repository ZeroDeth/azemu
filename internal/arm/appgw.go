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

const appGWTypeString = "Microsoft.Network/applicationGateways"

func appGWID(subID, rgName, name string) string {
	return fmt.Sprintf(
		"/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/applicationGateways/%s",
		subID, rgName, name,
	)
}

// appGWBody is the subset of the azurerm_application_gateway PUT payload that
// azemu understands. Unlike the load balancer, the azurerm provider sends all
// sub-configuration (backend pools, HTTP settings, listeners, routing rules,
// probes, SSL certs) as inline arrays inside properties — there are no
// separate child-resource endpoints. azemu stores them as-is and returns them
// verbatim on GET, which is sufficient for Terraform plan/apply round-trips.
//
// The SKU lives at the top level of the ARM document with three sub-fields:
// name (e.g. "Standard_v2"), tier (e.g. "Standard_v2"), and capacity. It is
// stashed in Properties["_sku"] for storage and promoted back on response.
type appGWBody struct {
	Location   string                 `json:"location"`
	Tags       map[string]string      `json:"tags"`
	Sku        map[string]interface{} `json:"sku"`
	Properties map[string]interface{} `json:"properties"`
}

func (a *Router) putAppGW(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	name := chi.URLParam(r, "appGWName")

	var body appGWBody
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
	body.Properties["operationalState"] = "Running"

	// Store the SKU under a private key so it can be reconstructed on response.
	if body.Sku != nil {
		body.Properties["_sku"] = body.Sku
	} else if _, has := body.Properties["_sku"]; !has {
		body.Properties["_sku"] = map[string]interface{}{
			"name":     "Standard_v2",
			"tier":     "Standard_v2",
			"capacity": float64(2),
		}
	}

	id := appGWID(subID, rgName, name)
	res := &store.Resource{
		ID:         id,
		Name:       name,
		Type:       appGWTypeString,
		Location:   strings.ToLower(body.Location),
		Tags:       normaliseTags(body.Tags),
		Properties: body.Properties,
	}

	_, exists := a.store.Get(id)
	if err := a.store.Put(id, res); err != nil {
		writeAzureError(w, http.StatusInternalServerError, "InternalServerError",
			fmt.Sprintf("put application gateway %q: %s", name, err))
		return
	}

	status := http.StatusCreated
	if exists {
		status = http.StatusOK
	}
	log.Info().Str("resource_id", id).Bool("existed", exists).Msg("application gateway upsert")
	writeJSON(w, status, appGWResponse(res))
}

func (a *Router) getAppGW(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	name := chi.URLParam(r, "appGWName")
	id := appGWID(subID, rgName, name)

	res, ok := a.store.Get(id)
	if !ok {
		writeAzureError(w, http.StatusNotFound, "ResourceNotFound",
			fmt.Sprintf("Application gateway '%s' could not be found.", name))
		return
	}
	writeJSON(w, http.StatusOK, appGWResponse(res))
}

func (a *Router) headAppGW(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	name := chi.URLParam(r, "appGWName")
	id := appGWID(subID, rgName, name)

	if _, ok := a.store.Get(id); !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *Router) deleteAppGW(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	name := chi.URLParam(r, "appGWName")
	id := appGWID(subID, rgName, name)

	if !a.store.Delete(id) {
		writeAzureError(w, http.StatusNotFound, "ResourceNotFound",
			fmt.Sprintf("Application gateway '%s' could not be found.", name))
		return
	}

	log.Info().Str("resource_id", id).Msg("application gateway deleted")
	w.Header().Set("Location",
		fmt.Sprintf("/subscriptions/%s/operationresults/%s", subID, uuid.New().String()))
	w.WriteHeader(http.StatusAccepted)
}

func (a *Router) listAppGWsByRG(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	prefix := fmt.Sprintf(
		"/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/applicationGateways/",
		subID, rgName,
	)
	a.writeAppGWList(w, prefix)
}

func (a *Router) listAppGWsBySub(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	prefix := fmt.Sprintf("/subscriptions/%s/resourceGroups/", subID)
	a.writeAppGWList(w, prefix)
}

func (a *Router) writeAppGWList(w http.ResponseWriter, prefix string) {
	resources := a.store.List(prefix)
	items := []map[string]interface{}{}
	for _, res := range resources {
		if res.Type != appGWTypeString {
			continue
		}
		items = append(items, appGWResponse(res))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"value": items})
}

// appGWResponse builds the canonical ARM response for an application gateway.
// The SKU stored in Properties["_sku"] is promoted to the top-level "sku"
// field. All other inline property arrays (backend pools, listeners, etc.) are
// returned verbatim as stored.
func appGWResponse(g *store.Resource) map[string]interface{} {
	props := map[string]interface{}{
		"provisioningState": "Succeeded",
		"operationalState":  "Running",
	}
	var sku map[string]interface{}
	for k, v := range g.Properties {
		switch k {
		case "_sku":
			if s, ok := v.(map[string]interface{}); ok {
				sku = s
			}
		case "provisioningState", "operationalState":
			// always authoritative above
		default:
			props[k] = v
		}
	}
	if sku == nil {
		sku = map[string]interface{}{
			"name":     "Standard_v2",
			"tier":     "Standard_v2",
			"capacity": float64(2),
		}
	}

	return map[string]interface{}{
		"id":         g.ID,
		"name":       g.Name,
		"type":       g.Type,
		"location":   g.Location,
		"tags":       g.Tags,
		"sku":        sku,
		"properties": props,
	}
}

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

const publicIPTypeString = "Microsoft.Network/publicIPAddresses"

func publicIPID(subID, rgName, name string) string {
	return fmt.Sprintf(
		"/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/publicIPAddresses/%s",
		subID, rgName, name,
	)
}

// publicIPBody is the subset of the azurerm_public_ip PUT payload that azemu
// understands. The SKU lives at the top level of the ARM document, not inside
// properties — we stash it in Properties["_sku"] for storage and reconstruct
// the top-level field on response, keeping store.Resource unchanged.
type publicIPBody struct {
	Location   string                 `json:"location"`
	Tags       map[string]string      `json:"tags"`
	Sku        map[string]interface{} `json:"sku"`
	Properties map[string]interface{} `json:"properties"`
}

func (a *Router) putPublicIP(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	name := chi.URLParam(r, "publicIPName")

	var body publicIPBody
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

	// Assign a fake public IP address unless one was already stored (i.e. on
	// an update where the resource already exists). For Dynamic allocation the
	// IP would normally be unset until the resource is attached, but returning
	// a value is harmless and simplifies Terraform state reads.
	id := publicIPID(subID, rgName, name)
	if existing, ok := a.store.Get(id); ok {
		if ip, has := existing.Properties["ipAddress"]; has {
			body.Properties["ipAddress"] = ip
		}
	}
	if _, set := body.Properties["ipAddress"]; !set {
		body.Properties["ipAddress"] = fakePIP()
	}

	// Store the SKU under a private key so it can be reconstructed on response.
	if body.Sku != nil {
		body.Properties["_sku"] = body.Sku
	} else if _, has := body.Properties["_sku"]; !has {
		// Default to Basic SKU when the client omits it.
		body.Properties["_sku"] = map[string]interface{}{"name": "Basic", "tier": "Regional"}
	}

	res := &store.Resource{
		ID:         id,
		Name:       name,
		Type:       publicIPTypeString,
		Location:   strings.ToLower(body.Location),
		Tags:       normaliseTags(body.Tags),
		Properties: body.Properties,
	}

	_, exists := a.store.Get(id)
	if err := a.store.Put(id, res); err != nil {
		writeAzureError(w, http.StatusInternalServerError, "InternalServerError",
			fmt.Sprintf("put public IP address %q: %s", name, err))
		return
	}

	status := http.StatusCreated
	if exists {
		status = http.StatusOK
	}
	log.Info().Str("resource_id", id).Bool("existed", exists).Msg("public IP upsert")
	writeJSON(w, status, publicIPResponse(res))
}

func (a *Router) getPublicIP(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	name := chi.URLParam(r, "publicIPName")
	id := publicIPID(subID, rgName, name)

	res, ok := a.store.Get(id)
	if !ok {
		writeAzureError(w, http.StatusNotFound, "ResourceNotFound",
			fmt.Sprintf("Public IP address '%s' could not be found.", name))
		return
	}
	writeJSON(w, http.StatusOK, publicIPResponse(res))
}

func (a *Router) headPublicIP(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	name := chi.URLParam(r, "publicIPName")
	id := publicIPID(subID, rgName, name)

	if _, ok := a.store.Get(id); !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *Router) deletePublicIP(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	name := chi.URLParam(r, "publicIPName")
	id := publicIPID(subID, rgName, name)

	if !a.store.Delete(id) {
		writeAzureError(w, http.StatusNotFound, "ResourceNotFound",
			fmt.Sprintf("Public IP address '%s' could not be found.", name))
		return
	}

	log.Info().Str("resource_id", id).Msg("public IP deleted")
	w.Header().Set("Location",
		fmt.Sprintf("/subscriptions/%s/operationresults/%s", subID, uuid.New().String()))
	w.WriteHeader(http.StatusAccepted)
}

func (a *Router) listPublicIPsByRG(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	prefix := fmt.Sprintf(
		"/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/publicIPAddresses/",
		subID, rgName,
	)
	a.writePublicIPList(w, prefix)
}

func (a *Router) listPublicIPsBySub(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	prefix := fmt.Sprintf("/subscriptions/%s/resourceGroups/", subID)
	a.writePublicIPList(w, prefix)
}

func (a *Router) writePublicIPList(w http.ResponseWriter, prefix string) {
	resources := a.store.List(prefix)
	items := []map[string]interface{}{}
	for _, res := range resources {
		if res.Type != publicIPTypeString {
			continue
		}
		items = append(items, publicIPResponse(res))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"value": items})
}

// publicIPResponse builds the canonical ARM response for a public IP address.
// The SKU is stored in Properties["_sku"] and promoted to the top-level "sku"
// field, which is where the azurerm provider expects to find it.
func publicIPResponse(p *store.Resource) map[string]interface{} {
	props := map[string]interface{}{
		"provisioningState": "Succeeded",
	}
	var sku map[string]interface{}
	for k, v := range p.Properties {
		switch k {
		case "_sku":
			if s, ok := v.(map[string]interface{}); ok {
				sku = s
			}
		case "provisioningState":
			// always authoritative above
		default:
			props[k] = v
		}
	}
	if sku == nil {
		sku = map[string]interface{}{"name": "Basic", "tier": "Regional"}
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

// fakePIP returns a unique-ish fake public IP in the RFC 5737 documentation
// range (192.0.2.0/24). Using the first byte of a random UUID for the last
// octet gives 253 distinct values, which is sufficient for local testing.
func fakePIP() string {
	u := uuid.New()
	return fmt.Sprintf("192.0.2.%d", int(u[0])%253+1)
}

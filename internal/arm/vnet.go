package arm

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/zerodeth/azemu/internal/store"
)

// vnetTypeString is the canonical ARM type returned in response bodies.
const vnetTypeString = "Microsoft.Network/virtualNetworks"

// vnetID builds the canonical (camelCase) ARM id for a virtual network.
// Response bodies use this casing even though the chi route literals are
// lowercase — this mirrors the existing resourceGroup convention at
// router.go:119 so all new code stays consistent.
func vnetID(subID, rgName, vnetName string) string {
	return fmt.Sprintf(
		"/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/virtualNetworks/%s",
		subID, rgName, vnetName,
	)
}

// vnetBody is the subset of the azurerm_virtual_network PUT payload that
// azemu understands. Unknown fields on "properties" are passed through to
// the store so GET responses echo them back to the client.
type vnetBody struct {
	Location   string                 `json:"location"`
	Tags       map[string]string      `json:"tags"`
	Properties map[string]interface{} `json:"properties"`
}

// validateAddressPrefixes checks that every CIDR in the slice is parseable
// and that no two prefixes overlap. Returns a non-nil error on the first
// violation found.
func validateAddressPrefixes(prefixes []string) error {
	nets := make([]*net.IPNet, 0, len(prefixes))
	for _, p := range prefixes {
		_, ipNet, err := net.ParseCIDR(p)
		if err != nil {
			return fmt.Errorf("invalid address prefix %q: not a valid CIDR block", p)
		}
		nets = append(nets, ipNet)
	}
	for i := 0; i < len(nets); i++ {
		for j := i + 1; j < len(nets); j++ {
			if nets[i].Contains(nets[j].IP) || nets[j].Contains(nets[i].IP) {
				return fmt.Errorf("address prefixes %q and %q overlap", prefixes[i], prefixes[j])
			}
		}
	}
	return nil
}

func (a *Router) putVNet(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	name := chi.URLParam(r, "vnetName")

	var body vnetBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeAzureError(w, http.StatusBadRequest, "InvalidRequestContent", err.Error())
		return
	}
	if strings.TrimSpace(body.Location) == "" {
		writeAzureError(w, http.StatusBadRequest, "InvalidRequestContent",
			"location is required")
		return
	}

	// Validate addressSpace.addressPrefixes when the caller supplies them.
	if body.Properties != nil {
		if addrSpace, ok := body.Properties["addressSpace"].(map[string]interface{}); ok {
			if rawPrefixes, ok := addrSpace["addressPrefixes"].([]interface{}); ok {
				prefixes := make([]string, 0, len(rawPrefixes))
				for _, v := range rawPrefixes {
					s, ok := v.(string)
					if !ok {
						writeAzureError(w, http.StatusBadRequest, "InvalidAddressPrefix",
							"addressPrefixes entries must be strings")
						return
					}
					prefixes = append(prefixes, s)
				}
				if err := validateAddressPrefixes(prefixes); err != nil {
					writeAzureError(w, http.StatusBadRequest, "InvalidAddressPrefix", err.Error())
					return
				}
			}
		}
	}

	// Drop any inline "subnets" the client sent — azemu v0.1 only recognises
	// subnets created via the separate subnets endpoint, as that is what the
	// azurerm_subnet Terraform resource issues. Leaving the inline array in
	// the stored properties would create a split brain between the in-line
	// data and the child store entries read by getVNet.
	if body.Properties == nil {
		body.Properties = map[string]interface{}{}
	}
	delete(body.Properties, "subnets")
	body.Properties["provisioningState"] = "Succeeded"

	id := vnetID(subID, rgName, name)
	res := &store.Resource{
		ID:         id,
		Name:       name,
		Type:       vnetTypeString,
		Location:   strings.ToLower(body.Location),
		Tags:       normaliseTags(body.Tags),
		Properties: body.Properties,
	}

	_, exists := a.store.Get(id)
	if err := a.store.Put(id, res); err != nil {
		writeAzureError(w, http.StatusInternalServerError, "InternalServerError",
			fmt.Sprintf("put virtual network %q: %s", name, err))
		return
	}

	status := http.StatusCreated
	if exists {
		status = http.StatusOK
	}
	log.Info().Str("resource_id", id).Bool("existed", exists).Msg("vnet upsert")

	// Response is built identically to getVNet so clients see the same shape
	// whether they just created or just fetched. On create the subnet list
	// is empty; on update we still re-read children.
	writeJSON(w, status, vnetResponse(res, a.store.List(id+"/subnets/")))
}

func (a *Router) getVNet(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	name := chi.URLParam(r, "vnetName")
	id := vnetID(subID, rgName, name)

	res, ok := a.store.Get(id)
	if !ok {
		writeAzureError(w, http.StatusNotFound, "ResourceNotFound",
			fmt.Sprintf("Virtual network '%s' could not be found.", name))
		return
	}
	writeJSON(w, http.StatusOK, vnetResponse(res, a.store.List(id+"/subnets/")))
}

func (a *Router) headVNet(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	name := chi.URLParam(r, "vnetName")
	id := vnetID(subID, rgName, name)

	if _, ok := a.store.Get(id); !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *Router) deleteVNet(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	name := chi.URLParam(r, "vnetName")
	id := vnetID(subID, rgName, name)

	// store.Delete cascades by key prefix (memory.go:64), so any child
	// /subnets/<name> entries are removed in the same call for free.
	if !a.store.Delete(id) {
		writeAzureError(w, http.StatusNotFound, "ResourceNotFound",
			fmt.Sprintf("Virtual network '%s' could not be found.", name))
		return
	}

	log.Info().Str("resource_id", id).Msg("vnet deleted")
	w.Header().Set("Location",
		fmt.Sprintf("/subscriptions/%s/operationresults/%s", subID, uuid.New().String()))
	w.WriteHeader(http.StatusAccepted)
}

func (a *Router) listVNetsByRG(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	prefix := fmt.Sprintf(
		"/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/virtualNetworks/",
		subID, rgName,
	)
	a.writeVNetList(w, prefix)
}

func (a *Router) listVNetsBySub(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	// Subscription-scope listing has no "resourceGroups" segment in the URL,
	// but stored ids always include one. Walk every resourceGroup under the
	// subscription and filter by vnet type.
	prefix := fmt.Sprintf("/subscriptions/%s/resourceGroups/", subID)
	a.writeVNetList(w, prefix)
}

// writeVNetList scans store entries by prefix, keeps only vnet resources
// (filtering out subnets and any other child types that share the prefix),
// and writes the {"value":[...]} wrapper. A nil slice must become an empty
// array in JSON so Terraform's list-is-empty checks succeed.
func (a *Router) writeVNetList(w http.ResponseWriter, prefix string) {
	resources := a.store.List(prefix)
	items := []map[string]interface{}{}
	for _, res := range resources {
		if res.Type != vnetTypeString {
			continue
		}
		items = append(items, vnetResponse(res, a.store.List(res.ID+"/subnets/")))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"value": items})
}

// vnetResponse builds the canonical ARM response body for a virtual network,
// embedding any child subnets that live in the store under this vnet's id.
// Passing an empty slice for children produces `"subnets": []` (not null),
// which is what the azurerm provider expects.
func vnetResponse(v *store.Resource, children []*store.Resource) map[string]interface{} {
	props := map[string]interface{}{
		"provisioningState": "Succeeded",
	}
	// Passthrough any caller-supplied properties (e.g. addressSpace,
	// dhcpOptions) but keep provisioningState authoritative.
	for k, val := range v.Properties {
		if k == "provisioningState" || k == "subnets" {
			continue
		}
		props[k] = val
	}
	subnets := []map[string]interface{}{}
	for _, c := range children {
		if c.Type != subnetTypeString {
			continue
		}
		subnets = append(subnets, subnetEmbedded(c))
	}
	props["subnets"] = subnets

	return map[string]interface{}{
		"id":         v.ID,
		"name":       v.Name,
		"type":       v.Type,
		"location":   v.Location,
		"tags":       v.Tags,
		"properties": props,
	}
}

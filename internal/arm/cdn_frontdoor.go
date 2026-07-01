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

// Azure Front Door (Standard/Premium) shares the Microsoft.Cdn/profiles root
// with classic CDN; the profile handler in cdn.go already accepts any SKU
// (Standard_AzureFrontDoor, Premium_AzureFrontDoor), so only the AFD-specific
// child graph lives here: afdEndpoints, originGroups, origins, and routes. SKU
// is a no-op in emulation; what matters is the deterministic *.azurefd.net host
// the endpoint advertises and the route -> origin-group -> origin chain the
// data plane (cdn_frontdoor_dataplane.go) walks to find the Blob origin.
//
// The azurerm provider pins the child types to the track1 cdn/2021-06-01 SDK,
// whose long-running-operation future is satisfied by a terminal 200/201 on
// create with no async header (azemu writes synchronously). DELETE reuses the
// shared acceptAsyncDelete 202 path. azemu's RequireAPIVersion middleware is
// value-agnostic, so the 2021-06-01 vs 2024-02-01 split does not matter here.
const (
	afdEndpointTypeString    = "Microsoft.Cdn/profiles/afdEndpoints"
	afdOriginGroupTypeString = "Microsoft.Cdn/profiles/originGroups"
	afdOriginTypeString      = "Microsoft.Cdn/profiles/originGroups/origins"
	afdRouteTypeString       = "Microsoft.Cdn/profiles/afdEndpoints/routes"
)

// afdHostSuffix is the Front Door endpoint host suffix. afdEndpointID and the
// host-name generator below keep it in one place so the control plane and the
// data plane agree on the host the endpoint advertises.
const afdHostSuffix = ".azurefd.net"

func afdEndpointID(subID, rgName, profileName, endpointName string) string {
	return fmt.Sprintf(
		"/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Cdn/profiles/%s/afdEndpoints/%s",
		subID, rgName, profileName, endpointName,
	)
}

func afdOriginGroupID(subID, rgName, profileName, groupName string) string {
	return fmt.Sprintf(
		"/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Cdn/profiles/%s/originGroups/%s",
		subID, rgName, profileName, groupName,
	)
}

func afdOriginID(subID, rgName, profileName, groupName, originName string) string {
	return fmt.Sprintf(
		"/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Cdn/profiles/%s/originGroups/%s/origins/%s",
		subID, rgName, profileName, groupName, originName,
	)
}

func afdRouteID(subID, rgName, profileName, endpointName, routeName string) string {
	return fmt.Sprintf(
		"/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Cdn/profiles/%s/afdEndpoints/%s/routes/%s",
		subID, rgName, profileName, endpointName, routeName,
	)
}

// afdGeneratedHostName returns the deterministic host the data plane muxes on.
// Real Azure mints "{name}-{hash}.z01.azurefd.net"; azemu uses the stable
// "{name}.azurefd.net" so the same value the provider reads from the response
// (azurerm_cdn_frontdoor_endpoint.X.host_name) is what the host-mux resolves.
// DNS hostnames are case-insensitive, and the data plane lowercases the
// incoming Host header before matching, so the generated name must already be
// lowercase or an endpoint with an uppercase name would never resolve.
func afdGeneratedHostName(endpointName string) string {
	return strings.ToLower(endpointName) + afdHostSuffix
}

// afdChildBody is the PUT payload shared by every AFD child resource. Only the
// afdEndpoint carries a location ("global"); originGroups, origins, and routes
// have no location field. Properties are stored verbatim so the provider's
// round-trip reads (originGroup.id, linkToDefaultDomain, healthProbeSettings,
// ...) are satisfied with the exact JSON the provider sent.
type afdChildBody struct {
	Location   string                 `json:"location"`
	Tags       map[string]string      `json:"tags"`
	Properties map[string]interface{} `json:"properties"`
}

// --- afdEndpoints (Microsoft.Cdn/profiles/afdEndpoints) ---

func (a *Router) putAFDEndpoint(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	profileName := chi.URLParam(r, "profileName")
	endpointName := chi.URLParam(r, "endpointName")

	if _, ok := a.store.Get(cdnProfileID(subID, rgName, profileName)); !ok {
		writeAzureError(w, http.StatusNotFound, "ParentResourceNotFound",
			fmt.Sprintf("The parent resource 'Microsoft.Cdn/profiles/%s' was not found.", profileName))
		return
	}

	body, ok := decodeAFDChildBody(w, r)
	if !ok {
		return
	}
	body.Properties["provisioningState"] = "Succeeded"
	// hostName is a read-only property the provider copies into host_name. Azure
	// generates it; azemu computes the stable *.azurefd.net form the data plane
	// resolves.
	body.Properties["hostName"] = afdGeneratedHostName(endpointName)

	location := strings.ToLower(body.Location)
	if location == "" {
		location = "global"
	}

	id := afdEndpointID(subID, rgName, profileName, endpointName)
	a.upsertAFDChild(w, id, endpointName, afdEndpointTypeString, location, body, "AFD endpoint")
}

func (a *Router) getAFDEndpoint(w http.ResponseWriter, r *http.Request) {
	id := afdEndpointID(chi.URLParam(r, "subscriptionID"), chi.URLParam(r, "resourceGroupName"),
		chi.URLParam(r, "profileName"), chi.URLParam(r, "endpointName"))
	a.getAFDChild(w, id, "afdEndpoints", chi.URLParam(r, "endpointName"))
}

func (a *Router) headAFDEndpoint(w http.ResponseWriter, r *http.Request) {
	id := afdEndpointID(chi.URLParam(r, "subscriptionID"), chi.URLParam(r, "resourceGroupName"),
		chi.URLParam(r, "profileName"), chi.URLParam(r, "endpointName"))
	a.headAFDChild(w, id)
}

func (a *Router) deleteAFDEndpoint(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	id := afdEndpointID(subID, chi.URLParam(r, "resourceGroupName"),
		chi.URLParam(r, "profileName"), chi.URLParam(r, "endpointName"))
	a.deleteAFDChild(w, r, id, subID, "afdEndpoints", chi.URLParam(r, "endpointName"))
}

func (a *Router) listAFDEndpoints(w http.ResponseWriter, r *http.Request) {
	prefix := afdEndpointID(chi.URLParam(r, "subscriptionID"), chi.URLParam(r, "resourceGroupName"),
		chi.URLParam(r, "profileName"), "")
	a.writeAFDChildList(w, prefix, afdEndpointTypeString)
}

// --- originGroups (Microsoft.Cdn/profiles/originGroups) ---

func (a *Router) putAFDOriginGroup(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	profileName := chi.URLParam(r, "profileName")
	groupName := chi.URLParam(r, "originGroupName")

	if _, ok := a.store.Get(cdnProfileID(subID, rgName, profileName)); !ok {
		writeAzureError(w, http.StatusNotFound, "ParentResourceNotFound",
			fmt.Sprintf("The parent resource 'Microsoft.Cdn/profiles/%s' was not found.", profileName))
		return
	}

	body, ok := decodeAFDChildBody(w, r)
	if !ok {
		return
	}
	body.Properties["provisioningState"] = "Succeeded"

	id := afdOriginGroupID(subID, rgName, profileName, groupName)
	a.upsertAFDChild(w, id, groupName, afdOriginGroupTypeString, "", body, "AFD origin group")
}

func (a *Router) getAFDOriginGroup(w http.ResponseWriter, r *http.Request) {
	id := afdOriginGroupID(chi.URLParam(r, "subscriptionID"), chi.URLParam(r, "resourceGroupName"),
		chi.URLParam(r, "profileName"), chi.URLParam(r, "originGroupName"))
	a.getAFDChild(w, id, "originGroups", chi.URLParam(r, "originGroupName"))
}

func (a *Router) headAFDOriginGroup(w http.ResponseWriter, r *http.Request) {
	id := afdOriginGroupID(chi.URLParam(r, "subscriptionID"), chi.URLParam(r, "resourceGroupName"),
		chi.URLParam(r, "profileName"), chi.URLParam(r, "originGroupName"))
	a.headAFDChild(w, id)
}

func (a *Router) deleteAFDOriginGroup(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	id := afdOriginGroupID(subID, chi.URLParam(r, "resourceGroupName"),
		chi.URLParam(r, "profileName"), chi.URLParam(r, "originGroupName"))
	a.deleteAFDChild(w, r, id, subID, "originGroups", chi.URLParam(r, "originGroupName"))
}

func (a *Router) listAFDOriginGroups(w http.ResponseWriter, r *http.Request) {
	prefix := afdOriginGroupID(chi.URLParam(r, "subscriptionID"), chi.URLParam(r, "resourceGroupName"),
		chi.URLParam(r, "profileName"), "")
	a.writeAFDChildList(w, prefix, afdOriginGroupTypeString)
}

// --- origins (Microsoft.Cdn/profiles/originGroups/origins) ---

func (a *Router) putAFDOrigin(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	profileName := chi.URLParam(r, "profileName")
	groupName := chi.URLParam(r, "originGroupName")
	originName := chi.URLParam(r, "originName")

	if _, ok := a.store.Get(afdOriginGroupID(subID, rgName, profileName, groupName)); !ok {
		writeAzureError(w, http.StatusNotFound, "ParentResourceNotFound",
			fmt.Sprintf("The parent resource 'Microsoft.Cdn/profiles/%s/originGroups/%s' was not found.", profileName, groupName))
		return
	}

	body, ok := decodeAFDChildBody(w, r)
	if !ok {
		return
	}
	body.Properties["provisioningState"] = "Succeeded"

	id := afdOriginID(subID, rgName, profileName, groupName, originName)
	a.upsertAFDChild(w, id, originName, afdOriginTypeString, "", body, "AFD origin")
}

func (a *Router) getAFDOrigin(w http.ResponseWriter, r *http.Request) {
	id := afdOriginID(chi.URLParam(r, "subscriptionID"), chi.URLParam(r, "resourceGroupName"),
		chi.URLParam(r, "profileName"), chi.URLParam(r, "originGroupName"), chi.URLParam(r, "originName"))
	a.getAFDChild(w, id, "origins", chi.URLParam(r, "originName"))
}

func (a *Router) headAFDOrigin(w http.ResponseWriter, r *http.Request) {
	id := afdOriginID(chi.URLParam(r, "subscriptionID"), chi.URLParam(r, "resourceGroupName"),
		chi.URLParam(r, "profileName"), chi.URLParam(r, "originGroupName"), chi.URLParam(r, "originName"))
	a.headAFDChild(w, id)
}

func (a *Router) deleteAFDOrigin(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	id := afdOriginID(subID, chi.URLParam(r, "resourceGroupName"), chi.URLParam(r, "profileName"),
		chi.URLParam(r, "originGroupName"), chi.URLParam(r, "originName"))
	a.deleteAFDChild(w, r, id, subID, "origins", chi.URLParam(r, "originName"))
}

func (a *Router) listAFDOrigins(w http.ResponseWriter, r *http.Request) {
	prefix := afdOriginID(chi.URLParam(r, "subscriptionID"), chi.URLParam(r, "resourceGroupName"),
		chi.URLParam(r, "profileName"), chi.URLParam(r, "originGroupName"), "")
	a.writeAFDChildList(w, prefix, afdOriginTypeString)
}

// --- routes (Microsoft.Cdn/profiles/afdEndpoints/routes) ---

func (a *Router) putAFDRoute(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	profileName := chi.URLParam(r, "profileName")
	endpointName := chi.URLParam(r, "endpointName")
	routeName := chi.URLParam(r, "routeName")

	if _, ok := a.store.Get(afdEndpointID(subID, rgName, profileName, endpointName)); !ok {
		writeAzureError(w, http.StatusNotFound, "ParentResourceNotFound",
			fmt.Sprintf("The parent resource 'Microsoft.Cdn/profiles/%s/afdEndpoints/%s' was not found.", profileName, endpointName))
		return
	}

	body, ok := decodeAFDChildBody(w, r)
	if !ok {
		return
	}
	body.Properties["provisioningState"] = "Succeeded"

	id := afdRouteID(subID, rgName, profileName, endpointName, routeName)
	a.upsertAFDChild(w, id, routeName, afdRouteTypeString, "", body, "AFD route")
}

func (a *Router) getAFDRoute(w http.ResponseWriter, r *http.Request) {
	id := afdRouteID(chi.URLParam(r, "subscriptionID"), chi.URLParam(r, "resourceGroupName"),
		chi.URLParam(r, "profileName"), chi.URLParam(r, "endpointName"), chi.URLParam(r, "routeName"))
	a.getAFDChild(w, id, "routes", chi.URLParam(r, "routeName"))
}

func (a *Router) headAFDRoute(w http.ResponseWriter, r *http.Request) {
	id := afdRouteID(chi.URLParam(r, "subscriptionID"), chi.URLParam(r, "resourceGroupName"),
		chi.URLParam(r, "profileName"), chi.URLParam(r, "endpointName"), chi.URLParam(r, "routeName"))
	a.headAFDChild(w, id)
}

func (a *Router) deleteAFDRoute(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	id := afdRouteID(subID, chi.URLParam(r, "resourceGroupName"), chi.URLParam(r, "profileName"),
		chi.URLParam(r, "endpointName"), chi.URLParam(r, "routeName"))
	a.deleteAFDChild(w, r, id, subID, "routes", chi.URLParam(r, "routeName"))
}

func (a *Router) listAFDRoutes(w http.ResponseWriter, r *http.Request) {
	prefix := afdRouteID(chi.URLParam(r, "subscriptionID"), chi.URLParam(r, "resourceGroupName"),
		chi.URLParam(r, "profileName"), chi.URLParam(r, "endpointName"), "")
	a.writeAFDChildList(w, prefix, afdRouteTypeString)
}

// --- shared AFD child helpers ---

func decodeAFDChildBody(w http.ResponseWriter, r *http.Request) (afdChildBody, bool) {
	var body afdChildBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeAzureError(w, http.StatusBadRequest, "InvalidRequestContent", err.Error())
		return body, false
	}
	if body.Properties == nil {
		body.Properties = map[string]interface{}{}
	}
	return body, true
}

// upsertAFDChild stores an AFD child resource and writes the ARM envelope,
// returning 201 on first write and 200 on update, mirroring putCDNEndpoint.
func (a *Router) upsertAFDChild(w http.ResponseWriter, id, name, typeStr, location string, body afdChildBody, logLabel string) {
	res := &store.Resource{
		ID:         id,
		Name:       name,
		Type:       typeStr,
		Location:   location,
		Tags:       normaliseTags(body.Tags),
		Properties: body.Properties,
	}

	_, exists := a.store.Get(id)
	if err := a.store.Put(id, res); err != nil {
		writeAzureError(w, http.StatusInternalServerError, "InternalServerError",
			fmt.Sprintf("put %s %q: %s", logLabel, name, err))
		return
	}

	status := http.StatusCreated
	if exists {
		status = http.StatusOK
	}
	log.Info().Str("resource_id", id).Bool("existed", exists).Str("kind", logLabel).Msg("AFD child upsert")
	writeJSON(w, status, afdChildResponse(res))
}

func (a *Router) getAFDChild(w http.ResponseWriter, id, segment, name string) {
	res, ok := a.store.Get(id)
	if !ok {
		writeAzureError(w, http.StatusNotFound, "ResourceNotFound",
			fmt.Sprintf("The Resource 'Microsoft.Cdn/profiles/.../%s/%s' was not found.", segment, name))
		return
	}
	writeJSON(w, http.StatusOK, afdChildResponse(res))
}

func (a *Router) headAFDChild(w http.ResponseWriter, id string) {
	if _, ok := a.store.Get(id); !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *Router) deleteAFDChild(w http.ResponseWriter, r *http.Request, id, subID, segment, name string) {
	if !a.store.Delete(id) {
		writeAzureError(w, http.StatusNotFound, "ResourceNotFound",
			fmt.Sprintf("The Resource 'Microsoft.Cdn/profiles/.../%s/%s' was not found.", segment, name))
		return
	}
	log.Info().Str("resource_id", id).Str("kind", segment).Msg("AFD child deleted")
	a.acceptAsyncDelete(w, r, subID)
}

func (a *Router) writeAFDChildList(w http.ResponseWriter, prefix, typeStr string) {
	items := []map[string]interface{}{}
	for _, res := range a.store.List(prefix) {
		if res.Type != typeStr {
			continue
		}
		items = append(items, afdChildResponse(res))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"value": items})
}

// afdChildResponse builds the ARM envelope for an AFD child resource. The
// stored properties are echoed verbatim (with a terminal provisioningState) so
// the provider's reads of originGroup.id, hostName, linkToDefaultDomain, the
// health-probe block, and the rest round-trip exactly as sent. location is
// emitted only when set (the afdEndpoint is "global"; the other three child
// types have no location). tags is always emitted: upsertAFDChild normalises a
// nil Tags map to an empty one, so res.Tags is never nil.
func afdChildResponse(res *store.Resource) map[string]interface{} {
	props := map[string]interface{}{"provisioningState": "Succeeded"}
	for k, val := range res.Properties {
		if k == "provisioningState" {
			continue
		}
		props[k] = val
	}
	out := map[string]interface{}{
		"id":         res.ID,
		"name":       res.Name,
		"type":       res.Type,
		"tags":       res.Tags,
		"properties": props,
	}
	if res.Location != "" {
		out["location"] = res.Location
	}
	return out
}

package arm

import (
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/zerodeth/azemu/internal/store"
)

// The Front Door content data plane answers requests to a generated
// "{endpoint}.azurefd.net" host. Unlike classic CDN, where the endpoint carries
// its origin inline, Front Door spreads the origin across a resource graph:
// afdEndpoint -> route -> originGroup -> origin. ServeAFDContent walks that
// chain to find the Blob origin, then reuses the shared blob proxy. The two
// data planes coexist (the host-mux in serve.go dispatches by suffix), so users
// pinned to classic CDN (azurerm < 4.35, *.azureedge.net) keep working while
// Front Door scenarios (azurerm >= 4.35, *.azurefd.net) run side by side.

// afdEndpointNameFromHost returns the endpoint name encoded in a
// "{endpoint}.azurefd.net" host, mirroring the hostName azemu generates on
// afdEndpoint create. Returns false for any other host shape (an ARM host,
// plain localhost, a classic *.azureedge.net host, or a multi-label name).
func afdEndpointNameFromHost(host string) (string, bool) {
	hostname := host
	if h, _, err := net.SplitHostPort(host); err == nil {
		hostname = h
	}
	// DNS hostnames are case-insensitive; normalise before the suffix check.
	hostname = strings.ToLower(hostname)
	if !strings.HasSuffix(hostname, afdHostSuffix) {
		return "", false
	}
	name := strings.TrimSuffix(hostname, afdHostSuffix)
	if name == "" || strings.Contains(name, ".") {
		return "", false
	}
	return name, true
}

// IsAFDContentHost reports whether a request Host targets the Front Door content
// data plane. The serve.go host-mux uses it to route between the AFD proxy and
// the ARM control plane, alongside the classic IsCDNContentHost check.
func IsAFDContentHost(host string) bool {
	_, ok := afdEndpointNameFromHost(host)
	return ok
}

// ServeAFDContent reverse-proxies a Front Door endpoint content request to its
// Blob origin (Azurite) and streams the response back. It resolves the origin
// by walking afdEndpoint -> route -> originGroup -> origin, then hands off to
// the shared blob proxy so the origin's Content-Type and Cache-Control reach
// the client unchanged (essential for OTA: the multipart manifest must keep its
// boundary and short TTL). Only GET and HEAD are served, as on a real endpoint.
func (a *Router) ServeAFDContent(w http.ResponseWriter, r *http.Request) {
	name, ok := afdEndpointNameFromHost(r.Host)
	if !ok {
		writeAzureError(w, http.StatusNotFound, "ResourceNotFound",
			"Unrecognised Front Door endpoint host.")
		return
	}

	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		writeAzureError(w, http.StatusMethodNotAllowed, "MethodNotAllowed",
			"Front Door endpoints serve GET and HEAD only.")
		return
	}

	endpoint, ok := a.findAFDEndpoint(name)
	if !ok {
		writeAzureError(w, http.StatusNotFound, "ResourceNotFound",
			fmt.Sprintf("Front Door endpoint %q was not found.", name))
		return
	}

	account, ok := a.resolveAFDOriginAccount(endpoint)
	if !ok {
		writeAzureError(w, http.StatusBadGateway, "OriginUnresolved",
			"Front Door endpoint does not resolve to a recognised Azure Blob origin.")
		return
	}

	a.proxyBlobObject(w, r, account, name)
}

// findAFDEndpoint returns the stored afdEndpoint resource whose name matches,
// scanning the store for the afdEndpoint type. Endpoint names are unique enough
// within a local emulator, the same assumption the classic CDN and Key Vault
// host resolvers make.
func (a *Router) findAFDEndpoint(name string) (*store.Resource, bool) {
	for _, res := range a.store.List("/subscriptions/") {
		if res.Type == afdEndpointTypeString && res.Name == name {
			return res, true
		}
	}
	return nil, false
}

// resolveAFDOriginAccount walks the Front Door resource graph from an endpoint
// to its backing Blob storage account: it finds a route under the endpoint,
// follows the route's originGroup reference, picks the preferred origin in that
// group, and parses the storage account from the origin's hostName. ARM IDs are
// compared case-insensitively because the originGroup reference the provider
// writes into the route may differ in casing from azemu's stored key.
func (a *Router) resolveAFDOriginAccount(endpoint *store.Resource) (string, bool) {
	originGroupID, ok := a.routeOriginGroupID(endpoint.ID)
	if !ok {
		return "", false
	}
	origin, ok := a.preferredOrigin(originGroupID)
	if !ok {
		return "", false
	}
	host, _ := origin.Properties["hostName"].(string)
	return blobAccountFromHost(host)
}

// routeOriginGroupID returns the originGroup ARM ID referenced by the first
// route under the given afdEndpoint. A minimal Front Door config has a single
// route with link_to_default_domain enabled; if a scenario ever adds multiple
// routes, the first one with an origin-group reference wins (pattern-priority
// selection is out of scope until a scenario needs it).
func (a *Router) routeOriginGroupID(endpointID string) (string, bool) {
	for _, res := range a.store.List(endpointID + "/") {
		if res.Type != afdRouteTypeString {
			continue
		}
		og, ok := res.Properties["originGroup"].(map[string]interface{})
		if !ok {
			continue
		}
		if id, ok := og["id"].(string); ok && id != "" {
			return id, true
		}
	}
	return "", false
}

// preferredOrigin returns the highest-precedence origin in the referenced
// origin group: lowest priority value first, then highest weight. The group is
// matched case-insensitively against stored origin IDs. In the emulated single-
// origin case this just returns that origin; the ordering matters only when a
// group lists several.
func (a *Router) preferredOrigin(originGroupID string) (*store.Resource, bool) {
	wantPrefix := strings.ToLower(originGroupID) + "/origins/"
	var best *store.Resource
	var bestPriority, bestWeight float64
	for _, res := range a.store.List("/subscriptions/") {
		if res.Type != afdOriginTypeString {
			continue
		}
		if !strings.HasPrefix(strings.ToLower(res.ID), wantPrefix) {
			continue
		}
		priority := floatProp(res.Properties, "priority", 1)
		weight := floatProp(res.Properties, "weight", 1000)
		if best == nil || priority < bestPriority || (priority == bestPriority && weight > bestWeight) {
			best, bestPriority, bestWeight = res, priority, weight
		}
	}
	return best, best != nil
}

// floatProp reads a numeric property as float64, falling back to def when the
// key is absent or not a JSON number.
func floatProp(props map[string]interface{}, key string, def float64) float64 {
	if v, ok := props[key].(float64); ok {
		return v
	}
	return def
}

// blobAccountFromHost extracts the storage account label from an Azure Blob
// origin host ("{account}.blob.core.windows.net"). azemu serves blobs path-
// style from Azurite, so only the account label is needed to build the origin
// URL. Mirrors cdnOriginAccount's parse for the classic CDN data plane.
func blobAccountFromHost(host string) (string, bool) {
	if host == "" {
		return "", false
	}
	labels := strings.Split(host, ".")
	if len(labels) >= 2 && labels[0] != "" && labels[1] == "blob" {
		return labels[0], true
	}
	return "", false
}

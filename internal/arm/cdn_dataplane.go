package arm

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/zerodeth/azemu/internal/store"
)

// cdnHostSuffix is the classic Azure CDN endpoint host suffix. A request whose
// Host is "{endpoint}.azureedge.net" targets the CDN content data plane (the
// read path) rather than the ARM control plane.
const cdnHostSuffix = ".azureedge.net"

// cdnOriginClient fetches blobs from the Azurite origin. It carries an overall
// timeout so a slow or wedged origin cannot pin the handler goroutine, and it
// returns 3xx responses as-is (CheckRedirect) so the edge proxies redirects
// rather than following them.
var cdnOriginClient = &http.Client{
	Timeout: 30 * time.Second,
	CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

// cdnEndpointNameFromHost returns the endpoint name encoded in a
// "{endpoint}.azureedge.net" host, mirroring the hostName the azurerm provider
// computes for azurerm_cdn_endpoint. Returns false for any other host shape
// (plain localhost, an ARM host, a multi-label name).
func cdnEndpointNameFromHost(host string) (string, bool) {
	hostname := host
	if h, _, err := net.SplitHostPort(host); err == nil {
		hostname = h
	}
	// DNS hostnames are case-insensitive; normalise before the suffix check.
	hostname = strings.ToLower(hostname)
	if !strings.HasSuffix(hostname, cdnHostSuffix) {
		return "", false
	}
	name := strings.TrimSuffix(hostname, cdnHostSuffix)
	if name == "" || strings.Contains(name, ".") {
		return "", false
	}
	return name, true
}

// IsCDNContentHost reports whether a request Host targets the CDN content data
// plane. The serve.go host-mux uses it to route between the CDN proxy and the
// ARM control plane, mirroring how Key Vault data-plane hosts
// ({vault}.vault.localhost) are distinguished from the ARM host.
func IsCDNContentHost(host string) bool {
	_, ok := cdnEndpointNameFromHost(host)
	return ok
}

// ServeCDNContent reverse-proxies a CDN endpoint content request to its Blob
// origin (Azurite) and streams the response back, passing the origin's
// Content-Type and Cache-Control through unchanged. That mirrors Azure CDN's
// default behaviour: the edge honours the origin's content metadata and caching
// headers rather than synthesising its own. The handler is generic on purpose
// -- it serves any blob path and is unaware of what the bytes mean -- so every
// scenario that fronts Blob storage with a CDN (static-site, ota-delivery, ...)
// reuses it. Only GET and HEAD are served, as on a real CDN endpoint.
func (a *Router) ServeCDNContent(w http.ResponseWriter, r *http.Request) {
	name, ok := cdnEndpointNameFromHost(r.Host)
	if !ok {
		writeAzureError(w, http.StatusNotFound, "ResourceNotFound",
			"Unrecognised CDN endpoint host.")
		return
	}

	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		writeAzureError(w, http.StatusMethodNotAllowed, "MethodNotAllowed",
			"CDN endpoints serve GET and HEAD only.")
		return
	}

	endpoint, ok := a.findCDNEndpoint(name)
	if !ok {
		writeAzureError(w, http.StatusNotFound, "ResourceNotFound",
			fmt.Sprintf("CDN endpoint %q was not found.", name))
		return
	}

	account, ok := cdnOriginAccount(endpoint)
	if !ok {
		writeAzureError(w, http.StatusBadGateway, "OriginUnresolved",
			"CDN endpoint origin is not a recognised Azure Blob host.")
		return
	}

	a.proxyBlobObject(w, r, account, name)
}

// proxyBlobObject reverse-proxies a content request to a Blob account on the
// Azurite origin and streams the response back, passing the origin's
// Content-Type, Cache-Control, and the rest of the content/caching headers
// through unchanged. It is shared by the classic CDN (*.azureedge.net) and
// Front Door (*.azurefd.net) data planes: both resolve an endpoint to a Blob
// account and then proxy identically. logLabel identifies the resolved
// endpoint in the structured log. The caller is responsible for restricting
// the method to GET/HEAD before calling.
func (a *Router) proxyBlobObject(w http.ResponseWriter, r *http.Request, account, logLabel string) {
	// Use the escaped path so encoded blob keys (%20, %23, %2F, ...) reach the
	// origin intact rather than being decoded by net/http.
	originURL := a.blobOriginURL(account, r.URL.EscapedPath(), r.URL.RawQuery)
	proxyReq, err := http.NewRequestWithContext(r.Context(), r.Method, originURL, nil)
	if err != nil {
		writeAzureError(w, http.StatusInternalServerError, "InternalServerError",
			fmt.Sprintf("build CDN origin request: %s", err))
		return
	}

	resp, err := cdnOriginClient.Do(proxyReq)
	if err != nil {
		log.Error().Err(err).Str("endpoint", logLabel).Str("origin", originURL).
			Msg("cdn origin fetch failed")
		writeAzureError(w, http.StatusBadGateway, "OriginUnreachable",
			fmt.Sprintf("could not reach CDN origin: %s", err))
		return
	}
	defer resp.Body.Close()

	// Forward the content and caching headers a CDN client cares about. Azure
	// CDN honours the origin's values by default; azemu forwards them unchanged.
	for _, h := range []string{
		"Content-Type", "Cache-Control", "Content-Encoding",
		"ETag", "Last-Modified", "Content-Length",
	} {
		if v := resp.Header.Get(h); v != "" {
			w.Header().Set(h, v)
		}
	}
	// Mirror Azure's X-Cache header so a client can tell the request traversed
	// the (emulated) edge rather than hitting the origin directly.
	w.Header().Set("X-Cache", "CONFIG_NOCACHE")

	log.Info().Str("endpoint", logLabel).Str("account", account).
		Str("path", r.URL.Path).Int("status", resp.StatusCode).
		Msg("cdn content served")

	w.WriteHeader(resp.StatusCode)
	if r.Method == http.MethodGet {
		_, _ = io.Copy(w, resp.Body)
	}
}

// findCDNEndpoint returns the stored CDN endpoint resource whose name matches,
// scanning the store for the CDN-endpoint type. Endpoint names are unique
// enough within a local emulator, the same assumption the Key Vault host
// resolver makes for vault names.
func (a *Router) findCDNEndpoint(name string) (*store.Resource, bool) {
	for _, res := range a.store.List("/subscriptions/") {
		if res.Type == cdnEndpointTypeString && res.Name == name {
			return res, true
		}
	}
	return nil, false
}

// cdnOriginAccount extracts the backing storage account name from a CDN
// endpoint's origin host. The azurerm provider stores it as
// "{account}.blob.core.windows.net" (the real Azure Blob origin shape); azemu
// serves blobs path-style from Azurite, so only the account label is needed.
func cdnOriginAccount(endpoint *store.Resource) (string, bool) {
	host := cdnOriginHost(endpoint)
	if host == "" {
		return "", false
	}
	labels := strings.Split(host, ".")
	if len(labels) >= 2 && labels[0] != "" && labels[1] == "blob" {
		return labels[0], true
	}
	return "", false
}

// cdnOriginHost returns the origin host for an endpoint, preferring an explicit
// originHostHeader and falling back to the first origin's hostName.
func cdnOriginHost(endpoint *store.Resource) string {
	if h, ok := endpoint.Properties["originHostHeader"].(string); ok && h != "" {
		return h
	}
	origins, ok := endpoint.Properties["origins"].([]interface{})
	if !ok {
		return ""
	}
	for _, o := range origins {
		om, ok := o.(map[string]interface{})
		if !ok {
			continue
		}
		props, ok := om["properties"].(map[string]interface{})
		if !ok {
			continue
		}
		if h, ok := props["hostName"].(string); ok && h != "" {
			return h
		}
	}
	return ""
}

// blobOriginURL builds the path-style Azurite URL for a blob in the given
// account, matching the blob primaryEndpoint that storage accounts advertise
// ({base}/{account}/{container}/{blob}). reqPath already carries its leading
// slash and the container as its first segment.
func (a *Router) blobOriginURL(account, reqPath, rawQuery string) string {
	u := fmt.Sprintf("%s/%s%s", blobServiceBase(a.azuriteEndpoint), account, reqPath)
	if rawQuery != "" {
		u += "?" + rawQuery
	}
	return u
}

// blobServiceBase normalises an Azurite endpoint to its blob service base URL.
// An explicit port is respected (so a test can point at an httptest origin);
// otherwise the conventional blob port 10000 is appended, matching
// storagePrimaryEndpoints.
func blobServiceBase(azuriteEndpoint string) string {
	u, err := url.Parse(azuriteEndpoint)
	if err != nil || u.Host == "" {
		return strings.TrimRight(azuriteEndpoint, "/")
	}
	if u.Port() == "" {
		return fmt.Sprintf("%s://%s:10000", u.Scheme, u.Hostname())
	}
	return fmt.Sprintf("%s://%s", u.Scheme, u.Host)
}

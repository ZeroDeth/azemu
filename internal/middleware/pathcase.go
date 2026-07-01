package middleware

import (
	"net/http"
	"regexp"
	"strings"
)

// canonicalLiteralSegments is the set of known ARM URL literal segments
// that the middleware will normalize to a canonical lowercase form before
// chi routing. The chi router in azemu registers routes using lowercase
// literals (e.g. "/resourcegroups/", "/microsoft.network/"), but real
// Azure clients (the hashicorp/azurerm Terraform provider, az CLI,
// go-azure-sdk) send Azure-canonical camelCase (e.g. "/resourceGroups/",
// "/Microsoft.Network/"). chi v5 is case-sensitive on path literals with
// no built-in normalization, so without this middleware every ARM call
// from a real client would fall through to the NotFound handler.
//
// Each entry maps a known literal segment to its lowercase form. The
// match is case-insensitive: any incoming segment that lowercases to a
// key in this map is rewritten to the corresponding value. User-supplied
// path parameters (subscription IDs, resource group names, resource
// names) are NOT in this map and pass through unchanged.
//
// Edge case: a user who names a resource exactly "resourceGroups" or
// "Microsoft.Network" will see that name normalized to its lowercase
// form. This is a self-inflicted DOS and is documented in TODO.md.
var canonicalLiteralSegments = map[string]string{
	"subscriptions":       "subscriptions",
	"resourcegroups":      "resourcegroups",
	"providers":           "providers",
	"microsoft.network":   "microsoft.network",
	"microsoft.resources": "microsoft.resources",
	"microsoft.storage":   "microsoft.storage",
	"microsoft.compute":   "microsoft.compute",
	"microsoft.keyvault":  "microsoft.keyvault",
	"microsoft.web":       "microsoft.web",
	"microsoft.dns":       "microsoft.dns",
	"virtualnetworks":     "virtualnetworks",
	"subnets":             "subnets",
	// Phase 6 resource types — each maps the camelCase ARM literal to its
	// lowercase chi route form so real clients (azurerm, az CLI) are matched.
	"networksecuritygroups": "networksecuritygroups",
	"securityrules":         "securityrules",
	"publicipaddresses":     "publicipaddresses",
	"loadbalancers":         "loadbalancers",
	"backendaddresspools":   "backendaddresspools",
	"loadbalancingrules":    "loadbalancingrules",
	"probes":                "probes",
	"applicationgateways":   "applicationgateways",
	"dnszones":              "dnszones",
	"recordsets":            "recordsets",
	// Phase 7 resource types
	"storageaccounts": "storageaccounts",
	"blobservices":    "blobservices",
	"containers":      "containers",
	"vaults":          "vaults",
	"microsoft.cdn":   "microsoft.cdn",
	"profiles":        "profiles",
	"endpoints":       "endpoints",
	// Front Door (Standard/Premium) child segments under Microsoft.Cdn/profiles.
	"afdendpoints":    "afdendpoints",
	"origingroups":    "origingroups",
	"origins":         "origins",
	"routes":          "routes",
	"microsoft.cache": "microsoft.cache",
	"redis":           "redis",
	// Phase 8 resource types
	"microsoft.managedidentity":    "microsoft.managedidentity",
	"userassignedidentities":       "userassignedidentities",
	"federatedidentitycredentials": "federatedidentitycredentials",
	"microsoft.containerservice":   "microsoft.containerservice",
	"managedclusters":              "managedclusters",
	"agentpools":                   "agentpools",
	// POST .../managedClusters/{name}/listClusterUserCredential and
	// .../listClusterAdminCredential (azurerm kube_config reads)
	"listclusterusercredential":  "listclusterusercredential",
	"listclusteradmincredential": "listclusteradmincredential",
	// ARM action segments and subscription-scoped paths
	"listkeys":      "listkeys",      // POST .../storageAccounts/{name}/listKeys or .../redis/{name}/listKeys
	"deletedvaults": "deletedvaults", // GET .../providers/Microsoft.KeyVault/locations/{loc}/deletedVaults/{name}
	"locations":     "locations",     // subscription-scoped provider path segment
	"fileservices":  "fileservices",  // GET .../storageAccounts/{name}/fileServices/default
}

// duplicateSlashes matches runs of two or more "/" characters. Some real
// Azure clients build URLs by concatenating a host like "127.0.0.1:4567"
// with a path like "/metadata/endpoints" and end up emitting
// "https://127.0.0.1:4567//metadata/endpoints" (note the leading "//").
// chi treats "//metadata/endpoints" and "/metadata/endpoints" as different
// routes, so the duplicate-slash collapse below ensures both forms reach
// the same handler.
var duplicateSlashes = regexp.MustCompile(`/+`)

// NormalizePath rewrites incoming request paths so chi can match them
// against its lowercase, single-slash route literals regardless of how
// the client cased the segments or how it composed the URL.
//
// Specifically the middleware does two transformations:
//
//  1. Collapse runs of "/" to a single "/", so "//metadata/endpoints"
//     and "/metadata///endpoints" both become "/metadata/endpoints".
//  2. For each path segment, if its lowercase form is a known ARM literal
//     (see canonicalLiteralSegments), replace the segment with the
//     canonical lowercase value. User-supplied parameter values pass
//     through untouched.
//
// The middleware mutates r.URL.Path in place before forwarding to next.
// chi.RouteContext sees the rewritten path; the request's RawPath is left
// alone so any handler that needs the original can still recover it.
func NormalizePath(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := duplicateSlashes.ReplaceAllString(r.URL.Path, "/")

		if strings.ContainsAny(path, "ABCDEFGHIJKLMNOPQRSTUVWXYZ") {
			segments := strings.Split(path, "/")
			for i, seg := range segments {
				if seg == "" {
					continue
				}
				if canonical, ok := canonicalLiteralSegments[strings.ToLower(seg)]; ok {
					segments[i] = canonical
				}
			}
			path = strings.Join(segments, "/")
		}

		r.URL.Path = path
		next.ServeHTTP(w, r)
	})
}

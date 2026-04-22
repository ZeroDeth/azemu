package arm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"github.com/zerodeth/azemu/internal/store"
)

type Router struct {
	store           store.Store
	azuriteEndpoint string // e.g. "http://azurite:10000" — blob service base URL
}

func NewRouter(s store.Store, azuriteEndpoint string) *Router {
	return &Router{store: s, azuriteEndpoint: azuriteEndpoint}
}

func (a *Router) Routes(r chi.Router) {
	// Subscription info
	r.Get("/", a.listSubscriptions)
	r.Get("/{subscriptionID}", a.getSubscription)

	// Provider registration (always succeeds)
	r.Get("/{subscriptionID}/providers", a.listProviders)
	r.Get("/{subscriptionID}/providers/{namespace}", a.getProvider)
	r.Post("/{subscriptionID}/providers/{namespace}/register", a.registerProvider)

	// Resource groups
	r.Put("/{subscriptionID}/resourcegroups/{resourceGroupName}", a.putResourceGroup)
	r.Get("/{subscriptionID}/resourcegroups/{resourceGroupName}", a.getResourceGroup)
	r.Delete("/{subscriptionID}/resourcegroups/{resourceGroupName}", a.deleteResourceGroup)
	r.Get("/{subscriptionID}/resourcegroups", a.listResourceGroups)
	r.Head("/{subscriptionID}/resourcegroups/{resourceGroupName}", a.headResourceGroup)
	// List child resources within an RG. The azurerm provider calls this
	// during `terraform destroy` to verify the RG is empty (or to enumerate
	// what would be cascade-deleted) BEFORE issuing the DELETE on the RG.
	// Returning 501 here surfaces as the cryptic provider-side error
	// "a polling status of `Failed` should be surfaced as a PollingFailedError".
	r.Get("/{subscriptionID}/resourcegroups/{resourceGroupName}/resources", a.listResourceGroupResources)

	// Virtual networks (Microsoft.Network/virtualNetworks)
	r.Put("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/virtualnetworks/{vnetName}", a.putVNet)
	r.Get("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/virtualnetworks/{vnetName}", a.getVNet)
	r.Head("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/virtualnetworks/{vnetName}", a.headVNet)
	r.Delete("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/virtualnetworks/{vnetName}", a.deleteVNet)
	r.Get("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/virtualnetworks", a.listVNetsByRG)
	r.Get("/{subscriptionID}/providers/microsoft.network/virtualnetworks", a.listVNetsBySub)

	// Subnets (Microsoft.Network/virtualNetworks/subnets)
	r.Put("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/virtualnetworks/{vnetName}/subnets/{subnetName}", a.putSubnet)
	r.Get("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/virtualnetworks/{vnetName}/subnets/{subnetName}", a.getSubnet)
	r.Head("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/virtualnetworks/{vnetName}/subnets/{subnetName}", a.headSubnet)
	r.Delete("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/virtualnetworks/{vnetName}/subnets/{subnetName}", a.deleteSubnet)
	r.Get("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/virtualnetworks/{vnetName}/subnets", a.listSubnets)

	// Network Security Groups (Microsoft.Network/networkSecurityGroups)
	r.Put("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/networksecuritygroups/{nsgName}", a.putNSG)
	r.Get("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/networksecuritygroups/{nsgName}", a.getNSG)
	r.Head("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/networksecuritygroups/{nsgName}", a.headNSG)
	r.Delete("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/networksecuritygroups/{nsgName}", a.deleteNSG)
	r.Get("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/networksecuritygroups", a.listNSGsByRG)
	r.Get("/{subscriptionID}/providers/microsoft.network/networksecuritygroups", a.listNSGsBySub)

	// Security Rules (Microsoft.Network/networkSecurityGroups/securityRules)
	r.Put("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/networksecuritygroups/{nsgName}/securityrules/{ruleName}", a.putRule)
	r.Get("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/networksecuritygroups/{nsgName}/securityrules/{ruleName}", a.getRule)
	r.Head("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/networksecuritygroups/{nsgName}/securityrules/{ruleName}", a.headRule)
	r.Delete("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/networksecuritygroups/{nsgName}/securityrules/{ruleName}", a.deleteRule)
	r.Get("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/networksecuritygroups/{nsgName}/securityrules", a.listRules)

	// Public IP addresses (Microsoft.Network/publicIPAddresses)
	r.Put("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/publicipaddresses/{publicIPName}", a.putPublicIP)
	r.Get("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/publicipaddresses/{publicIPName}", a.getPublicIP)
	r.Head("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/publicipaddresses/{publicIPName}", a.headPublicIP)
	r.Delete("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/publicipaddresses/{publicIPName}", a.deletePublicIP)
	r.Get("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/publicipaddresses", a.listPublicIPsByRG)
	r.Get("/{subscriptionID}/providers/microsoft.network/publicipaddresses", a.listPublicIPsBySub)

	// Load Balancers (Microsoft.Network/loadBalancers)
	r.Put("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/loadbalancers/{lbName}", a.putLB)
	r.Get("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/loadbalancers/{lbName}", a.getLB)
	r.Head("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/loadbalancers/{lbName}", a.headLB)
	r.Delete("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/loadbalancers/{lbName}", a.deleteLB)
	r.Get("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/loadbalancers", a.listLBsByRG)
	r.Get("/{subscriptionID}/providers/microsoft.network/loadbalancers", a.listLBsBySub)

	// Backend Address Pools (Microsoft.Network/loadBalancers/backendAddressPools)
	r.Put("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/loadbalancers/{lbName}/backendaddresspools/{poolName}", a.putLBBackendPool)
	r.Get("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/loadbalancers/{lbName}/backendaddresspools/{poolName}", a.getLBBackendPool)
	r.Head("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/loadbalancers/{lbName}/backendaddresspools/{poolName}", a.headLBBackendPool)
	r.Delete("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/loadbalancers/{lbName}/backendaddresspools/{poolName}", a.deleteLBBackendPool)
	r.Get("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/loadbalancers/{lbName}/backendaddresspools", a.listLBBackendPools)

	// Load Balancing Rules (Microsoft.Network/loadBalancers/loadBalancingRules)
	r.Put("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/loadbalancers/{lbName}/loadbalancingrules/{ruleName}", a.putLBRule)
	r.Get("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/loadbalancers/{lbName}/loadbalancingrules/{ruleName}", a.getLBRule)
	r.Head("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/loadbalancers/{lbName}/loadbalancingrules/{ruleName}", a.headLBRule)
	r.Delete("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/loadbalancers/{lbName}/loadbalancingrules/{ruleName}", a.deleteLBRule)
	r.Get("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/loadbalancers/{lbName}/loadbalancingrules", a.listLBRules)

	// Probes (Microsoft.Network/loadBalancers/probes)
	r.Put("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/loadbalancers/{lbName}/probes/{probeName}", a.putLBProbe)
	r.Get("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/loadbalancers/{lbName}/probes/{probeName}", a.getLBProbe)
	r.Head("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/loadbalancers/{lbName}/probes/{probeName}", a.headLBProbe)
	r.Delete("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/loadbalancers/{lbName}/probes/{probeName}", a.deleteLBProbe)
	r.Get("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/loadbalancers/{lbName}/probes", a.listLBProbes)

	// Application Gateways (Microsoft.Network/applicationGateways)
	r.Put("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/applicationgateways/{appGWName}", a.putAppGW)
	r.Get("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/applicationgateways/{appGWName}", a.getAppGW)
	r.Head("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/applicationgateways/{appGWName}", a.headAppGW)
	r.Delete("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/applicationgateways/{appGWName}", a.deleteAppGW)
	r.Get("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/applicationgateways", a.listAppGWsByRG)
	r.Get("/{subscriptionID}/providers/microsoft.network/applicationgateways", a.listAppGWsBySub)

	// DNS Zones (Microsoft.Network/dnsZones)
	r.Put("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/dnszones/{zoneName}", a.putDNSZone)
	r.Get("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/dnszones/{zoneName}", a.getDNSZone)
	r.Head("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/dnszones/{zoneName}", a.headDNSZone)
	r.Delete("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/dnszones/{zoneName}", a.deleteDNSZone)
	r.Get("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/dnszones", a.listDNSZonesByRG)
	r.Get("/{subscriptionID}/providers/microsoft.network/dnszones", a.listDNSZonesBySub)

	// DNS Record Sets (Microsoft.Network/dnsZones/{recordType}/{recordName})
	// The {recordType} parameter captures the lowercase record type (a, aaaa, cname, txt, mx, srv, ns, soa)
	// after NormalizePath lowercases the path. Handlers uppercase it for storage.
	r.Put("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/dnszones/{zoneName}/{recordType}/{recordName}", a.putDNSRecordSet)
	r.Get("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/dnszones/{zoneName}/{recordType}/{recordName}", a.getDNSRecordSet)
	r.Head("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/dnszones/{zoneName}/{recordType}/{recordName}", a.headDNSRecordSet)
	r.Delete("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/dnszones/{zoneName}/{recordType}/{recordName}", a.deleteDNSRecordSet)
	r.Get("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/dnszones/{zoneName}/{recordType}", a.listDNSRecordSetsByType)
	r.Get("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.network/dnszones/{zoneName}/recordsets", a.listAllDNSRecordSets)

	// Storage Accounts (Microsoft.Storage/storageAccounts)
	r.Put("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.storage/storageaccounts/{accountName}", a.putStorageAccount)
	r.Get("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.storage/storageaccounts/{accountName}", a.getStorageAccount)
	r.Head("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.storage/storageaccounts/{accountName}", a.headStorageAccount)
	r.Delete("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.storage/storageaccounts/{accountName}", a.deleteStorageAccount)
	r.Get("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.storage/storageaccounts", a.listStorageAccountsByRG)
	r.Get("/{subscriptionID}/providers/microsoft.storage/storageaccounts", a.listStorageAccountsBySub)
	// listKeys — called by the azurerm provider to populate account key in state.
	// Returns Azurite's well-known key so SDK clients can authenticate against
	// the Azurite sidecar without extra configuration.
	r.Post("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.storage/storageaccounts/{accountName}/listkeys", a.listStorageAccountKeys)

	// Key Vaults (Microsoft.KeyVault/vaults)
	r.Put("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.keyvault/vaults/{vaultName}", a.putKeyVault)
	r.Get("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.keyvault/vaults/{vaultName}", a.getKeyVault)
	r.Head("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.keyvault/vaults/{vaultName}", a.headKeyVault)
	r.Delete("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.keyvault/vaults/{vaultName}", a.deleteKeyVault)
	r.Get("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.keyvault/vaults", a.listKeyVaultsByRG)
	r.Get("/{subscriptionID}/providers/microsoft.keyvault/vaults", a.listKeyVaultsBySub)

	// Storage Blob Containers (Microsoft.Storage/storageAccounts/blobServices/containers)
	// The path segment "default" is a fixed literal (not a parameter) matching the real ARM API.
	r.Put("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.storage/storageaccounts/{accountName}/blobservices/default/containers/{containerName}", a.putStorageContainer)
	r.Get("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.storage/storageaccounts/{accountName}/blobservices/default/containers/{containerName}", a.getStorageContainer)
	r.Head("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.storage/storageaccounts/{accountName}/blobservices/default/containers/{containerName}", a.headStorageContainer)
	r.Delete("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.storage/storageaccounts/{accountName}/blobservices/default/containers/{containerName}", a.deleteStorageContainer)
	r.Get("/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.storage/storageaccounts/{accountName}/blobservices/default/containers", a.listStorageContainers)
}

// --- Subscriptions ---

func (a *Router) listSubscriptions(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"value": []map[string]interface{}{
			{
				"id":             "/subscriptions/00000000-0000-0000-0000-000000000000",
				"subscriptionId": "00000000-0000-0000-0000-000000000000",
				"tenantId":       "00000000-0000-0000-0000-000000000001",
				"displayName":    "azemu-default",
				"state":          "Enabled",
			},
		},
	})
}

func (a *Router) getSubscription(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id":             "/subscriptions/" + subID,
		"subscriptionId": subID,
		"tenantId":       "00000000-0000-0000-0000-000000000001",
		"displayName":    "azemu-default",
		"state":          "Enabled",
	})
}

// --- Providers (always registered) ---

var defaultProviders = []string{
	"Microsoft.Resources", "Microsoft.Network", "Microsoft.Storage",
	"Microsoft.Compute", "Microsoft.KeyVault", "Microsoft.Web",
	"Microsoft.ContainerRegistry", "Microsoft.Dns",
}

func (a *Router) listProviders(w http.ResponseWriter, r *http.Request) {
	var providers []map[string]interface{}
	for _, ns := range defaultProviders {
		providers = append(providers, providerEntry(ns))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"value": providers})
}

func (a *Router) getProvider(w http.ResponseWriter, r *http.Request) {
	ns := chi.URLParam(r, "namespace")
	writeJSON(w, http.StatusOK, providerEntry(ns))
}

func (a *Router) registerProvider(w http.ResponseWriter, r *http.Request) {
	ns := chi.URLParam(r, "namespace")
	writeJSON(w, http.StatusOK, providerEntry(ns))
}

func providerEntry(ns string) map[string]interface{} {
	return map[string]interface{}{
		"id":                "/subscriptions/00000000-0000-0000-0000-000000000000/providers/" + ns,
		"namespace":         ns,
		"registrationState": "Registered",
		"resourceTypes":     []interface{}{},
	}
}

// --- Resource Groups ---

func (a *Router) putResourceGroup(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	name := chi.URLParam(r, "resourceGroupName")

	var body struct {
		Location   string                 `json:"location"`
		Tags       map[string]string      `json:"tags"`
		Properties map[string]interface{} `json:"properties"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeAzureError(w, http.StatusBadRequest, "InvalidRequestContent", err.Error())
		return
	}
	// Mirror the validation pattern used by vnet.go / subnet.go: a PUT with
	// no location is rejected with 400 InvalidRequestContent. Previously
	// putResourceGroup predated this pattern and accepted `{}` silently.
	if strings.TrimSpace(body.Location) == "" {
		writeAzureError(w, http.StatusBadRequest, "InvalidRequestContent",
			"location is required")
		return
	}

	id := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s", subID, name)
	res := &store.Resource{
		ID:       id,
		Name:     name,
		Type:     "Microsoft.Resources/resourceGroups",
		Location: strings.ToLower(body.Location),
		Tags:     normaliseTags(body.Tags),
		Properties: map[string]interface{}{
			"provisioningState": "Succeeded",
		},
	}

	_, exists := a.store.Get(id)
	if err := a.store.Put(id, res); err != nil {
		writeAzureError(w, http.StatusInternalServerError, "InternalServerError",
			fmt.Sprintf("put resource group %q: %s", name, err))
		return
	}

	status := http.StatusCreated
	if exists {
		status = http.StatusOK
	}
	log.Info().Str("resource_id", id).Bool("existed", exists).Msg("resource group upsert")
	writeJSON(w, status, resourceGroupResponse(res))
}

func (a *Router) getResourceGroup(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	name := chi.URLParam(r, "resourceGroupName")
	id := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s", subID, name)

	res, ok := a.store.Get(id)
	if !ok {
		writeAzureError(w, http.StatusNotFound, "ResourceGroupNotFound",
			fmt.Sprintf("Resource group '%s' could not be found.", name))
		return
	}
	writeJSON(w, http.StatusOK, resourceGroupResponse(res))
}

func (a *Router) headResourceGroup(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	name := chi.URLParam(r, "resourceGroupName")
	id := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s", subID, name)

	if _, ok := a.store.Get(id); !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *Router) deleteResourceGroup(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	name := chi.URLParam(r, "resourceGroupName")
	id := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s", subID, name)

	if !a.store.Delete(id) {
		writeAzureError(w, http.StatusNotFound, "ResourceGroupNotFound",
			fmt.Sprintf("Resource group '%s' could not be found.", name))
		return
	}

	// ARM returns 202 Accepted with a tracking header for async delete
	w.Header().Set("Location", fmt.Sprintf("/subscriptions/%s/operationresults/%s", subID, uuid.New().String()))
	w.WriteHeader(http.StatusAccepted)
}

func (a *Router) listResourceGroups(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	prefix := fmt.Sprintf("/subscriptions/%s/resourceGroups/", subID)
	resources := a.store.List(prefix)

	items := []map[string]interface{}{}
	for _, res := range resources {
		if res.Type == "Microsoft.Resources/resourceGroups" {
			items = append(items, resourceGroupResponse(res))
		}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"value": items})
}

// listResourceGroupResources returns every resource the store holds whose
// id is a path-extension of the given resource group, EXCLUDING the
// resource group itself. The azurerm provider calls this during destroy
// to verify the cascade is safe; returning an empty value array for an
// RG that contains nothing is the right behaviour. Query params like
// $expand and $top are accepted but ignored — azemu always returns the
// full result set.
func (a *Router) listResourceGroupResources(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	prefix := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/", subID, rgName)
	resources := a.store.List(prefix)

	items := []map[string]interface{}{}
	for _, res := range resources {
		// Exclude the parent RG itself; the prefix matches it as a
		// substring of any child id.
		if res.Type == "Microsoft.Resources/resourceGroups" {
			continue
		}
		items = append(items, map[string]interface{}{
			"id":         res.ID,
			"name":       res.Name,
			"type":       res.Type,
			"location":   res.Location,
			"tags":       res.Tags,
			"properties": res.Properties,
		})
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"value": items})
}

func resourceGroupResponse(r *store.Resource) map[string]interface{} {
	return map[string]interface{}{
		"id":       r.ID,
		"name":     r.Name,
		"type":     r.Type,
		"location": r.Location,
		"tags":     r.Tags,
		"properties": map[string]interface{}{
			"provisioningState": "Succeeded",
		},
	}
}

// --- Helpers ---

// normaliseTags ensures tags is always a non-nil map so JSON responses
// serialise as {} instead of null, matching real Azure behaviour.
func normaliseTags(tags map[string]string) map[string]string {
	if tags == nil {
		return map[string]string{}
	}
	return tags
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(v); err != nil {
		log.Error().Err(err).Msg("writeJSON: encode failed")
		http.Error(w, `{"error":{"code":"InternalServerError","message":"response encoding failed"}}`,
			http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = buf.WriteTo(w)
}

func writeAzureError(w http.ResponseWriter, status int, code, message string) {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(map[string]interface{}{
		"error": map[string]interface{}{
			"code":    code,
			"message": message,
		},
	}); err != nil {
		log.Error().Err(err).Msg("writeAzureError: encode failed")
		http.Error(w, `{"error":{"code":"InternalServerError","message":"error encoding failed"}}`,
			http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = buf.WriteTo(w)
}

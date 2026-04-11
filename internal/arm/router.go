package arm

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/zerodeth/azemu/internal/store"
)

type Router struct {
	store store.Store
}

func NewRouter(s store.Store) *Router {
	return &Router{store: s}
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
		Tags:     body.Tags,
		Properties: map[string]interface{}{
			"provisioningState": "Succeeded",
		},
	}

	_, exists := a.store.Get(id)
	a.store.Put(id, res)

	status := http.StatusCreated
	if exists {
		status = http.StatusOK
	}
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

	var items []map[string]interface{}
	for _, res := range resources {
		if res.Type == "Microsoft.Resources/resourceGroups" {
			items = append(items, resourceGroupResponse(res))
		}
	}
	if items == nil {
		items = []map[string]interface{}{}
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

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeAzureError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"error": map[string]interface{}{
			"code":    code,
			"message": message,
		},
	})
}

package arm

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

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
		Location   string            `json:"location"`
		Tags       map[string]string `json:"tags"`
		Properties map[string]interface{} `json:"properties"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeAzureError(w, http.StatusBadRequest, "InvalidRequestContent", err.Error())
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

// azureTimestamp returns ARM-style timestamp.
func azureTimestamp() string {
	return time.Now().UTC().Format("2006-01-02T15:04:05.0000000Z")
}

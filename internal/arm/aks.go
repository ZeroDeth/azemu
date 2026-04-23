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

const (
	aksClusterTypeString  = "Microsoft.ContainerService/managedClusters"
	aksNodePoolTypeString = "Microsoft.ContainerService/managedClusters/agentPools"
)

func aksClusterID(subID, rgName, name string) string {
	return fmt.Sprintf(
		"/subscriptions/%s/resourceGroups/%s/providers/Microsoft.ContainerService/managedClusters/%s",
		subID, rgName, name,
	)
}

func aksNodePoolID(subID, rgName, clusterName, poolName string) string {
	return fmt.Sprintf(
		"/subscriptions/%s/resourceGroups/%s/providers/Microsoft.ContainerService/managedClusters/%s/agentPools/%s",
		subID, rgName, clusterName, poolName,
	)
}

type aksClusterBody struct {
	Location   string                 `json:"location"`
	Tags       map[string]string      `json:"tags"`
	Sku        map[string]interface{} `json:"sku"`
	Properties map[string]interface{} `json:"properties"`
	Identity   map[string]interface{} `json:"identity"`
}

func (a *Router) putAKSCluster(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	name := chi.URLParam(r, "clusterName")

	var body aksClusterBody
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
	body.Properties["powerState"] = map[string]interface{}{"code": "Running"}

	if _, ok := body.Properties["kubernetesVersion"]; !ok {
		body.Properties["kubernetesVersion"] = "1.29.0"
	}
	body.Properties["currentKubernetesVersion"] = body.Properties["kubernetesVersion"]

	body.Properties["fqdn"] = fmt.Sprintf("%s-dns-%s.hcp.%s.azmk8s.io", name,
		uuid.New().String()[:8], strings.ToLower(body.Location))

	if _, ok := body.Properties["nodeResourceGroup"]; !ok {
		body.Properties["nodeResourceGroup"] = fmt.Sprintf("MC_%s_%s_%s", rgName, name, strings.ToLower(body.Location))
	}

	if body.Sku == nil {
		body.Sku = map[string]interface{}{"name": "Base", "tier": "Free"}
	}
	body.Properties["_sku"] = body.Sku

	if body.Identity != nil {
		body.Properties["_identity"] = body.Identity
	}

	id := aksClusterID(subID, rgName, name)
	res := &store.Resource{
		ID:         id,
		Name:       name,
		Type:       aksClusterTypeString,
		Location:   strings.ToLower(body.Location),
		Tags:       normaliseTags(body.Tags),
		Properties: body.Properties,
	}

	_, exists := a.store.Get(id)
	if err := a.store.Put(id, res); err != nil {
		writeAzureError(w, http.StatusInternalServerError, "InternalServerError",
			fmt.Sprintf("put AKS cluster %q: %s", name, err))
		return
	}

	status := http.StatusCreated
	if exists {
		status = http.StatusOK
	}
	log.Info().Str("resource_id", id).Bool("existed", exists).Msg("AKS cluster upsert")
	writeJSON(w, status, aksClusterResponse(res))
}

func (a *Router) getAKSCluster(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	name := chi.URLParam(r, "clusterName")
	id := aksClusterID(subID, rgName, name)

	res, ok := a.store.Get(id)
	if !ok {
		writeAzureError(w, http.StatusNotFound, "ResourceNotFound",
			fmt.Sprintf("The Resource 'Microsoft.ContainerService/managedClusters/%s' under resource group '%s' was not found.", name, rgName))
		return
	}
	writeJSON(w, http.StatusOK, aksClusterResponse(res))
}

func (a *Router) headAKSCluster(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	name := chi.URLParam(r, "clusterName")
	id := aksClusterID(subID, rgName, name)

	if _, ok := a.store.Get(id); !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *Router) deleteAKSCluster(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	name := chi.URLParam(r, "clusterName")
	id := aksClusterID(subID, rgName, name)

	if !a.store.Delete(id) {
		writeAzureError(w, http.StatusNotFound, "ResourceNotFound",
			fmt.Sprintf("The Resource 'Microsoft.ContainerService/managedClusters/%s' under resource group '%s' was not found.", name, rgName))
		return
	}

	// Cascade-delete all agent pools belonging to this cluster.
	poolsPrefix := fmt.Sprintf(
		"/subscriptions/%s/resourceGroups/%s/providers/Microsoft.ContainerService/managedClusters/%s/agentPools/",
		subID, rgName, name,
	)
	for _, poolRes := range a.store.List(poolsPrefix) {
		a.store.Delete(poolRes.ID)
	}

	log.Info().Str("resource_id", id).Msg("AKS cluster deleted")
	w.Header().Set("Location",
		fmt.Sprintf("/subscriptions/%s/operationresults/%s", subID, uuid.New().String()))
	w.WriteHeader(http.StatusAccepted)
}

func (a *Router) listAKSClustersByRG(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	prefix := fmt.Sprintf(
		"/subscriptions/%s/resourceGroups/%s/providers/Microsoft.ContainerService/managedClusters/",
		subID, rgName,
	)
	a.writeAKSClusterList(w, prefix)
}

func (a *Router) listAKSClustersBySub(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	prefix := fmt.Sprintf("/subscriptions/%s/resourceGroups/", subID)
	a.writeAKSClusterList(w, prefix)
}

func (a *Router) writeAKSClusterList(w http.ResponseWriter, prefix string) {
	items := []map[string]interface{}{}
	for _, res := range a.store.List(prefix) {
		if res.Type != aksClusterTypeString {
			continue
		}
		items = append(items, aksClusterResponse(res))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"value": items})
}

func aksClusterResponse(c *store.Resource) map[string]interface{} {
	props := map[string]interface{}{
		"provisioningState": "Succeeded",
		"powerState":        map[string]interface{}{"code": "Running"},
	}
	for k, val := range c.Properties {
		if k == "provisioningState" || k == "powerState" {
			continue
		}
		props[k] = val
	}
	sku := map[string]interface{}{"name": "Base", "tier": "Free"}
	if s, ok := props["_sku"]; ok {
		if sm, ok := s.(map[string]interface{}); ok {
			sku = sm
		}
		delete(props, "_sku")
	}
	var identity map[string]interface{}
	if ident, ok := props["_identity"]; ok {
		if im, ok := ident.(map[string]interface{}); ok {
			identity = im
		}
		delete(props, "_identity")
	}

	out := map[string]interface{}{
		"id":         c.ID,
		"name":       c.Name,
		"type":       c.Type,
		"location":   c.Location,
		"tags":       c.Tags,
		"sku":        sku,
		"properties": props,
	}
	if identity != nil {
		out["identity"] = identity
	}
	return out
}

// --- AKS Agent Pools ---

type aksNodePoolBody struct {
	Location   string                 `json:"location"`
	Tags       map[string]string      `json:"tags"`
	Properties map[string]interface{} `json:"properties"`
}

func (a *Router) putAKSNodePool(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	clusterName := chi.URLParam(r, "clusterName")
	poolName := chi.URLParam(r, "poolName")

	clusterID := aksClusterID(subID, rgName, clusterName)
	if _, ok := a.store.Get(clusterID); !ok {
		writeAzureError(w, http.StatusNotFound, "ParentResourceNotFound",
			fmt.Sprintf("The parent resource 'Microsoft.ContainerService/managedClusters/%s' was not found.", clusterName))
		return
	}

	var body aksNodePoolBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeAzureError(w, http.StatusBadRequest, "InvalidRequestContent", err.Error())
		return
	}

	if body.Properties == nil {
		body.Properties = map[string]interface{}{}
	}
	body.Properties["provisioningState"] = "Succeeded"
	body.Properties["powerState"] = map[string]interface{}{"code": "Running"}

	if _, ok := body.Properties["vmSize"]; !ok {
		body.Properties["vmSize"] = "Standard_DS2_v2"
	}
	if _, ok := body.Properties["count"]; !ok {
		body.Properties["count"] = float64(1)
	}
	if _, ok := body.Properties["osType"]; !ok {
		body.Properties["osType"] = "Linux"
	}
	if _, ok := body.Properties["mode"]; !ok {
		body.Properties["mode"] = "User"
	}

	loc := strings.ToLower(body.Location)
	if loc == "" {
		if clusterRes, ok := a.store.Get(clusterID); ok {
			loc = clusterRes.Location
		}
	}

	id := aksNodePoolID(subID, rgName, clusterName, poolName)
	res := &store.Resource{
		ID:         id,
		Name:       poolName,
		Type:       aksNodePoolTypeString,
		Location:   loc,
		Tags:       normaliseTags(body.Tags),
		Properties: body.Properties,
	}

	_, exists := a.store.Get(id)
	if err := a.store.Put(id, res); err != nil {
		writeAzureError(w, http.StatusInternalServerError, "InternalServerError",
			fmt.Sprintf("put AKS node pool %q: %s", poolName, err))
		return
	}

	status := http.StatusCreated
	if exists {
		status = http.StatusOK
	}
	log.Info().Str("resource_id", id).Bool("existed", exists).Msg("AKS node pool upsert")
	writeJSON(w, status, aksNodePoolResponse(res))
}

func (a *Router) getAKSNodePool(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	clusterName := chi.URLParam(r, "clusterName")
	poolName := chi.URLParam(r, "poolName")
	id := aksNodePoolID(subID, rgName, clusterName, poolName)

	res, ok := a.store.Get(id)
	if !ok {
		writeAzureError(w, http.StatusNotFound, "ResourceNotFound",
			fmt.Sprintf("The Resource 'Microsoft.ContainerService/managedClusters/%s/agentPools/%s' was not found.", clusterName, poolName))
		return
	}
	writeJSON(w, http.StatusOK, aksNodePoolResponse(res))
}

func (a *Router) headAKSNodePool(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	clusterName := chi.URLParam(r, "clusterName")
	poolName := chi.URLParam(r, "poolName")
	id := aksNodePoolID(subID, rgName, clusterName, poolName)

	if _, ok := a.store.Get(id); !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *Router) deleteAKSNodePool(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	clusterName := chi.URLParam(r, "clusterName")
	poolName := chi.URLParam(r, "poolName")
	id := aksNodePoolID(subID, rgName, clusterName, poolName)

	if !a.store.Delete(id) {
		writeAzureError(w, http.StatusNotFound, "ResourceNotFound",
			fmt.Sprintf("The Resource 'Microsoft.ContainerService/managedClusters/%s/agentPools/%s' was not found.", clusterName, poolName))
		return
	}

	log.Info().Str("resource_id", id).Msg("AKS node pool deleted")
	w.Header().Set("Location",
		fmt.Sprintf("/subscriptions/%s/operationresults/%s", subID, uuid.New().String()))
	w.WriteHeader(http.StatusAccepted)
}

func (a *Router) listAKSNodePools(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	clusterName := chi.URLParam(r, "clusterName")
	prefix := fmt.Sprintf(
		"/subscriptions/%s/resourceGroups/%s/providers/Microsoft.ContainerService/managedClusters/%s/agentPools/",
		subID, rgName, clusterName,
	)
	items := []map[string]interface{}{}
	for _, res := range a.store.List(prefix) {
		if res.Type != aksNodePoolTypeString {
			continue
		}
		items = append(items, aksNodePoolResponse(res))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"value": items})
}

func aksNodePoolResponse(p *store.Resource) map[string]interface{} {
	props := map[string]interface{}{
		"provisioningState": "Succeeded",
		"powerState":        map[string]interface{}{"code": "Running"},
	}
	for k, val := range p.Properties {
		if k == "provisioningState" || k == "powerState" {
			continue
		}
		props[k] = val
	}
	return map[string]interface{}{
		"id":         p.ID,
		"name":       p.Name,
		"type":       p.Type,
		"location":   p.Location,
		"tags":       p.Tags,
		"properties": props,
	}
}

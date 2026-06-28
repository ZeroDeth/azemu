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

const (
	lbTypeString      = "Microsoft.Network/loadBalancers"
	lbBackendPoolType = "Microsoft.Network/loadBalancers/backendAddressPools"
	lbRuleType        = "Microsoft.Network/loadBalancers/loadBalancingRules"
	lbProbeType       = "Microsoft.Network/loadBalancers/probes"
)

// --- ID builders ---

func lbID(subID, rgName, lbName string) string {
	return fmt.Sprintf(
		"/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/loadBalancers/%s",
		subID, rgName, lbName,
	)
}

func lbBackendPoolID(subID, rgName, lbName, poolName string) string {
	return fmt.Sprintf(
		"/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/loadBalancers/%s/backendAddressPools/%s",
		subID, rgName, lbName, poolName,
	)
}

func lbRuleID(subID, rgName, lbName, ruleName string) string {
	return fmt.Sprintf(
		"/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/loadBalancers/%s/loadBalancingRules/%s",
		subID, rgName, lbName, ruleName,
	)
}

func lbProbeID(subID, rgName, lbName, probeName string) string {
	return fmt.Sprintf(
		"/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/loadBalancers/%s/probes/%s",
		subID, rgName, lbName, probeName,
	)
}

// --- Load Balancer (parent resource) ---

// lbBody is the subset of the azurerm_lb PUT payload that azemu understands.
// The SKU lives at the top level of the ARM document; it is stashed in
// Properties["_sku"] for storage and promoted back to the top-level "sku"
// field on response, keeping store.Resource unchanged. frontendIPConfigurations
// is stored inline in properties (it has no separate child endpoint in the
// azurerm provider). Backend pools, rules, and probes are stored as children
// under the LB id prefix so cascade delete works for free.
type lbBody struct {
	Location   string                 `json:"location"`
	Tags       map[string]string      `json:"tags"`
	Sku        map[string]interface{} `json:"sku"`
	Properties map[string]interface{} `json:"properties"`
}

func (a *Router) putLB(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	name := chi.URLParam(r, "lbName")

	var body lbBody
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

	id := lbID(subID, rgName, name)
	loc := strings.ToLower(body.Location)

	// probes and loadBalancingRules have no standalone ARM create operation;
	// the azurerm provider manages azurerm_lb_probe / azurerm_lb_rule by
	// read-modify-write on the parent LB, writing the full desired array inline
	// via this PUT. Capture each array (and whether the key was present) before
	// stripping it from the stored body, then reconcile child store entries to
	// match after the parent write succeeds. See TODO.md M8.
	probesRaw, probesPresent := body.Properties["probes"].([]interface{})
	rulesRaw, rulesPresent := body.Properties["loadBalancingRules"].([]interface{})

	// Drop child arrays that must not be stored inline; they are re-assembled
	// from child store entries at response time.
	delete(body.Properties, "backendAddressPools")
	delete(body.Properties, "loadBalancingRules")
	delete(body.Properties, "probes")
	delete(body.Properties, "outboundRules")
	delete(body.Properties, "inboundNatRules")
	delete(body.Properties, "inboundNatPools")

	// Store the SKU under a private key so it can be reconstructed on response.
	if body.Sku != nil {
		body.Properties["_sku"] = body.Sku
	} else if _, has := body.Properties["_sku"]; !has {
		body.Properties["_sku"] = map[string]interface{}{"name": "Basic", "tier": "Regional"}
	}
	res := &store.Resource{
		ID:         id,
		Name:       name,
		Type:       lbTypeString,
		Location:   loc,
		Tags:       normaliseTags(body.Tags),
		Properties: body.Properties,
	}

	_, exists := a.store.Get(id)
	if err := a.store.Put(id, res); err != nil {
		writeAzureError(w, http.StatusInternalServerError, "InternalServerError",
			fmt.Sprintf("put load balancer %q: %s", name, err))
		return
	}

	// Reconcile inline children only after the parent write succeeds, so a
	// failed parent PUT never leaves orphaned probe/rule entries behind. The
	// key-present guard means an azurerm_lb PUT (which omits these arrays) does
	// not disturb children managed by azurerm_lb_probe / azurerm_lb_rule.
	if probesPresent {
		if err := a.reconcileLBInlineChildren(id, loc, probesRaw, "probes", lbProbeType); err != nil {
			writeAzureError(w, http.StatusInternalServerError, "InternalServerError", err.Error())
			return
		}
	}
	if rulesPresent {
		if err := a.reconcileLBInlineChildren(id, loc, rulesRaw, "loadBalancingRules", lbRuleType); err != nil {
			writeAzureError(w, http.StatusInternalServerError, "InternalServerError", err.Error())
			return
		}
	}

	status := http.StatusCreated
	if exists {
		status = http.StatusOK
	}
	log.Info().Str("resource_id", id).Bool("existed", exists).Msg("load balancer upsert")
	writeJSON(w, status, lbResponse(res, a.lbChildren(id)))
}

func (a *Router) getLB(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	name := chi.URLParam(r, "lbName")
	id := lbID(subID, rgName, name)

	res, ok := a.store.Get(id)
	if !ok {
		writeAzureError(w, http.StatusNotFound, "ResourceNotFound",
			fmt.Sprintf("Load balancer '%s' could not be found.", name))
		return
	}
	writeJSON(w, http.StatusOK, lbResponse(res, a.lbChildren(id)))
}

func (a *Router) headLB(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	name := chi.URLParam(r, "lbName")
	id := lbID(subID, rgName, name)

	if _, ok := a.store.Get(id); !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *Router) deleteLB(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	name := chi.URLParam(r, "lbName")
	id := lbID(subID, rgName, name)

	// store.Delete cascades by key prefix, removing backend pools, rules, and
	// probes in the same call.
	if !a.store.Delete(id) {
		writeAzureError(w, http.StatusNotFound, "ResourceNotFound",
			fmt.Sprintf("Load balancer '%s' could not be found.", name))
		return
	}

	log.Info().Str("resource_id", id).Msg("load balancer deleted")
	a.acceptAsyncDelete(w, r, subID)
}

func (a *Router) listLBsByRG(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	prefix := fmt.Sprintf(
		"/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/loadBalancers/",
		subID, rgName,
	)
	a.writeLBList(w, prefix)
}

func (a *Router) listLBsBySub(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	prefix := fmt.Sprintf("/subscriptions/%s/resourceGroups/", subID)
	a.writeLBList(w, prefix)
}

func (a *Router) writeLBList(w http.ResponseWriter, prefix string) {
	resources := a.store.List(prefix)
	items := []map[string]interface{}{}
	for _, res := range resources {
		if res.Type != lbTypeString {
			continue
		}
		items = append(items, lbResponse(res, a.lbChildren(res.ID)))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"value": items})
}

// lbChildren returns all direct child resources of the given LB id: backend
// pools, rules, and probes. Only immediate children (one path segment deeper)
// are returned.
func (a *Router) lbChildren(lbid string) []*store.Resource {
	return a.store.List(lbid + "/")
}

// reconcileLBInlineChildren makes the child store entries for the given key
// (probes / loadBalancingRules) match the array the azurerm provider wrote
// inline on the parent LB PUT. Each array element is upserted as a child store
// entry that getLB re-embeds on read-back; any existing child of the same kind
// that the array no longer lists is deleted, because the azurerm_lb_probe and
// azurerm_lb_rule resources signal a removal by PUTting the parent LB with the
// element dropped from the array. The caller invokes this only when the key was
// present in the payload, so an azurerm_lb PUT that omits the array leaves
// these children untouched. See TODO.md M8.
func (a *Router) reconcileLBInlineChildren(lbid, location string, raw []interface{}, key, childType string) error {
	prefix := fmt.Sprintf("%s/%s/", lbid, key)
	keep := make(map[string]struct{}, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		name, _ := m["name"].(string)
		if strings.TrimSpace(name) == "" {
			continue
		}
		childProps, _ := m["properties"].(map[string]interface{})
		if childProps == nil {
			childProps = map[string]interface{}{}
		}
		childProps["provisioningState"] = "Succeeded"
		childID := prefix + name
		if err := a.store.Put(childID, &store.Resource{
			ID:         childID,
			Name:       name,
			Type:       childType,
			Location:   location,
			Properties: childProps,
		}); err != nil {
			return fmt.Errorf("put inline lb child %q: %w", childID, err)
		}
		keep[childID] = struct{}{}
		log.Info().Str("resource_id", childID).Str("via", "inline-lb-put").Msg("lb child upsert")
	}
	// Remove children the incoming array no longer lists.
	for _, child := range a.store.List(prefix) {
		if _, ok := keep[child.ID]; ok {
			continue
		}
		if a.store.Delete(child.ID) {
			log.Info().Str("resource_id", child.ID).Str("via", "inline-lb-put").Msg("lb child reconcile delete")
		}
	}
	return nil
}

// lbResponse builds the canonical ARM response for a load balancer. The SKU
// stored in Properties["_sku"] is promoted to the top-level "sku" field. Child
// resources (backend pools, rules, probes) are embedded as arrays inside
// properties, matching the real Azure GET response shape.
func lbResponse(lb *store.Resource, children []*store.Resource) map[string]interface{} {
	props := map[string]interface{}{
		"provisioningState": "Succeeded",
	}
	var sku map[string]interface{}
	for k, v := range lb.Properties {
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

	// Embed child arrays.
	pools := []map[string]interface{}{}
	rules := []map[string]interface{}{}
	probes := []map[string]interface{}{}
	for _, c := range children {
		switch c.Type {
		case lbBackendPoolType:
			pools = append(pools, lbBackendPoolResponse(c))
		case lbRuleType:
			rules = append(rules, lbChildResponse(c))
		case lbProbeType:
			probes = append(probes, lbChildResponse(c))
		}
	}
	props["backendAddressPools"] = pools
	props["loadBalancingRules"] = rules
	props["probes"] = probes

	return map[string]interface{}{
		"id":         lb.ID,
		"name":       lb.Name,
		"type":       lb.Type,
		"location":   lb.Location,
		"tags":       lb.Tags,
		"sku":        sku,
		"properties": props,
	}
}

// --- Backend Address Pools (child resource) ---

type lbChildBody struct {
	Properties map[string]interface{} `json:"properties"`
}

func (a *Router) putLBBackendPool(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	lbName := chi.URLParam(r, "lbName")
	name := chi.URLParam(r, "poolName")

	parentID := lbID(subID, rgName, lbName)
	if _, ok := a.store.Get(parentID); !ok {
		writeAzureError(w, http.StatusNotFound, "ParentResourceNotFound",
			fmt.Sprintf("Load balancer '%s' could not be found.", lbName))
		return
	}

	var body lbChildBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeAzureError(w, http.StatusBadRequest, "InvalidRequestContent", err.Error())
		return
	}
	if body.Properties == nil {
		body.Properties = map[string]interface{}{}
	}
	body.Properties["provisioningState"] = "Succeeded"

	id := lbBackendPoolID(subID, rgName, lbName, name)
	parent, _ := a.store.Get(parentID)
	res := &store.Resource{
		ID:         id,
		Name:       name,
		Type:       lbBackendPoolType,
		Location:   parent.Location,
		Properties: body.Properties,
	}

	_, exists := a.store.Get(id)
	if err := a.store.Put(id, res); err != nil {
		writeAzureError(w, http.StatusInternalServerError, "InternalServerError",
			fmt.Sprintf("put backend address pool %q: %s", name, err))
		return
	}

	status := http.StatusCreated
	if exists {
		status = http.StatusOK
	}
	log.Info().Str("resource_id", id).Bool("existed", exists).Msg("lb backend pool upsert")
	writeJSON(w, status, lbBackendPoolResponse(res))
}

func (a *Router) getLBBackendPool(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	lbName := chi.URLParam(r, "lbName")
	name := chi.URLParam(r, "poolName")
	id := lbBackendPoolID(subID, rgName, lbName, name)

	res, ok := a.store.Get(id)
	if !ok {
		writeAzureError(w, http.StatusNotFound, "ResourceNotFound",
			fmt.Sprintf("Backend address pool '%s' could not be found.", name))
		return
	}
	writeJSON(w, http.StatusOK, lbBackendPoolResponse(res))
}

func (a *Router) headLBBackendPool(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	lbName := chi.URLParam(r, "lbName")
	name := chi.URLParam(r, "poolName")
	id := lbBackendPoolID(subID, rgName, lbName, name)

	if _, ok := a.store.Get(id); !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *Router) deleteLBBackendPool(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	lbName := chi.URLParam(r, "lbName")
	name := chi.URLParam(r, "poolName")
	id := lbBackendPoolID(subID, rgName, lbName, name)

	if !a.store.Delete(id) {
		writeAzureError(w, http.StatusNotFound, "ResourceNotFound",
			fmt.Sprintf("Backend address pool '%s' could not be found.", name))
		return
	}

	log.Info().Str("resource_id", id).Msg("lb backend pool deleted")
	a.acceptAsyncDelete(w, r, subID)
}

func (a *Router) listLBBackendPools(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	lbName := chi.URLParam(r, "lbName")
	prefix := lbID(subID, rgName, lbName) + "/backendAddressPools/"

	resources := a.store.List(prefix)
	items := []map[string]interface{}{}
	for _, res := range resources {
		if res.Type != lbBackendPoolType {
			continue
		}
		items = append(items, lbBackendPoolResponse(res))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"value": items})
}

// lbBackendPoolResponse builds the canonical ARM response for a backend address
// pool. The real ARM API includes a loadBalancingRules backref array; azemu
// returns an empty slice which is sufficient for Terraform plan/apply.
func lbBackendPoolResponse(p *store.Resource) map[string]interface{} {
	props := map[string]interface{}{
		"provisioningState":  "Succeeded",
		"loadBalancingRules": []interface{}{},
	}
	for k, v := range p.Properties {
		if k == "provisioningState" || k == "loadBalancingRules" {
			continue
		}
		props[k] = v
	}
	return map[string]interface{}{
		"id":         p.ID,
		"name":       p.Name,
		"type":       p.Type,
		"properties": props,
	}
}

// --- Load Balancing Rules (child resource) ---

func (a *Router) putLBRule(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	lbName := chi.URLParam(r, "lbName")
	name := chi.URLParam(r, "ruleName")

	parentID := lbID(subID, rgName, lbName)
	if _, ok := a.store.Get(parentID); !ok {
		writeAzureError(w, http.StatusNotFound, "ParentResourceNotFound",
			fmt.Sprintf("Load balancer '%s' could not be found.", lbName))
		return
	}

	var body lbChildBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeAzureError(w, http.StatusBadRequest, "InvalidRequestContent", err.Error())
		return
	}
	if body.Properties == nil {
		body.Properties = map[string]interface{}{}
	}
	body.Properties["provisioningState"] = "Succeeded"

	id := lbRuleID(subID, rgName, lbName, name)
	parent, _ := a.store.Get(parentID)
	res := &store.Resource{
		ID:         id,
		Name:       name,
		Type:       lbRuleType,
		Location:   parent.Location,
		Properties: body.Properties,
	}

	_, exists := a.store.Get(id)
	if err := a.store.Put(id, res); err != nil {
		writeAzureError(w, http.StatusInternalServerError, "InternalServerError",
			fmt.Sprintf("put load balancing rule %q: %s", name, err))
		return
	}

	status := http.StatusCreated
	if exists {
		status = http.StatusOK
	}
	log.Info().Str("resource_id", id).Bool("existed", exists).Msg("lb rule upsert")
	writeJSON(w, status, lbChildResponse(res))
}

func (a *Router) getLBRule(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	lbName := chi.URLParam(r, "lbName")
	name := chi.URLParam(r, "ruleName")
	id := lbRuleID(subID, rgName, lbName, name)

	res, ok := a.store.Get(id)
	if !ok {
		writeAzureError(w, http.StatusNotFound, "ResourceNotFound",
			fmt.Sprintf("Load balancing rule '%s' could not be found.", name))
		return
	}
	writeJSON(w, http.StatusOK, lbChildResponse(res))
}

func (a *Router) headLBRule(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	lbName := chi.URLParam(r, "lbName")
	name := chi.URLParam(r, "ruleName")
	id := lbRuleID(subID, rgName, lbName, name)

	if _, ok := a.store.Get(id); !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *Router) deleteLBRule(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	lbName := chi.URLParam(r, "lbName")
	name := chi.URLParam(r, "ruleName")
	id := lbRuleID(subID, rgName, lbName, name)

	if !a.store.Delete(id) {
		writeAzureError(w, http.StatusNotFound, "ResourceNotFound",
			fmt.Sprintf("Load balancing rule '%s' could not be found.", name))
		return
	}

	log.Info().Str("resource_id", id).Msg("lb rule deleted")
	a.acceptAsyncDelete(w, r, subID)
}

func (a *Router) listLBRules(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	lbName := chi.URLParam(r, "lbName")
	prefix := lbID(subID, rgName, lbName) + "/loadBalancingRules/"

	resources := a.store.List(prefix)
	items := []map[string]interface{}{}
	for _, res := range resources {
		if res.Type != lbRuleType {
			continue
		}
		items = append(items, lbChildResponse(res))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"value": items})
}

// --- Probes (child resource) ---

func (a *Router) putLBProbe(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	lbName := chi.URLParam(r, "lbName")
	name := chi.URLParam(r, "probeName")

	parentID := lbID(subID, rgName, lbName)
	if _, ok := a.store.Get(parentID); !ok {
		writeAzureError(w, http.StatusNotFound, "ParentResourceNotFound",
			fmt.Sprintf("Load balancer '%s' could not be found.", lbName))
		return
	}

	var body lbChildBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeAzureError(w, http.StatusBadRequest, "InvalidRequestContent", err.Error())
		return
	}
	if body.Properties == nil {
		body.Properties = map[string]interface{}{}
	}
	body.Properties["provisioningState"] = "Succeeded"

	id := lbProbeID(subID, rgName, lbName, name)
	parent, _ := a.store.Get(parentID)
	res := &store.Resource{
		ID:         id,
		Name:       name,
		Type:       lbProbeType,
		Location:   parent.Location,
		Properties: body.Properties,
	}

	_, exists := a.store.Get(id)
	if err := a.store.Put(id, res); err != nil {
		writeAzureError(w, http.StatusInternalServerError, "InternalServerError",
			fmt.Sprintf("put probe %q: %s", name, err))
		return
	}

	status := http.StatusCreated
	if exists {
		status = http.StatusOK
	}
	log.Info().Str("resource_id", id).Bool("existed", exists).Msg("lb probe upsert")
	writeJSON(w, status, lbChildResponse(res))
}

func (a *Router) getLBProbe(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	lbName := chi.URLParam(r, "lbName")
	name := chi.URLParam(r, "probeName")
	id := lbProbeID(subID, rgName, lbName, name)

	res, ok := a.store.Get(id)
	if !ok {
		writeAzureError(w, http.StatusNotFound, "ResourceNotFound",
			fmt.Sprintf("Probe '%s' could not be found.", name))
		return
	}
	writeJSON(w, http.StatusOK, lbChildResponse(res))
}

func (a *Router) headLBProbe(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	lbName := chi.URLParam(r, "lbName")
	name := chi.URLParam(r, "probeName")
	id := lbProbeID(subID, rgName, lbName, name)

	if _, ok := a.store.Get(id); !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *Router) deleteLBProbe(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	lbName := chi.URLParam(r, "lbName")
	name := chi.URLParam(r, "probeName")
	id := lbProbeID(subID, rgName, lbName, name)

	if !a.store.Delete(id) {
		writeAzureError(w, http.StatusNotFound, "ResourceNotFound",
			fmt.Sprintf("Probe '%s' could not be found.", name))
		return
	}

	log.Info().Str("resource_id", id).Msg("lb probe deleted")
	a.acceptAsyncDelete(w, r, subID)
}

func (a *Router) listLBProbes(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	lbName := chi.URLParam(r, "lbName")
	prefix := lbID(subID, rgName, lbName) + "/probes/"

	resources := a.store.List(prefix)
	items := []map[string]interface{}{}
	for _, res := range resources {
		if res.Type != lbProbeType {
			continue
		}
		items = append(items, lbChildResponse(res))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"value": items})
}

// lbChildResponse builds a canonical ARM response for a load balancer child
// resource (backend pool member excluded). Used for rules and probes.
func lbChildResponse(c *store.Resource) map[string]interface{} {
	props := map[string]interface{}{
		"provisioningState": "Succeeded",
	}
	for k, v := range c.Properties {
		if k == "provisioningState" {
			continue
		}
		props[k] = v
	}
	return map[string]interface{}{
		"id":         c.ID,
		"name":       c.Name,
		"type":       c.Type,
		"properties": props,
	}
}

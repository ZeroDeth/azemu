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
	nsgTypeString  = "Microsoft.Network/networkSecurityGroups"
	ruleTypeString = "Microsoft.Network/networkSecurityGroups/securityRules"
)

func nsgID(subID, rgName, nsgName string) string {
	return fmt.Sprintf(
		"/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/networkSecurityGroups/%s",
		subID, rgName, nsgName,
	)
}

func ruleID(subID, rgName, nsgName, ruleName string) string {
	return fmt.Sprintf(
		"/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/networkSecurityGroups/%s/securityRules/%s",
		subID, rgName, nsgName, ruleName,
	)
}

// nsgBody is the subset of the azurerm_network_security_group PUT payload that
// azemu understands. Inline security_rule blocks inside the azurerm_network_security_group
// resource are sent as properties.securityRules; azemu stores them as children
// under the NSG id prefix so cascade delete works for free.
type nsgBody struct {
	Location   string                 `json:"location"`
	Tags       map[string]string      `json:"tags"`
	Properties map[string]interface{} `json:"properties"`
}

func (a *Router) putNSG(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	name := chi.URLParam(r, "nsgName")

	var body nsgBody
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

	// Drop inline securityRules from the stored properties: azemu stores each
	// rule as a child resource under the NSG id prefix. On response the rules
	// are re-assembled from child store entries, preventing a split-brain
	// between inline data and child entries (same reasoning as vnet subnets).
	delete(body.Properties, "securityRules")

	id := nsgID(subID, rgName, name)
	res := &store.Resource{
		ID:         id,
		Name:       name,
		Type:       nsgTypeString,
		Location:   strings.ToLower(body.Location),
		Tags:       normaliseTags(body.Tags),
		Properties: body.Properties,
	}

	_, exists := a.store.Get(id)
	if err := a.store.Put(id, res); err != nil {
		writeAzureError(w, http.StatusInternalServerError, "InternalServerError",
			fmt.Sprintf("put network security group %q: %s", name, err))
		return
	}

	status := http.StatusCreated
	if exists {
		status = http.StatusOK
	}
	log.Info().Str("resource_id", id).Bool("existed", exists).Msg("nsg upsert")
	writeJSON(w, status, nsgResponse(res, a.store.List(id+"/securityRules/")))
}

func (a *Router) getNSG(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	name := chi.URLParam(r, "nsgName")
	id := nsgID(subID, rgName, name)

	res, ok := a.store.Get(id)
	if !ok {
		writeAzureError(w, http.StatusNotFound, "ResourceNotFound",
			fmt.Sprintf("Network security group '%s' could not be found.", name))
		return
	}
	writeJSON(w, http.StatusOK, nsgResponse(res, a.store.List(id+"/securityRules/")))
}

func (a *Router) headNSG(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	name := chi.URLParam(r, "nsgName")
	id := nsgID(subID, rgName, name)

	if _, ok := a.store.Get(id); !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *Router) deleteNSG(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	name := chi.URLParam(r, "nsgName")
	id := nsgID(subID, rgName, name)

	// store.Delete cascades by key prefix, so any securityRules/<name> entries
	// are removed in the same call.
	if !a.store.Delete(id) {
		writeAzureError(w, http.StatusNotFound, "ResourceNotFound",
			fmt.Sprintf("Network security group '%s' could not be found.", name))
		return
	}

	log.Info().Str("resource_id", id).Msg("nsg deleted")
	a.acceptAsyncDelete(w, r, subID)
}

func (a *Router) listNSGsByRG(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	prefix := fmt.Sprintf(
		"/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/networkSecurityGroups/",
		subID, rgName,
	)
	a.writeNSGList(w, prefix)
}

func (a *Router) listNSGsBySub(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	prefix := fmt.Sprintf("/subscriptions/%s/resourceGroups/", subID)
	a.writeNSGList(w, prefix)
}

func (a *Router) writeNSGList(w http.ResponseWriter, prefix string) {
	resources := a.store.List(prefix)
	items := []map[string]interface{}{}
	for _, res := range resources {
		if res.Type != nsgTypeString {
			continue
		}
		items = append(items, nsgResponse(res, a.store.List(res.ID+"/securityRules/")))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"value": items})
}

// --- Security Rules (child resource) ---

// ruleBody is the subset of the azurerm_network_security_rule PUT payload
// that azemu understands.
type ruleBody struct {
	Properties map[string]interface{} `json:"properties"`
}

func (a *Router) putRule(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	nsgName := chi.URLParam(r, "nsgName")
	name := chi.URLParam(r, "ruleName")

	// Parent-exists check: mirror the subnet pattern for fidelity.
	parentID := nsgID(subID, rgName, nsgName)
	if _, ok := a.store.Get(parentID); !ok {
		writeAzureError(w, http.StatusNotFound, "ParentResourceNotFound",
			fmt.Sprintf("Network security group '%s' could not be found.", nsgName))
		return
	}

	var body ruleBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeAzureError(w, http.StatusBadRequest, "InvalidRequestContent", err.Error())
		return
	}
	if body.Properties == nil {
		body.Properties = map[string]interface{}{}
	}
	body.Properties["provisioningState"] = "Succeeded"

	id := ruleID(subID, rgName, nsgName, name)
	// Rules inherit location from their parent NSG.
	parent, _ := a.store.Get(parentID)
	res := &store.Resource{
		ID:         id,
		Name:       name,
		Type:       ruleTypeString,
		Location:   parent.Location,
		Properties: body.Properties,
	}

	_, exists := a.store.Get(id)
	if err := a.store.Put(id, res); err != nil {
		writeAzureError(w, http.StatusInternalServerError, "InternalServerError",
			fmt.Sprintf("put security rule %q: %s", name, err))
		return
	}

	status := http.StatusCreated
	if exists {
		status = http.StatusOK
	}
	log.Info().Str("resource_id", id).Bool("existed", exists).Msg("security rule upsert")
	writeJSON(w, status, ruleResponse(res))
}

func (a *Router) getRule(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	nsgName := chi.URLParam(r, "nsgName")
	name := chi.URLParam(r, "ruleName")
	id := ruleID(subID, rgName, nsgName, name)

	res, ok := a.store.Get(id)
	if !ok {
		writeAzureError(w, http.StatusNotFound, "ResourceNotFound",
			fmt.Sprintf("Security rule '%s' could not be found.", name))
		return
	}
	writeJSON(w, http.StatusOK, ruleResponse(res))
}

func (a *Router) headRule(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	nsgName := chi.URLParam(r, "nsgName")
	name := chi.URLParam(r, "ruleName")
	id := ruleID(subID, rgName, nsgName, name)

	if _, ok := a.store.Get(id); !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *Router) deleteRule(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	nsgName := chi.URLParam(r, "nsgName")
	name := chi.URLParam(r, "ruleName")
	id := ruleID(subID, rgName, nsgName, name)

	if !a.store.Delete(id) {
		writeAzureError(w, http.StatusNotFound, "ResourceNotFound",
			fmt.Sprintf("Security rule '%s' could not be found.", name))
		return
	}

	log.Info().Str("resource_id", id).Msg("security rule deleted")
	a.acceptAsyncDelete(w, r, subID)
}

func (a *Router) listRules(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	nsgName := chi.URLParam(r, "nsgName")
	prefix := nsgID(subID, rgName, nsgName) + "/securityRules/"

	resources := a.store.List(prefix)
	items := []map[string]interface{}{}
	for _, res := range resources {
		if res.Type != ruleTypeString {
			continue
		}
		items = append(items, ruleResponse(res))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"value": items})
}

// nsgResponse builds the canonical ARM response for an NSG, embedding its
// child security rules from the store. An empty slice serialises as `[]`
// (not null), matching real Azure behaviour.
func nsgResponse(n *store.Resource, children []*store.Resource) map[string]interface{} {
	props := map[string]interface{}{
		"provisioningState": "Succeeded",
	}
	for k, v := range n.Properties {
		if k == "provisioningState" || k == "securityRules" {
			continue
		}
		props[k] = v
	}
	rules := []map[string]interface{}{}
	for _, c := range children {
		if c.Type != ruleTypeString {
			continue
		}
		rules = append(rules, ruleResponse(c))
	}
	props["securityRules"] = rules

	return map[string]interface{}{
		"id":         n.ID,
		"name":       n.Name,
		"type":       n.Type,
		"location":   n.Location,
		"tags":       n.Tags,
		"properties": props,
	}
}

// ruleResponse builds the canonical ARM response for a security rule.
func ruleResponse(r *store.Resource) map[string]interface{} {
	props := map[string]interface{}{
		"provisioningState": "Succeeded",
	}
	for k, v := range r.Properties {
		if k == "provisioningState" {
			continue
		}
		props[k] = v
	}
	return map[string]interface{}{
		"id":         r.ID,
		"name":       r.Name,
		"type":       r.Type,
		"properties": props,
	}
}

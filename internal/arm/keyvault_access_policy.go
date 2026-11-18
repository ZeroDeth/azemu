package arm

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/zerodeth/azemu/internal/store"
)

func (a *Router) updateKeyVaultAccessPolicy(w http.ResponseWriter, r *http.Request) {
	id := keyVaultID(chi.URLParam(r, "subscriptionID"), chi.URLParam(r, "resourceGroupName"), chi.URLParam(r, "vaultName"))
	vault, ok := a.store.Get(id)
	if !ok {
		writeAzureError(w, http.StatusNotFound, "ParentResourceNotFound", "Key Vault was not found.")
		return
	}
	var body struct {
		Properties struct {
			Policies []map[string]any `json:"accessPolicies"`
		} `json:"properties"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.Properties.Policies) == 0 {
		writeAzureError(w, http.StatusBadRequest, "InvalidRequestContent", "properties.accessPolicies is required.")
		return
	}
	operation := chi.URLParam(r, "policyOperation")
	if operation != "add" && operation != "replace" && operation != "remove" {
		writeAzureError(w, http.StatusBadRequest, "InvalidRequestContent", "Invalid access policy operation.")
		return
	}
	for _, policy := range body.Properties.Policies {
		tenant, _ := policy["tenantId"].(string)
		object, _ := policy["objectId"].(string)
		tenantID, err := uuid.Parse(tenant)
		if err != nil {
			writeAzureError(w, http.StatusBadRequest, "InvalidRequestContent", "tenantId must be a UUID.")
			return
		}
		objectID, err := uuid.Parse(object)
		vaultTenant, _ := vault.Properties["tenantId"].(string)
		if err != nil || (vaultTenant != "" && !strings.EqualFold(vaultTenant, tenantID.String())) {
			writeAzureError(w, http.StatusBadRequest, "InvalidRequestContent", "Invalid objectId or vault tenant mismatch.")
			return
		}
		policy["tenantId"], policy["objectId"] = tenantID.String(), objectID.String()
		if app, _ := policy["applicationId"].(string); app != "" {
			appID, err := uuid.Parse(app)
			if err != nil {
				writeAzureError(w, http.StatusBadRequest, "InvalidRequestContent", "applicationId must be a UUID.")
				return
			}
			policy["applicationId"] = appID.String()
		}
		if operation != "remove" {
			permissions, valid := policy["permissions"].(map[string]any)
			if !valid {
				writeAzureError(w, http.StatusBadRequest, "InvalidRequestContent", "permissions is required.")
				return
			}
			for _, raw := range permissions {
				list, valid := raw.([]any)
				if !valid {
					writeAzureError(w, http.StatusBadRequest, "InvalidRequestContent", "Permissions must be string arrays.")
					return
				}
				for _, item := range list {
					if _, valid := item.(string); !valid {
						writeAzureError(w, http.StatusBadRequest, "InvalidRequestContent", "Permissions must be strings.")
						return
					}
				}
			}
		}
	}
	policies, _ := vault.Properties["accessPolicies"].([]any)
	if operation == "replace" {
		policies = nil
	}
	for _, incoming := range body.Properties.Policies {
		next := []any{}
		for _, raw := range policies {
			current, _ := raw.(map[string]any)
			currentTenant, _ := current["tenantId"].(string)
			currentObject, _ := current["objectId"].(string)
			currentApp, _ := current["applicationId"].(string)
			incomingTenant, _ := incoming["tenantId"].(string)
			incomingObject, _ := incoming["objectId"].(string)
			incomingApp, _ := incoming["applicationId"].(string)
			if strings.EqualFold(currentTenant, incomingTenant) && strings.EqualFold(currentObject, incomingObject) &&
				strings.EqualFold(currentApp, incomingApp) {
				if operation == "add" {
					merged := map[string]any{}
					existingPermissions, _ := current["permissions"].(map[string]any)
					for category, raw := range existingPermissions {
						merged[category] = raw
					}
					permissions, _ := incoming["permissions"].(map[string]any)
					for category, raw := range permissions {
						values, _ := merged[category].([]any)
						values = append([]any{}, values...)
						for _, value := range raw.([]any) {
							found := false
							for _, old := range values {
								oldValue, _ := old.(string)
								if strings.EqualFold(oldValue, value.(string)) {
									found = true
								}
							}
							if !found {
								values = append(values, value)
							}
						}
						merged[category] = values
					}
					incoming["permissions"] = merged
				}
				continue
			}
			next = append(next, raw)
		}
		if operation != "remove" {
			next = append(next, incoming)
		}
		policies = next
	}
	props := map[string]any{}
	for key, value := range vault.Properties {
		props[key] = value
	}
	props["accessPolicies"] = policies
	vault.Properties = props
	if err := a.store.Put(id, vault); err != nil {
		writeAzureError(w, http.StatusInternalServerError, "InternalServerError", "Failed to store Key Vault access policies.")
		return
	}
	log.Info().Str("resource_id", id).Str("operation", operation).Msg("key vault access policies updated")
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "name": vault.Name, "type": vault.Type,
		"properties": map[string]any{"accessPolicies": policies}})
}

func keyVaultPolicyAllows(vault *store.Resource, principal, category, permission string) bool {
	policies, _ := vault.Properties["accessPolicies"].([]any)
	vaultTenant, _ := vault.Properties["tenantId"].(string)
	for _, raw := range policies {
		policy, _ := raw.(map[string]any)
		object, _ := policy["objectId"].(string)
		tenant, _ := policy["tenantId"].(string)
		if !strings.EqualFold(object, principal) || !strings.EqualFold(tenant, vaultTenant) {
			continue
		}
		permissions, _ := policy["permissions"].(map[string]any)
		values, _ := permissions[category].([]any)
		for _, raw := range values {
			value, _ := raw.(string)
			if strings.EqualFold(value, permission) {
				return true
			}
		}
	}
	return false
}

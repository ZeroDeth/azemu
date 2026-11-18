package arm

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"

	"github.com/zerodeth/azemu/internal/store"
)

func (a *Router) apimParent(w http.ResponseWriter, r *http.Request) (*store.Resource, bool) {
	res, ok := a.store.Get(apimRequestID(r))
	if !ok {
		writeAzureError(w, http.StatusNotFound, "ParentResourceNotFound", "API Management service was not found.")
	}
	return res, ok
}

func (a *Router) listAPIMInitialChildren(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.apimParent(w, r); !ok {
		return
	}
	// Newly created services have no sample APIs or products in azemu.
	writeJSON(w, http.StatusOK, map[string]any{"value": []any{}})
}

func (a *Router) getAPIMServicePolicy(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.apimParent(w, r); !ok {
		return
	}
	writeAzureError(w, http.StatusNotFound, "ResourceNotFound", "No service policy is configured.")
}

func apimSettingDefaults(setting string) map[string]any {
	switch setting {
	case "signin":
		return map[string]any{"enabled": false}
	case "signup":
		return map[string]any{"enabled": false, "termsOfService": map[string]any{
			"enabled": false, "consentRequired": false, "text": "",
		}}
	case "delegation":
		return map[string]any{
			"subscriptions":    map[string]any{"enabled": false},
			"userRegistration": map[string]any{"enabled": false},
		}
	case "access":
		return map[string]any{"enabled": false}
	default:
		return nil
	}
}

func (a *Router) apimSettings(w http.ResponseWriter, r *http.Request) {
	parent, ok := a.apimParent(w, r)
	if !ok {
		return
	}
	setting := chi.URLParam(r, "setting")
	props := apimSettingDefaults(setting)
	if props == nil {
		writeAzureError(w, http.StatusNotFound, "ResourceNotFound", "Unknown API Management setting.")
		return
	}
	category := "portalsettings"
	if setting == "access" {
		category = "tenant"
	}
	id := parent.ID + "/" + category + "/" + setting
	if saved, exists := a.store.Get(id); exists {
		for key, value := range saved.Properties {
			props[key] = value
		}
	}
	if r.Method == http.MethodPut || r.Method == http.MethodPatch {
		var body struct {
			Properties map[string]any `json:"properties"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeAzureError(w, http.StatusBadRequest, "InvalidRequestContent", err.Error())
			return
		}
		if body.Properties == nil {
			writeAzureError(w, http.StatusBadRequest, "InvalidRequestContent", "properties is required.")
			return
		}
		for key, value := range body.Properties {
			props[key] = value
		}
		if enabled, exists := props["enabled"]; exists {
			if _, valid := enabled.(bool); !valid {
				writeAzureError(w, http.StatusBadRequest, "InvalidRequestContent", "properties.enabled must be a boolean.")
				return
			}
		}
		if err := a.store.Put(id, &store.Resource{
			ID: id, Name: setting, Type: apimServiceType + "/" + category, Properties: props,
		}); err != nil {
			log.Error().Err(err).Str("resource_id", id).Msg("failed to store API Management settings")
			writeAzureError(w, http.StatusInternalServerError, "InternalServerError", "Failed to store API Management settings.")
			return
		}
		log.Info().Str("resource_id", id).Msg("API Management settings upsert")
	}
	if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/listsecrets") {
		if setting == "delegation" {
			writeJSON(w, http.StatusOK, map[string]any{"validationKey": props["validationKey"]})
		} else {
			writeJSON(w, http.StatusOK, map[string]any{"enabled": props["enabled"]})
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id": id, "name": setting, "type": apimServiceType + "/" + category, "properties": props,
	})
}

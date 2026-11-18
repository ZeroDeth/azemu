package arm

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"

	"github.com/zerodeth/azemu/internal/store"
)

const apimLoggerType = apimServiceType + "/loggers"

func (a *Router) apimLoggerRoutes(r chi.Router) {
	base := "/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.apimanagement/service/{serviceName}/loggers"
	r.Get(base, a.listAPIMLoggers)
	r.Put(base+"/{loggerName}", a.putAPIMLogger)
	r.Patch(base+"/{loggerName}", a.putAPIMLogger)
	r.Get(base+"/{loggerName}", a.getAPIMLogger)
	r.Head(base+"/{loggerName}", a.headAPIMLogger)
	r.Delete(base+"/{loggerName}", a.deleteAPIMLogger)
}

func (a *Router) putAPIMLogger(w http.ResponseWriter, r *http.Request) {
	parent, ok := a.apimParent(w, r)
	if !ok {
		return
	}
	id := parent.ID + "/loggers/" + chi.URLParam(r, "loggerName")
	existing, exists := a.store.Get(id)
	if r.Method == http.MethodPatch && !exists {
		writeAzureError(w, 404, "ResourceNotFound", "Logger was not found.")
		return
	}
	var body struct {
		Properties map[string]any `json:"properties"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil || body.Properties == nil {
		writeAzureError(w, 400, "InvalidRequestContent", "properties is required.")
		return
	}
	props := map[string]any{"isBuffered": true}
	if r.Method == http.MethodPatch {
		for k, v := range existing.Properties {
			props[k] = v
		}
	}
	for k, v := range body.Properties {
		switch k {
		case "loggerType", "description", "resourceId":
			if _, valid := v.(string); !valid {
				writeAzureError(w, 400, "ValidationError", k+" must be a string.")
				return
			}
		case "isBuffered":
			if _, valid := v.(bool); !valid {
				writeAzureError(w, 400, "ValidationError", "isBuffered must be a boolean.")
				return
			}
		case "credentials":
			values, valid := v.(map[string]any)
			if !valid {
				writeAzureError(w, 400, "ValidationError", "credentials must be an object.")
				return
			}
			for key, value := range values {
				if key != "instrumentationKey" && key != "connectionString" {
					writeAzureError(w, 501, "NotImplemented", "Logger credential "+key+" is not implemented.")
					return
				}
				if s, valid := value.(string); !valid || strings.TrimSpace(s) == "" {
					writeAzureError(w, 400, "ValidationError", "Logger credentials must be nonempty strings.")
					return
				}
			}
		default:
			writeAzureError(w, 501, "NotImplemented", "Logger property "+k+" is not implemented.")
			return
		}
		props[k] = v
	}
	if props["loggerType"] != "applicationInsights" {
		writeAzureError(w, 501, "NotImplemented", "Only Application Insights loggers are implemented.")
		return
	}
	credentials, _ := props["credentials"].(map[string]any)
	if len(credentials) == 0 {
		writeAzureError(w, 400, "ValidationError", "instrumentationKey or connectionString is required.")
		return
	}
	if resourceID, _ := props["resourceId"].(string); resourceID != "" {
		dependency, found := a.store.Get(resourceID)
		if !found || dependency.Type != applicationInsightsType {
			writeAzureError(w, 400, "ValidationError", "resourceId must reference an existing Application Insights component.")
			return
		}
	}
	res := &store.Resource{ID: id, Name: chi.URLParam(r, "loggerName"), Type: apimLoggerType, Properties: props}
	if err := a.store.Put(id, res); err != nil {
		log.Error().Err(err).Str("resource_id", id).Msg("failed to store APIM logger")
		writeAzureError(w, 500, "InternalServerError", "Failed to store logger.")
		return
	}
	status := http.StatusCreated
	if exists {
		status = http.StatusOK
	}
	log.Info().Str("resource_id", id).Bool("existed", exists).Msg("APIM logger upsert")
	writeJSON(w, status, apimAPIResponse(res))
}

func (a *Router) getAPIMLogger(w http.ResponseWriter, r *http.Request) {
	res, ok := a.store.Get(apimRequestID(r) + "/loggers/" + chi.URLParam(r, "loggerName"))
	if !ok {
		writeAzureError(w, 404, "ResourceNotFound", "Logger was not found.")
		return
	}
	writeJSON(w, 200, apimAPIResponse(res))
}

func (a *Router) headAPIMLogger(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.store.Get(apimRequestID(r) + "/loggers/" + chi.URLParam(r, "loggerName")); !ok {
		w.WriteHeader(404)
		return
	}
	w.WriteHeader(204)
}

func (a *Router) deleteAPIMLogger(w http.ResponseWriter, r *http.Request) {
	id := apimRequestID(r) + "/loggers/" + chi.URLParam(r, "loggerName")
	for _, child := range a.store.List(apimRequestID(r) + "/apis/") {
		loggerID, _ := child.Properties["loggerId"].(string)
		if child.Type == apimAPIDiagnosticType && strings.EqualFold(loggerID, id) {
			writeAzureError(w, 409, "ResourceInUse", "Logger is referenced by an API diagnostic.")
			return
		}
	}
	a.store.Delete(id)
	log.Info().Str("resource_id", id).Msg("APIM logger deleted")
	// Logger Delete 2022-08-01 accepts only 200/204.
	w.WriteHeader(http.StatusNoContent)
}

func (a *Router) listAPIMLoggers(w http.ResponseWriter, r *http.Request) {
	items := []map[string]any{}
	for _, res := range a.store.List(apimRequestID(r) + "/loggers/") {
		if res.Type == apimLoggerType {
			items = append(items, apimAPIResponse(res))
		}
	}
	writeJSON(w, 200, map[string]any{"value": items})
}

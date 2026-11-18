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

const apimAPIDiagnosticType = apimAPIType + "/diagnostics"

func (a *Router) apimAPIDiagnosticRoutes(r chi.Router) {
	base := "/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.apimanagement/service/{serviceName}/apis/{apiName}/diagnostics"
	r.Get(base, a.listAPIMAPIDiagnostics)
	r.Put(base+"/{diagnosticName}", a.putAPIMAPIDiagnostic)
	r.Get(base+"/{diagnosticName}", a.getAPIMAPIDiagnostic)
	r.Head(base+"/{diagnosticName}", a.headAPIMAPIDiagnostic)
	r.Delete(base+"/{diagnosticName}", a.deleteAPIMAPIDiagnostic)
}

func validateAPIMDiagnosticPipeline(value any) error {
	pipeline, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("pipeline must be an object")
	}
	for direction, raw := range pipeline {
		if direction != "request" && direction != "response" {
			return fmt.Errorf("pipeline field %s is not implemented", direction)
		}
		message, ok := raw.(map[string]any)
		if !ok {
			return fmt.Errorf("message must be an object")
		}
		for key, v := range message {
			switch key {
			case "body":
				body, ok := v.(map[string]any)
				if !ok || len(body) != 1 {
					return fmt.Errorf("body must contain bytes")
				}
				n, ok := body["bytes"].(float64)
				if !ok || n < 0 || n > 8192 || n != float64(int(n)) {
					return fmt.Errorf("body.bytes must be an integer from 0 to 8192")
				}
			case "headers":
				values, ok := v.([]any)
				if !ok {
					return fmt.Errorf("headers must be an array")
				}
				for _, header := range values {
					if s, ok := header.(string); !ok || strings.TrimSpace(s) == "" {
						return fmt.Errorf("headers must contain nonempty strings")
					}
				}
			default:
				return fmt.Errorf("message field %s is not implemented", key)
			}
		}
	}
	return nil
}

func (a *Router) putAPIMAPIDiagnostic(w http.ResponseWriter, r *http.Request) {
	parent, ok := a.findAPIMAPI(r)
	if !ok {
		writeAzureError(w, 404, "ParentResourceNotFound", "API was not found.")
		return
	}
	name := chi.URLParam(r, "diagnosticName")
	if name != "applicationinsights" {
		writeAzureError(w, 501, "NotImplemented", "Only applicationinsights API diagnostics are implemented.")
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
	props := body.Properties
	loggerID, _ := props["loggerId"].(string)
	logger, exists := a.store.Get(loggerID)
	if !exists || logger.Type != apimLoggerType || !strings.EqualFold(logger.ID, apimRequestID(r)+"/loggers/"+logger.Name) {
		writeAzureError(w, 400, "ValidationError", "loggerId must reference a logger in this API Management service.")
		return
	}
	for key, value := range props {
		valid := true
		switch key {
		case "loggerId":
		case "alwaysLog":
			valid = value == "allErrors"
		case "verbosity":
			valid = value == "verbose" || value == "information" || value == "error"
		case "httpCorrelationProtocol":
			valid = value == "None" || value == "Legacy" || value == "W3C"
		case "operationNameFormat":
			valid = value == "Name" || value == "Url"
		case "logClientIp", "metrics":
			_, valid = value.(bool)
		case "sampling":
			sampling, ok := value.(map[string]any)
			n, number := sampling["percentage"].(float64)
			valid = ok && number && n >= 0 && n <= 100 && sampling["samplingType"] == "fixed" && len(sampling) == 2
		case "frontend", "backend":
			if err := validateAPIMDiagnosticPipeline(value); err != nil {
				writeAzureError(w, 400, "ValidationError", err.Error())
				return
			}
		default:
			writeAzureError(w, 501, "NotImplemented", "Diagnostic property "+key+" is not implemented.")
			return
		}
		if !valid {
			writeAzureError(w, 400, "ValidationError", "Invalid "+key+".")
			return
		}
	}
	if _, ok := props["operationNameFormat"]; !ok {
		props["operationNameFormat"] = "Name"
	}
	id := parent.ID + "/diagnostics/" + name
	_, exists = a.store.Get(id)
	res := &store.Resource{ID: id, Name: name, Type: apimAPIDiagnosticType, Properties: props}
	if err := a.store.Put(id, res); err != nil {
		log.Error().Err(err).Str("resource_id", id).Msg("failed to store API diagnostic")
		writeAzureError(w, 500, "InternalServerError", "Failed to store API diagnostic.")
		return
	}
	status := http.StatusCreated
	if exists {
		status = http.StatusOK
	}
	log.Info().Str("resource_id", id).Bool("existed", exists).Msg("APIM API diagnostic upsert")
	writeJSON(w, status, apimAPIResponse(res))
}

func (a *Router) getAPIMAPIDiagnostic(w http.ResponseWriter, r *http.Request) {
	if parent, ok := a.findAPIMAPI(r); ok {
		if res, ok := a.store.Get(parent.ID + "/diagnostics/" + chi.URLParam(r, "diagnosticName")); ok {
			writeJSON(w, 200, apimAPIResponse(res))
			return
		}
	}
	writeAzureError(w, 404, "ResourceNotFound", "API diagnostic was not found.")
}

func (a *Router) headAPIMAPIDiagnostic(w http.ResponseWriter, r *http.Request) {
	if parent, ok := a.findAPIMAPI(r); ok {
		if _, ok := a.store.Get(parent.ID + "/diagnostics/" + chi.URLParam(r, "diagnosticName")); ok {
			w.WriteHeader(204)
			return
		}
	}
	w.WriteHeader(404)
}

func (a *Router) deleteAPIMAPIDiagnostic(w http.ResponseWriter, r *http.Request) {
	if parent, ok := a.findAPIMAPI(r); ok {
		id := parent.ID + "/diagnostics/" + chi.URLParam(r, "diagnosticName")
		a.store.Delete(id)
		log.Info().Str("resource_id", id).Msg("APIM API diagnostic deleted")
	}
	// ApiDiagnostic Delete 2022-08-01 is synchronous.
	w.WriteHeader(http.StatusNoContent)
}

func (a *Router) listAPIMAPIDiagnostics(w http.ResponseWriter, r *http.Request) {
	items := []map[string]any{}
	if parent, ok := a.findAPIMAPI(r); ok {
		for _, res := range a.store.List(parent.ID + "/diagnostics/") {
			if res.Type == apimAPIDiagnosticType {
				items = append(items, apimAPIResponse(res))
			}
		}
	}
	writeJSON(w, 200, map[string]any{"value": items})
}

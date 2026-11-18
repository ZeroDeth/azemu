package arm

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"

	"github.com/zerodeth/azemu/internal/store"
)

const apimAPIOperationPolicyType = apimAPIOperationType + "/policies"

func (a *Router) apimAPIOperationPolicyRoutes(r chi.Router) {
	base := "/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.apimanagement/service/{serviceName}/apis/{apiName}/operations/{operationName}/policies"
	r.Put(base+"/policy", a.putAPIMAPIOperationPolicy)
	r.Get(base+"/policy", a.getAPIMAPIOperationPolicy)
	r.Head(base+"/policy", a.headAPIMAPIOperationPolicy)
	r.Delete(base+"/policy", a.deleteAPIMAPIOperationPolicy)
	r.Get(base, a.listAPIMAPIOperationPolicies)
}

func (a *Router) putAPIMAPIOperationPolicy(w http.ResponseWriter, r *http.Request) {
	parent, ok := a.findAPIMAPIOperation(r)
	if !ok {
		writeAzureError(w, 404, "ParentResourceNotFound", "API operation was not found.")
		return
	}
	var body struct {
		Properties map[string]any `json:"properties"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil || body.Properties == nil {
		writeAzureError(w, 400, "InvalidRequestContent", "A valid properties object is required.")
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		writeAzureError(w, 400, "InvalidRequestContent", "Exactly one JSON object is required.")
		return
	}
	for key := range body.Properties {
		if key != "format" && key != "value" {
			writeAzureError(w, 501, "NotImplemented", "Policy property "+key+" is not implemented.")
			return
		}
	}
	format := "xml"
	if raw, present := body.Properties["format"]; present {
		var valid bool
		format, valid = raw.(string)
		if !valid {
			writeAzureError(w, 400, "ValidationError", "format must be a string.")
			return
		}
	}
	switch format {
	case "xml", "rawxml":
	case "xml-link", "rawxml-link":
		writeAzureError(w, 501, "NotImplemented", "Linked policy documents are not implemented.")
		return
	default:
		writeAzureError(w, 400, "ValidationError", "Invalid policy content format.")
		return
	}
	value, valid := body.Properties["value"].(string)
	if !valid || strings.TrimSpace(value) == "" {
		writeAzureError(w, 400, "ValidationError", "value must be a nonempty string.")
		return
	}
	// API and operation policies share XML syntax, not execution or scope.
	if status, message := validateAPIMAPIPolicy(value); status != 0 {
		code := "ValidationError"
		if status == http.StatusNotImplemented {
			code = "NotImplemented"
		}
		writeAzureError(w, status, code, message)
		return
	}
	id := parent.ID + "/policies/policy"
	existing, exists := a.store.Get(id)
	if match := r.Header.Get("If-Match"); match != "" && (!exists || match != "*" && match != apimAPIPolicyETag(existing)) {
		writeAzureError(w, 412, "PreconditionFailed", "Policy ETag does not match.")
		return
	}
	// Preserve original bytes; XML re-encoding would mutate configured content.
	res := &store.Resource{ID: id, Name: "policy", Type: apimAPIOperationPolicyType, Properties: map[string]any{"value": value}}
	if err := a.store.Put(id, res); err != nil {
		log.Error().Err(err).Str("resource_id", id).Msg("failed to store operation policy")
		writeAzureError(w, 500, "InternalServerError", "Failed to store operation policy.")
		return
	}
	status := http.StatusCreated
	if exists {
		status = http.StatusOK
	}
	w.Header().Set("ETag", apimAPIPolicyETag(res))
	log.Info().Str("resource_id", id).Bool("existed", exists).Msg("APIM operation policy upsert")
	writeJSON(w, status, apimAPIResponse(res))
}

func (a *Router) findAPIMAPIOperationPolicy(r *http.Request) (*store.Resource, bool) {
	if parent, ok := a.findAPIMAPIOperation(r); ok {
		return a.store.Get(parent.ID + "/policies/policy")
	}
	return nil, false
}

func (a *Router) getAPIMAPIOperationPolicy(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	if _, present := query["effective"]; present {
		writeAzureError(w, 501, "NotImplemented", "Effective policy expansion is not implemented.")
		return
	}
	if formats, present := query["format"]; present && (len(formats) != 1 || formats[0] != "xml" && formats[0] != "rawxml") {
		writeAzureError(w, 400, "ValidationError", "Policy export format must be xml or rawxml.")
		return
	}
	if res, ok := a.findAPIMAPIOperationPolicy(r); ok {
		w.Header().Set("ETag", apimAPIPolicyETag(res))
		writeJSON(w, 200, apimAPIResponse(res))
		return
	}
	writeAzureError(w, 404, "ResourceNotFound", "Operation policy was not found.")
}

func (a *Router) headAPIMAPIOperationPolicy(w http.ResponseWriter, r *http.Request) {
	if res, ok := a.findAPIMAPIOperationPolicy(r); ok {
		w.Header().Set("ETag", apimAPIPolicyETag(res))
		w.WriteHeader(http.StatusOK) // ApiOperationPolicy GetEntityTag accepts 200.
		return
	}
	w.WriteHeader(http.StatusNotFound)
}

func (a *Router) deleteAPIMAPIOperationPolicy(w http.ResponseWriter, r *http.Request) {
	if res, ok := a.findAPIMAPIOperationPolicy(r); ok {
		if match := r.Header.Get("If-Match"); match != "" && match != "*" && match != apimAPIPolicyETag(res) {
			writeAzureError(w, 412, "PreconditionFailed", "Policy ETag does not match.")
			return
		}
		a.store.Delete(res.ID)
		log.Info().Str("resource_id", res.ID).Msg("APIM operation policy deleted")
	}
	// SDK Delete is synchronous and accepts 200/204, not generic ARM 202.
	w.WriteHeader(http.StatusNoContent)
}

func (a *Router) listAPIMAPIOperationPolicies(w http.ResponseWriter, r *http.Request) {
	parent, ok := a.findAPIMAPIOperation(r)
	if !ok {
		writeAzureError(w, 404, "ParentResourceNotFound", "API operation was not found.")
		return
	}
	items := []map[string]any{}
	if res, exists := a.store.Get(parent.ID + "/policies/policy"); exists {
		items = append(items, apimAPIResponse(res))
	}
	writeJSON(w, 200, map[string]any{"value": items})
}

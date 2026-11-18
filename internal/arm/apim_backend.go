package arm

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"

	"github.com/zerodeth/azemu/internal/store"
)

const apimBackendType = apimServiceType + "/backends"

func (a *Router) apimBackendRoutes(r chi.Router) {
	base := "/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.apimanagement/service/{serviceName}/backends"
	r.Get(base, a.listAPIMBackends)
	r.Put(base+"/{backendName}", a.putAPIMBackend)
	r.Patch(base+"/{backendName}", a.putAPIMBackend)
	r.Get(base+"/{backendName}", a.getAPIMBackend)
	r.Head(base+"/{backendName}", a.getAPIMBackend)
	r.Delete(base+"/{backendName}", a.deleteAPIMBackend)
}

func apimBackendID(r *http.Request) string {
	return apimRequestID(r) + "/backends/" + chi.URLParam(r, "backendName")
}

func (a *Router) putAPIMBackend(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.apimParent(w, r); !ok {
		return
	}
	id := apimBackendID(r)
	existing, exists := a.store.Get(id)
	if r.Method == http.MethodPatch && !exists {
		writeAzureError(w, http.StatusNotFound, "ResourceNotFound", "Backend was not found.")
		return
	}
	var body struct {
		Properties map[string]any `json:"properties"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil || body.Properties == nil {
		writeAzureError(w, http.StatusBadRequest, "InvalidRequestContent", "A valid properties object is required.")
		return
	}
	props := map[string]any{}
	if r.Method == http.MethodPatch {
		for key, value := range existing.Properties {
			props[key] = value
		}
	}
	for key, value := range body.Properties {
		switch key {
		case "url", "protocol", "description", "title", "resourceId":
			if _, valid := value.(string); !valid {
				writeAzureError(w, http.StatusBadRequest, "ValidationError", key+" must be a string.")
				return
			}
		case "tls":
			if value == nil {
				break
			}
			tls, valid := value.(map[string]any)
			if !valid {
				writeAzureError(w, http.StatusBadRequest, "ValidationError", "tls must be an object.")
				return
			}
			for field, setting := range tls {
				if field != "validateCertificateChain" && field != "validateCertificateName" {
					writeAzureError(w, http.StatusNotImplemented, "NotImplemented", "Backend TLS property "+field+" is not implemented.")
					return
				}
				if _, valid := setting.(bool); !valid {
					writeAzureError(w, http.StatusBadRequest, "ValidationError", "Backend TLS settings must be booleans.")
					return
				}
			}
		default:
			writeAzureError(w, http.StatusNotImplemented, "NotImplemented", "Backend property "+key+" is not implemented.")
			return
		}
		props[key] = value
	}
	if protocol, _ := props["protocol"].(string); protocol == "" {
		writeAzureError(w, http.StatusBadRequest, "ValidationError", "protocol is required.")
		return
	} else if protocol != "http" {
		writeAzureError(w, http.StatusNotImplemented, "NotImplemented", "Only HTTP backends are implemented.")
		return
	}
	rawURL, _ := props["url"].(string)
	endpoint, err := url.Parse(rawURL)
	if err != nil || endpoint.Hostname() == "" || endpoint.Scheme != "http" && endpoint.Scheme != "https" {
		writeAzureError(w, http.StatusBadRequest, "ValidationError", "url must be an absolute HTTP or HTTPS URL.")
		return
	}
	res := &store.Resource{
		ID: id, Name: chi.URLParam(r, "backendName"), Type: apimBackendType, Properties: props,
	}
	if err := a.store.Put(id, res); err != nil {
		log.Error().Err(err).Str("resource_id", id).Msg("failed to store APIM backend")
		writeAzureError(w, http.StatusInternalServerError, "InternalServerError", "Failed to store backend.")
		return
	}
	status := http.StatusCreated
	if exists {
		status = http.StatusOK
	}
	log.Info().Str("resource_id", id).Bool("existed", exists).Msg("APIM backend upsert")
	writeJSON(w, status, apimAPIResponse(res))
}

func (a *Router) getAPIMBackend(w http.ResponseWriter, r *http.Request) {
	res, exists := a.store.Get(apimBackendID(r))
	if !exists {
		writeAzureError(w, http.StatusNotFound, "ResourceNotFound", "Backend was not found.")
		return
	}
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	writeJSON(w, http.StatusOK, apimAPIResponse(res))
}

func (a *Router) deleteAPIMBackend(w http.ResponseWriter, r *http.Request) {
	id := apimBackendID(r)
	a.store.Delete(id)
	log.Info().Str("resource_id", id).Msg("APIM backend deleted")
	// Backend Delete 2024-05-01 accepts only 200/204.
	w.WriteHeader(http.StatusNoContent)
}

func (a *Router) listAPIMBackends(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.apimParent(w, r); !ok {
		return
	}
	items := []map[string]any{}
	for _, res := range a.store.List(apimRequestID(r) + "/backends/") {
		if strings.EqualFold(res.Type, apimBackendType) {
			items = append(items, apimAPIResponse(res))
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"value": items})
}

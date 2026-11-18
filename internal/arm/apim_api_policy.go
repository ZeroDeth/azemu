package arm

import (
	"crypto/sha256"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"

	"github.com/zerodeth/azemu/internal/store"
)

const apimAPIPolicyType = apimAPIType + "/policies"

func (a *Router) apimAPIPolicyRoutes(r chi.Router) {
	base := "/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.apimanagement/service/{serviceName}/apis/{apiName}/policies"
	r.Get(base, a.listAPIMAPIPolicies)
	// PolicyIdName has exactly one value. Do not accept arbitrary policy names
	// or accidentally implement service/operation policies with this route.
	r.Put(base+"/policy", a.putAPIMAPIPolicy)
	r.Get(base+"/policy", a.getAPIMAPIPolicy)
	r.Head(base+"/policy", a.headAPIMAPIPolicy)
	r.Delete(base+"/policy", a.deleteAPIMAPIPolicy)
}

// validateAPIMAPIPolicy validates the supported management-plane subset.
// No policy is executed. Other statements and .NET policy expressions need
// their own validation/encoding support and must not silently succeed.
func validateAPIMAPIPolicy(value string) (int, string) {
	decoder := xml.NewDecoder(strings.NewReader(value))
	depth, roots := 0, 0
	sections := map[string]bool{}
	section := ""
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			if strings.Contains(value, "@(") || strings.Contains(value, "@{") {
				return http.StatusNotImplemented, "Policy expressions are not implemented."
			}
			return http.StatusBadRequest, "value must be a well-formed XML policy document."
		}
		switch token := token.(type) {
		case xml.StartElement:
			if token.Name.Space != "" {
				return http.StatusNotImplemented, "Namespaced policies are not implemented."
			}
			attrs := map[string]string{}
			for _, attr := range token.Attr {
				if _, exists := attrs[attr.Name.Local]; exists {
					return http.StatusBadRequest, "Duplicate policy attribute."
				}
				if attr.Name.Space != "" || attr.Name.Local == "xmlns" {
					return http.StatusNotImplemented, "Namespaced policies are not implemented."
				}
				if strings.Contains(attr.Value, "@(") || strings.Contains(attr.Value, "@{") {
					return http.StatusNotImplemented, "Policy expressions are not implemented."
				}
				attrs[attr.Name.Local] = attr.Value
			}
			switch depth {
			case 0:
				roots++
				if roots != 1 || token.Name.Local != "policies" || len(attrs) != 0 {
					return http.StatusBadRequest, "A single policies root without attributes is required."
				}
			case 1:
				section = token.Name.Local
				switch section {
				case "inbound", "backend", "outbound", "on-error":
				default:
					return http.StatusBadRequest, "Invalid policy section."
				}
				if sections[section] || len(attrs) != 0 {
					return http.StatusBadRequest, "Policy sections must be unique and have no attributes."
				}
				sections[section] = true
			case 2:
				switch token.Name.Local {
				case "base":
					if len(attrs) != 0 {
						return http.StatusBadRequest, "base does not accept attributes."
					}
				case "find-and-replace":
					_, hasTo := attrs["to"]
					if attrs["from"] == "" || !hasTo || len(attrs) != 2 || section == "backend" {
						return http.StatusBadRequest, "find-and-replace requires from and to attributes and a non-backend section."
					}
				default:
					return http.StatusNotImplemented, "Policy statement " + token.Name.Local + " is not implemented."
				}
			default:
				return http.StatusBadRequest, "This policy statement cannot contain child elements."
			}
			depth++
		case xml.EndElement:
			depth--
		case xml.CharData:
			if strings.TrimSpace(string(token)) != "" {
				return http.StatusBadRequest, "Unexpected text in policy document."
			}
		case xml.Directive:
			return http.StatusBadRequest, "XML directives and DTDs are not allowed."
		case xml.ProcInst:
			if token.Target != "xml" || roots != 0 {
				return http.StatusBadRequest, "Unsupported XML processing instruction."
			}
		}
	}
	if roots != 1 || depth != 0 {
		return http.StatusBadRequest, "A policies document is required."
	}
	return 0, ""
}

func apimAPIPolicyETag(res *store.Resource) string {
	return fmt.Sprintf(`"%x"`, sha256.Sum256([]byte(res.Properties["value"].(string))))
}

func (a *Router) putAPIMAPIPolicy(w http.ResponseWriter, r *http.Request) {
	parent, ok := a.findAPIMAPI(r)
	if !ok {
		writeAzureError(w, http.StatusNotFound, "ParentResourceNotFound", "API was not found.")
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
	if err := decoder.Decode(new(any)); err != io.EOF {
		writeAzureError(w, http.StatusBadRequest, "InvalidRequestContent", "Exactly one JSON object is required.")
		return
	}
	for key := range body.Properties {
		if key != "format" && key != "value" {
			writeAzureError(w, http.StatusNotImplemented, "NotImplemented", "Policy property "+key+" is not implemented.")
			return
		}
	}
	// The ARM default is xml, not rawxml. Neither format means that the
	// entire document is HTML-escaped. xml requires XML-encoded contents;
	// rawxml additionally permits unescaped .NET expressions (unsupported).
	format := "xml"
	if raw, present := body.Properties["format"]; present {
		var valid bool
		format, valid = raw.(string)
		if !valid {
			writeAzureError(w, http.StatusBadRequest, "ValidationError", "format must be a string.")
			return
		}
	}
	switch format {
	case "xml", "rawxml":
	case "xml-link", "rawxml-link":
		writeAzureError(w, http.StatusNotImplemented, "NotImplemented", "Linked policy documents are not implemented.")
		return
	default:
		writeAzureError(w, http.StatusBadRequest, "ValidationError", "Invalid policy content format.")
		return
	}
	value, valid := body.Properties["value"].(string)
	if !valid || strings.TrimSpace(value) == "" {
		writeAzureError(w, http.StatusBadRequest, "ValidationError", "value must be a nonempty string.")
		return
	}
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
		writeAzureError(w, http.StatusPreconditionFailed, "PreconditionFailed", "Policy ETag does not match.")
		return
	}
	// Preserve the original XML, including indentation, line endings and
	// entities. Re-marshalling XML would change Terraform's configured value.
	res := &store.Resource{ID: id, Name: "policy", Type: apimAPIPolicyType, Properties: map[string]any{"value": value}}
	if err := a.store.Put(id, res); err != nil {
		log.Error().Err(err).Str("resource_id", id).Msg("failed to store API policy")
		writeAzureError(w, http.StatusInternalServerError, "InternalServerError", "Failed to store API policy.")
		return
	}
	status := http.StatusCreated
	if exists {
		status = http.StatusOK
	}
	w.Header().Set("ETag", apimAPIPolicyETag(res))
	log.Info().Str("resource_id", id).Bool("existed", exists).Msg("APIM API policy upsert")
	writeJSON(w, status, apimAPIResponse(res))
}

func (a *Router) getAPIMAPIPolicy(w http.ResponseWriter, r *http.Request) {
	// The SDK always sends format=xml. rawxml is equivalent for our
	// well-formed XML subset; unsupported export/effective formats must fail.
	query := r.URL.Query()
	if _, present := query["effective"]; present {
		writeAzureError(w, http.StatusNotImplemented, "NotImplemented", "Effective policy expansion is not implemented.")
		return
	}
	if formats, present := query["format"]; present && (len(formats) != 1 || formats[0] != "xml" && formats[0] != "rawxml") {
		writeAzureError(w, http.StatusBadRequest, "ValidationError", "Policy export format must be xml or rawxml.")
		return
	}
	if parent, ok := a.findAPIMAPI(r); ok {
		if res, exists := a.store.Get(parent.ID + "/policies/policy"); exists {
			w.Header().Set("ETag", apimAPIPolicyETag(res))
			writeJSON(w, http.StatusOK, apimAPIResponse(res))
			return
		}
	}
	writeAzureError(w, http.StatusNotFound, "ResourceNotFound", "API policy was not found.")
}

func (a *Router) headAPIMAPIPolicy(w http.ResponseWriter, r *http.Request) {
	if parent, ok := a.findAPIMAPI(r); ok {
		if res, exists := a.store.Get(parent.ID + "/policies/policy"); exists {
			w.Header().Set("ETag", apimAPIPolicyETag(res))
			// ApiPolicy GetEntityTag 2022-08-01 accepts only 200, not 204.
			w.WriteHeader(http.StatusOK)
			return
		}
	}
	w.WriteHeader(http.StatusNotFound)
}

func (a *Router) deleteAPIMAPIPolicy(w http.ResponseWriter, r *http.Request) {
	if parent, ok := a.findAPIMAPI(r); ok {
		id := parent.ID + "/policies/policy"
		if res, exists := a.store.Get(id); exists {
			if match := r.Header.Get("If-Match"); match != "" && match != "*" && match != apimAPIPolicyETag(res) {
				writeAzureError(w, http.StatusPreconditionFailed, "PreconditionFailed", "Policy ETag does not match.")
				return
			}
			a.store.Delete(id)
			log.Info().Str("resource_id", id).Msg("APIM API policy deleted")
		}
	}
	// ApiPolicy Delete 2022-08-01 accepts only synchronous 200/204.
	w.WriteHeader(http.StatusNoContent)
}

func (a *Router) listAPIMAPIPolicies(w http.ResponseWriter, r *http.Request) {
	parent, ok := a.findAPIMAPI(r)
	if !ok {
		writeAzureError(w, http.StatusNotFound, "ParentResourceNotFound", "API was not found.")
		return
	}
	items := []map[string]any{}
	if res, exists := a.store.Get(parent.ID + "/policies/policy"); exists {
		items = append(items, apimAPIResponse(res))
	}
	writeJSON(w, http.StatusOK, map[string]any{"value": items})
}

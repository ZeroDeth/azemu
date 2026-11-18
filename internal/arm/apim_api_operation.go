package arm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"

	"github.com/zerodeth/azemu/internal/store"
)

const apimAPIOperationType = apimAPIType + "/operations"

func (a *Router) apimAPIOperationRoutes(r chi.Router) {
	base := "/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.apimanagement/service/{serviceName}/apis/{apiName}/operations"
	r.Get(base, a.listAPIMAPIOperations)
	r.Put(base+"/{operationName}", a.putAPIMAPIOperation)
	r.Get(base+"/{operationName}", a.getAPIMAPIOperation)
	r.Head(base+"/{operationName}", a.headAPIMAPIOperation)
	r.Delete(base+"/{operationName}", a.deleteAPIMAPIOperation)
}

// These private contracts validate the 2022-08-01 SDK payload. Store the
// original properties map, not a re-encoded struct, to preserve empty arrays,
// false required flags and arbitrary JSON example values exactly.
type apimOperationExample struct {
	Description   *string `json:"description,omitempty"`
	ExternalValue *string `json:"externalValue,omitempty"`
	Summary       *string `json:"summary,omitempty"`
	Value         any     `json:"value,omitempty"`
}

type apimOperationParameter struct {
	Name         string                          `json:"name"`
	Type         string                          `json:"type"`
	Required     *bool                           `json:"required,omitempty"`
	Description  *string                         `json:"description,omitempty"`
	DefaultValue *string                         `json:"defaultValue,omitempty"`
	SchemaID     *string                         `json:"schemaId,omitempty"`
	TypeName     *string                         `json:"typeName,omitempty"`
	Values       []string                        `json:"values,omitempty"`
	Examples     map[string]apimOperationExample `json:"examples,omitempty"`
}

type apimOperationRepresentation struct {
	ContentType    string                          `json:"contentType"`
	SchemaID       *string                         `json:"schemaId,omitempty"`
	TypeName       *string                         `json:"typeName,omitempty"`
	FormParameters []apimOperationParameter        `json:"formParameters,omitempty"`
	Examples       map[string]apimOperationExample `json:"examples,omitempty"`
}

type apimOperationRequest struct {
	Description     *string                       `json:"description,omitempty"`
	Headers         []apimOperationParameter      `json:"headers,omitempty"`
	QueryParameters []apimOperationParameter      `json:"queryParameters,omitempty"`
	Representations []apimOperationRepresentation `json:"representations,omitempty"`
}

type apimOperationResponse struct {
	StatusCode      *int64                        `json:"statusCode"`
	Description     *string                       `json:"description,omitempty"`
	Headers         []apimOperationParameter      `json:"headers,omitempty"`
	Representations []apimOperationRepresentation `json:"representations,omitempty"`
}

type apimOperationProperties struct {
	DisplayName        string                   `json:"displayName"`
	Method             string                   `json:"method"`
	URLTemplate        string                   `json:"urlTemplate"`
	Description        *string                  `json:"description,omitempty"`
	Request            *apimOperationRequest    `json:"request,omitempty"`
	Responses          []apimOperationResponse  `json:"responses,omitempty"`
	TemplateParameters []apimOperationParameter `json:"templateParameters,omitempty"`
}

func validateAPIMOperationParameters(parameters []apimOperationParameter) error {
	for _, parameter := range parameters {
		if strings.TrimSpace(parameter.Name) == "" || strings.TrimSpace(parameter.Type) == "" {
			return fmt.Errorf("parameter name and type are required")
		}
	}
	return nil
}

func validateAPIMOperationRepresentations(representations []apimOperationRepresentation) error {
	for _, representation := range representations {
		if strings.TrimSpace(representation.ContentType) == "" {
			return fmt.Errorf("representation contentType is required")
		}
		if err := validateAPIMOperationParameters(representation.FormParameters); err != nil {
			return err
		}
	}
	return nil
}

var apimOperationTemplateParameter = regexp.MustCompile(`\{(\*?[^{}]+)\}`)
var apimOperationMethod = regexp.MustCompile("^[!#$%&'*+.^_`|~0-9A-Za-z-]+$")
var apimOperationName = regexp.MustCompile(`^[^*#&+:<>? /\\]+$`)

func validateAPIMOperation(props apimOperationProperties) error {
	if strings.TrimSpace(props.DisplayName) == "" || utf8.RuneCountInString(props.DisplayName) > 300 {
		return fmt.Errorf("displayName must contain 1 to 300 characters")
	}
	if !apimOperationMethod.MatchString(props.Method) {
		return fmt.Errorf("method must be a nonempty HTTP token")
	}
	if !strings.HasPrefix(props.URLTemplate, "/") || utf8.RuneCountInString(props.URLTemplate) > 1000 {
		return fmt.Errorf("urlTemplate must start with / and contain at most 1000 characters")
	}
	if props.Description != nil && utf8.RuneCountInString(*props.Description) > 1000 {
		return fmt.Errorf("description must contain at most 1000 characters")
	}
	if err := validateAPIMOperationParameters(props.TemplateParameters); err != nil {
		return err
	}
	names := map[string]bool{}
	for _, parameter := range props.TemplateParameters {
		if names[parameter.Name] {
			return fmt.Errorf("duplicate template parameter")
		}
		names[parameter.Name] = true
	}
	used := map[string]bool{}
	for _, match := range apimOperationTemplateParameter.FindAllStringSubmatch(props.URLTemplate, -1) {
		name := strings.TrimPrefix(match[1], "*")
		if !names[name] {
			return fmt.Errorf("urlTemplate parameter %s is not defined", name)
		}
		used[name] = true
	}
	if len(used) != len(names) {
		return fmt.Errorf("templateParameters must match urlTemplate")
	}
	if request := props.Request; request != nil {
		if err := validateAPIMOperationParameters(append(append([]apimOperationParameter{}, request.Headers...), request.QueryParameters...)); err != nil {
			return err
		}
		if err := validateAPIMOperationRepresentations(request.Representations); err != nil {
			return err
		}
	}
	for _, response := range props.Responses {
		if response.StatusCode == nil || (*response.StatusCode != 0 && (*response.StatusCode < 100 || *response.StatusCode > 599)) {
			return fmt.Errorf("response statusCode must be 0 (default) or 100 to 599")
		}
		if err := validateAPIMOperationParameters(response.Headers); err != nil {
			return err
		}
		if err := validateAPIMOperationRepresentations(response.Representations); err != nil {
			return err
		}
	}
	return nil
}

func (a *Router) putAPIMAPIOperation(w http.ResponseWriter, r *http.Request) {
	parent, ok := a.findAPIMAPI(r)
	if !ok {
		writeAzureError(w, http.StatusNotFound, "ParentResourceNotFound", "API was not found.")
		return
	}
	name := chi.URLParam(r, "operationName")
	if !apimOperationName.MatchString(name) || utf8.RuneCountInString(name) > 80 {
		writeAzureError(w, http.StatusBadRequest, "ValidationError", "Invalid operation name.")
		return
	}
	var body struct {
		Properties json.RawMessage `json:"properties"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil || len(body.Properties) == 0 || bytes.Equal(body.Properties, []byte("null")) {
		writeAzureError(w, http.StatusBadRequest, "InvalidRequestContent", "A JSON object with properties is required.")
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		writeAzureError(w, http.StatusBadRequest, "InvalidRequestContent", "Only one JSON object is allowed.")
		return
	}
	var contract apimOperationProperties
	decoder = json.NewDecoder(bytes.NewReader(body.Properties))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&contract); err != nil {
		writeAzureError(w, http.StatusBadRequest, "InvalidRequestContent", err.Error())
		return
	}
	if err := validateAPIMOperation(contract); err != nil {
		writeAzureError(w, http.StatusBadRequest, "ValidationError", err.Error())
		return
	}
	var props map[string]any
	if err := json.Unmarshal(body.Properties, &props); err != nil {
		writeAzureError(w, http.StatusBadRequest, "InvalidRequestContent", err.Error())
		return
	}
	props["_explicitOperation"] = true
	id := parent.ID + "/operations/" + name
	_, exists := a.store.Get(id)
	res := &store.Resource{ID: id, Name: name, Type: apimAPIOperationType, Location: parent.Location, Properties: props}
	if err := a.store.Put(id, res); err != nil {
		log.Error().Err(err).Str("resource_id", id).Msg("failed to store API operation")
		writeAzureError(w, http.StatusInternalServerError, "InternalServerError", "Failed to store API operation.")
		return
	}
	log.Info().Str("resource_id", id).Bool("existed", exists).Msg("API Management operation upsert")
	status := http.StatusCreated
	if exists {
		status = http.StatusOK
	}
	// OperationContract is an APIM child contract, not a tracked resource:
	// no location, tags or provisioningState appear in the SDK response.
	writeJSON(w, status, apimAPIResponse(res))
}

func (a *Router) findAPIMAPIOperation(r *http.Request) (*store.Resource, bool) {
	if parent, ok := a.findAPIMAPI(r); ok {
		return a.store.Get(parent.ID + "/operations/" + chi.URLParam(r, "operationName"))
	}
	return nil, false
}

func (a *Router) getAPIMAPIOperation(w http.ResponseWriter, r *http.Request) {
	if res, ok := a.findAPIMAPIOperation(r); ok {
		writeJSON(w, http.StatusOK, apimAPIResponse(res))
		return
	}
	writeAzureError(w, http.StatusNotFound, "ResourceNotFound", "API operation was not found.")
}

func (a *Router) headAPIMAPIOperation(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.findAPIMAPIOperation(r); !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *Router) deleteAPIMAPIOperation(w http.ResponseWriter, r *http.Request) {
	if res, ok := a.findAPIMAPIOperation(r); ok {
		a.store.Delete(res.ID)
		log.Info().Str("resource_id", res.ID).Msg("API Management operation deleted")
	}
	// APIM 2022-08-01 operation Delete accepts only 200/204, not 202.
	w.WriteHeader(http.StatusNoContent)
}

func (a *Router) listAPIMAPIOperations(w http.ResponseWriter, r *http.Request) {
	parent, ok := a.findAPIMAPI(r)
	if !ok {
		writeAzureError(w, http.StatusNotFound, "ParentResourceNotFound", "API was not found.")
		return
	}
	items := []map[string]any{}
	for _, res := range a.store.List(parent.ID + "/operations/") {
		if res.Type == apimAPIOperationType {
			items = append(items, apimAPIResponse(res))
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i]["id"].(string) < items[j]["id"].(string) })
	writeJSON(w, http.StatusOK, map[string]any{"value": items})
}

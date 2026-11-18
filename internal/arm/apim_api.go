package arm

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"

	"github.com/zerodeth/azemu/internal/store"
)

const apimAPIType = apimServiceType + "/apis"

func (a *Router) apimAPIRoutes(r chi.Router) {
	base := "/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.apimanagement/service/{serviceName}/apis"
	r.Get(base, a.listAPIMAPIs)
	r.Put(base+"/{apiName}", a.putAPIMAPI)
	r.Get(base+"/{apiName}", a.getAPIMAPI)
	r.Head(base+"/{apiName}", a.headAPIMAPI)
	r.Delete(base+"/{apiName}", a.deleteAPIMAPI)
	r.Get(base+"/{apiName}/revisions", a.listAPIMAPIRevisions)
	r.Get(base+"/{apiName}/schemas", a.listAPIMAPIImportedChildren)
	r.Get(base+"/{apiName}/schemas/{importedName}", a.getAPIMAPIImportedChild)
}

func apimAPIName(name string) (string, string, bool) {
	parts := strings.Split(name, ";rev=")
	if len(parts) == 1 && parts[0] != "" {
		return parts[0], "", true
	}
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	for _, c := range parts[1] {
		if c < '0' || c > '9' {
			return "", "", false
		}
	}
	return parts[0], parts[1], true
}

func apimAPIResponse(res *store.Resource) map[string]any {
	props := map[string]any{}
	for key, value := range res.Properties {
		if !strings.HasPrefix(key, "_") {
			props[key] = value
		}
	}
	// APIM API contracts are child resources, with no top-level location/tags.
	return map[string]any{"id": res.ID, "name": res.Name, "type": res.Type, "properties": props}
}

func (a *Router) findAPIMAPI(r *http.Request) (*store.Resource, bool) {
	name, revision, valid := apimAPIName(chi.URLParam(r, "apiName"))
	if !valid {
		return nil, false
	}
	prefix := apimRequestID(r) + "/apis/" + name
	if revision != "" {
		return a.store.Get(prefix + ";rev=" + revision)
	}
	for _, res := range a.store.List(prefix + ";rev=") {
		if res.Type == apimAPIType && res.Properties["isCurrent"] == true {
			return res, true
		}
	}
	return nil, false
}

func (a *Router) putAPIMAPI(w http.ResponseWriter, r *http.Request) {
	parent, ok := a.apimParent(w, r)
	if !ok {
		return
	}
	name, revision, valid := apimAPIName(chi.URLParam(r, "apiName"))
	if !valid {
		writeAzureError(w, http.StatusBadRequest, "ValidationError", "Invalid API name or revision.")
		return
	}
	var body struct {
		Properties map[string]any `json:"properties"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil || body.Properties == nil {
		writeAzureError(w, http.StatusBadRequest, "InvalidRequestContent", "A JSON object with properties is required.")
		return
	}
	incoming := body.Properties
	// Reject features that require other APIM resources instead of accepting
	// and then silently discarding their configured semantics.
	for key, value := range incoming {
		switch key {
		case "path", "displayName", "description", "serviceUrl", "apiRevision", "apiRevisionDescription",
			"apiVersionDescription", "termsOfServiceUrl", "type", "apiType", "format", "value":
			if _, ok := value.(string); !ok {
				writeAzureError(w, http.StatusBadRequest, "ValidationError", key+" must be a string.")
				return
			}
		case "subscriptionRequired", "isCurrent", "isOnline":
			if _, ok := value.(bool); !ok {
				writeAzureError(w, http.StatusBadRequest, "ValidationError", key+" must be a boolean.")
				return
			}
		case "protocols":
			values, ok := value.([]any)
			if !ok || len(values) == 0 {
				writeAzureError(w, http.StatusBadRequest, "ValidationError", "protocols must be a nonempty array.")
				return
			}
			for _, protocol := range values {
				if protocol != "http" && protocol != "https" {
					writeAzureError(w, http.StatusBadRequest, "ValidationError", "Only http and https protocols are supported.")
					return
				}
			}
		case "contact", "license", "subscriptionKeyParameterNames", "authenticationSettings":
			object, ok := value.(map[string]any)
			if value == nil && (key == "contact" || key == "license") {
				continue
			}
			if !ok {
				writeAzureError(w, http.StatusBadRequest, "ValidationError", key+" must be an object.")
				return
			}
			if key == "authenticationSettings" && len(object) != 0 {
				writeAzureError(w, http.StatusNotImplemented, "NotImplemented", "API authentication providers are not implemented.")
				return
			}
			allowed := map[string]string{
				"contact": " name email url ", "license": " name url ",
				"subscriptionKeyParameterNames": " header query ",
			}
			for field, v := range object {
				if _, ok := v.(string); !ok || !strings.Contains(allowed[key], " "+field+" ") {
					writeAzureError(w, http.StatusBadRequest, "ValidationError", "Invalid "+key+" field: "+field)
					return
				}
			}
		default:
			writeAzureError(w, http.StatusNotImplemented, "NotImplemented", "API property "+key+" is not implemented.")
			return
		}
	}
	if _, ok := incoming["path"]; !ok {
		writeAzureError(w, http.StatusBadRequest, "ValidationError", "properties.path is required (and may be empty).")
		return
	}
	for _, key := range []string{"type", "apiType"} {
		if value, ok := incoming[key]; ok && value != "http" {
			writeAzureError(w, http.StatusNotImplemented, "NotImplemented", "Only HTTP APIs are implemented.")
			return
		}
	}
	if revision == "" {
		if current, exists := a.findAPIMAPI(r); exists {
			revision, _ = current.Properties["apiRevision"].(string)
		} else {
			revision = "1"
		}
	}
	if requested, ok := incoming["apiRevision"]; ok && requested != revision {
		writeAzureError(w, http.StatusBadRequest, "ValidationError", "apiRevision must match the URI revision.")
		return
	}
	id := parent.ID + "/apis/" + name + ";rev=" + revision
	existing, exists := a.store.Get(id)
	props := map[string]any{
		"apiRevision": revision, "type": "http", "isOnline": true,
		"subscriptionRequired": true, "authenticationSettings": map[string]any{},
		"subscriptionKeyParameterNames": map[string]any{"header": "Ocp-Apim-Subscription-Key", "query": "subscription-key"},
	}
	currentExists := false
	for _, sibling := range a.store.List(parent.ID + "/apis/" + name + ";rev=") {
		if sibling.Type == apimAPIType && sibling.Properties["isCurrent"] == true {
			currentExists = true
		}
	}
	props["isCurrent"] = !currentExists
	if exists {
		for key, value := range existing.Properties {
			props[key] = value
		}
	}
	if requested, ok := incoming["isCurrent"]; ok && requested != props["isCurrent"] {
		writeAzureError(w, http.StatusNotImplemented, "NotImplemented", "Changing the current revision requires API releases, which are not implemented.")
		return
	}
	var imported *apimSwaggerImport
	if format, present := incoming["format"]; present && format == "" {
		writeAzureError(w, http.StatusBadRequest, "ValidationError", "Import format must not be empty.")
		return
	}
	if format, _ := incoming["format"].(string); format != "" {
		if format != "swagger-json" && format != "swagger-link-json" {
			writeAzureError(w, http.StatusNotImplemented, "NotImplemented", "Only swagger-json and swagger-link-json imports are implemented.")
			return
		}
		var err error
		imported, err = importAPIMSwagger(r.Context(), format, incoming["value"])
		if err != nil {
			writeAzureError(w, http.StatusBadRequest, "ValidationError", err.Error())
			return
		}
		for key, value := range imported.properties {
			props[key] = value
		}
	} else if _, ok := incoming["value"]; ok {
		writeAzureError(w, http.StatusBadRequest, "ValidationError", "format is required with value.")
		return
	}
	if imported == nil {
		// Normal PUT replaces the optional contact/license contracts. In
		// AzureRM 5.14 resourceApiManagementApiUpdate, removing either block
		// sets its SDK pointer to nil; json:",omitempty" then omits it from
		// the PUT. Retaining these objects on omission makes removal
		// impossible. Import PUTs instead initialize them from info.
		// Optional strings are different: the same provider sends a
		// non-nil pointer to "" to clear description/termsOfServiceUrl.
		for _, key := range []string{"contact", "license"} {
			delete(props, key)
		}
	}
	for key, value := range incoming {
		if key != "format" && key != "value" && key != "apiType" {
			if value == nil && (key == "contact" || key == "license") {
				delete(props, key)
			} else {
				props[key] = value
			}
		}
	}
	if display, _ := props["displayName"].(string); display == "" || props["protocols"] == nil {
		writeAzureError(w, http.StatusBadRequest, "ValidationError", "displayName and protocols are required without an import.")
		return
	}
	// Explicit properties win over import-derived values. Do not rebuild
	// metadata from _importDocument on ordinary PUTs or reads: doing so
	// resurrects values which the caller has subsequently cleared.
	res := &store.Resource{ID: id, Name: name + ";rev=" + revision, Type: apimAPIType, Location: parent.Location, Properties: props}
	if err := a.store.Put(id, res); err != nil {
		log.Error().Err(err).Str("resource_id", id).Msg("failed to store API Management API")
		writeAzureError(w, http.StatusInternalServerError, "InternalServerError", "Failed to store API.")
		return
	}
	if imported != nil {
		if err := a.saveAPIMSwagger(id, imported); err != nil {
			log.Error().Err(err).Str("resource_id", id).Msg("failed to store imported API definitions")
			writeAzureError(w, http.StatusInternalServerError, "InternalServerError", "Failed to store imported API definitions.")
			return
		}
	}
	log.Info().Str("resource_id", id).Bool("existed", exists).Msg("API Management API upsert")
	status := http.StatusCreated
	if exists {
		status = http.StatusOK
	}
	writeJSON(w, status, apimAPIResponse(res))
}

func (a *Router) getAPIMAPI(w http.ResponseWriter, r *http.Request) {
	res, ok := a.findAPIMAPI(r)
	if !ok {
		writeAzureError(w, http.StatusNotFound, "ResourceNotFound", "API was not found.")
		return
	}
	writeJSON(w, http.StatusOK, apimAPIResponse(res))
}

func (a *Router) headAPIMAPI(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.findAPIMAPI(r); !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *Router) deleteAPIMAPI(w http.ResponseWriter, r *http.Request) {
	res, ok := a.findAPIMAPI(r)
	if !ok {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	all := r.URL.Query().Get("deleteRevisions")
	if all != "" && all != "true" && all != "false" {
		writeAzureError(w, http.StatusBadRequest, "ValidationError", "deleteRevisions must be a boolean.")
		return
	}
	if all == "true" {
		name, _, _ := apimAPIName(res.Name)
		for _, revision := range a.store.List(apimRequestID(r) + "/apis/" + name + ";rev=") {
			if revision.Type == apimAPIType {
				a.store.Delete(revision.ID)
			}
		}
	} else {
		a.store.Delete(res.ID)
	}
	log.Info().Str("resource_id", res.ID).Msg("API Management API deleted")
	// APIM 2022-08-01 Delete is synchronous: the exact 5.14 SDK accepts
	// only 200/204, not the generic ARM async-delete response.
	w.WriteHeader(http.StatusNoContent)
}

func (a *Router) listAPIMAPIs(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.apimParent(w, r); !ok {
		return
	}
	items := []map[string]any{}
	for _, res := range a.store.List(apimRequestID(r) + "/apis/") {
		if res.Type == apimAPIType {
			items = append(items, apimAPIResponse(res))
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i]["id"].(string) < items[j]["id"].(string) })
	writeJSON(w, http.StatusOK, map[string]any{"value": items})
}

func (a *Router) listAPIMAPIRevisions(w http.ResponseWriter, r *http.Request) {
	name, _, valid := apimAPIName(chi.URLParam(r, "apiName"))
	if !valid {
		writeAzureError(w, http.StatusBadRequest, "ValidationError", "Invalid API name.")
		return
	}
	items := []map[string]any{}
	for _, res := range a.store.List(apimRequestID(r) + "/apis/" + name + ";rev=") {
		if res.Type == apimAPIType {
			items = append(items, map[string]any{
				"apiId": res.ID, "apiRevision": res.Properties["apiRevision"],
				"isCurrent": res.Properties["isCurrent"], "isOnline": res.Properties["isOnline"],
				"description": res.Properties["apiRevisionDescription"],
			})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"value": items})
}

func (a *Router) listAPIMAPIImportedChildren(w http.ResponseWriter, r *http.Request) {
	res, ok := a.findAPIMAPI(r)
	if !ok {
		writeAzureError(w, http.StatusNotFound, "ResourceNotFound", "API was not found.")
		return
	}
	collection := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
	items := []map[string]any{}
	for _, child := range a.store.List(res.ID + "/" + collection + "/") {
		items = append(items, apimAPIResponse(child))
	}
	writeJSON(w, http.StatusOK, map[string]any{"value": items})
}

func (a *Router) getAPIMAPIImportedChild(w http.ResponseWriter, r *http.Request) {
	res, ok := a.findAPIMAPI(r)
	if ok {
		parts := strings.Split(r.URL.Path, "/")
		id := fmt.Sprintf("%s/%s/%s", res.ID, parts[len(parts)-2], chi.URLParam(r, "importedName"))
		if child, exists := a.store.Get(id); exists {
			writeJSON(w, http.StatusOK, apimAPIResponse(child))
			return
		}
	}
	writeAzureError(w, http.StatusNotFound, "ResourceNotFound", "Imported API resource was not found.")
}

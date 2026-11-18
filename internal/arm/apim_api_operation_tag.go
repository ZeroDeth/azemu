package arm

import (
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"

	"github.com/zerodeth/azemu/internal/store"
)

const apimAPIOperationTagType = apimAPIOperationType + "/tags"

func (a *Router) apimAPIOperationTagRoutes(r chi.Router) {
	base := "/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.apimanagement/service/{serviceName}/apis/{apiName}/operations/{operationName}/tags"
	r.Put(base+"/{tagName}", a.putAPIMAPIOperationTag)
	r.Get(base+"/{tagName}", a.getAPIMAPIOperationTag)
	r.Head(base+"/{tagName}", a.headAPIMAPIOperationTag)
	r.Delete(base+"/{tagName}", a.deleteAPIMAPIOperationTag)
	r.Get(base, a.listAPIMAPIOperationTags)
}

func (a *Router) putAPIMAPIOperationTag(w http.ResponseWriter, r *http.Request) {
	parent, ok := a.findAPIMAPIOperation(r)
	if !ok {
		writeAzureError(w, 404, "ParentResourceNotFound", "API operation was not found.")
		return
	}
	// Tag_AssignToOperation has no request body. The service tag must exist.
	if n, err := r.Body.Read(make([]byte, 1)); n != 0 || err != io.EOF {
		writeAzureError(w, 400, "InvalidRequestContent", "Tag assignment does not accept a request body.")
		return
	}
	name := chi.URLParam(r, "tagName")
	tagID := apimRequestID(r) + "/tags/" + name
	tag, exists := a.store.Get(tagID)
	if !exists {
		writeAzureError(w, 404, "ResourceNotFound", "The referenced service tag was not found.")
		return
	}
	id := parent.ID + "/tags/" + name
	_, exists = a.store.Get(id)
	res := &store.Resource{ID: id, Name: name, Type: apimAPIOperationTagType, Properties: map[string]any{"_tagID": tag.ID}}
	if err := a.store.Put(id, res); err != nil {
		log.Error().Err(err).Str("resource_id", id).Msg("failed to assign APIM operation tag")
		writeAzureError(w, 500, "InternalServerError", "Failed to assign operation tag.")
		return
	}
	status := http.StatusCreated
	if exists {
		status = http.StatusOK
	}
	log.Info().Str("resource_id", id).Bool("existed", exists).Msg("APIM operation tag assigned")
	w.Header().Set("ETag", apimTagETag(tag))
	writeJSON(w, status, a.apimAPIOperationTagResponse(res))
}

func (a *Router) findAPIMAPIOperationTag(r *http.Request) (*store.Resource, bool) {
	if parent, ok := a.findAPIMAPIOperation(r); ok {
		return a.store.Get(parent.ID + "/tags/" + chi.URLParam(r, "tagName"))
	}
	return nil, false
}

func (a *Router) apimAPIOperationTagResponse(res *store.Resource) map[string]any {
	// Azure's TagContract identifies the referenced service tag, not the
	// association path (ApiManagementCreate/GetApiOperationTag examples).
	if tag, ok := a.store.Get(res.Properties["_tagID"].(string)); ok {
		return apimAPIResponse(tag)
	}
	return nil
}

func (a *Router) getAPIMAPIOperationTag(w http.ResponseWriter, r *http.Request) {
	if res, ok := a.findAPIMAPIOperationTag(r); ok {
		tag, exists := a.store.Get(res.Properties["_tagID"].(string))
		if exists {
			w.Header().Set("ETag", apimTagETag(tag))
			writeJSON(w, 200, a.apimAPIOperationTagResponse(res))
			return
		}
	}
	writeAzureError(w, 404, "ResourceNotFound", "Operation tag was not found.")
}

func (a *Router) headAPIMAPIOperationTag(w http.ResponseWriter, r *http.Request) {
	if res, ok := a.findAPIMAPIOperationTag(r); ok {
		if tag, exists := a.store.Get(res.Properties["_tagID"].(string)); exists {
			w.Header().Set("ETag", apimTagETag(tag))
			w.WriteHeader(http.StatusOK) // TagGetEntityStateByOperation accepts 200.
			return
		}
	}
	w.WriteHeader(http.StatusNotFound)
}

func (a *Router) deleteAPIMAPIOperationTag(w http.ResponseWriter, r *http.Request) {
	if res, ok := a.findAPIMAPIOperationTag(r); ok {
		a.store.Delete(res.ID)
		log.Info().Str("resource_id", res.ID).Msg("APIM operation tag detached")
	}
	// Detaching does not delete the service-level tag.
	w.WriteHeader(http.StatusNoContent)
}

func (a *Router) listAPIMAPIOperationTags(w http.ResponseWriter, r *http.Request) {
	parent, ok := a.findAPIMAPIOperation(r)
	if !ok {
		writeAzureError(w, 404, "ParentResourceNotFound", "API operation was not found.")
		return
	}
	for key := range r.URL.Query() {
		if strings.HasPrefix(key, "$") {
			writeAzureError(w, 501, "NotImplemented", "Operation tag OData queries are not implemented.")
			return
		}
	}
	items := []map[string]any{}
	for _, res := range a.store.List(parent.ID + "/tags/") {
		if res.Type == apimAPIOperationTagType {
			if tag := a.apimAPIOperationTagResponse(res); tag != nil {
				items = append(items, tag)
			}
		}
	}
	writeJSON(w, 200, map[string]any{"value": items})
}

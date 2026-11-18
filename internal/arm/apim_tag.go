package arm

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"

	"github.com/zerodeth/azemu/internal/store"
)

const apimTagType = apimServiceType + "/tags"

var apimTagName = regexp.MustCompile(`^[^*#&+:<>?]+$`)

func (a *Router) apimTagRoutes(r chi.Router) {
	base := "/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.apimanagement/service/{serviceName}/tags"
	r.Put(base+"/{tagName}", a.putAPIMTag)
	r.Get(base+"/{tagName}", a.getAPIMTag)
	r.Head(base+"/{tagName}", a.headAPIMTag)
	r.Delete(base+"/{tagName}", a.deleteAPIMTag)
	r.Get(base, a.listAPIMTags)
}

func apimTagETag(res *store.Resource) string {
	return fmt.Sprintf(`"%x"`, sha256.Sum256([]byte(res.Properties["displayName"].(string))))
}

func (a *Router) putAPIMTag(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.apimParent(w, r); !ok {
		return
	}
	name := chi.URLParam(r, "tagName")
	if !apimTagName.MatchString(name) || utf8.RuneCountInString(name) > 80 {
		writeAzureError(w, 400, "ValidationError", "Invalid tag name.")
		return
	}
	var body struct {
		Properties *struct {
			DisplayName string `json:"displayName"`
		} `json:"properties"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil || body.Properties == nil {
		writeAzureError(w, 400, "InvalidRequestContent", "A tag properties object is required.")
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		writeAzureError(w, 400, "InvalidRequestContent", "Exactly one JSON object is required.")
		return
	}
	displayName := body.Properties.DisplayName
	if strings.TrimSpace(displayName) == "" || utf8.RuneCountInString(displayName) > 160 {
		writeAzureError(w, 400, "ValidationError", "displayName must contain 1 to 160 characters.")
		return
	}
	id := apimRequestID(r) + "/tags/" + name
	existing, exists := a.store.Get(id)
	if match := r.Header.Get("If-Match"); match != "" && (!exists || match != "*" && match != apimTagETag(existing)) {
		writeAzureError(w, 412, "PreconditionFailed", "Tag ETag does not match.")
		return
	}
	// Service tag display names are unique within the service.
	for _, tag := range a.store.List(apimRequestID(r) + "/tags/") {
		if !strings.EqualFold(tag.ID, id) && strings.EqualFold(tag.Properties["displayName"].(string), displayName) {
			writeAzureError(w, 400, "ValidationError", "A tag with this displayName already exists.")
			return
		}
	}
	res := &store.Resource{ID: id, Name: name, Type: apimTagType, Properties: map[string]any{"displayName": displayName}}
	if err := a.store.Put(id, res); err != nil {
		log.Error().Err(err).Str("resource_id", id).Msg("failed to store APIM tag")
		writeAzureError(w, 500, "InternalServerError", "Failed to store tag.")
		return
	}
	status := http.StatusCreated
	if exists {
		status = http.StatusOK
	}
	w.Header().Set("ETag", apimTagETag(res))
	log.Info().Str("resource_id", id).Bool("existed", exists).Msg("APIM tag upsert")
	writeJSON(w, status, apimAPIResponse(res))
}

func (a *Router) getAPIMTag(w http.ResponseWriter, r *http.Request) {
	if res, ok := a.store.Get(apimRequestID(r) + "/tags/" + chi.URLParam(r, "tagName")); ok {
		w.Header().Set("ETag", apimTagETag(res))
		writeJSON(w, 200, apimAPIResponse(res))
		return
	}
	writeAzureError(w, 404, "ResourceNotFound", "Tag was not found.")
}

func (a *Router) headAPIMTag(w http.ResponseWriter, r *http.Request) {
	if res, ok := a.store.Get(apimRequestID(r) + "/tags/" + chi.URLParam(r, "tagName")); ok {
		w.Header().Set("ETag", apimTagETag(res))
		w.WriteHeader(http.StatusOK) // Tag GetEntityState accepts 200 only.
		return
	}
	w.WriteHeader(http.StatusNotFound)
}

func (a *Router) deleteAPIMTag(w http.ResponseWriter, r *http.Request) {
	id := apimRequestID(r) + "/tags/" + chi.URLParam(r, "tagName")
	if res, exists := a.store.Get(id); exists {
		if match := r.Header.Get("If-Match"); match != "" && match != "*" && match != apimTagETag(res) {
			writeAzureError(w, 412, "PreconditionFailed", "Tag ETag does not match.")
			return
		}
		// Associations reference the service tag, not a copied display name.
		for _, child := range a.store.List(apimRequestID(r) + "/apis/") {
			if child.Type == apimAPIOperationTagType && strings.EqualFold(child.Properties["_tagID"].(string), id) {
				a.store.Delete(child.ID)
			}
		}
		a.store.Delete(id)
		log.Info().Str("resource_id", id).Msg("APIM tag deleted")
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *Router) listAPIMTags(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.apimParent(w, r); !ok {
		return
	}
	for key := range r.URL.Query() {
		if strings.HasPrefix(key, "$") {
			writeAzureError(w, 501, "NotImplemented", "Tag OData queries are not implemented.")
			return
		}
	}
	items := []map[string]any{}
	for _, res := range a.store.List(apimRequestID(r) + "/tags/") {
		if res.Type == apimTagType {
			items = append(items, apimAPIResponse(res))
		}
	}
	writeJSON(w, 200, map[string]any{"value": items})
}

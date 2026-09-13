package arm

import (
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"strings"

	"github.com/rs/zerolog/log"

	"github.com/zerodeth/azemu/internal/store"
)

// patchResource applies an ARM PATCH to an existing resource.
//
// Azure's resource providers are not uniform about update: some accept a full
// PUT, others require PATCH, and azurerm follows whichever the provider uses.
// Where a provider PATCHes, a PUT-only route answers 405 and the second
// `terraform apply` fails, so every resource whose provider updates by PATCH
// needs this.
//
// The semantics PATCH must honour, which are what separate it from PUT:
//
//   - Keys absent from the request keep their stored values. Only what is sent
//     is changed.
//   - `tags` is replaced wholesale when present, not merged key by key. That is
//     what ARM does, and it is how Terraform removes a tag.
//   - A PATCH to a resource that does not exist is 404. It never creates.
//   - The response is 200 carrying the same envelope as GET, so the provider
//     can read the result back without a second request.
//
// A generic merge cannot know a resource's own rules, so the caller supplies
// two hooks:
//
//   - finalise re-applies fields the server owns and re-runs whatever the PUT
//     path validates. Without it PATCH is a hole straight past PUT's checks:
//     a client could set a Key Vault's vaultUri, which is how azurerm routes
//     data-plane calls, or store a SKU that PUT would reject.
//   - respond renders the resource in its own response shape.
func (a *Router) patchResource(
	w http.ResponseWriter,
	r *http.Request,
	id string,
	kind string,
	finalise func(*store.Resource) (code, message string, ok bool),
	respond func(*store.Resource) interface{},
) {
	existing, ok := a.store.Get(id)
	if !ok {
		writeAzureError(w, http.StatusNotFound, "ResourceNotFound",
			fmt.Sprintf("%s %q was not found.", kind, id))
		return
	}

	// Decoding to RawMessage keeps "absent" distinguishable from "null" and
	// from an empty object, which is the whole point of a merge.
	var body map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeAzureError(w, http.StatusBadRequest, "InvalidRequestContent", err.Error())
		return
	}

	// store.Get returns a shallow copy: the struct is copied but Tags and
	// Properties still alias the stored maps. Merging into them directly would
	// mutate the store before Put, and would leave a rejected request having
	// changed state anyway.
	updated := *existing
	updated.Properties = cloneProperties(existing.Properties)
	updated.Tags = cloneTags(existing.Tags)

	if raw, present := body["tags"]; present {
		var tags map[string]string
		if err := json.Unmarshal(raw, &tags); err != nil {
			writeAzureError(w, http.StatusBadRequest, "InvalidRequestContent",
				fmt.Sprintf("tags: %s", err))
			return
		}
		// Wholesale replacement, so sending {"tags":{}} clears them.
		updated.Tags = normaliseTags(tags)
	}

	if raw, present := body["location"]; present {
		var location string
		if err := json.Unmarshal(raw, &location); err != nil {
			writeAzureError(w, http.StatusBadRequest, "InvalidRequestContent",
				fmt.Sprintf("location: %s", err))
			return
		}
		if strings.TrimSpace(location) != "" {
			updated.Location = strings.ToLower(location)
		}
	}

	if raw, present := body["properties"]; present {
		var props map[string]interface{}
		if err := json.Unmarshal(raw, &props); err != nil {
			writeAzureError(w, http.StatusBadRequest, "InvalidRequestContent",
				fmt.Sprintf("properties: %s", err))
			return
		}
		// A one-level merge. ARM's own PATCH semantics are shallow for these
		// resource types: sending properties.x replaces x entirely rather than
		// merging inside it.
		maps.Copy(updated.Properties, props)
	}

	// A resource that was provisioned stays provisioned; an update never
	// returns it to a pending state.
	if updated.Properties == nil {
		updated.Properties = map[string]interface{}{}
	}
	updated.Properties["provisioningState"] = "Succeeded"

	if finalise != nil {
		if code, message, ok := finalise(&updated); !ok {
			writeAzureError(w, http.StatusBadRequest, code, message)
			return
		}
	}

	if err := a.store.Put(id, &updated); err != nil {
		writeAzureError(w, http.StatusInternalServerError, "InternalServerError",
			fmt.Sprintf("patch %s %q: %s", kind, id, err))
		return
	}

	log.Info().Str("resource_id", id).Msg(strings.ToLower(kind) + " patch")
	writeJSON(w, http.StatusOK, respond(&updated))
}

// cloneProperties always returns a non-nil map: the merge below writes into it,
// and maps.Clone(nil) would hand back nil.
func cloneProperties(in map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(in))
	maps.Copy(out, in)
	return out
}

func cloneTags(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	maps.Copy(out, in)
	return out
}

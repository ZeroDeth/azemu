package arm

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"

	"github.com/zerodeth/azemu/internal/store"
)

const apimUserType = apimServiceType + "/users"

func (a *Router) apimUserRoutes(r chi.Router) {
	base := "/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.apimanagement/service/{serviceName}/users"
	r.Put(base+"/{userName}", a.putAPIMUser)
	r.Patch(base+"/{userName}", a.putAPIMUser)
	r.Get(base+"/{userName}", a.getAPIMUser)
	r.Head(base+"/{userName}", a.headAPIMUser)
	r.Delete(base+"/{userName}", a.deleteAPIMUser)
	r.Get(base, a.listAPIMUsers)
}

func apimUserETag(res *store.Resource) string {
	// Properties have already been validated as JSON values.
	data, _ := json.Marshal(res.Properties)
	return fmt.Sprintf(`"%x"`, sha256.Sum256(data))
}

func validateAPIMUserQuery(w http.ResponseWriter, r *http.Request) bool {
	for _, field := range []string{"notify", "deleteSubscriptions"} {
		if values, present := r.URL.Query()[field]; present {
			if len(values) != 1 || values[0] != "true" && values[0] != "false" {
				writeAzureError(w, 400, "ValidationError", field+" must be a boolean.")
				return false
			}
			if field == "notify" && values[0] == "true" {
				writeAzureError(w, 501, "NotImplemented", "User email delivery is not implemented.")
				return false
			}
		}
	}
	if values, present := r.URL.Query()["appType"]; present && (len(values) != 1 || values[0] != "portal" && values[0] != "developerPortal") {
		writeAzureError(w, 400, "ValidationError", "Invalid user appType.")
		return false
	}
	return true
}

func (a *Router) putAPIMUser(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.apimParent(w, r); !ok {
		return
	}
	name := chi.URLParam(r, "userName")
	if strings.TrimSpace(name) == "" || strings.ContainsAny(name, `/\?#`) || utf8.RuneCountInString(name) > 80 {
		writeAzureError(w, 400, "ValidationError", "Invalid user identifier.")
		return
	}
	if !validateAPIMUserQuery(w, r) {
		return
	}
	id := apimRequestID(r) + "/users/" + name
	existing, exists := a.store.Get(id)
	if r.Method == http.MethodPatch && !exists {
		writeAzureError(w, 404, "ResourceNotFound", "User was not found.")
		return
	}
	var body struct {
		Properties map[string]any `json:"properties"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil || body.Properties == nil {
		// Never echo JSON decoder errors: malformed input may contain a password.
		writeAzureError(w, 400, "InvalidRequestContent", "A valid user properties object is required.")
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		writeAzureError(w, 400, "InvalidRequestContent", "Exactly one JSON object is required.")
		return
	}
	props := map[string]any{"state": "active"}
	if r.Method == http.MethodPatch {
		for key, value := range existing.Properties {
			props[key] = value
		}
	}
	for field, value := range body.Properties {
		switch field {
		case "firstName", "lastName", "email", "note", "state":
			if _, valid := value.(string); !valid {
				writeAzureError(w, 400, "ValidationError", "User profile fields must be strings.")
				return
			}
			props[field] = value
		case "password":
			if _, valid := value.(string); !valid {
				writeAzureError(w, 400, "ValidationError", "password must be a string.")
				return
			}
			// There is no APIM portal authentication subsystem. Do not retain
			// plaintext, invent a credential verifier, or fake password success.
			writeAzureError(w, 501, "NotImplemented", "User password provisioning is not implemented.")
			return
		case "confirmation":
			if value != "signup" && value != "invite" {
				writeAzureError(w, 400, "ValidationError", "Invalid user confirmation.")
				return
			}
			writeAzureError(w, 501, "NotImplemented", "User confirmation emails are not implemented.")
			return
		case "appType":
			if value != "portal" && value != "developerPortal" {
				writeAzureError(w, 400, "ValidationError", "Invalid user appType.")
				return
			}
			// Request provenance, not part of UserContract's response.
			props["_appType"] = value
		case "identities":
			// Federated identities require actual identity-provider contracts.
			writeAzureError(w, 501, "NotImplemented", "Configuring user identities is not implemented.")
			return
		default:
			// Do not echo attacker-controlled property names (possible secrets).
			writeAzureError(w, 400, "InvalidRequestContent", "Unsupported user property.")
			return
		}
	}
	for field, max := range map[string]int{"firstName": 100, "lastName": 100, "email": 254} {
		value, valid := props[field].(string)
		if !valid || strings.TrimSpace(value) == "" || utf8.RuneCountInString(value) > max {
			writeAzureError(w, 400, "ValidationError", field+" must be nonempty and within its length limit.")
			return
		}
	}
	email := props["email"].(string)
	address, err := mail.ParseAddress(email)
	if err != nil || address.Address != email {
		writeAzureError(w, 400, "ValidationError", "A valid email address is required.")
		return
	}
	switch props["state"] {
	case "active", "blocked", "pending":
	case "deleted":
		writeAzureError(w, 501, "NotImplemented", "Closing an account via state is not implemented; use DELETE.")
		return
	default:
		writeAzureError(w, 400, "ValidationError", "Invalid user state.")
		return
	}
	for _, user := range a.store.List(apimRequestID(r) + "/users/") {
		if user.Type == apimUserType && user.ID != id && strings.EqualFold(user.Properties["email"].(string), email) {
			writeAzureError(w, 400, "ValidationError", "A user with this email already exists.")
			return
		}
	}
	if match := r.Header.Get("If-Match"); match != "" && (!exists || match != "*" && match != apimUserETag(existing)) {
		writeAzureError(w, 412, "PreconditionFailed", "User ETag does not match.")
		return
	}
	props["registrationDate"] = time.Now().UTC().Format(time.RFC3339Nano)
	if exists {
		props["registrationDate"] = existing.Properties["registrationDate"]
	}
	props["groups"] = []any{}
	props["identities"] = []any{map[string]any{"provider": "Basic", "id": email}}
	res := &store.Resource{ID: id, Name: name, Type: apimUserType, Properties: props}
	if err := a.store.Put(id, res); err != nil {
		log.Error().Err(err).Str("resource_id", id).Msg("failed to store APIM user")
		writeAzureError(w, 500, "InternalServerError", "Failed to store user.")
		return
	}
	status := http.StatusCreated
	if exists {
		status = http.StatusOK
	}
	w.Header().Set("ETag", apimUserETag(res))
	log.Info().Str("resource_id", id).Bool("existed", exists).Msg("APIM user upsert")
	writeJSON(w, status, apimAPIResponse(res))
}

func (a *Router) getAPIMUser(w http.ResponseWriter, r *http.Request) {
	if res, ok := a.store.Get(apimRequestID(r) + "/users/" + chi.URLParam(r, "userName")); ok {
		w.Header().Set("ETag", apimUserETag(res))
		writeJSON(w, 200, apimAPIResponse(res))
		return
	}
	writeAzureError(w, 404, "ResourceNotFound", "User was not found.")
}

func (a *Router) headAPIMUser(w http.ResponseWriter, r *http.Request) {
	if res, ok := a.store.Get(apimRequestID(r) + "/users/" + chi.URLParam(r, "userName")); ok {
		w.Header().Set("ETag", apimUserETag(res))
		w.WriteHeader(http.StatusOK) // User GetEntityTag accepts only 200.
		return
	}
	w.WriteHeader(http.StatusNotFound)
}

func (a *Router) deleteAPIMUser(w http.ResponseWriter, r *http.Request) {
	if !validateAPIMUserQuery(w, r) {
		return
	}
	id := apimRequestID(r) + "/users/" + chi.URLParam(r, "userName")
	if res, exists := a.store.Get(id); exists {
		if match := r.Header.Get("If-Match"); match != "" && match != "*" && match != apimUserETag(res) {
			writeAzureError(w, 412, "PreconditionFailed", "User ETag does not match.")
			return
		}
		// Subscriptions are service siblings, not user children. Do not
		// orphan imported references or pretend to implement that contract.
		for _, subscription := range a.store.List(apimRequestID(r) + "/subscriptions/") {
			owner, _ := subscription.Properties["ownerId"].(string)
			if strings.EqualFold(owner, id) {
				writeAzureError(w, 501, "NotImplemented", "Deleting users with subscriptions is not implemented.")
				return
			}
		}
		a.store.Delete(id)
		log.Info().Str("resource_id", id).Msg("APIM user deleted")
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *Router) listAPIMUsers(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.apimParent(w, r); !ok {
		return
	}
	for key := range r.URL.Query() {
		if strings.HasPrefix(key, "$") || key == "expandGroups" {
			writeAzureError(w, 501, "NotImplemented", "User OData queries and group expansion are not implemented.")
			return
		}
	}
	items := []map[string]any{}
	for _, res := range a.store.List(apimRequestID(r) + "/users/") {
		if res.Type == apimUserType {
			items = append(items, apimAPIResponse(res))
		}
	}
	writeJSON(w, 200, map[string]any{"value": items})
}

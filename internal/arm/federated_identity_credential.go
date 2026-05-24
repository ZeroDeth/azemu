package arm

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"

	"github.com/zerodeth/azemu/internal/store"
)

const federatedIdentityCredentialTypeString = "Microsoft.ManagedIdentity/userAssignedIdentities/federatedIdentityCredentials"

func federatedIdentityCredentialID(subID, rgName, identityName, credentialName string) string {
	return fmt.Sprintf(
		"%s/federatedIdentityCredentials/%s",
		userAssignedIdentityID(subID, rgName, identityName),
		credentialName,
	)
}

type federatedIdentityCredentialBody struct {
	Properties federatedIdentityCredentialProperties `json:"properties"`
}

type federatedIdentityCredentialProperties struct {
	Audiences []string `json:"audiences"`
	Issuer    string   `json:"issuer"`
	Subject   string   `json:"subject"`
}

func (a *Router) putFederatedIdentityCredential(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	identityName := chi.URLParam(r, "identityName")
	credentialName := chi.URLParam(r, "credentialName")

	parentID := userAssignedIdentityID(subID, rgName, identityName)
	if _, ok := a.store.Get(parentID); !ok {
		writeAzureError(w, http.StatusNotFound, "ParentResourceNotFound",
			fmt.Sprintf("The Resource 'Microsoft.ManagedIdentity/userAssignedIdentities/%s' under resource group '%s' was not found.", identityName, rgName))
		return
	}

	var body federatedIdentityCredentialBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeAzureError(w, http.StatusBadRequest, "InvalidRequestContent", err.Error())
		return
	}
	if err := validateFederatedIdentityCredentialProperties(body.Properties); err != nil {
		writeAzureError(w, http.StatusBadRequest, "InvalidRequestContent", err.Error())
		return
	}

	id := federatedIdentityCredentialID(subID, rgName, identityName, credentialName)
	props := map[string]interface{}{
		"audiences": body.Properties.Audiences,
		"issuer":    body.Properties.Issuer,
		"subject":   body.Properties.Subject,
	}
	res := &store.Resource{
		ID:         id,
		Name:       credentialName,
		Type:       federatedIdentityCredentialTypeString,
		Properties: props,
	}

	_, exists := a.store.Get(id)
	if err := a.store.Put(id, res); err != nil {
		writeAzureError(w, http.StatusInternalServerError, "InternalServerError",
			fmt.Sprintf("put federated identity credential %q: %s", credentialName, err))
		return
	}

	status := http.StatusCreated
	if exists {
		status = http.StatusOK
	}
	log.Info().Str("resource_id", id).Bool("existed", exists).Msg("federated identity credential upsert")
	writeJSON(w, status, federatedIdentityCredentialResponse(res))
}

func validateFederatedIdentityCredentialProperties(props federatedIdentityCredentialProperties) error {
	if props.Issuer == "" {
		return fmt.Errorf("properties.issuer is required")
	}
	if props.Subject == "" {
		return fmt.Errorf("properties.subject is required")
	}
	if len(props.Audiences) == 0 {
		return fmt.Errorf("properties.audiences is required")
	}
	for _, aud := range props.Audiences {
		if aud == "" {
			return fmt.Errorf("properties.audiences must not contain empty values")
		}
	}
	return nil
}

func (a *Router) getFederatedIdentityCredential(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	identityName := chi.URLParam(r, "identityName")
	credentialName := chi.URLParam(r, "credentialName")
	id := federatedIdentityCredentialID(subID, rgName, identityName, credentialName)

	res, ok := a.store.Get(id)
	if !ok {
		writeAzureError(w, http.StatusNotFound, "ResourceNotFound",
			fmt.Sprintf("The Resource 'Microsoft.ManagedIdentity/userAssignedIdentities/%s/federatedIdentityCredentials/%s' under resource group '%s' was not found.", identityName, credentialName, rgName))
		return
	}
	writeJSON(w, http.StatusOK, federatedIdentityCredentialResponse(res))
}

func (a *Router) headFederatedIdentityCredential(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	identityName := chi.URLParam(r, "identityName")
	credentialName := chi.URLParam(r, "credentialName")
	id := federatedIdentityCredentialID(subID, rgName, identityName, credentialName)

	if _, ok := a.store.Get(id); !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *Router) deleteFederatedIdentityCredential(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	identityName := chi.URLParam(r, "identityName")
	credentialName := chi.URLParam(r, "credentialName")
	id := federatedIdentityCredentialID(subID, rgName, identityName, credentialName)

	if !a.store.Delete(id) {
		writeAzureError(w, http.StatusNotFound, "ResourceNotFound",
			fmt.Sprintf("The federated identity credential '%s' under user assigned identity '%s' could not be found.", credentialName, identityName))
		return
	}

	log.Info().Str("resource_id", id).Msg("federated identity credential deleted")
	// azurerm's FederatedIdentityCredentials client accepts 200 or 204 only.
	// FIC is a child resource; synchronous 200 OK is the correct response.
	w.WriteHeader(http.StatusOK)
}

func (a *Router) listFederatedIdentityCredentials(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	identityName := chi.URLParam(r, "identityName")
	parentID := userAssignedIdentityID(subID, rgName, identityName)

	if _, ok := a.store.Get(parentID); !ok {
		writeAzureError(w, http.StatusNotFound, "ParentResourceNotFound",
			fmt.Sprintf("The Resource 'Microsoft.ManagedIdentity/userAssignedIdentities/%s' under resource group '%s' was not found.", identityName, rgName))
		return
	}

	prefix := parentID + "/federatedIdentityCredentials/"
	items := []map[string]interface{}{}
	for _, res := range a.store.List(prefix) {
		if res.Type != federatedIdentityCredentialTypeString {
			continue
		}
		items = append(items, federatedIdentityCredentialResponse(res))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"value": items})
}

func federatedIdentityCredentialResponse(v *store.Resource) map[string]interface{} {
	props := map[string]interface{}{}
	for k, val := range v.Properties {
		props[k] = val
	}
	return map[string]interface{}{
		"id":         v.ID,
		"name":       v.Name,
		"type":       v.Type,
		"properties": props,
	}
}

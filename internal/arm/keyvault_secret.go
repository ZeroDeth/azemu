package arm

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/zerodeth/azemu/internal/store"
)

// secretStoreKey returns the store key for a specific secret version.
// Keys live under /keyvault/{vaultName}/secrets/{secretName}/{version} so that
// the store's cascade-delete (prefix "/" + id) cleans up all versions when the
// current pointer is deleted.
func secretStoreKey(vaultName, secretName, version string) string {
	return fmt.Sprintf("/keyvault/%s/secrets/%s/%s", vaultName, secretName, version)
}

// secretCurrentKey returns the store key used to track the current (latest) version UUID.
func secretCurrentKey(vaultName, secretName string) string {
	return fmt.Sprintf("/keyvault/%s/secrets/%s/current", vaultName, secretName)
}

// secretListPrefix returns the prefix for listing all secrets in a vault.
func secretListPrefix(vaultName string) string {
	return fmt.Sprintf("/keyvault/%s/secrets/", vaultName)
}

// kvSecretPutBody is the PUT request body for a Key Vault secret.
type kvSecretPutBody struct {
	Value       string                 `json:"value"`
	ContentType string                 `json:"contentType"`
	Tags        map[string]string      `json:"tags"`
	Attributes  map[string]interface{} `json:"attributes"`
}

// kvSecretID builds the canonical Key Vault secret ID including version.
// The format matches real Azure: {vaultUri}secrets/{name}/{version}
func (a *Router) kvSecretID(vaultName, secretName, version string) string {
	return fmt.Sprintf("%s/keyvault/%s/secrets/%s/%s", a.kvEndpoint, vaultName, secretName, version)
}

// kvSecretResponse builds the Key Vault data-plane response for a secret.
// This format differs from ARM responses: no "type", no "provisioningState".
func kvSecretResponse(props map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
		"value":       props["value"],
		"id":          props["id"],
		"contentType": props["contentType"],
		"tags":        props["tags"],
		"attributes":  props["attributes"],
	}
}

func (a *Router) requireKeyVaultBearerToken(w http.ResponseWriter, r *http.Request) bool {
	if a.tokenValidator == nil {
		return true
	}
	authz := r.Header.Get("Authorization")
	if !strings.HasPrefix(authz, "Bearer ") {
		writeAzureError(w, http.StatusUnauthorized, "Unauthorized", "missing bearer token")
		return false
	}
	if !a.tokenValidator.ValidateBearerToken(strings.TrimSpace(strings.TrimPrefix(authz, "Bearer "))) {
		writeAzureError(w, http.StatusUnauthorized, "Unauthorized", "invalid bearer token")
		return false
	}
	return true
}

func (a *Router) putKeyVaultSecret(w http.ResponseWriter, r *http.Request) {
	if !a.requireKeyVaultBearerToken(w, r) {
		return
	}
	vaultName := chi.URLParam(r, "vaultName")
	secretName := chi.URLParam(r, "secretName")

	var body kvSecretPutBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeAzureError(w, http.StatusBadRequest, "BadRequest", err.Error())
		return
	}

	version := uuid.New().String()
	now := time.Now().Unix()

	attrs := map[string]interface{}{
		"enabled":         true,
		"created":         now,
		"updated":         now,
		"recoveryLevel":   "Purgeable",
		"recoverableDays": float64(0),
	}
	// Merge caller-supplied attributes (enabled flag, expiry, etc.).
	for k, v := range body.Attributes {
		if k != "created" && k != "updated" && k != "recoveryLevel" && k != "recoverableDays" {
			attrs[k] = v
		}
	}

	secretID := a.kvSecretID(vaultName, secretName, version)
	props := map[string]interface{}{
		"value":       body.Value,
		"id":          secretID,
		"contentType": body.ContentType,
		"tags":        normaliseTags(body.Tags),
		"attributes":  attrs,
		"version":     version,
	}

	// Store versioned entry.
	versionedKey := secretStoreKey(vaultName, secretName, version)
	if err := a.store.Put(versionedKey, &store.Resource{
		ID:         versionedKey,
		Name:       secretName,
		Type:       "keyvault/secret",
		Properties: props,
	}); err != nil {
		writeAzureError(w, http.StatusInternalServerError, "InternalServerError",
			fmt.Sprintf("put secret %q: %s", secretName, err))
		return
	}

	// Determine 201 vs 200: first write if no current pointer yet exists.
	currentKey := secretCurrentKey(vaultName, secretName)
	_, isUpdate := a.store.Get(currentKey)
	if err := a.store.Put(currentKey, &store.Resource{
		ID:         currentKey,
		Name:       secretName,
		Type:       "keyvault/secret/current",
		Properties: map[string]interface{}{"version": version},
	}); err != nil {
		writeAzureError(w, http.StatusInternalServerError, "InternalServerError",
			fmt.Sprintf("update secret current pointer %q: %s", secretName, err))
		return
	}

	status := http.StatusCreated
	if isUpdate {
		status = http.StatusOK
	}
	log.Info().Str("vault", vaultName).Str("secret", secretName).Str("version", version).Msg("key vault secret upsert")
	writeJSON(w, status, kvSecretResponse(props))
}

func (a *Router) getKeyVaultSecret(w http.ResponseWriter, r *http.Request) {
	if !a.requireKeyVaultBearerToken(w, r) {
		return
	}
	vaultName := chi.URLParam(r, "vaultName")
	secretName := chi.URLParam(r, "secretName")

	currentKey := secretCurrentKey(vaultName, secretName)
	current, ok := a.store.Get(currentKey)
	if !ok {
		writeAzureError(w, http.StatusNotFound, "SecretNotFound",
			fmt.Sprintf("A secret with (name/id) %s/%s was not found in this key vault.", vaultName, secretName))
		return
	}

	version, _ := current.Properties["version"].(string)
	versionedKey := secretStoreKey(vaultName, secretName, version)
	res, ok := a.store.Get(versionedKey)
	if !ok {
		writeAzureError(w, http.StatusNotFound, "SecretNotFound",
			fmt.Sprintf("A secret with (name/id) %s/%s was not found in this key vault.", vaultName, secretName))
		return
	}
	writeJSON(w, http.StatusOK, kvSecretResponse(res.Properties))
}

func (a *Router) getKeyVaultSecretVersion(w http.ResponseWriter, r *http.Request) {
	if !a.requireKeyVaultBearerToken(w, r) {
		return
	}
	vaultName := chi.URLParam(r, "vaultName")
	secretName := chi.URLParam(r, "secretName")
	version := chi.URLParam(r, "version")

	versionedKey := secretStoreKey(vaultName, secretName, version)
	res, ok := a.store.Get(versionedKey)
	if !ok {
		writeAzureError(w, http.StatusNotFound, "SecretNotFound",
			fmt.Sprintf("A secret with (name/id) %s/%s/%s was not found in this key vault.", vaultName, secretName, version))
		return
	}
	writeJSON(w, http.StatusOK, kvSecretResponse(res.Properties))
}

func (a *Router) deleteKeyVaultSecret(w http.ResponseWriter, r *http.Request) {
	if !a.requireKeyVaultBearerToken(w, r) {
		return
	}
	vaultName := chi.URLParam(r, "vaultName")
	secretName := chi.URLParam(r, "secretName")

	// Check existence via current pointer.
	currentKey := secretCurrentKey(vaultName, secretName)
	if _, ok := a.store.Get(currentKey); !ok {
		writeAzureError(w, http.StatusNotFound, "SecretNotFound",
			fmt.Sprintf("A secret with (name/id) %s/%s was not found in this key vault.", vaultName, secretName))
		return
	}

	// Delete the current pointer and all versioned entries via prefix delete.
	// The store's Delete cascades to anything with the prefix currentKey + "/".
	// Versioned keys are at /keyvault/{v}/secrets/{s}/{version} — delete each.
	prefix := fmt.Sprintf("/keyvault/%s/secrets/%s/", vaultName, secretName)
	for _, res := range a.store.List(prefix) {
		a.store.Delete(res.ID)
	}
	a.store.Delete(currentKey)

	log.Info().Str("vault", vaultName).Str("secret", secretName).Msg("key vault secret deleted")
	// ARM async delete: 202 Accepted + Location header pointing at a stable
	// operation result URL. Callers that poll the Location receive 200 immediately
	// (azemu has no real async queue).
	opID := uuid.New().String()
	w.Header().Set("Location", fmt.Sprintf("%s/keyvault/%s/operations/%s", a.kvEndpoint, vaultName, opID))
	writeJSON(w, http.StatusAccepted, map[string]interface{}{
		"status":             "Succeeded",
		"recoveryId":         fmt.Sprintf("%s/keyvault/%s/deletedsecrets/%s", a.kvEndpoint, vaultName, secretName),
		"deletedDate":        time.Now().Unix(),
		"scheduledPurgeDate": time.Now().Add(90 * 24 * time.Hour).Unix(),
	})
}

func (a *Router) listKeyVaultSecrets(w http.ResponseWriter, r *http.Request) {
	if !a.requireKeyVaultBearerToken(w, r) {
		return
	}
	vaultName := chi.URLParam(r, "vaultName")
	prefix := secretListPrefix(vaultName)

	items := []map[string]interface{}{}
	for _, res := range a.store.List(prefix) {
		// Only include current-pointer entries (not versioned or version keys).
		if res.Type != "keyvault/secret/current" {
			continue
		}
		secretName := res.Name
		// Build a minimal list entry (no value exposed in list, matching real Azure).
		items = append(items, map[string]interface{}{
			"id": fmt.Sprintf("%s/keyvault/%s/secrets/%s", a.kvEndpoint, vaultName, secretName),
			"attributes": map[string]interface{}{
				"enabled":       true,
				"recoveryLevel": "Purgeable",
			},
		})
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"value": items})
}

func (a *Router) listKeyVaultSecretVersions(w http.ResponseWriter, r *http.Request) {
	if !a.requireKeyVaultBearerToken(w, r) {
		return
	}
	vaultName := chi.URLParam(r, "vaultName")
	secretName := chi.URLParam(r, "secretName")

	// Verify the secret exists.
	currentKey := secretCurrentKey(vaultName, secretName)
	if _, ok := a.store.Get(currentKey); !ok {
		writeAzureError(w, http.StatusNotFound, "SecretNotFound",
			fmt.Sprintf("A secret with (name/id) %s/%s was not found in this key vault.", vaultName, secretName))
		return
	}

	prefix := fmt.Sprintf("/keyvault/%s/secrets/%s/", vaultName, secretName)
	items := []map[string]interface{}{}
	for _, res := range a.store.List(prefix) {
		// Skip the current-pointer entry.
		if res.Type == "keyvault/secret/current" {
			continue
		}
		version, _ := res.Properties["version"].(string)
		if version == "" || strings.HasSuffix(res.ID, "/current") {
			continue
		}
		items = append(items, map[string]interface{}{
			"id":         res.Properties["id"],
			"attributes": res.Properties["attributes"],
		})
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"value": items})
}

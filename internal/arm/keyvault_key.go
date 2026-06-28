package arm

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/zerodeth/azemu/internal/store"
)

// keyStoreKey returns the store key for a specific key version.
// Entries live under /keyvault/{vaultName}/keys/{keyName}/{version} so the
// vault's cascade delete (prefix /keyvault/{vaultName}/) cleans them up.
func keyStoreKey(vaultName, keyName, version string) string {
	return fmt.Sprintf("/keyvault/%s/keys/%s/%s", vaultName, keyName, version)
}

// keyCurrentKey returns the store key used to track the current (latest) version UUID.
func keyCurrentKey(vaultName, keyName string) string {
	return fmt.Sprintf("/keyvault/%s/keys/%s/current", vaultName, keyName)
}

// keyListPrefix returns the prefix for listing all keys in a vault.
func keyListPrefix(vaultName string) string {
	return fmt.Sprintf("/keyvault/%s/keys/", vaultName)
}

// kvKeyID builds the canonical Key Vault key ID (kid) including version.
// The azurerm provider parses nested-item IDs with ParseNestedItemID, which
// requires exactly /keys/{name}/{version} in the URL path; the vault is
// identified by the host ({vault}.vault.localhost), mirroring real Azure's
// {vault}.vault.azure.net.
func (a *Router) kvKeyID(vaultName, keyName, version string) string {
	return fmt.Sprintf("%s/keys/%s/%s", a.vaultBaseURL(vaultName), keyName, version)
}

// signAlgorithms maps the supported JsonWebKeySignatureAlgorithm identifiers
// to their digest functions. Sign-only RSA scope: RS256 (RSASSA-PKCS1-v1_5
// over a SHA-256 digest) is all the OTA manifest pipeline needs. Adding
// RS384/RS512 is a one-line entry each if a consumer ever requires them.
var signAlgorithms = map[string]crypto.Hash{
	"RS256": crypto.SHA256,
}

// validRSAKeySizes are the modulus sizes real Key Vault accepts for kty=RSA.
var validRSAKeySizes = map[int]bool{2048: true, 3072: true, 4096: true}

// kvKeyCreateBody is the POST /keys/{name}/create request body.
type kvKeyCreateBody struct {
	Kty        string                 `json:"kty"`
	KeySize    int                    `json:"key_size"`
	KeyOps     []string               `json:"key_ops"`
	Attributes map[string]interface{} `json:"attributes"`
	Tags       map[string]string      `json:"tags"`
}

// kvKeyImportBody is the PUT /keys/{name} request body. The key field carries
// a full JSON Web Key including private parameters.
type kvKeyImportBody struct {
	Key struct {
		Kty    string   `json:"kty"`
		KeyOps []string `json:"key_ops"`
		N      string   `json:"n"`
		E      string   `json:"e"`
		D      string   `json:"d"`
		P      string   `json:"p"`
		Q      string   `json:"q"`
	} `json:"key"`
	Attributes map[string]interface{} `json:"attributes"`
	Tags       map[string]string      `json:"tags"`
}

// kvKeyUpdateBody is the PATCH /keys/{name}[/{version}] request body.
type kvKeyUpdateBody struct {
	KeyOps     []string               `json:"key_ops"`
	Attributes map[string]interface{} `json:"attributes"`
	Tags       map[string]string      `json:"tags"`
}

// kvSignBody is the POST /keys/{name}/{version}/sign request body. Value is
// the base64url-encoded digest of the data to sign (KV signs hashes, never
// raw content).
type kvSignBody struct {
	Alg   string `json:"alg"`
	Value string `json:"value"`
}

// encodePrivateKeyPKCS8 serialises an RSA private key to a JSON-safe string
// for storage in Resource.Properties. azemu is an emulator: the private key
// intentionally lives in the in-memory store and therefore also appears in
// state export dumps. Never copy this pattern to production code.
func encodePrivateKeyPKCS8(priv *rsa.PrivateKey) (string, error) {
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return "", fmt.Errorf("marshal private key: %w", err)
	}
	return base64.StdEncoding.EncodeToString(der), nil
}

func decodePrivateKeyPKCS8(s string) (*rsa.PrivateKey, error) {
	der, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("decode private key: %w", err)
	}
	parsed, err := x509.ParsePKCS8PrivateKey(der)
	if err != nil {
		return nil, fmt.Errorf("parse private key: %w", err)
	}
	priv, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("parse private key: not an RSA key")
	}
	return priv, nil
}

// base64urlToBigInt decodes an unpadded base64url JWK parameter. Padded input
// is tolerated by trimming '=' first.
func base64urlToBigInt(s string) (*big.Int, error) {
	b, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(s, "="))
	if err != nil {
		return nil, err
	}
	return new(big.Int).SetBytes(b), nil
}

// rsaKeyFromJWKParams rebuilds an RSA private key from base64url JWK fields.
func rsaKeyFromJWKParams(n, e, d, p, q string) (*rsa.PrivateKey, error) {
	nInt, err := base64urlToBigInt(n)
	if err != nil {
		return nil, fmt.Errorf("decode jwk n: %w", err)
	}
	eInt, err := base64urlToBigInt(e)
	if err != nil {
		return nil, fmt.Errorf("decode jwk e: %w", err)
	}
	dInt, err := base64urlToBigInt(d)
	if err != nil {
		return nil, fmt.Errorf("decode jwk d: %w", err)
	}
	pInt, err := base64urlToBigInt(p)
	if err != nil {
		return nil, fmt.Errorf("decode jwk p: %w", err)
	}
	qInt, err := base64urlToBigInt(q)
	if err != nil {
		return nil, fmt.Errorf("decode jwk q: %w", err)
	}
	priv := &rsa.PrivateKey{
		PublicKey: rsa.PublicKey{N: nInt, E: int(eInt.Int64())},
		D:         dInt,
		Primes:    []*big.Int{pInt, qInt},
	}
	priv.Precompute()
	if err := priv.Validate(); err != nil {
		return nil, fmt.Errorf("validate imported key: %w", err)
	}
	return priv, nil
}

// publicJWK builds the public-only JSON Web Key returned in key bundles.
// n and e use unpadded big-endian base64url, the same encoding as the JWKS
// endpoint in internal/auth — azurerm derives key_size and public_key_pem
// from these fields, so the encoding must be canonical.
func publicJWK(kid string, pub *rsa.PublicKey, keyOps []string) map[string]interface{} {
	return map[string]interface{}{
		"kid":     kid,
		"kty":     "RSA",
		"key_ops": keyOps,
		"n":       base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
		"e":       base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
	}
}

// kvKeyAttributes builds the default attribute block and merges caller input.
func kvKeyAttributes(callerAttrs map[string]interface{}) map[string]interface{} {
	now := time.Now().Unix()
	attrs := map[string]interface{}{
		"enabled":         true,
		"created":         now,
		"updated":         now,
		"recoveryLevel":   "Purgeable",
		"recoverableDays": float64(0),
	}
	for k, v := range callerAttrs {
		if k != "created" && k != "updated" && k != "recoveryLevel" && k != "recoverableDays" {
			attrs[k] = v
		}
	}
	return attrs
}

// kvKeyBundleResponse builds the Key Vault data-plane key bundle. The private
// key material in props["privateKeyPkcs8"] is never copied into the response.
func kvKeyBundleResponse(props map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
		"key":        props["key"],
		"attributes": props["attributes"],
		"tags":       props["tags"],
		"managed":    false,
	}
}

// storeKeyVersion persists one key version plus the current pointer.
func (a *Router) storeKeyVersion(vaultName, keyName, version string, props map[string]interface{}) error {
	versionedKey := keyStoreKey(vaultName, keyName, version)
	if err := a.store.Put(versionedKey, &store.Resource{
		ID:         versionedKey,
		Name:       keyName,
		Type:       "keyvault/key",
		Properties: props,
	}); err != nil {
		return fmt.Errorf("put key %q version %q: %w", keyName, version, err)
	}
	currentKey := keyCurrentKey(vaultName, keyName)
	if err := a.store.Put(currentKey, &store.Resource{
		ID:         currentKey,
		Name:       keyName,
		Type:       "keyvault/key/current",
		Properties: map[string]interface{}{"version": version},
	}); err != nil {
		return fmt.Errorf("update key current pointer %q: %w", keyName, err)
	}
	return nil
}

// resolveKeyVersion returns the versioned resource for keyName, following the
// current pointer when version is empty.
func (a *Router) resolveKeyVersion(vaultName, keyName, version string) (*store.Resource, bool) {
	if version == "" {
		current, ok := a.store.Get(keyCurrentKey(vaultName, keyName))
		if !ok {
			return nil, false
		}
		version, _ = current.Properties["version"].(string)
	}
	return a.store.Get(keyStoreKey(vaultName, keyName, version))
}

func writeKeyNotFound(w http.ResponseWriter, vaultName, ref string) {
	writeAzureError(w, http.StatusNotFound, "KeyNotFound",
		fmt.Sprintf("A key with (name/id) %s/%s was not found in this key vault.", vaultName, ref))
}

func (a *Router) createKeyVaultKey(w http.ResponseWriter, r *http.Request) {
	if !a.requireKeyVaultBearerToken(w, r) {
		return
	}
	vaultName := chi.URLParam(r, "vaultName")
	keyName := chi.URLParam(r, "keyName")

	var body kvKeyCreateBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeAzureError(w, http.StatusBadRequest, "BadParameter", err.Error())
		return
	}
	if body.Kty != "RSA" {
		writeAzureError(w, http.StatusBadRequest, "BadParameter",
			fmt.Sprintf("Key type '%s' is not supported. azemu supports kty=RSA only (sign-only scope).", body.Kty))
		return
	}
	if body.KeySize == 0 {
		body.KeySize = 2048
	}
	if !validRSAKeySizes[body.KeySize] {
		writeAzureError(w, http.StatusBadRequest, "BadParameter",
			fmt.Sprintf("Key size %d is not supported. Valid sizes: 2048, 3072, 4096.", body.KeySize))
		return
	}
	if len(body.KeyOps) == 0 {
		body.KeyOps = []string{"sign", "verify"}
	}

	priv, err := rsa.GenerateKey(rand.Reader, body.KeySize)
	if err != nil {
		writeAzureError(w, http.StatusInternalServerError, "InternalServerError",
			fmt.Sprintf("generate RSA key %q: %s", keyName, err))
		return
	}
	encoded, err := encodePrivateKeyPKCS8(priv)
	if err != nil {
		writeAzureError(w, http.StatusInternalServerError, "InternalServerError",
			fmt.Sprintf("encode key %q: %s", keyName, err))
		return
	}

	version := uuid.New().String()
	kid := a.kvKeyID(vaultName, keyName, version)
	props := map[string]interface{}{
		"key":             publicJWK(kid, &priv.PublicKey, body.KeyOps),
		"attributes":      kvKeyAttributes(body.Attributes),
		"tags":            normaliseTags(body.Tags),
		"version":         version,
		"privateKeyPkcs8": encoded,
	}
	if err := a.storeKeyVersion(vaultName, keyName, version, props); err != nil {
		writeAzureError(w, http.StatusInternalServerError, "InternalServerError", err.Error())
		return
	}

	log.Info().Str("vault", vaultName).Str("key", keyName).Str("version", version).
		Int("key_size", body.KeySize).Msg("key vault key created")
	// keyvault.BaseClient#CreateKey (autorest SDK) expects 200 OK for both the
	// first version and subsequent versions, mirroring SetSecret.
	writeJSON(w, http.StatusOK, kvKeyBundleResponse(props))
}

func (a *Router) importKeyVaultKey(w http.ResponseWriter, r *http.Request) {
	if !a.requireKeyVaultBearerToken(w, r) {
		return
	}
	vaultName := chi.URLParam(r, "vaultName")
	keyName := chi.URLParam(r, "keyName")

	var body kvKeyImportBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeAzureError(w, http.StatusBadRequest, "BadParameter", err.Error())
		return
	}
	if body.Key.Kty != "RSA" {
		writeAzureError(w, http.StatusBadRequest, "BadParameter",
			fmt.Sprintf("Key type '%s' is not supported. azemu supports kty=RSA only (sign-only scope).", body.Key.Kty))
		return
	}
	priv, err := rsaKeyFromJWKParams(body.Key.N, body.Key.E, body.Key.D, body.Key.P, body.Key.Q)
	if err != nil {
		writeAzureError(w, http.StatusBadRequest, "BadParameter",
			fmt.Sprintf("import key %q: %s", keyName, err))
		return
	}
	encoded, err := encodePrivateKeyPKCS8(priv)
	if err != nil {
		writeAzureError(w, http.StatusInternalServerError, "InternalServerError",
			fmt.Sprintf("encode key %q: %s", keyName, err))
		return
	}
	keyOps := body.Key.KeyOps
	if len(keyOps) == 0 {
		keyOps = []string{"sign", "verify"}
	}

	version := uuid.New().String()
	kid := a.kvKeyID(vaultName, keyName, version)
	props := map[string]interface{}{
		"key":             publicJWK(kid, &priv.PublicKey, keyOps),
		"attributes":      kvKeyAttributes(body.Attributes),
		"tags":            normaliseTags(body.Tags),
		"version":         version,
		"privateKeyPkcs8": encoded,
	}
	if err := a.storeKeyVersion(vaultName, keyName, version, props); err != nil {
		writeAzureError(w, http.StatusInternalServerError, "InternalServerError", err.Error())
		return
	}

	log.Info().Str("vault", vaultName).Str("key", keyName).Str("version", version).Msg("key vault key imported")
	writeJSON(w, http.StatusOK, kvKeyBundleResponse(props))
}

func (a *Router) getKeyVaultKey(w http.ResponseWriter, r *http.Request) {
	if !a.requireKeyVaultBearerToken(w, r) {
		return
	}
	vaultName := chi.URLParam(r, "vaultName")
	keyName := chi.URLParam(r, "keyName")

	res, ok := a.resolveKeyVersion(vaultName, keyName, "")
	if !ok {
		writeKeyNotFound(w, vaultName, keyName)
		return
	}
	writeJSON(w, http.StatusOK, kvKeyBundleResponse(res.Properties))
}

func (a *Router) getKeyVaultKeyVersion(w http.ResponseWriter, r *http.Request) {
	if !a.requireKeyVaultBearerToken(w, r) {
		return
	}
	vaultName := chi.URLParam(r, "vaultName")
	keyName := chi.URLParam(r, "keyName")
	version := chi.URLParam(r, "version")

	res, ok := a.store.Get(keyStoreKey(vaultName, keyName, version))
	if !ok {
		writeKeyNotFound(w, vaultName, fmt.Sprintf("%s/%s", keyName, version))
		return
	}
	writeJSON(w, http.StatusOK, kvKeyBundleResponse(res.Properties))
}

// updateKeyVaultKey handles PATCH for both /keys/{name} (autorest sends the
// versionless URL with a trailing slash that StripSlashes removes) and
// /keys/{name}/{version}. azurerm calls UpdateKey to change tags, key_ops,
// or attributes after create.
func (a *Router) updateKeyVaultKey(w http.ResponseWriter, r *http.Request) {
	if !a.requireKeyVaultBearerToken(w, r) {
		return
	}
	vaultName := chi.URLParam(r, "vaultName")
	keyName := chi.URLParam(r, "keyName")
	version := chi.URLParam(r, "version")

	res, ok := a.resolveKeyVersion(vaultName, keyName, version)
	if !ok {
		writeKeyNotFound(w, vaultName, keyName)
		return
	}

	var body kvKeyUpdateBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeAzureError(w, http.StatusBadRequest, "BadParameter", err.Error())
		return
	}

	if body.Tags != nil {
		res.Properties["tags"] = normaliseTags(body.Tags)
	}
	if len(body.KeyOps) > 0 {
		if jwk, ok := res.Properties["key"].(map[string]interface{}); ok {
			jwk["key_ops"] = body.KeyOps
		}
	}
	attrs, _ := res.Properties["attributes"].(map[string]interface{})
	if attrs == nil {
		attrs = kvKeyAttributes(nil)
	}
	for k, v := range body.Attributes {
		if k != "created" && k != "updated" && k != "recoveryLevel" && k != "recoverableDays" {
			attrs[k] = v
		}
	}
	attrs["updated"] = time.Now().Unix()
	res.Properties["attributes"] = attrs

	if err := a.store.Put(res.ID, res); err != nil {
		writeAzureError(w, http.StatusInternalServerError, "InternalServerError",
			fmt.Sprintf("update key %q: %s", keyName, err))
		return
	}

	log.Info().Str("vault", vaultName).Str("key", keyName).Msg("key vault key updated")
	writeJSON(w, http.StatusOK, kvKeyBundleResponse(res.Properties))
}

func (a *Router) deleteKeyVaultKey(w http.ResponseWriter, r *http.Request) {
	if !a.requireKeyVaultBearerToken(w, r) {
		return
	}
	vaultName := chi.URLParam(r, "vaultName")
	keyName := chi.URLParam(r, "keyName")

	currentKey := keyCurrentKey(vaultName, keyName)
	current, ok := a.store.Get(currentKey)
	if !ok {
		writeKeyNotFound(w, vaultName, keyName)
		return
	}
	version, _ := current.Properties["version"].(string)
	latest, _ := a.store.Get(keyStoreKey(vaultName, keyName, version))

	// Delete all versioned entries plus the current pointer.
	prefix := fmt.Sprintf("/keyvault/%s/keys/%s/", vaultName, keyName)
	for _, res := range a.store.List(prefix) {
		a.store.Delete(res.ID)
	}
	a.store.Delete(currentKey)

	log.Info().Str("vault", vaultName).Str("key", keyName).Msg("key vault key deleted")
	// Real KV DeleteKey returns 200 OK with a DeletedKeyBundle (not the
	// 202-style async delete used elsewhere in ARM). The autorest responder
	// used by azurerm expects 200 here.
	resp := map[string]interface{}{
		"recoveryId":         fmt.Sprintf("%s/keyvault/%s/deletedkeys/%s", a.kvEndpoint, vaultName, keyName),
		"deletedDate":        time.Now().Unix(),
		"scheduledPurgeDate": time.Now().Add(90 * 24 * time.Hour).Unix(),
	}
	if latest != nil {
		resp["key"] = latest.Properties["key"]
		resp["attributes"] = latest.Properties["attributes"]
		resp["tags"] = latest.Properties["tags"]
	}
	writeJSON(w, http.StatusOK, resp)
}

func (a *Router) listKeyVaultKeys(w http.ResponseWriter, r *http.Request) {
	if !a.requireKeyVaultBearerToken(w, r) {
		return
	}
	vaultName := chi.URLParam(r, "vaultName")

	items := []map[string]interface{}{}
	for _, res := range a.store.List(keyListPrefix(vaultName)) {
		// Only include current-pointer entries (one row per key, like real KV).
		if res.Type != "keyvault/key/current" {
			continue
		}
		items = append(items, map[string]interface{}{
			"kid": fmt.Sprintf("%s/keys/%s", a.vaultBaseURL(vaultName), res.Name),
			"attributes": map[string]interface{}{
				"enabled":       true,
				"recoveryLevel": "Purgeable",
			},
		})
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"value": items, "nextLink": nil})
}

func (a *Router) listKeyVaultKeyVersions(w http.ResponseWriter, r *http.Request) {
	if !a.requireKeyVaultBearerToken(w, r) {
		return
	}
	vaultName := chi.URLParam(r, "vaultName")
	keyName := chi.URLParam(r, "keyName")

	if _, ok := a.store.Get(keyCurrentKey(vaultName, keyName)); !ok {
		writeKeyNotFound(w, vaultName, keyName)
		return
	}

	prefix := fmt.Sprintf("/keyvault/%s/keys/%s/", vaultName, keyName)
	items := []map[string]interface{}{}
	for _, res := range a.store.List(prefix) {
		if res.Type == "keyvault/key/current" {
			continue
		}
		jwk, _ := res.Properties["key"].(map[string]interface{})
		if jwk == nil {
			continue
		}
		items = append(items, map[string]interface{}{
			"kid":        jwk["kid"],
			"attributes": res.Properties["attributes"],
		})
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"value": items, "nextLink": nil})
}

func (a *Router) signKeyVaultKey(w http.ResponseWriter, r *http.Request) {
	if !a.requireKeyVaultBearerToken(w, r) {
		return
	}
	vaultName := chi.URLParam(r, "vaultName")
	keyName := chi.URLParam(r, "keyName")
	// version is empty on the versionless route POST /keys/{name}/sign;
	// resolveKeyVersion then follows the current pointer. Azure SDKs and
	// `az keyvault key sign` omit the version to mean "latest".
	version := chi.URLParam(r, "version")

	res, ok := a.resolveKeyVersion(vaultName, keyName, version)
	if !ok {
		ref := keyName
		if version != "" {
			ref = fmt.Sprintf("%s/%s", keyName, version)
		}
		writeKeyNotFound(w, vaultName, ref)
		return
	}

	var body kvSignBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeAzureError(w, http.StatusBadRequest, "BadParameter", err.Error())
		return
	}
	hash, ok := signAlgorithms[body.Alg]
	if !ok {
		writeAzureError(w, http.StatusBadRequest, "BadParameter",
			fmt.Sprintf("Algorithm '%s' is not supported. azemu supports: RS256.", body.Alg))
		return
	}

	jwk, _ := res.Properties["key"].(map[string]interface{})
	if !keyOpsAllow(jwk, "sign") {
		writeAzureError(w, http.StatusForbidden, "Forbidden",
			fmt.Sprintf("Operation 'sign' is not permitted by the key_ops of key %s/%s.", vaultName, keyName))
		return
	}

	digest, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(body.Value, "="))
	if err != nil {
		writeAzureError(w, http.StatusBadRequest, "BadParameter",
			fmt.Sprintf("decode digest value: %s", err))
		return
	}
	if len(digest) != hash.Size() {
		writeAzureError(w, http.StatusBadRequest, "BadParameter",
			fmt.Sprintf("Digest length %d does not match algorithm %s (expected %d bytes).",
				len(digest), body.Alg, hash.Size()))
		return
	}

	encoded, _ := res.Properties["privateKeyPkcs8"].(string)
	priv, err := decodePrivateKeyPKCS8(encoded)
	if err != nil {
		writeAzureError(w, http.StatusInternalServerError, "InternalServerError",
			fmt.Sprintf("load key %q: %s", keyName, err))
		return
	}
	sig, err := rsa.SignPKCS1v15(rand.Reader, priv, hash, digest)
	if err != nil {
		writeAzureError(w, http.StatusInternalServerError, "InternalServerError",
			fmt.Sprintf("sign with key %q: %s", keyName, err))
		return
	}

	log.Info().Str("vault", vaultName).Str("key", keyName).Str("version", version).
		Str("alg", body.Alg).Msg("key vault sign")
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"kid":   jwk["kid"],
		"value": base64.RawURLEncoding.EncodeToString(sig),
	})
}

// keyOpsAllow reports whether the JWK's key_ops list permits op.
func keyOpsAllow(jwk map[string]interface{}, op string) bool {
	if jwk == nil {
		return false
	}
	switch ops := jwk["key_ops"].(type) {
	case []string:
		for _, o := range ops {
			if o == op {
				return true
			}
		}
	case []interface{}:
		for _, o := range ops {
			if s, ok := o.(string); ok && s == op {
				return true
			}
		}
	}
	return false
}

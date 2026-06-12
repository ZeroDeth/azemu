package arm

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"testing"
)

func kvKeyURL(srv, vault, key string) string {
	return fmt.Sprintf("%s/keyvault/%s/keys/%s", srv, vault, key)
}

func kvKeyVersionURL(srv, vault, key, version string) string {
	return fmt.Sprintf("%s/keyvault/%s/keys/%s/%s", srv, vault, key, version)
}

func kvKeySignURL(srv, vault, key, version string) string {
	return fmt.Sprintf("%s/keyvault/%s/keys/%s/%s/sign", srv, vault, key, version)
}

func kvKeyCreateURL(srv, vault, key string) string {
	return fmt.Sprintf("%s/keyvault/%s/keys/%s/create", srv, vault, key)
}

// createTestKey creates an RSA key via the data plane and returns the decoded
// bundle response.
func createTestKey(t *testing.T, srv, vault, key, body string) map[string]interface{} {
	t.Helper()
	resp := httpPost(t, kvKeyCreateURL(srv, vault, key), body)
	assertStatus(t, resp, http.StatusOK)
	return decodeJSON(t, resp)
}

// jwkFromBundle extracts the "key" JWK object from a bundle response.
func jwkFromBundle(t *testing.T, bundle map[string]interface{}) map[string]interface{} {
	t.Helper()
	jwk, ok := bundle["key"].(map[string]interface{})
	if !ok {
		t.Fatalf("bundle has no key object: %v", bundle)
	}
	return jwk
}

// rsaPublicFromJWK rebuilds an rsa.PublicKey from the n/e fields of a JWK.
func rsaPublicFromJWK(t *testing.T, jwk map[string]interface{}) *rsa.PublicKey {
	t.Helper()
	nB, err := base64.RawURLEncoding.DecodeString(jwk["n"].(string))
	if err != nil {
		t.Fatalf("decode jwk n: %v", err)
	}
	eB, err := base64.RawURLEncoding.DecodeString(jwk["e"].(string))
	if err != nil {
		t.Fatalf("decode jwk e: %v", err)
	}
	return &rsa.PublicKey{
		N: new(big.Int).SetBytes(nB),
		E: int(new(big.Int).SetBytes(eB).Int64()),
	}
}

func TestKVKey_createReturnsBundle(t *testing.T) {
	srv := newTestServer(t)
	bundle := createTestKey(t, srv.URL, "vault1", "ota-signing",
		`{"kty":"RSA","key_size":2048,"key_ops":["sign","verify"],"tags":{"env":"test"}}`)

	jwk := jwkFromBundle(t, bundle)
	kid, _ := jwk["kid"].(string)
	// Root-form kid: the azurerm provider's ParseNestedItemID requires
	// /keys/{name}/{version} with no vault path segment.
	wantPrefix := "https://vault1.vault.localhost/keys/ota-signing/"
	if !strings.HasPrefix(kid, wantPrefix) {
		t.Errorf("kid = %q, want prefix %q", kid, wantPrefix)
	}
	if jwk["kty"] != "RSA" {
		t.Errorf("kty = %v, want RSA", jwk["kty"])
	}
	pub := rsaPublicFromJWK(t, jwk)
	if got := pub.N.BitLen(); got != 2048 {
		t.Errorf("modulus bit length = %d, want 2048", got)
	}
	if pub.E != 65537 {
		t.Errorf("exponent = %d, want 65537", pub.E)
	}
	attrs, _ := bundle["attributes"].(map[string]interface{})
	if attrs["enabled"] != true {
		t.Errorf("attributes.enabled = %v, want true", attrs["enabled"])
	}
	tags, _ := bundle["tags"].(map[string]interface{})
	if tags["env"] != "test" {
		t.Errorf("tags.env = %v, want test", tags["env"])
	}
}

func TestKVKey_responseNeverLeaksPrivateMaterial(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPost(t, kvKeyCreateURL(srv.URL, "vault1", "leakcheck"), `{"kty":"RSA"}`)
	assertStatus(t, resp, http.StatusOK)
	body := readBody(t, resp)
	for _, field := range []string{`"d"`, `"p"`, `"q"`, `"dp"`, `"dq"`, `"qi"`, "privateKeyPkcs8"} {
		if strings.Contains(body, field) {
			t.Errorf("create response contains private field %s: %s", field, body)
		}
	}

	resp = httpGet(t, kvKeyURL(srv.URL, "vault1", "leakcheck"))
	assertStatus(t, resp, http.StatusOK)
	body = readBody(t, resp)
	if strings.Contains(body, "privateKeyPkcs8") {
		t.Errorf("GET response contains private key material: %s", body)
	}
}

func TestKVKey_createNewVersionPerCall(t *testing.T) {
	srv := newTestServer(t)
	b1 := createTestKey(t, srv.URL, "vault1", "rotating", `{"kty":"RSA"}`)
	b2 := createTestKey(t, srv.URL, "vault1", "rotating", `{"kty":"RSA"}`)

	kid1 := jwkFromBundle(t, b1)["kid"].(string)
	kid2 := jwkFromBundle(t, b2)["kid"].(string)
	if kid1 == kid2 {
		t.Fatalf("two creates returned the same kid %q", kid1)
	}

	// Versionless GET must return the latest version.
	resp := httpGet(t, kvKeyURL(srv.URL, "vault1", "rotating"))
	assertStatus(t, resp, http.StatusOK)
	got := jwkFromBundle(t, decodeJSON(t, resp))["kid"].(string)
	if got != kid2 {
		t.Errorf("versionless GET kid = %q, want latest %q", got, kid2)
	}
}

func TestKVKey_createRejectsUnsupported(t *testing.T) {
	srv := newTestServer(t)
	cases := []struct {
		name string
		body string
	}{
		{"EC", `{"kty":"EC","crv":"P-256"}`},
		{"EC-HSM", `{"kty":"EC-HSM"}`},
		{"oct", `{"kty":"oct"}`},
		{"RSA-HSM", `{"kty":"RSA-HSM"}`},
		{"missing kty", `{}`},
		{"key_size 1024", `{"kty":"RSA","key_size":1024}`},
		{"invalid json", `{not-json`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := httpPost(t, kvKeyCreateURL(srv.URL, "vault1", "bad"), tc.body)
			assertStatus(t, resp, http.StatusBadRequest)
			m := decodeJSON(t, resp)
			errObj, _ := m["error"].(map[string]interface{})
			if errObj["code"] != "BadParameter" {
				t.Errorf("error code = %v, want BadParameter", errObj["code"])
			}
		})
	}
}

func TestKVKey_importRoundTrip(t *testing.T) {
	srv := newTestServer(t)
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	b64 := func(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }
	body := fmt.Sprintf(
		`{"key":{"kty":"RSA","key_ops":["sign"],"n":"%s","e":"%s","d":"%s","p":"%s","q":"%s"}}`,
		b64(priv.N.Bytes()),
		b64(big.NewInt(int64(priv.E)).Bytes()),
		b64(priv.D.Bytes()),
		b64(priv.Primes[0].Bytes()),
		b64(priv.Primes[1].Bytes()),
	)

	resp := httpPut(t, kvKeyURL(srv.URL, "vault1", "imported"), body)
	assertStatus(t, resp, http.StatusOK)

	got := httpGet(t, kvKeyURL(srv.URL, "vault1", "imported"))
	assertStatus(t, got, http.StatusOK)
	pub := rsaPublicFromJWK(t, jwkFromBundle(t, decodeJSON(t, got)))
	if pub.N.Cmp(priv.N) != 0 {
		t.Error("imported key modulus does not match original")
	}
	if pub.E != priv.E {
		t.Errorf("imported key exponent = %d, want %d", pub.E, priv.E)
	}
}

func TestKVKey_getMissing404(t *testing.T) {
	srv := newTestServer(t)
	resp := httpGet(t, kvKeyURL(srv.URL, "vault1", "ghost"))
	assertStatus(t, resp, http.StatusNotFound)
	m := decodeJSON(t, resp)
	errObj, _ := m["error"].(map[string]interface{})
	if errObj["code"] != "KeyNotFound" {
		t.Errorf("error code = %v, want KeyNotFound", errObj["code"])
	}

	resp = httpGet(t, kvKeyVersionURL(srv.URL, "vault1", "ghost", "v1"))
	assertStatus(t, resp, http.StatusNotFound)
}

func TestKVKey_listAndVersions(t *testing.T) {
	srv := newTestServer(t)
	createTestKey(t, srv.URL, "vault1", "key-a", `{"kty":"RSA"}`)
	createTestKey(t, srv.URL, "vault1", "key-a", `{"kty":"RSA"}`)
	createTestKey(t, srv.URL, "vault1", "key-b", `{"kty":"RSA"}`)

	resp := httpGet(t, fmt.Sprintf("%s/keyvault/vault1/keys", srv.URL))
	assertStatus(t, resp, http.StatusOK)
	list := decodeJSON(t, resp)
	items, _ := list["value"].([]interface{})
	if len(items) != 2 {
		t.Errorf("list returned %d keys, want 2 (one row per key)", len(items))
	}
	if _, hasNext := list["nextLink"]; !hasNext {
		t.Error("list response missing nextLink field")
	}

	resp = httpGet(t, fmt.Sprintf("%s/keyvault/vault1/keys/key-a/versions", srv.URL))
	assertStatus(t, resp, http.StatusOK)
	versions := decodeJSON(t, resp)
	vItems, _ := versions["value"].([]interface{})
	if len(vItems) != 2 {
		t.Errorf("versions returned %d entries, want 2", len(vItems))
	}

	resp = httpGet(t, fmt.Sprintf("%s/keyvault/vault1/keys/ghost/versions", srv.URL))
	assertStatus(t, resp, http.StatusNotFound)
}

func TestKVKey_patchUpdatesTags(t *testing.T) {
	srv := newTestServer(t)
	bundle := createTestKey(t, srv.URL, "vault1", "patched", `{"kty":"RSA"}`)
	kid := jwkFromBundle(t, bundle)["kid"].(string)
	version := kid[strings.LastIndex(kid, "/")+1:]

	// Versionless PATCH (autorest sends /keys/{name}/ which StripSlashes reduces).
	resp := httpPatch(t, kvKeyURL(srv.URL, "vault1", "patched"), `{"tags":{"stage":"prod"}}`)
	assertStatus(t, resp, http.StatusOK)
	tags, _ := decodeJSON(t, resp)["tags"].(map[string]interface{})
	if tags["stage"] != "prod" {
		t.Errorf("tags.stage = %v, want prod", tags["stage"])
	}

	// Versioned PATCH.
	resp = httpPatch(t, kvKeyVersionURL(srv.URL, "vault1", "patched", version), `{"tags":{"stage":"canary"}}`)
	assertStatus(t, resp, http.StatusOK)
	tags, _ = decodeJSON(t, resp)["tags"].(map[string]interface{})
	if tags["stage"] != "canary" {
		t.Errorf("tags.stage = %v, want canary", tags["stage"])
	}

	resp = httpPatch(t, kvKeyURL(srv.URL, "vault1", "ghost"), `{"tags":{}}`)
	assertStatus(t, resp, http.StatusNotFound)
}

// TestKVKey_signVerifiableAgainstJWK is the headline test for the OTA
// pipeline: a signature produced by the sign endpoint must verify against
// the public JWK returned by GET, using standard RS256 verification.
func TestKVKey_signVerifiableAgainstJWK(t *testing.T) {
	srv := newTestServer(t)
	bundle := createTestKey(t, srv.URL, "vault1", "manifest-signer",
		`{"kty":"RSA","key_size":2048,"key_ops":["sign","verify"]}`)
	jwk := jwkFromBundle(t, bundle)
	kid := jwk["kid"].(string)
	version := kid[strings.LastIndex(kid, "/")+1:]

	manifest := []byte(`{"id":"0001","launchAsset":{"url":"https://example.com/bundle.js"}}`)
	digest := sha256.Sum256(manifest)
	signBody := fmt.Sprintf(`{"alg":"RS256","value":"%s"}`,
		base64.RawURLEncoding.EncodeToString(digest[:]))

	resp := httpPost(t, kvKeySignURL(srv.URL, "vault1", "manifest-signer", version), signBody)
	assertStatus(t, resp, http.StatusOK)
	result := decodeJSON(t, resp)
	if result["kid"] != kid {
		t.Errorf("sign kid = %v, want %v", result["kid"], kid)
	}

	sig, err := base64.RawURLEncoding.DecodeString(result["value"].(string))
	if err != nil {
		t.Fatalf("decode signature: %v", err)
	}
	pub := rsaPublicFromJWK(t, jwk)
	if err := rsa.VerifyPKCS1v15(pub, crypto.SHA256, digest[:], sig); err != nil {
		t.Fatalf("signature does not verify against returned JWK: %v", err)
	}
}

// TestKVKey_signVersionless covers POST /keys/{name}/sign without a version:
// Azure SDKs and `az keyvault key sign` omit the version to mean "latest".
// The signature must come from the newest version's key.
func TestKVKey_signVersionless(t *testing.T) {
	srv := newTestServer(t)
	createTestKey(t, srv.URL, "vault1", "latest-signer", `{"kty":"RSA"}`)
	b2 := createTestKey(t, srv.URL, "vault1", "latest-signer", `{"kty":"RSA"}`)
	latestJWK := jwkFromBundle(t, b2)
	latestKid := latestJWK["kid"].(string)

	digest := sha256.Sum256([]byte("versionless"))
	body := fmt.Sprintf(`{"alg":"RS256","value":"%s"}`,
		base64.RawURLEncoding.EncodeToString(digest[:]))
	resp := httpPost(t, kvKeyURL(srv.URL, "vault1", "latest-signer")+"/sign", body)
	assertStatus(t, resp, http.StatusOK)
	result := decodeJSON(t, resp)
	if result["kid"] != latestKid {
		t.Errorf("versionless sign kid = %v, want latest %v", result["kid"], latestKid)
	}

	sig, err := base64.RawURLEncoding.DecodeString(result["value"].(string))
	if err != nil {
		t.Fatalf("decode signature: %v", err)
	}
	pub := rsaPublicFromJWK(t, latestJWK)
	if err := rsa.VerifyPKCS1v15(pub, crypto.SHA256, digest[:], sig); err != nil {
		t.Fatalf("versionless signature does not verify against latest JWK: %v", err)
	}

	// Unknown key on the versionless route is still KeyNotFound.
	resp = httpPost(t, kvKeyURL(srv.URL, "vault1", "ghost")+"/sign", body)
	assertStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
}

func TestKVKey_signRejections(t *testing.T) {
	srv := newTestServer(t)
	bundle := createTestKey(t, srv.URL, "vault1", "strict", `{"kty":"RSA"}`)
	kid := jwkFromBundle(t, bundle)["kid"].(string)
	version := kid[strings.LastIndex(kid, "/")+1:]

	digest := sha256.Sum256([]byte("data"))
	goodValue := base64.RawURLEncoding.EncodeToString(digest[:])

	cases := []struct {
		name       string
		url        string
		body       string
		wantStatus int
		wantCode   string
	}{
		{"RS384 unsupported", kvKeySignURL(srv.URL, "vault1", "strict", version),
			fmt.Sprintf(`{"alg":"RS384","value":"%s"}`, goodValue), http.StatusBadRequest, "BadParameter"},
		{"ES256 unsupported", kvKeySignURL(srv.URL, "vault1", "strict", version),
			fmt.Sprintf(`{"alg":"ES256","value":"%s"}`, goodValue), http.StatusBadRequest, "BadParameter"},
		{"PS256 unsupported", kvKeySignURL(srv.URL, "vault1", "strict", version),
			fmt.Sprintf(`{"alg":"PS256","value":"%s"}`, goodValue), http.StatusBadRequest, "BadParameter"},
		{"empty alg", kvKeySignURL(srv.URL, "vault1", "strict", version),
			fmt.Sprintf(`{"value":"%s"}`, goodValue), http.StatusBadRequest, "BadParameter"},
		{"short digest", kvKeySignURL(srv.URL, "vault1", "strict", version),
			fmt.Sprintf(`{"alg":"RS256","value":"%s"}`, base64.RawURLEncoding.EncodeToString([]byte("too-short"))),
			http.StatusBadRequest, "BadParameter"},
		{"bad base64", kvKeySignURL(srv.URL, "vault1", "strict", version),
			`{"alg":"RS256","value":"!!!not-base64!!!"}`, http.StatusBadRequest, "BadParameter"},
		{"unknown key", kvKeySignURL(srv.URL, "vault1", "ghost", "v1"),
			fmt.Sprintf(`{"alg":"RS256","value":"%s"}`, goodValue), http.StatusNotFound, "KeyNotFound"},
		{"unknown version", kvKeySignURL(srv.URL, "vault1", "strict", "no-such-version"),
			fmt.Sprintf(`{"alg":"RS256","value":"%s"}`, goodValue), http.StatusNotFound, "KeyNotFound"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := httpPost(t, tc.url, tc.body)
			assertStatus(t, resp, tc.wantStatus)
			m := decodeJSON(t, resp)
			errObj, _ := m["error"].(map[string]interface{})
			if errObj["code"] != tc.wantCode {
				t.Errorf("error code = %v, want %s", errObj["code"], tc.wantCode)
			}
		})
	}
}

func TestKVKey_signForbiddenWithoutSignOp(t *testing.T) {
	srv := newTestServer(t)
	bundle := createTestKey(t, srv.URL, "vault1", "verify-only",
		`{"kty":"RSA","key_ops":["verify"]}`)
	kid := jwkFromBundle(t, bundle)["kid"].(string)
	version := kid[strings.LastIndex(kid, "/")+1:]

	digest := sha256.Sum256([]byte("data"))
	body := fmt.Sprintf(`{"alg":"RS256","value":"%s"}`,
		base64.RawURLEncoding.EncodeToString(digest[:]))
	resp := httpPost(t, kvKeySignURL(srv.URL, "vault1", "verify-only", version), body)
	assertStatus(t, resp, http.StatusForbidden)
	m := decodeJSON(t, resp)
	errObj, _ := m["error"].(map[string]interface{})
	if errObj["code"] != "Forbidden" {
		t.Errorf("error code = %v, want Forbidden", errObj["code"])
	}
}

// TestKVKey_rootLevelRoutes covers the access pattern the azurerm provider
// actually uses after parsing a kid with ParseNestedItemID: requests against
// {host}/keys/{name}[/{version}] with no vault path segment. azemu resolves
// the owning vault by key name.
func TestKVKey_rootLevelRoutes(t *testing.T) {
	srv := newTestServer(t)
	bundle := createTestKey(t, srv.URL, "vault1", "root-routed", `{"kty":"RSA"}`)
	jwk := jwkFromBundle(t, bundle)
	kid := jwk["kid"].(string)
	version := kid[strings.LastIndex(kid, "/")+1:]

	// kid itself must be a fetchable URL on the test server after swapping
	// the configured kvEndpoint host for the test server's.
	if !strings.HasPrefix(kid, "https://vault1.vault.localhost/keys/root-routed/") {
		t.Fatalf("kid = %q, want root-form prefix", kid)
	}

	// Versionless GET at root.
	resp := httpGet(t, srv.URL+"/keys/root-routed")
	assertStatus(t, resp, http.StatusOK)
	if got := jwkFromBundle(t, decodeJSON(t, resp))["kid"]; got != kid {
		t.Errorf("root GET kid = %v, want %v", got, kid)
	}

	// Versioned GET at root.
	resp = httpGet(t, srv.URL+"/keys/root-routed/"+version)
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	// Sign at root (versioned and versionless).
	digest := sha256.Sum256([]byte("root"))
	signBody := fmt.Sprintf(`{"alg":"RS256","value":"%s"}`,
		base64.RawURLEncoding.EncodeToString(digest[:]))
	resp = httpPost(t, srv.URL+"/keys/root-routed/"+version+"/sign", signBody)
	assertStatus(t, resp, http.StatusOK)
	sig, err := base64.RawURLEncoding.DecodeString(decodeJSON(t, resp)["value"].(string))
	if err != nil {
		t.Fatalf("decode signature: %v", err)
	}
	if err := rsa.VerifyPKCS1v15(rsaPublicFromJWK(t, jwk), crypto.SHA256, digest[:], sig); err != nil {
		t.Fatalf("root-level signature does not verify: %v", err)
	}
	resp = httpPost(t, srv.URL+"/keys/root-routed/sign", signBody)
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	// PATCH at root.
	resp = httpPatch(t, srv.URL+"/keys/root-routed", `{"tags":{"via":"root"}}`)
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	// Unknown name resolves no vault: 404 KeyNotFound.
	resp = httpGet(t, srv.URL+"/keys/ghost")
	assertStatus(t, resp, http.StatusNotFound)
	m := decodeJSON(t, resp)
	errObj, _ := m["error"].(map[string]interface{})
	if errObj["code"] != "KeyNotFound" {
		t.Errorf("error code = %v, want KeyNotFound", errObj["code"])
	}

	// DELETE at root removes the key from its vault.
	resp = httpDelete(t, srv.URL+"/keys/root-routed")
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()
	assertStatus(t, httpGet(t, kvKeyURL(srv.URL, "vault1", "root-routed")), http.StatusNotFound)
}

// TestKVSecret_rootLevelRoutes mirrors TestKVKey_rootLevelRoutes for secrets,
// whose ids are parsed by the provider the same way.
func TestKVSecret_rootLevelRoutes(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, fmt.Sprintf("%s/keyvault/vault1/secrets/root-secret", srv.URL), `{"value":"s3cret"}`)
	assertStatus(t, resp, http.StatusOK)
	id, _ := decodeJSON(t, resp)["id"].(string)
	if !strings.HasPrefix(id, "https://vault1.vault.localhost/secrets/root-secret/") {
		t.Fatalf("id = %q, want root-form prefix", id)
	}

	resp = httpGet(t, srv.URL+"/secrets/root-secret")
	assertStatus(t, resp, http.StatusOK)
	if got := decodeJSON(t, resp)["value"]; got != "s3cret" {
		t.Errorf("root GET value = %v, want s3cret", got)
	}

	version := id[strings.LastIndex(id, "/")+1:]
	resp = httpGet(t, srv.URL+"/secrets/root-secret/"+version)
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	resp = httpDelete(t, srv.URL+"/secrets/root-secret")
	assertStatus(t, resp, http.StatusAccepted)
	resp.Body.Close()
	assertStatus(t, httpGet(t, srv.URL+"/secrets/root-secret"), http.StatusNotFound)
}

// TestKVVault_dataPlaneRootPing covers the provider's availability poll of
// GET {vaultUri}: 200 for existing vaults, 404 for unknown ones.
func TestKVVault_dataPlaneRootPing(t *testing.T) {
	srv := newTestServer(t)
	vaultURL := srv.URL + "/subscriptions/sub1/resourcegroups/rg1/providers/microsoft.keyvault/vaults/pingvault"
	resp := httpPut(t, vaultURL, `{"location":"uksouth","properties":{"tenantId":"t1","sku":{"family":"A","name":"standard"}}}`)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	assertStatus(t, httpGet(t, srv.URL+"/keyvault/pingvault"), http.StatusOK)
	assertStatus(t, httpGet(t, srv.URL+"/keyvault/ghostvault"), http.StatusNotFound)
}

func TestKVKey_deleteCascadesVersions(t *testing.T) {
	srv := newTestServer(t)
	createTestKey(t, srv.URL, "vault1", "doomed", `{"kty":"RSA"}`)
	createTestKey(t, srv.URL, "vault1", "doomed", `{"kty":"RSA"}`)

	resp := httpDelete(t, kvKeyURL(srv.URL, "vault1", "doomed"))
	assertStatus(t, resp, http.StatusOK)
	m := decodeJSON(t, resp)
	if _, ok := m["recoveryId"]; !ok {
		t.Error("delete response missing recoveryId")
	}
	if _, ok := m["key"]; !ok {
		t.Error("delete response missing key bundle")
	}

	assertStatus(t, httpGet(t, kvKeyURL(srv.URL, "vault1", "doomed")), http.StatusNotFound)
	assertStatus(t, httpGet(t, fmt.Sprintf("%s/keyvault/vault1/keys/doomed/versions", srv.URL)), http.StatusNotFound)

	// Deleting again is 404; recreating works.
	assertStatus(t, httpDelete(t, kvKeyURL(srv.URL, "vault1", "doomed")), http.StatusNotFound)
	createTestKey(t, srv.URL, "vault1", "doomed", `{"kty":"RSA"}`)
}

// TestKVKey_vaultDeleteCascadesKeys is the regression test for the vault
// cascade: ARM-deleting the vault must remove data-plane keys too.
func TestKVKey_vaultDeleteCascadesKeys(t *testing.T) {
	srv := newTestServer(t)
	vaultURL := srv.URL + "/subscriptions/sub1/resourcegroups/rg1/providers/microsoft.keyvault/vaults/cascade-vault"
	resp := httpPut(t, vaultURL, `{"location":"uksouth","properties":{"tenantId":"t1","sku":{"family":"A","name":"standard"}}}`)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	createTestKey(t, srv.URL, "cascade-vault", "orphan", `{"kty":"RSA"}`)

	resp = httpDelete(t, vaultURL)
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	assertStatus(t, httpGet(t, kvKeyURL(srv.URL, "cascade-vault", "orphan")), http.StatusNotFound)
}

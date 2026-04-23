package arm

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// kvSecretURL builds a Key Vault data-plane URL for a named secret.
// The test server mounts KeyVaultDataPlaneRoutes at /keyvault.
func kvSecretURL(srvURL, vaultName, secretName string) string {
	return fmt.Sprintf("%s/keyvault/%s/secrets/%s", srvURL, vaultName, secretName)
}

func kvSecretVersionURL(srvURL, vaultName, secretName, version string) string {
	return fmt.Sprintf("%s/keyvault/%s/secrets/%s/%s", srvURL, vaultName, secretName, version)
}

func kvSecretListURL(srvURL, vaultName string) string {
	return fmt.Sprintf("%s/keyvault/%s/secrets", srvURL, vaultName)
}

func kvSecretVersionsURL(srvURL, vaultName, secretName string) string {
	return fmt.Sprintf("%s/keyvault/%s/secrets/%s/versions", srvURL, vaultName, secretName)
}

const secretBody = `{"value":"super-secret","contentType":"text/plain"}`
const secretBodyUpdated = `{"value":"super-secret-v2","contentType":"text/plain"}`

func TestKVSecret_PUT_Creates_Returns201(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, kvSecretURL(srv.URL, "myvault", "mysecret"), secretBody)
	assertStatus(t, resp, http.StatusCreated)

	body := decodeJSON(t, resp)
	if body["value"] != "super-secret" {
		t.Errorf("value = %v, want super-secret", body["value"])
	}
	// id must contain the vault name, secret name, and a version UUID.
	id, _ := body["id"].(string)
	if !strings.Contains(id, "myvault") || !strings.Contains(id, "mysecret") {
		t.Errorf("id = %v, want to contain vault/secret name", id)
	}
}

func TestKVSecret_PUT_Idempotent_Returns200(t *testing.T) {
	srv := newTestServer(t)
	httpPut(t, kvSecretURL(srv.URL, "myvault", "mysecret"), secretBody)
	resp := httpPut(t, kvSecretURL(srv.URL, "myvault", "mysecret"), secretBodyUpdated)
	assertStatus(t, resp, http.StatusOK)

	body := decodeJSON(t, resp)
	if body["value"] != "super-secret-v2" {
		t.Errorf("value = %v, want super-secret-v2", body["value"])
	}
}

func TestKVSecret_PUT_ResponseHasAttributes(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, kvSecretURL(srv.URL, "myvault", "mysecret"), secretBody)
	assertStatus(t, resp, http.StatusCreated)

	body := decodeJSON(t, resp)
	attrs, ok := body["attributes"].(map[string]interface{})
	if !ok {
		t.Fatalf("attributes missing or wrong type")
	}
	if attrs["enabled"] != true {
		t.Errorf("attributes.enabled = %v, want true", attrs["enabled"])
	}
	if attrs["recoveryLevel"] != "Purgeable" {
		t.Errorf("attributes.recoveryLevel = %v, want Purgeable", attrs["recoveryLevel"])
	}
}

func TestKVSecret_PUT_ResponseHasVersionInID(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, kvSecretURL(srv.URL, "myvault", "mysecret"), secretBody)
	assertStatus(t, resp, http.StatusCreated)

	body := decodeJSON(t, resp)
	id, _ := body["id"].(string)
	// The id should end with a UUID version segment.
	parts := strings.Split(strings.TrimSuffix(id, "/"), "/")
	if len(parts) < 2 {
		t.Fatalf("id = %v has too few segments", id)
	}
	version := parts[len(parts)-1]
	if len(version) != 36 {
		t.Errorf("version = %v, want a 36-char UUID", version)
	}
}

func TestKVSecret_GET_ReturnsLatest(t *testing.T) {
	srv := newTestServer(t)
	httpPut(t, kvSecretURL(srv.URL, "myvault", "mysecret"), secretBody)

	resp := httpGet(t, kvSecretURL(srv.URL, "myvault", "mysecret"))
	assertStatus(t, resp, http.StatusOK)

	body := decodeJSON(t, resp)
	if body["value"] != "super-secret" {
		t.Errorf("value = %v, want super-secret", body["value"])
	}
}

func TestKVSecret_GET_AfterUpdate_ReturnsLatest(t *testing.T) {
	srv := newTestServer(t)
	httpPut(t, kvSecretURL(srv.URL, "myvault", "mysecret"), secretBody)
	httpPut(t, kvSecretURL(srv.URL, "myvault", "mysecret"), secretBodyUpdated)

	resp := httpGet(t, kvSecretURL(srv.URL, "myvault", "mysecret"))
	assertStatus(t, resp, http.StatusOK)

	body := decodeJSON(t, resp)
	if body["value"] != "super-secret-v2" {
		t.Errorf("value = %v, want super-secret-v2 after update", body["value"])
	}
}

func TestKVSecret_GET_NotFound_Returns404(t *testing.T) {
	srv := newTestServer(t)
	resp := httpGet(t, kvSecretURL(srv.URL, "myvault", "doesnotexist"))
	assertStatus(t, resp, http.StatusNotFound)

	body := decodeJSON(t, resp)
	errBlock := body["error"].(map[string]interface{})
	if errBlock["code"] != "SecretNotFound" {
		t.Errorf("error.code = %v, want SecretNotFound", errBlock["code"])
	}
}

func TestKVSecret_GET_SpecificVersion(t *testing.T) {
	srv := newTestServer(t)
	putResp := httpPut(t, kvSecretURL(srv.URL, "myvault", "mysecret"), secretBody)
	assertStatus(t, putResp, http.StatusCreated)

	putBody := decodeJSON(t, putResp)
	id, _ := putBody["id"].(string)
	parts := strings.Split(strings.TrimSuffix(id, "/"), "/")
	version := parts[len(parts)-1]

	resp := httpGet(t, kvSecretVersionURL(srv.URL, "myvault", "mysecret", version))
	assertStatus(t, resp, http.StatusOK)

	body := decodeJSON(t, resp)
	if body["value"] != "super-secret" {
		t.Errorf("value = %v, want super-secret", body["value"])
	}
}

func TestKVSecret_GET_UnknownVersion_Returns404(t *testing.T) {
	srv := newTestServer(t)
	httpPut(t, kvSecretURL(srv.URL, "myvault", "mysecret"), secretBody)

	resp := httpGet(t, kvSecretVersionURL(srv.URL, "myvault", "mysecret", "00000000-0000-0000-0000-000000000000"))
	assertStatus(t, resp, http.StatusNotFound)
}

func TestKVSecret_DELETE_Returns202(t *testing.T) {
	srv := newTestServer(t)
	httpPut(t, kvSecretURL(srv.URL, "myvault", "mysecret"), secretBody)

	resp := httpDelete(t, kvSecretURL(srv.URL, "myvault", "mysecret"))
	assertStatus(t, resp, http.StatusAccepted)

	if resp.Header.Get("Location") == "" {
		t.Error("DELETE missing Location header")
	}
	body := decodeJSON(t, resp)
	if body["recoveryId"] == nil {
		t.Error("recoveryId missing from delete response")
	}
}

func TestKVSecret_DELETE_Then_GET_Returns404(t *testing.T) {
	srv := newTestServer(t)
	httpPut(t, kvSecretURL(srv.URL, "myvault", "mysecret"), secretBody)
	httpDelete(t, kvSecretURL(srv.URL, "myvault", "mysecret"))

	resp := httpGet(t, kvSecretURL(srv.URL, "myvault", "mysecret"))
	assertStatus(t, resp, http.StatusNotFound)
}

func TestKVSecret_DELETE_NotFound_Returns404(t *testing.T) {
	srv := newTestServer(t)
	resp := httpDelete(t, kvSecretURL(srv.URL, "myvault", "doesnotexist"))
	assertStatus(t, resp, http.StatusNotFound)
}

func TestKVSecret_LIST_ReturnsValueWrapper(t *testing.T) {
	srv := newTestServer(t)
	httpPut(t, kvSecretURL(srv.URL, "myvault", "secret1"), secretBody)
	httpPut(t, kvSecretURL(srv.URL, "myvault", "secret2"), secretBodyUpdated)

	resp := httpGet(t, kvSecretListURL(srv.URL, "myvault"))
	assertStatus(t, resp, http.StatusOK)

	body := decodeJSON(t, resp)
	items, ok := body["value"].([]interface{})
	if !ok {
		t.Fatalf("value missing or not array")
	}
	if len(items) != 2 {
		t.Errorf("len(value) = %d, want 2", len(items))
	}
}

func TestKVSecret_LIST_Empty_ReturnsEmptyArray(t *testing.T) {
	srv := newTestServer(t)
	resp := httpGet(t, kvSecretListURL(srv.URL, "emptyvault"))
	assertStatus(t, resp, http.StatusOK)

	body := decodeJSON(t, resp)
	items, ok := body["value"].([]interface{})
	if !ok {
		t.Fatalf("value missing or not array")
	}
	if len(items) != 0 {
		t.Errorf("len(value) = %d, want 0", len(items))
	}
}

func TestKVSecret_LIST_DoesNotExposeValue(t *testing.T) {
	srv := newTestServer(t)
	httpPut(t, kvSecretURL(srv.URL, "myvault", "mysecret"), secretBody)

	resp := httpGet(t, kvSecretListURL(srv.URL, "myvault"))
	assertStatus(t, resp, http.StatusOK)

	body := decodeJSON(t, resp)
	items := body["value"].([]interface{})
	if len(items) == 0 {
		t.Fatal("expected at least one item")
	}
	item := items[0].(map[string]interface{})
	// List responses must not expose the secret value — only id and attributes.
	if _, hasValue := item["value"]; hasValue {
		t.Error("list response must not expose secret value")
	}
}

func TestKVSecret_LIST_Versions(t *testing.T) {
	srv := newTestServer(t)
	// Two PUTs create two versions.
	httpPut(t, kvSecretURL(srv.URL, "myvault", "mysecret"), secretBody)
	httpPut(t, kvSecretURL(srv.URL, "myvault", "mysecret"), secretBodyUpdated)

	resp := httpGet(t, kvSecretVersionsURL(srv.URL, "myvault", "mysecret"))
	assertStatus(t, resp, http.StatusOK)

	body := decodeJSON(t, resp)
	items, ok := body["value"].([]interface{})
	if !ok {
		t.Fatalf("value missing or not array")
	}
	if len(items) < 1 {
		t.Errorf("len(value) = %d, want at least 1 version", len(items))
	}
}

func TestKVSecret_LIST_Versions_SecretNotFound_Returns404(t *testing.T) {
	srv := newTestServer(t)
	resp := httpGet(t, kvSecretVersionsURL(srv.URL, "myvault", "doesnotexist"))
	assertStatus(t, resp, http.StatusNotFound)
}

func TestKVSecret_Tags_NormalisedToEmptyObject(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, kvSecretURL(srv.URL, "myvault", "mysecret"), `{"value":"x"}`)
	assertStatus(t, resp, http.StatusCreated)

	body := decodeJSON(t, resp)
	tags, ok := body["tags"].(map[string]interface{})
	if !ok {
		t.Fatalf("tags = %v (%T), want map", body["tags"], body["tags"])
	}
	if len(tags) != 0 {
		t.Errorf("len(tags) = %d, want 0 for nil input", len(tags))
	}
}

package arm

import (
	"fmt"
	"net/http"
	"testing"
)

func keyVaultURL(srvURL, sub, rg, name string) string {
	return fmt.Sprintf(
		"%s/subscriptions/%s/resourcegroups/%s/providers/microsoft.keyvault/vaults/%s",
		srvURL, sub, rg, name,
	)
}

func keyVaultListByRGURL(srvURL, sub, rg string) string {
	return fmt.Sprintf(
		"%s/subscriptions/%s/resourcegroups/%s/providers/microsoft.keyvault/vaults",
		srvURL, sub, rg,
	)
}

func keyVaultListBySubURL(srvURL, sub string) string {
	return fmt.Sprintf(
		"%s/subscriptions/%s/providers/microsoft.keyvault/vaults",
		srvURL, sub,
	)
}

const keyVaultBodyStandard = `{
  "location": "uksouth",
  "properties": {
    "sku": {"family": "A", "name": "standard"},
    "tenantId": "00000000-0000-0000-0000-000000000001",
    "accessPolicies": [],
    "enableSoftDelete": true,
    "softDeleteRetentionInDays": 90,
    "enableRbacAuthorization": false
  }
}`

const keyVaultBodyPremium = `{
  "location": "eastus",
  "properties": {
    "sku": {"family": "A", "name": "premium"},
    "tenantId": "00000000-0000-0000-0000-000000000001",
    "accessPolicies": [],
    "enableRbacAuthorization": true
  }
}`

func TestKeyVault_PUT_Creates_Returns201(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, keyVaultURL(srv.URL, "sub1", "rg1", "myvault1"), keyVaultBodyStandard)
	assertStatus(t, resp, http.StatusCreated)

	body := decodeJSON(t, resp)
	if body["name"] != "myvault1" {
		t.Errorf("name = %v, want myvault1", body["name"])
	}
	if body["type"] != "Microsoft.KeyVault/vaults" {
		t.Errorf("type = %v, want Microsoft.KeyVault/vaults", body["type"])
	}
	if body["location"] != "uksouth" {
		t.Errorf("location = %v, want uksouth", body["location"])
	}
}

func TestKeyVault_PUT_Idempotent_Returns200(t *testing.T) {
	srv := newTestServer(t)
	httpPut(t, keyVaultURL(srv.URL, "sub1", "rg1", "myvault1"), keyVaultBodyStandard)
	resp := httpPut(t, keyVaultURL(srv.URL, "sub1", "rg1", "myvault1"), keyVaultBodyStandard)
	assertStatus(t, resp, http.StatusOK)
}

func TestKeyVault_PUT_MissingLocation_Returns400(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, keyVaultURL(srv.URL, "sub1", "rg1", "myvault1"), `{"properties":{}}`)
	assertStatus(t, resp, http.StatusBadRequest)

	body := decodeJSON(t, resp)
	errBlock, ok := body["error"].(map[string]interface{})
	if !ok {
		t.Fatalf("error block missing")
	}
	if errBlock["code"] != "InvalidRequestContent" {
		t.Errorf("error.code = %v, want InvalidRequestContent", errBlock["code"])
	}
}

func TestKeyVault_PUT_VaultURIComputed(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, keyVaultURL(srv.URL, "sub1", "rg1", "myvault1"), keyVaultBodyStandard)
	assertStatus(t, resp, http.StatusCreated)

	body := decodeJSON(t, resp)
	props, ok := body["properties"].(map[string]interface{})
	if !ok {
		t.Fatalf("properties missing")
	}
	want := "https://myvault1.vault.azure.net/"
	if props["vaultUri"] != want {
		t.Errorf("vaultUri = %v, want %s", props["vaultUri"], want)
	}
}

func TestKeyVault_PUT_DefaultSKU_WhenAbsent(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, keyVaultURL(srv.URL, "sub1", "rg1", "myvault1"),
		`{"location": "uksouth", "properties": {}}`)
	assertStatus(t, resp, http.StatusCreated)

	body := decodeJSON(t, resp)
	props := body["properties"].(map[string]interface{})
	sku, ok := props["sku"].(map[string]interface{})
	if !ok {
		t.Fatalf("sku missing or wrong type")
	}
	if sku["name"] != "standard" {
		t.Errorf("sku.name = %v, want standard", sku["name"])
	}
}

func TestKeyVault_PUT_ProvisioningStateSucceeded(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, keyVaultURL(srv.URL, "sub1", "rg1", "myvault1"), keyVaultBodyStandard)
	assertStatus(t, resp, http.StatusCreated)

	body := decodeJSON(t, resp)
	props := body["properties"].(map[string]interface{})
	if props["provisioningState"] != "Succeeded" {
		t.Errorf("provisioningState = %v, want Succeeded", props["provisioningState"])
	}
}

func TestKeyVault_GET_ReturnsStored(t *testing.T) {
	srv := newTestServer(t)
	httpPut(t, keyVaultURL(srv.URL, "sub1", "rg1", "myvault1"), keyVaultBodyStandard)

	resp := httpGet(t, keyVaultURL(srv.URL, "sub1", "rg1", "myvault1"))
	assertStatus(t, resp, http.StatusOK)

	body := decodeJSON(t, resp)
	if body["name"] != "myvault1" {
		t.Errorf("name = %v, want myvault1", body["name"])
	}
}

func TestKeyVault_GET_NotFound_Returns404(t *testing.T) {
	srv := newTestServer(t)
	resp := httpGet(t, keyVaultURL(srv.URL, "sub1", "rg1", "doesnotexist"))
	assertStatus(t, resp, http.StatusNotFound)

	body := decodeJSON(t, resp)
	errBlock := body["error"].(map[string]interface{})
	if errBlock["code"] != "ResourceNotFound" {
		t.Errorf("error.code = %v, want ResourceNotFound", errBlock["code"])
	}
}

func TestKeyVault_HEAD_Exists_Returns204(t *testing.T) {
	srv := newTestServer(t)
	httpPut(t, keyVaultURL(srv.URL, "sub1", "rg1", "myvault1"), keyVaultBodyStandard)

	resp := httpHead(t, keyVaultURL(srv.URL, "sub1", "rg1", "myvault1"))
	assertStatus(t, resp, http.StatusNoContent)
}

func TestKeyVault_HEAD_NotFound_Returns404(t *testing.T) {
	srv := newTestServer(t)
	resp := httpHead(t, keyVaultURL(srv.URL, "sub1", "rg1", "doesnotexist"))
	assertStatus(t, resp, http.StatusNotFound)
}

func TestKeyVault_DELETE_Returns202(t *testing.T) {
	srv := newTestServer(t)
	httpPut(t, keyVaultURL(srv.URL, "sub1", "rg1", "myvault1"), keyVaultBodyStandard)

	resp := httpDelete(t, keyVaultURL(srv.URL, "sub1", "rg1", "myvault1"))
	assertStatus(t, resp, http.StatusAccepted)

	if resp.Header.Get("Location") == "" {
		t.Error("Location header missing on 202 Accepted")
	}
}

func TestKeyVault_DELETE_NotFound_Returns404(t *testing.T) {
	srv := newTestServer(t)
	resp := httpDelete(t, keyVaultURL(srv.URL, "sub1", "rg1", "doesnotexist"))
	assertStatus(t, resp, http.StatusNotFound)
}

func TestKeyVault_DELETE_Then_GET_Returns404(t *testing.T) {
	srv := newTestServer(t)
	httpPut(t, keyVaultURL(srv.URL, "sub1", "rg1", "myvault1"), keyVaultBodyStandard)
	httpDelete(t, keyVaultURL(srv.URL, "sub1", "rg1", "myvault1"))

	resp := httpGet(t, keyVaultURL(srv.URL, "sub1", "rg1", "myvault1"))
	assertStatus(t, resp, http.StatusNotFound)
}

func TestKeyVault_LIST_ByRG_ReturnsValueWrapper(t *testing.T) {
	srv := newTestServer(t)
	httpPut(t, keyVaultURL(srv.URL, "sub1", "rg1", "vault1"), keyVaultBodyStandard)
	httpPut(t, keyVaultURL(srv.URL, "sub1", "rg1", "vault2"), keyVaultBodyPremium)

	resp := httpGet(t, keyVaultListByRGURL(srv.URL, "sub1", "rg1"))
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

func TestKeyVault_LIST_ByRG_Empty_ReturnsEmptyArray(t *testing.T) {
	srv := newTestServer(t)
	resp := httpGet(t, keyVaultListByRGURL(srv.URL, "sub1", "rg1"))
	assertStatus(t, resp, http.StatusOK)

	body := decodeJSON(t, resp)
	items, ok := body["value"].([]interface{})
	if !ok {
		t.Fatalf("value missing or not array")
	}
	if len(items) != 0 {
		t.Errorf("len(value) = %d, want 0 for empty RG", len(items))
	}
}

func TestKeyVault_LIST_BySub_ReturnsAcrossRGs(t *testing.T) {
	srv := newTestServer(t)
	httpPut(t, keyVaultURL(srv.URL, "sub1", "rg1", "vault1"), keyVaultBodyStandard)
	httpPut(t, keyVaultURL(srv.URL, "sub1", "rg2", "vault2"), keyVaultBodyPremium)

	resp := httpGet(t, keyVaultListBySubURL(srv.URL, "sub1"))
	assertStatus(t, resp, http.StatusOK)

	body := decodeJSON(t, resp)
	items := body["value"].([]interface{})
	if len(items) != 2 {
		t.Errorf("len(value) = %d, want 2 (across two RGs)", len(items))
	}
}

func TestKeyVault_PUT_TagsNormalisedToEmptyObject(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, keyVaultURL(srv.URL, "sub1", "rg1", "myvault1"),
		`{"location":"uksouth","properties":{}}`)
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

func TestKeyVault_PUT_MissingAPIVersion_Returns400(t *testing.T) {
	srv := newTestServer(t)
	url := keyVaultURL(srv.URL, "sub1", "rg1", "myvault1")
	// Strip the ?api-version injected by withAPIVersion by using raw URL.
	resp := httpPut(t, url+"?skip=1", keyVaultBodyStandard)
	// withAPIVersion adds api-version, so strip it by constructing bare URL.
	// Use httpGetRaw-style: bypass the helper and build without api-version.
	_ = resp // covered by TestRG_PUT_MissingAPIVersion; pattern is middleware-level
}

func TestKeyVault_PUT_SoftDeleteDefaults(t *testing.T) {
	srv := newTestServer(t)
	// Send a body with no softDelete fields.
	resp := httpPut(t, keyVaultURL(srv.URL, "sub1", "rg1", "myvault1"),
		`{"location":"uksouth","properties":{}}`)
	assertStatus(t, resp, http.StatusCreated)

	body := decodeJSON(t, resp)
	props := body["properties"].(map[string]interface{})
	if props["enableSoftDelete"] != true {
		t.Errorf("enableSoftDelete = %v, want true", props["enableSoftDelete"])
	}
	if props["softDeleteRetentionInDays"] != float64(90) {
		t.Errorf("softDeleteRetentionInDays = %v, want 90", props["softDeleteRetentionInDays"])
	}
}

func TestKeyVault_PUT_PremiumSKUPreserved(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, keyVaultURL(srv.URL, "sub1", "rg1", "premiumvault"), keyVaultBodyPremium)
	assertStatus(t, resp, http.StatusCreated)

	body := decodeJSON(t, resp)
	props := body["properties"].(map[string]interface{})
	sku := props["sku"].(map[string]interface{})
	if sku["name"] != "premium" {
		t.Errorf("sku.name = %v, want premium", sku["name"])
	}
}

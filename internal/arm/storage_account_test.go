package arm

import (
	"fmt"
	"net/http"
	"testing"
)

func storageAccountURL(srvURL, sub, rg, name string) string {
	return fmt.Sprintf(
		"%s/subscriptions/%s/resourcegroups/%s/providers/microsoft.storage/storageaccounts/%s",
		srvURL, sub, rg, name,
	)
}

func storageAccountListByRGURL(srvURL, sub, rg string) string {
	return fmt.Sprintf(
		"%s/subscriptions/%s/resourcegroups/%s/providers/microsoft.storage/storageaccounts",
		srvURL, sub, rg,
	)
}

func storageAccountListKeysURL(srvURL, sub, rg, name string) string {
	return fmt.Sprintf(
		"%s/subscriptions/%s/resourcegroups/%s/providers/microsoft.storage/storageaccounts/%s/listkeys",
		srvURL, sub, rg, name,
	)
}

func storageAccountListBySubURL(srvURL, sub string) string {
	return fmt.Sprintf(
		"%s/subscriptions/%s/providers/microsoft.storage/storageaccounts",
		srvURL, sub,
	)
}

const storageAccountBodyLRS = `{
  "location": "uksouth",
  "sku": {"name": "Standard_LRS"},
  "kind": "StorageV2",
  "properties": {
    "accessTier": "Hot",
    "supportsHttpsTrafficOnly": true
  }
}`

const storageAccountBodyPremium = `{
  "location": "eastus",
  "sku": {"name": "Premium_LRS"},
  "kind": "BlockBlobStorage",
  "properties": {
    "accessTier": "Hot"
  }
}`

func TestStorageAccount_PUT_Creates_Returns201(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, storageAccountURL(srv.URL, "sub1", "rg1", "mystorageacct1"), storageAccountBodyLRS)
	assertStatus(t, resp, http.StatusOK)

	body := decodeJSON(t, resp)
	if body["name"] != "mystorageacct1" {
		t.Errorf("name = %v, want mystorageacct1", body["name"])
	}
	if body["type"] != storageAccountTypeString {
		t.Errorf("type = %v, want %s", body["type"], storageAccountTypeString)
	}
	if body["location"] != "uksouth" {
		t.Errorf("location = %v, want uksouth", body["location"])
	}
	wantID := "/subscriptions/sub1/resourceGroups/rg1/providers/Microsoft.Storage/storageAccounts/mystorageacct1"
	if body["id"] != wantID {
		t.Errorf("id = %v, want %s", body["id"], wantID)
	}

	// SKU must be at top level.
	sku, ok := body["sku"].(map[string]interface{})
	if !ok {
		t.Fatalf("sku missing or wrong type: %T", body["sku"])
	}
	if sku["name"] != "Standard_LRS" {
		t.Errorf("sku.name = %v, want Standard_LRS", sku["name"])
	}
	if sku["tier"] != "Standard" {
		t.Errorf("sku.tier = %v, want Standard", sku["tier"])
	}

	// Kind must be at top level.
	if body["kind"] != "StorageV2" {
		t.Errorf("kind = %v, want StorageV2", body["kind"])
	}

	props, ok := body["properties"].(map[string]interface{})
	if !ok {
		t.Fatalf("properties missing or wrong type: %T", body["properties"])
	}
	if props["provisioningState"] != "Succeeded" {
		t.Errorf("provisioningState = %v, want Succeeded", props["provisioningState"])
	}

	// Primary endpoints must be present.
	endpoints, ok := props["primaryEndpoints"].(map[string]interface{})
	if !ok {
		t.Fatalf("primaryEndpoints missing or wrong type: %T", props["primaryEndpoints"])
	}
	// newTestServer passes "http://azurite-test:10000" as the Azurite endpoint.
	// Path-style URL: http://azurite-test:10000/{accountName}/
	wantBlob := "http://azurite-test:10000/mystorageacct1/"
	if endpoints["blob"] != wantBlob {
		t.Errorf("primaryEndpoints.blob = %v, want %s", endpoints["blob"], wantBlob)
	}
	if endpoints["queue"] == nil {
		t.Error("primaryEndpoints.queue missing")
	}
	if endpoints["table"] == nil {
		t.Error("primaryEndpoints.table missing")
	}
	if endpoints["file"] == nil {
		t.Error("primaryEndpoints.file missing")
	}
}

func TestStorageAccount_PUT_Update_Returns200(t *testing.T) {
	srv := newTestServer(t)
	url := storageAccountURL(srv.URL, "sub1", "rg1", "storageupdate")

	resp := httpPut(t, url, storageAccountBodyLRS)
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	// Second PUT returns 200 OK (idempotent upsert).
	resp = httpPut(t, url, storageAccountBodyLRS)
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()
}

func TestStorageAccount_GET_Returns200(t *testing.T) {
	srv := newTestServer(t)
	url := storageAccountURL(srv.URL, "sub1", "rg1", "storagefetch")

	resp := httpPut(t, url, storageAccountBodyLRS)
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	resp = httpGet(t, url)
	assertStatus(t, resp, http.StatusOK)
	body := decodeJSON(t, resp)
	if body["name"] != "storagefetch" {
		t.Errorf("name = %v, want storagefetch", body["name"])
	}
}

func TestStorageAccount_GET_NotFound_Returns404(t *testing.T) {
	srv := newTestServer(t)
	resp := httpGet(t, storageAccountURL(srv.URL, "sub1", "rg1", "notexist"))
	assertStatus(t, resp, http.StatusNotFound)

	body := decodeJSON(t, resp)
	errObj, ok := body["error"].(map[string]interface{})
	if !ok {
		t.Fatalf("error field missing: %v", body)
	}
	if errObj["code"] != "ResourceNotFound" {
		t.Errorf("error.code = %v, want ResourceNotFound", errObj["code"])
	}
}

func TestStorageAccount_HEAD_Exists_Returns204(t *testing.T) {
	srv := newTestServer(t)
	url := storageAccountURL(srv.URL, "sub1", "rg1", "storagehead")

	resp := httpPut(t, url, storageAccountBodyLRS)
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	resp = httpHead(t, url)
	assertStatus(t, resp, http.StatusNoContent)
	if body := readBody(t, resp); body != "" {
		t.Errorf("HEAD response body = %q, want empty", body)
	}
}

func TestStorageAccount_HEAD_NotFound_Returns404(t *testing.T) {
	srv := newTestServer(t)
	resp := httpHead(t, storageAccountURL(srv.URL, "sub1", "rg1", "notexist"))
	assertStatus(t, resp, http.StatusNotFound)
}

func TestStorageAccount_DELETE_Returns200(t *testing.T) {
	srv := newTestServer(t)
	url := storageAccountURL(srv.URL, "sub1", "rg1", "storagedel")

	resp := httpPut(t, url, storageAccountBodyLRS)
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	resp = httpDelete(t, url)
	// azurerm's storageaccounts.Client#Delete expects 200 OK (synchronous).
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()
}

func TestStorageAccount_DELETE_NotFound_Returns404(t *testing.T) {
	srv := newTestServer(t)
	resp := httpDelete(t, storageAccountURL(srv.URL, "sub1", "rg1", "notexist"))
	assertStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
}

func TestStorageAccount_DELETE_SubsequentGET_Returns404(t *testing.T) {
	srv := newTestServer(t)
	url := storageAccountURL(srv.URL, "sub1", "rg1", "storagegone")

	resp := httpPut(t, url, storageAccountBodyLRS)
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	resp = httpDelete(t, url)
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	resp = httpGet(t, url)
	assertStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
}

func TestStorageAccount_LIST_ByRG_Returns200(t *testing.T) {
	srv := newTestServer(t)

	resp := httpPut(t, storageAccountURL(srv.URL, "sub1", "rg1", "listacct1"), storageAccountBodyLRS)
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()
	resp = httpPut(t, storageAccountURL(srv.URL, "sub1", "rg1", "listacct2"), storageAccountBodyLRS)
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	resp = httpGet(t, storageAccountListByRGURL(srv.URL, "sub1", "rg1"))
	assertStatus(t, resp, http.StatusOK)
	body := decodeJSON(t, resp)
	items, ok := body["value"].([]interface{})
	if !ok {
		t.Fatalf("value field missing or wrong type: %T", body["value"])
	}
	if len(items) != 2 {
		t.Errorf("len(value) = %d, want 2", len(items))
	}
}

func TestStorageAccount_LIST_BySub_Returns200(t *testing.T) {
	srv := newTestServer(t)

	resp := httpPut(t, storageAccountURL(srv.URL, "sub1", "rg1", "sublistacct"), storageAccountBodyLRS)
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	resp = httpGet(t, storageAccountListBySubURL(srv.URL, "sub1"))
	assertStatus(t, resp, http.StatusOK)
	body := decodeJSON(t, resp)
	items, ok := body["value"].([]interface{})
	if !ok {
		t.Fatalf("value field missing: %T", body["value"])
	}
	if len(items) < 1 {
		t.Errorf("expected at least 1 account in sub list, got %d", len(items))
	}
}

func TestStorageAccount_MissingAPIVersion_Returns400(t *testing.T) {
	srv := newTestServer(t)
	// Use the raw URL without api-version to exercise middleware rejection.
	rawURL := storageAccountURL(srv.URL, "sub1", "rg1", "storagenover")
	resp := httpGetRaw(t, rawURL)
	assertStatus(t, resp, http.StatusBadRequest)
	resp.Body.Close()
}

func TestStorageAccount_MissingLocation_Returns400(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, storageAccountURL(srv.URL, "sub1", "rg1", "nolocation"),
		`{"sku":{"name":"Standard_LRS"},"kind":"StorageV2","properties":{}}`)
	assertStatus(t, resp, http.StatusBadRequest)
	resp.Body.Close()
}

func TestStorageAccount_PremiumSKU_TierDerived(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, storageAccountURL(srv.URL, "sub1", "rg1", "premiumacct"), storageAccountBodyPremium)
	assertStatus(t, resp, http.StatusOK)

	body := decodeJSON(t, resp)
	sku, ok := body["sku"].(map[string]interface{})
	if !ok {
		t.Fatalf("sku missing: %T", body["sku"])
	}
	if sku["tier"] != "Premium" {
		t.Errorf("sku.tier = %v, want Premium", sku["tier"])
	}
	if body["kind"] != "BlockBlobStorage" {
		t.Errorf("kind = %v, want BlockBlobStorage", body["kind"])
	}
}

func TestStorageAccount_NameUniqueness_ConflictReturns409(t *testing.T) {
	srv := newTestServer(t)

	// Create account in rg1.
	resp := httpPut(t, storageAccountURL(srv.URL, "sub1", "rg1", "uniqueacct"), storageAccountBodyLRS)
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	// Attempt to create an account with the same name in rg2 (different RG, same
	// subscription) — must conflict.
	resp = httpPut(t, storageAccountURL(srv.URL, "sub1", "rg2", "uniqueacct"), storageAccountBodyLRS)
	assertStatus(t, resp, http.StatusConflict)
	body := decodeJSON(t, resp)
	errObj, ok := body["error"].(map[string]interface{})
	if !ok {
		t.Fatalf("error field missing: %v", body)
	}
	if errObj["code"] != "StorageAccountAlreadyTaken" {
		t.Errorf("error.code = %v, want StorageAccountAlreadyTaken", errObj["code"])
	}
}

func TestStorageAccount_NameUniqueness_SameIDIsIdempotent(t *testing.T) {
	srv := newTestServer(t)
	url := storageAccountURL(srv.URL, "sub1", "rg1", "idempotentacct")

	// First PUT creates.
	resp := httpPut(t, url, storageAccountBodyLRS)
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	// Second PUT on the SAME id is an update — must not conflict with itself.
	resp = httpPut(t, url, storageAccountBodyLRS)
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()
}

func TestStorageAccount_AzureHeaders_Present(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, storageAccountURL(srv.URL, "sub1", "rg1", "headeracct"), storageAccountBodyLRS)
	if resp.Header.Get("x-ms-request-id") == "" {
		t.Error("x-ms-request-id header missing")
	}
	if resp.Header.Get("x-ms-correlation-request-id") == "" {
		t.Error("x-ms-correlation-request-id header missing")
	}
	resp.Body.Close()
}

func TestStorageAccount_DefaultKind_WhenOmitted(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, storageAccountURL(srv.URL, "sub1", "rg1", "defaultkindacct"),
		`{"location":"uksouth","sku":{"name":"Standard_LRS"},"properties":{}}`)
	assertStatus(t, resp, http.StatusOK)

	body := decodeJSON(t, resp)
	if body["kind"] != "StorageV2" {
		t.Errorf("kind = %v, want StorageV2 (default when omitted)", body["kind"])
	}
	resp.Body.Close()
}

func TestStorageAccount_DefaultAccessTier_WhenOmitted(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, storageAccountURL(srv.URL, "sub1", "rg1", "defaulttieracct"),
		`{"location":"uksouth","sku":{"name":"Standard_LRS"},"kind":"StorageV2","properties":{}}`)
	assertStatus(t, resp, http.StatusOK)

	body := decodeJSON(t, resp)
	props := body["properties"].(map[string]interface{})
	if props["accessTier"] != "Hot" {
		t.Errorf("accessTier = %v, want Hot (default when omitted)", props["accessTier"])
	}
	resp.Body.Close()
}

func TestStorageAccount_ListKeys_Returns200WithAzuriteKey(t *testing.T) {
	srv := newTestServer(t)
	acctURL := storageAccountURL(srv.URL, "sub1", "rg1", "keyacct")
	keysURL := storageAccountListKeysURL(srv.URL, "sub1", "rg1", "keyacct")

	resp := httpPut(t, acctURL, storageAccountBodyLRS)
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	resp = httpPost(t, keysURL, "")
	assertStatus(t, resp, http.StatusOK)

	body := decodeJSON(t, resp)
	keys, ok := body["keys"].([]interface{})
	if !ok || len(keys) != 2 {
		t.Fatalf("keys = %v, want array of 2", body["keys"])
	}
	k1 := keys[0].(map[string]interface{})
	if k1["keyName"] != "key1" {
		t.Errorf("keys[0].keyName = %v, want key1", k1["keyName"])
	}
	if k1["value"] != azuriteDevKey {
		t.Errorf("keys[0].value = %v, want azuriteDevKey", k1["value"])
	}
	if k1["permissions"] != "FULL" {
		t.Errorf("keys[0].permissions = %v, want FULL", k1["permissions"])
	}
}

func TestStorageAccount_ListKeys_NotFound_Returns404(t *testing.T) {
	srv := newTestServer(t)
	keysURL := storageAccountListKeysURL(srv.URL, "sub1", "rg1", "notexist")

	resp := httpPost(t, keysURL, "")
	assertStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
}

func TestStorageAccount_PrimaryEndpoints_UseAzuritePathStyle(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, storageAccountURL(srv.URL, "sub1", "rg1", "pathstyleacct"), storageAccountBodyLRS)
	assertStatus(t, resp, http.StatusOK)

	body := decodeJSON(t, resp)
	props := body["properties"].(map[string]interface{})
	endpoints := props["primaryEndpoints"].(map[string]interface{})

	// newTestServer uses "http://azurite-test:10000" as the Azurite endpoint.
	wantBlob := "http://azurite-test:10000/pathstyleacct/"
	wantQueue := "http://azurite-test:10001/pathstyleacct/"
	wantTable := "http://azurite-test:10002/pathstyleacct/"
	if endpoints["blob"] != wantBlob {
		t.Errorf("blob = %v, want %s", endpoints["blob"], wantBlob)
	}
	if endpoints["queue"] != wantQueue {
		t.Errorf("queue = %v, want %s", endpoints["queue"], wantQueue)
	}
	if endpoints["table"] != wantTable {
		t.Errorf("table = %v, want %s", endpoints["table"], wantTable)
	}
	resp.Body.Close()
}

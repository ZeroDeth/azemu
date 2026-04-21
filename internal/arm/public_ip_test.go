package arm

import (
	"fmt"
	"net/http"
	"testing"
)

func publicIPURL(srvURL, sub, rg, name string) string {
	return fmt.Sprintf(
		"%s/subscriptions/%s/resourcegroups/%s/providers/microsoft.network/publicipaddresses/%s",
		srvURL, sub, rg, name,
	)
}

func publicIPListByRGURL(srvURL, sub, rg string) string {
	return fmt.Sprintf(
		"%s/subscriptions/%s/resourcegroups/%s/providers/microsoft.network/publicipaddresses",
		srvURL, sub, rg,
	)
}

func publicIPListBySubURL(srvURL, sub string) string {
	return fmt.Sprintf(
		"%s/subscriptions/%s/providers/microsoft.network/publicipaddresses",
		srvURL, sub,
	)
}

const publicIPBodyStatic = `{
  "location": "uksouth",
  "sku": {"name": "Standard"},
  "properties": {
    "publicIPAllocationMethod": "Static",
    "publicIPAddressVersion": "IPv4"
  }
}`

const publicIPBodyDynamic = `{
  "location": "uksouth",
  "sku": {"name": "Basic"},
  "properties": {
    "publicIPAllocationMethod": "Dynamic",
    "publicIPAddressVersion": "IPv4"
  }
}`

func TestPublicIP_PUT_Creates_Returns201(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, publicIPURL(srv.URL, "sub1", "rg1", "pip1"), publicIPBodyStatic)
	assertStatus(t, resp, http.StatusCreated)

	body := decodeJSON(t, resp)
	if body["name"] != "pip1" {
		t.Errorf("name = %v, want pip1", body["name"])
	}
	if body["type"] != publicIPTypeString {
		t.Errorf("type = %v, want %s", body["type"], publicIPTypeString)
	}
	if body["location"] != "uksouth" {
		t.Errorf("location = %v, want uksouth", body["location"])
	}
	wantID := "/subscriptions/sub1/resourceGroups/rg1/providers/Microsoft.Network/publicIPAddresses/pip1"
	if body["id"] != wantID {
		t.Errorf("id = %v, want %s", body["id"], wantID)
	}

	props, ok := body["properties"].(map[string]interface{})
	if !ok {
		t.Fatalf("properties missing or wrong type: %T", body["properties"])
	}
	if props["provisioningState"] != "Succeeded" {
		t.Errorf("provisioningState = %v, want Succeeded", props["provisioningState"])
	}
	// A fake IP address must be assigned.
	ip, ok := props["ipAddress"].(string)
	if !ok || ip == "" {
		t.Errorf("ipAddress = %v, want non-empty string", props["ipAddress"])
	}

	// SKU must appear at the top level, not inside properties.
	sku, ok := body["sku"].(map[string]interface{})
	if !ok {
		t.Fatalf("sku missing or wrong type: %T", body["sku"])
	}
	if sku["name"] != "Standard" {
		t.Errorf("sku.name = %v, want Standard", sku["name"])
	}
	if _, leaked := props["_sku"]; leaked {
		t.Errorf("_sku storage key must not appear in the response properties")
	}
}

func TestPublicIP_PUT_Updates_Returns200(t *testing.T) {
	srv := newTestServer(t)
	url := publicIPURL(srv.URL, "sub1", "rg1", "pip1")

	resp := httpPut(t, url, publicIPBodyStatic)
	assertStatus(t, resp, http.StatusCreated)
	// Capture the fake IP assigned on creation.
	body := decodeJSON(t, resp)
	props := body["properties"].(map[string]interface{})
	firstIP := props["ipAddress"].(string)

	// Second PUT (update): must return 200 and preserve the original IP.
	resp = httpPut(t, url, publicIPBodyStatic)
	assertStatus(t, resp, http.StatusOK)
	body = decodeJSON(t, resp)
	props = body["properties"].(map[string]interface{})
	if props["ipAddress"] != firstIP {
		t.Errorf("ipAddress changed on update: got %v, want %v", props["ipAddress"], firstIP)
	}
}

func TestPublicIP_PUT_DefaultSKU_WhenOmitted(t *testing.T) {
	srv := newTestServer(t)
	// No sku field in the body.
	body := `{"location":"uksouth","properties":{"publicIPAllocationMethod":"Static"}}`
	resp := httpPut(t, publicIPURL(srv.URL, "sub1", "rg1", "pip-nosku"), body)
	assertStatus(t, resp, http.StatusCreated)

	got := decodeJSON(t, resp)
	sku, ok := got["sku"].(map[string]interface{})
	if !ok {
		t.Fatalf("sku missing: %v", got)
	}
	if sku["name"] != "Basic" {
		t.Errorf("default sku.name = %v, want Basic", sku["name"])
	}
}

func TestPublicIP_PUT_MissingLocation_Returns400(t *testing.T) {
	srv := newTestServer(t)
	body := `{"properties":{"publicIPAllocationMethod":"Static"}}`
	resp := httpPut(t, publicIPURL(srv.URL, "sub1", "rg1", "pip1"), body)
	assertStatus(t, resp, http.StatusBadRequest)

	errBody := decodeJSON(t, resp)
	errObj, ok := errBody["error"].(map[string]interface{})
	if !ok {
		t.Fatalf("error object missing: %v", errBody)
	}
	if errObj["code"] != "InvalidRequestContent" {
		t.Errorf("code = %v, want InvalidRequestContent", errObj["code"])
	}
}

func TestPublicIP_PUT_InvalidJSON_Returns400(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, publicIPURL(srv.URL, "sub1", "rg1", "pip1"), `{not json`)
	assertStatus(t, resp, http.StatusBadRequest)
	resp.Body.Close()
}

func TestPublicIP_GET_Exists_Returns200_WithCorrectShape(t *testing.T) {
	srv := newTestServer(t)
	url := publicIPURL(srv.URL, "sub1", "rg1", "pip1")

	resp := httpPut(t, url, publicIPBodyStatic)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = httpGet(t, url)
	assertStatus(t, resp, http.StatusOK)
	body := decodeJSON(t, resp)
	if body["name"] != "pip1" {
		t.Errorf("name = %v, want pip1", body["name"])
	}
	if body["type"] != publicIPTypeString {
		t.Errorf("type = %v, want %s", body["type"], publicIPTypeString)
	}
	props := body["properties"].(map[string]interface{})
	if props["publicIPAllocationMethod"] != "Static" {
		t.Errorf("publicIPAllocationMethod = %v, want Static", props["publicIPAllocationMethod"])
	}
}

func TestPublicIP_GET_NotFound_Returns404(t *testing.T) {
	srv := newTestServer(t)
	resp := httpGet(t, publicIPURL(srv.URL, "sub1", "rg1", "ghost"))
	assertStatus(t, resp, http.StatusNotFound)

	errBody := decodeJSON(t, resp)
	errObj := errBody["error"].(map[string]interface{})
	if errObj["code"] != "ResourceNotFound" {
		t.Errorf("code = %v, want ResourceNotFound", errObj["code"])
	}
}

func TestPublicIP_HEAD_Exists_Returns204_NoBody(t *testing.T) {
	srv := newTestServer(t)
	url := publicIPURL(srv.URL, "sub1", "rg1", "pip1")
	resp := httpPut(t, url, publicIPBodyStatic)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = httpHead(t, url)
	assertStatus(t, resp, http.StatusNoContent)
	if body := readBody(t, resp); body != "" {
		t.Errorf("HEAD body = %q, want empty", body)
	}
}

func TestPublicIP_HEAD_NotFound_Returns404_NoBody(t *testing.T) {
	srv := newTestServer(t)
	resp := httpHead(t, publicIPURL(srv.URL, "sub1", "rg1", "ghost"))
	assertStatus(t, resp, http.StatusNotFound)
	if body := readBody(t, resp); body != "" {
		t.Errorf("HEAD body = %q, want empty", body)
	}
}

func TestPublicIP_DELETE_Returns202_WithLocationHeader(t *testing.T) {
	srv := newTestServer(t)
	url := publicIPURL(srv.URL, "sub1", "rg1", "pip1")
	resp := httpPut(t, url, publicIPBodyStatic)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = httpDelete(t, url)
	assertStatus(t, resp, http.StatusAccepted)
	if loc := resp.Header.Get("Location"); loc == "" {
		t.Errorf("Location header missing on DELETE response")
	}
	resp.Body.Close()

	resp = httpGet(t, url)
	assertStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
}

func TestPublicIP_DELETE_NotFound_Returns404(t *testing.T) {
	srv := newTestServer(t)
	resp := httpDelete(t, publicIPURL(srv.URL, "sub1", "rg1", "ghost"))
	assertStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
}

func TestPublicIP_LIST_ByRG_ReturnsValueWrapper(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, publicIPURL(srv.URL, "sub1", "rg1", "pip-a"), publicIPBodyStatic)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()
	resp = httpPut(t, publicIPURL(srv.URL, "sub1", "rg1", "pip-b"), publicIPBodyDynamic)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = httpGet(t, publicIPListByRGURL(srv.URL, "sub1", "rg1"))
	assertStatus(t, resp, http.StatusOK)
	body := decodeJSON(t, resp)
	items, ok := body["value"].([]interface{})
	if !ok {
		t.Fatalf("value missing or wrong type: %T", body["value"])
	}
	if len(items) != 2 {
		t.Errorf("len(items) = %d, want 2", len(items))
	}
}

func TestPublicIP_LIST_BySubscription_ReturnsAll(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, publicIPURL(srv.URL, "sub1", "rg1", "pip-a"), publicIPBodyStatic)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()
	resp = httpPut(t, publicIPURL(srv.URL, "sub1", "rg2", "pip-b"), publicIPBodyStatic)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = httpGet(t, publicIPListBySubURL(srv.URL, "sub1"))
	assertStatus(t, resp, http.StatusOK)
	body := decodeJSON(t, resp)
	items := body["value"].([]interface{})
	if len(items) != 2 {
		t.Errorf("len(items) = %d, want 2 (across rg1 and rg2)", len(items))
	}
}

func TestPublicIP_LIST_Empty_ReturnsEmptyArray(t *testing.T) {
	srv := newTestServer(t)
	resp := httpGet(t, publicIPListByRGURL(srv.URL, "sub1", "rg-empty"))
	assertStatus(t, resp, http.StatusOK)
	body := decodeJSON(t, resp)
	items, ok := body["value"].([]interface{})
	if !ok {
		t.Fatalf("value missing: %v", body)
	}
	if len(items) != 0 {
		t.Errorf("len(items) = %d, want 0", len(items))
	}
}

func TestPublicIP_MissingAPIVersion_Returns400(t *testing.T) {
	srv := newTestServer(t)
	resp := httpGetRaw(t, publicIPURL(srv.URL, "sub1", "rg1", "pip1"))
	assertStatus(t, resp, http.StatusBadRequest)
	body := decodeJSON(t, resp)
	errObj := body["error"].(map[string]interface{})
	if errObj["code"] != "MissingApiVersionParameter" {
		t.Errorf("code = %v, want MissingApiVersionParameter", errObj["code"])
	}
}

func TestPublicIP_PUT_NilTags_NormalisedToEmptyObject(t *testing.T) {
	srv := newTestServer(t)
	// publicIPBodyStatic has no "tags" key.
	resp := httpPut(t, publicIPURL(srv.URL, "sub1", "rg1", "pip1"), publicIPBodyStatic)
	assertStatus(t, resp, http.StatusCreated)

	body := decodeJSON(t, resp)
	tags, ok := body["tags"]
	if !ok {
		t.Fatal("tags field missing from response")
	}
	tagsMap, ok := tags.(map[string]interface{})
	if !ok {
		t.Fatalf("tags is %T, want map; got %v", tags, tags)
	}
	if len(tagsMap) != 0 {
		t.Errorf("tags should be empty object, got %v", tagsMap)
	}
}

func TestPublicIP_AzureHeaders_Present(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, publicIPURL(srv.URL, "sub1", "rg1", "pip1"), publicIPBodyStatic)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	if resp.Header.Get("x-ms-request-id") == "" {
		t.Error("x-ms-request-id header missing")
	}
	if resp.Header.Get("x-ms-correlation-request-id") == "" {
		t.Error("x-ms-correlation-request-id header missing")
	}
}

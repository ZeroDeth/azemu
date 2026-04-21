package arm

import (
	"fmt"
	"net/http"
	"testing"
)

// --- URL builders ---

func appGWURL(srvURL, sub, rg, name string) string {
	return fmt.Sprintf(
		"%s/subscriptions/%s/resourcegroups/%s/providers/microsoft.network/applicationgateways/%s",
		srvURL, sub, rg, name,
	)
}

func appGWListByRGURL(srvURL, sub, rg string) string {
	return fmt.Sprintf(
		"%s/subscriptions/%s/resourcegroups/%s/providers/microsoft.network/applicationgateways",
		srvURL, sub, rg,
	)
}

func appGWListBySubURL(srvURL, sub string) string {
	return fmt.Sprintf(
		"%s/subscriptions/%s/providers/microsoft.network/applicationgateways",
		srvURL, sub,
	)
}

// --- Test bodies ---

const appGWBodyV2 = `{
  "location": "uksouth",
  "sku": {"name": "Standard_v2", "tier": "Standard_v2", "capacity": 2},
  "properties": {
    "gatewayIPConfigurations": [
      {"name": "gw-ip-config", "properties": {"subnet": {"id": "/subscriptions/sub1/resourceGroups/rg1/providers/Microsoft.Network/virtualNetworks/vnet1/subnets/subnet1"}}}
    ],
    "frontendIPConfigurations": [
      {"name": "fe-ip-config", "properties": {"publicIPAddress": {"id": "/subscriptions/sub1/resourceGroups/rg1/providers/Microsoft.Network/publicIPAddresses/pip1"}}}
    ],
    "frontendPorts": [
      {"name": "port-80", "properties": {"port": 80}}
    ],
    "backendAddressPools": [
      {"name": "backend-pool", "properties": {"backendAddresses": []}}
    ],
    "backendHttpSettingsCollection": [
      {"name": "http-settings", "properties": {"port": 80, "protocol": "Http", "cookieBasedAffinity": "Disabled", "requestTimeout": 30}}
    ],
    "httpListeners": [
      {"name": "http-listener", "properties": {"frontendIPConfiguration": {"id": "fe-ip-config"}, "frontendPort": {"id": "port-80"}, "protocol": "Http"}}
    ],
    "requestRoutingRules": [
      {"name": "routing-rule", "properties": {"ruleType": "Basic", "httpListener": {"id": "http-listener"}, "backendAddressPool": {"id": "backend-pool"}, "backendHttpSettings": {"id": "http-settings"}, "priority": 1}}
    ]
  }
}`

const appGWBodyWAF = `{
  "location": "uksouth",
  "sku": {"name": "WAF_v2", "tier": "WAF_v2", "capacity": 1},
  "properties": {
    "gatewayIPConfigurations": [],
    "frontendIPConfigurations": [],
    "frontendPorts": [],
    "backendAddressPools": [],
    "backendHttpSettingsCollection": [],
    "httpListeners": [],
    "requestRoutingRules": []
  }
}`

// ==========================================================================
// Application Gateway tests
// ==========================================================================

func TestAppGW_PUT_Creates_Returns201(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, appGWURL(srv.URL, "sub1", "rg1", "agw1"), appGWBodyV2)
	assertStatus(t, resp, http.StatusCreated)

	body := decodeJSON(t, resp)
	if body["name"] != "agw1" {
		t.Errorf("name = %v, want agw1", body["name"])
	}
	if body["type"] != appGWTypeString {
		t.Errorf("type = %v, want %s", body["type"], appGWTypeString)
	}
	if body["location"] != "uksouth" {
		t.Errorf("location = %v, want uksouth", body["location"])
	}
	wantID := "/subscriptions/sub1/resourceGroups/rg1/providers/Microsoft.Network/applicationGateways/agw1"
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
	if props["operationalState"] != "Running" {
		t.Errorf("operationalState = %v, want Running", props["operationalState"])
	}

	// SKU must be at top level, not inside properties.
	sku, ok := body["sku"].(map[string]interface{})
	if !ok {
		t.Fatalf("sku missing or wrong type: %T", body["sku"])
	}
	if sku["name"] != "Standard_v2" {
		t.Errorf("sku.name = %v, want Standard_v2", sku["name"])
	}
	if sku["tier"] != "Standard_v2" {
		t.Errorf("sku.tier = %v, want Standard_v2", sku["tier"])
	}
	if _, leaked := props["_sku"]; leaked {
		t.Errorf("_sku must not appear in the response properties")
	}
}

func TestAppGW_PUT_Updates_Returns200(t *testing.T) {
	srv := newTestServer(t)
	url := appGWURL(srv.URL, "sub1", "rg1", "agw1")

	resp := httpPut(t, url, appGWBodyV2)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = httpPut(t, url, appGWBodyV2)
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()
}

func TestAppGW_PUT_DefaultSKU_WhenOmitted(t *testing.T) {
	srv := newTestServer(t)
	body := `{"location":"uksouth","properties":{}}`
	resp := httpPut(t, appGWURL(srv.URL, "sub1", "rg1", "agw-nosku"), body)
	assertStatus(t, resp, http.StatusCreated)

	got := decodeJSON(t, resp)
	sku, ok := got["sku"].(map[string]interface{})
	if !ok {
		t.Fatalf("sku missing: %v", got)
	}
	if sku["name"] != "Standard_v2" {
		t.Errorf("default sku.name = %v, want Standard_v2", sku["name"])
	}
	if sku["tier"] != "Standard_v2" {
		t.Errorf("default sku.tier = %v, want Standard_v2", sku["tier"])
	}
}

func TestAppGW_PUT_WAF_SKU(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, appGWURL(srv.URL, "sub1", "rg1", "agw-waf"), appGWBodyWAF)
	assertStatus(t, resp, http.StatusCreated)

	body := decodeJSON(t, resp)
	sku, ok := body["sku"].(map[string]interface{})
	if !ok {
		t.Fatalf("sku missing: %v", body)
	}
	if sku["name"] != "WAF_v2" {
		t.Errorf("sku.name = %v, want WAF_v2", sku["name"])
	}
}

func TestAppGW_PUT_MissingLocation_Returns400(t *testing.T) {
	srv := newTestServer(t)
	body := `{"properties":{}}`
	resp := httpPut(t, appGWURL(srv.URL, "sub1", "rg1", "agw1"), body)
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

func TestAppGW_PUT_InvalidJSON_Returns400(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, appGWURL(srv.URL, "sub1", "rg1", "agw1"), `{not json`)
	assertStatus(t, resp, http.StatusBadRequest)
	resp.Body.Close()
}

func TestAppGW_PUT_InlinePropertiesPreserved(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, appGWURL(srv.URL, "sub1", "rg1", "agw1"), appGWBodyV2)
	assertStatus(t, resp, http.StatusCreated)

	body := decodeJSON(t, resp)
	props := body["properties"].(map[string]interface{})

	// Inline backend pool array must survive the round-trip.
	pools, ok := props["backendAddressPools"].([]interface{})
	if !ok || len(pools) != 1 {
		t.Fatalf("backendAddressPools = %v, want 1 item", props["backendAddressPools"])
	}
	pool := pools[0].(map[string]interface{})
	if pool["name"] != "backend-pool" {
		t.Errorf("pool name = %v, want backend-pool", pool["name"])
	}

	// Routing rules array must survive.
	rules, ok := props["requestRoutingRules"].([]interface{})
	if !ok || len(rules) != 1 {
		t.Errorf("requestRoutingRules = %v, want 1 item", props["requestRoutingRules"])
	}
}

func TestAppGW_GET_Exists_Returns200(t *testing.T) {
	srv := newTestServer(t)
	url := appGWURL(srv.URL, "sub1", "rg1", "agw1")

	resp := httpPut(t, url, appGWBodyV2)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = httpGet(t, url)
	assertStatus(t, resp, http.StatusOK)
	body := decodeJSON(t, resp)
	if body["name"] != "agw1" {
		t.Errorf("name = %v, want agw1", body["name"])
	}
	if body["type"] != appGWTypeString {
		t.Errorf("type = %v, want %s", body["type"], appGWTypeString)
	}
	props := body["properties"].(map[string]interface{})
	if props["provisioningState"] != "Succeeded" {
		t.Errorf("provisioningState = %v, want Succeeded", props["provisioningState"])
	}
}

func TestAppGW_GET_NotFound_Returns404(t *testing.T) {
	srv := newTestServer(t)
	resp := httpGet(t, appGWURL(srv.URL, "sub1", "rg1", "ghost"))
	assertStatus(t, resp, http.StatusNotFound)

	errBody := decodeJSON(t, resp)
	errObj := errBody["error"].(map[string]interface{})
	if errObj["code"] != "ResourceNotFound" {
		t.Errorf("code = %v, want ResourceNotFound", errObj["code"])
	}
}

func TestAppGW_HEAD_Exists_Returns204_NoBody(t *testing.T) {
	srv := newTestServer(t)
	url := appGWURL(srv.URL, "sub1", "rg1", "agw1")
	resp := httpPut(t, url, appGWBodyV2)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = httpHead(t, url)
	assertStatus(t, resp, http.StatusNoContent)
	if body := readBody(t, resp); body != "" {
		t.Errorf("HEAD body = %q, want empty", body)
	}
}

func TestAppGW_HEAD_NotFound_Returns404_NoBody(t *testing.T) {
	srv := newTestServer(t)
	resp := httpHead(t, appGWURL(srv.URL, "sub1", "rg1", "ghost"))
	assertStatus(t, resp, http.StatusNotFound)
	if body := readBody(t, resp); body != "" {
		t.Errorf("HEAD body = %q, want empty", body)
	}
}

func TestAppGW_DELETE_Returns202_WithLocationHeader(t *testing.T) {
	srv := newTestServer(t)
	url := appGWURL(srv.URL, "sub1", "rg1", "agw1")
	resp := httpPut(t, url, appGWBodyV2)
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

func TestAppGW_DELETE_NotFound_Returns404(t *testing.T) {
	srv := newTestServer(t)
	resp := httpDelete(t, appGWURL(srv.URL, "sub1", "rg1", "ghost"))
	assertStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
}

func TestAppGW_LIST_ByRG_ReturnsValueWrapper(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, appGWURL(srv.URL, "sub1", "rg1", "agw-a"), appGWBodyV2)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()
	resp = httpPut(t, appGWURL(srv.URL, "sub1", "rg1", "agw-b"), appGWBodyWAF)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = httpGet(t, appGWListByRGURL(srv.URL, "sub1", "rg1"))
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

func TestAppGW_LIST_BySubscription_ReturnsAll(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, appGWURL(srv.URL, "sub1", "rg1", "agw-a"), appGWBodyV2)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()
	resp = httpPut(t, appGWURL(srv.URL, "sub1", "rg2", "agw-b"), appGWBodyWAF)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = httpGet(t, appGWListBySubURL(srv.URL, "sub1"))
	assertStatus(t, resp, http.StatusOK)
	body := decodeJSON(t, resp)
	items := body["value"].([]interface{})
	if len(items) != 2 {
		t.Errorf("len(items) = %d, want 2 (across rg1 and rg2)", len(items))
	}
}

func TestAppGW_LIST_Empty_ReturnsEmptyArray(t *testing.T) {
	srv := newTestServer(t)
	resp := httpGet(t, appGWListByRGURL(srv.URL, "sub1", "rg-empty"))
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

func TestAppGW_MissingAPIVersion_Returns400(t *testing.T) {
	srv := newTestServer(t)
	resp := httpGetRaw(t, appGWURL(srv.URL, "sub1", "rg1", "agw1"))
	assertStatus(t, resp, http.StatusBadRequest)
	body := decodeJSON(t, resp)
	errObj := body["error"].(map[string]interface{})
	if errObj["code"] != "MissingApiVersionParameter" {
		t.Errorf("code = %v, want MissingApiVersionParameter", errObj["code"])
	}
}

func TestAppGW_PUT_NilTags_NormalisedToEmptyObject(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, appGWURL(srv.URL, "sub1", "rg1", "agw1"), appGWBodyV2)
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

func TestAppGW_AzureHeaders_Present(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, appGWURL(srv.URL, "sub1", "rg1", "agw1"), appGWBodyV2)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	if resp.Header.Get("x-ms-request-id") == "" {
		t.Error("x-ms-request-id header missing")
	}
	if resp.Header.Get("x-ms-correlation-request-id") == "" {
		t.Error("x-ms-correlation-request-id header missing")
	}
}

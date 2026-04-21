package arm

import (
	"fmt"
	"net/http"
	"testing"
)

// --- URL builders ---

func lbURL(srvURL, sub, rg, name string) string {
	return fmt.Sprintf(
		"%s/subscriptions/%s/resourcegroups/%s/providers/microsoft.network/loadbalancers/%s",
		srvURL, sub, rg, name,
	)
}

func lbListByRGURL(srvURL, sub, rg string) string {
	return fmt.Sprintf(
		"%s/subscriptions/%s/resourcegroups/%s/providers/microsoft.network/loadbalancers",
		srvURL, sub, rg,
	)
}

func lbListBySubURL(srvURL, sub string) string {
	return fmt.Sprintf(
		"%s/subscriptions/%s/providers/microsoft.network/loadbalancers",
		srvURL, sub,
	)
}

func lbBackendPoolURL(srvURL, sub, rg, lb, pool string) string {
	return fmt.Sprintf(
		"%s/subscriptions/%s/resourcegroups/%s/providers/microsoft.network/loadbalancers/%s/backendaddresspools/%s",
		srvURL, sub, rg, lb, pool,
	)
}

func lbBackendPoolListURL(srvURL, sub, rg, lb string) string {
	return fmt.Sprintf(
		"%s/subscriptions/%s/resourcegroups/%s/providers/microsoft.network/loadbalancers/%s/backendaddresspools",
		srvURL, sub, rg, lb,
	)
}

func lbRuleURL(srvURL, sub, rg, lb, rule string) string {
	return fmt.Sprintf(
		"%s/subscriptions/%s/resourcegroups/%s/providers/microsoft.network/loadbalancers/%s/loadbalancingrules/%s",
		srvURL, sub, rg, lb, rule,
	)
}

func lbRuleListURL(srvURL, sub, rg, lb string) string {
	return fmt.Sprintf(
		"%s/subscriptions/%s/resourcegroups/%s/providers/microsoft.network/loadbalancers/%s/loadbalancingrules",
		srvURL, sub, rg, lb,
	)
}

func lbProbeURL(srvURL, sub, rg, lb, probe string) string {
	return fmt.Sprintf(
		"%s/subscriptions/%s/resourcegroups/%s/providers/microsoft.network/loadbalancers/%s/probes/%s",
		srvURL, sub, rg, lb, probe,
	)
}

func lbProbeListURL(srvURL, sub, rg, lb string) string {
	return fmt.Sprintf(
		"%s/subscriptions/%s/resourcegroups/%s/providers/microsoft.network/loadbalancers/%s/probes",
		srvURL, sub, rg, lb,
	)
}

// --- Test bodies ---

const lbBodyStandard = `{
  "location": "uksouth",
  "sku": {"name": "Standard"},
  "properties": {
    "frontendIPConfigurations": [
      {
        "name": "fe-config",
        "properties": {"privateIPAllocationMethod": "Dynamic"}
      }
    ]
  }
}`

const lbBodyBasic = `{
  "location": "uksouth",
  "sku": {"name": "Basic"},
  "properties": {
    "frontendIPConfigurations": []
  }
}`

const backendPoolBody = `{"properties": {}}`

const lbRuleBody = `{
  "properties": {
    "protocol": "Tcp",
    "frontendPort": 80,
    "backendPort": 80,
    "idleTimeoutInMinutes": 4,
    "enableFloatingIP": false
  }
}`

const lbProbeBody = `{
  "properties": {
    "protocol": "Http",
    "port": 80,
    "requestPath": "/health",
    "intervalInSeconds": 15,
    "numberOfProbes": 2
  }
}`

// ==========================================================================
// Load Balancer tests
// ==========================================================================

func TestLB_PUT_Creates_Returns201(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, lbURL(srv.URL, "sub1", "rg1", "lb1"), lbBodyStandard)
	assertStatus(t, resp, http.StatusCreated)

	body := decodeJSON(t, resp)
	if body["name"] != "lb1" {
		t.Errorf("name = %v, want lb1", body["name"])
	}
	if body["type"] != lbTypeString {
		t.Errorf("type = %v, want %s", body["type"], lbTypeString)
	}
	if body["location"] != "uksouth" {
		t.Errorf("location = %v, want uksouth", body["location"])
	}
	wantID := "/subscriptions/sub1/resourceGroups/rg1/providers/Microsoft.Network/loadBalancers/lb1"
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

	sku, ok := body["sku"].(map[string]interface{})
	if !ok {
		t.Fatalf("sku missing or wrong type: %T", body["sku"])
	}
	if sku["name"] != "Standard" {
		t.Errorf("sku.name = %v, want Standard", sku["name"])
	}
	if _, leaked := props["_sku"]; leaked {
		t.Errorf("_sku must not appear in the response properties")
	}
}

func TestLB_PUT_Updates_Returns200(t *testing.T) {
	srv := newTestServer(t)
	url := lbURL(srv.URL, "sub1", "rg1", "lb1")

	resp := httpPut(t, url, lbBodyStandard)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = httpPut(t, url, lbBodyStandard)
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()
}

func TestLB_PUT_DefaultSKU_WhenOmitted(t *testing.T) {
	srv := newTestServer(t)
	body := `{"location":"uksouth","properties":{}}`
	resp := httpPut(t, lbURL(srv.URL, "sub1", "rg1", "lb-nosku"), body)
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

func TestLB_PUT_MissingLocation_Returns400(t *testing.T) {
	srv := newTestServer(t)
	body := `{"properties":{}}`
	resp := httpPut(t, lbURL(srv.URL, "sub1", "rg1", "lb1"), body)
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

func TestLB_PUT_InvalidJSON_Returns400(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, lbURL(srv.URL, "sub1", "rg1", "lb1"), `{not json`)
	assertStatus(t, resp, http.StatusBadRequest)
	resp.Body.Close()
}

func TestLB_PUT_ChildArraysDropped(t *testing.T) {
	srv := newTestServer(t)
	// backendAddressPools sent inline must be dropped from stored properties.
	body := `{"location":"uksouth","properties":{"backendAddressPools":[{"name":"pool1"}],"loadBalancingRules":[]}}`
	resp := httpPut(t, lbURL(srv.URL, "sub1", "rg1", "lb1"), body)
	assertStatus(t, resp, http.StatusCreated)

	got := decodeJSON(t, resp)
	props := got["properties"].(map[string]interface{})
	// Pools should be an empty array (no children were created as child resources).
	pools, ok := props["backendAddressPools"].([]interface{})
	if !ok {
		t.Fatalf("backendAddressPools missing: %v", props)
	}
	if len(pools) != 0 {
		t.Errorf("inline backendAddressPools must be dropped; got %d items", len(pools))
	}
}

func TestLB_GET_Exists_Returns200(t *testing.T) {
	srv := newTestServer(t)
	url := lbURL(srv.URL, "sub1", "rg1", "lb1")

	resp := httpPut(t, url, lbBodyStandard)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = httpGet(t, url)
	assertStatus(t, resp, http.StatusOK)
	body := decodeJSON(t, resp)
	if body["name"] != "lb1" {
		t.Errorf("name = %v, want lb1", body["name"])
	}
	if body["type"] != lbTypeString {
		t.Errorf("type = %v, want %s", body["type"], lbTypeString)
	}
}

func TestLB_GET_NotFound_Returns404(t *testing.T) {
	srv := newTestServer(t)
	resp := httpGet(t, lbURL(srv.URL, "sub1", "rg1", "ghost"))
	assertStatus(t, resp, http.StatusNotFound)

	errBody := decodeJSON(t, resp)
	errObj := errBody["error"].(map[string]interface{})
	if errObj["code"] != "ResourceNotFound" {
		t.Errorf("code = %v, want ResourceNotFound", errObj["code"])
	}
}

func TestLB_GET_EmbedsChildArrays(t *testing.T) {
	srv := newTestServer(t)
	url := lbURL(srv.URL, "sub1", "rg1", "lb1")

	resp := httpPut(t, url, lbBodyStandard)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	// Add one pool, one rule, one probe.
	resp = httpPut(t, lbBackendPoolURL(srv.URL, "sub1", "rg1", "lb1", "pool1"), backendPoolBody)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()
	resp = httpPut(t, lbRuleURL(srv.URL, "sub1", "rg1", "lb1", "rule1"), lbRuleBody)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()
	resp = httpPut(t, lbProbeURL(srv.URL, "sub1", "rg1", "lb1", "probe1"), lbProbeBody)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = httpGet(t, url)
	assertStatus(t, resp, http.StatusOK)
	body := decodeJSON(t, resp)
	props := body["properties"].(map[string]interface{})

	pools := props["backendAddressPools"].([]interface{})
	if len(pools) != 1 {
		t.Errorf("backendAddressPools count = %d, want 1", len(pools))
	}
	rules := props["loadBalancingRules"].([]interface{})
	if len(rules) != 1 {
		t.Errorf("loadBalancingRules count = %d, want 1", len(rules))
	}
	probes := props["probes"].([]interface{})
	if len(probes) != 1 {
		t.Errorf("probes count = %d, want 1", len(probes))
	}
}

func TestLB_HEAD_Exists_Returns204_NoBody(t *testing.T) {
	srv := newTestServer(t)
	url := lbURL(srv.URL, "sub1", "rg1", "lb1")
	resp := httpPut(t, url, lbBodyStandard)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = httpHead(t, url)
	assertStatus(t, resp, http.StatusNoContent)
	if body := readBody(t, resp); body != "" {
		t.Errorf("HEAD body = %q, want empty", body)
	}
}

func TestLB_HEAD_NotFound_Returns404_NoBody(t *testing.T) {
	srv := newTestServer(t)
	resp := httpHead(t, lbURL(srv.URL, "sub1", "rg1", "ghost"))
	assertStatus(t, resp, http.StatusNotFound)
	if body := readBody(t, resp); body != "" {
		t.Errorf("HEAD body = %q, want empty", body)
	}
}

func TestLB_DELETE_Returns202_WithLocationHeader(t *testing.T) {
	srv := newTestServer(t)
	url := lbURL(srv.URL, "sub1", "rg1", "lb1")
	resp := httpPut(t, url, lbBodyStandard)
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

func TestLB_DELETE_NotFound_Returns404(t *testing.T) {
	srv := newTestServer(t)
	resp := httpDelete(t, lbURL(srv.URL, "sub1", "rg1", "ghost"))
	assertStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
}

func TestLB_DELETE_CascadesChildren(t *testing.T) {
	srv := newTestServer(t)
	url := lbURL(srv.URL, "sub1", "rg1", "lb1")

	resp := httpPut(t, url, lbBodyStandard)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	poolURL := lbBackendPoolURL(srv.URL, "sub1", "rg1", "lb1", "pool1")
	resp = httpPut(t, poolURL, backendPoolBody)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	ruleURL := lbRuleURL(srv.URL, "sub1", "rg1", "lb1", "rule1")
	resp = httpPut(t, ruleURL, lbRuleBody)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	probeURL := lbProbeURL(srv.URL, "sub1", "rg1", "lb1", "probe1")
	resp = httpPut(t, probeURL, lbProbeBody)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	// Delete LB — cascade removes all children.
	resp = httpDelete(t, url)
	assertStatus(t, resp, http.StatusAccepted)
	resp.Body.Close()

	resp = httpGet(t, poolURL)
	assertStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()

	resp = httpGet(t, ruleURL)
	assertStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()

	resp = httpGet(t, probeURL)
	assertStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
}

func TestLB_LIST_ByRG_ReturnsValueWrapper(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, lbURL(srv.URL, "sub1", "rg1", "lb-a"), lbBodyStandard)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()
	resp = httpPut(t, lbURL(srv.URL, "sub1", "rg1", "lb-b"), lbBodyBasic)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = httpGet(t, lbListByRGURL(srv.URL, "sub1", "rg1"))
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

func TestLB_LIST_BySubscription_ReturnsAll(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, lbURL(srv.URL, "sub1", "rg1", "lb-a"), lbBodyStandard)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()
	resp = httpPut(t, lbURL(srv.URL, "sub1", "rg2", "lb-b"), lbBodyBasic)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = httpGet(t, lbListBySubURL(srv.URL, "sub1"))
	assertStatus(t, resp, http.StatusOK)
	body := decodeJSON(t, resp)
	items := body["value"].([]interface{})
	if len(items) != 2 {
		t.Errorf("len(items) = %d, want 2 (across rg1 and rg2)", len(items))
	}
}

func TestLB_LIST_FiltersOutChildren(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, lbURL(srv.URL, "sub1", "rg1", "lb1"), lbBodyStandard)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()
	resp = httpPut(t, lbBackendPoolURL(srv.URL, "sub1", "rg1", "lb1", "pool1"), backendPoolBody)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = httpGet(t, lbListByRGURL(srv.URL, "sub1", "rg1"))
	assertStatus(t, resp, http.StatusOK)
	body := decodeJSON(t, resp)
	items := body["value"].([]interface{})
	if len(items) != 1 {
		t.Errorf("lb list should contain only the LB, not children; got %d items", len(items))
	}
}

func TestLB_MissingAPIVersion_Returns400(t *testing.T) {
	srv := newTestServer(t)
	resp := httpGetRaw(t, lbURL(srv.URL, "sub1", "rg1", "lb1"))
	assertStatus(t, resp, http.StatusBadRequest)
	body := decodeJSON(t, resp)
	errObj := body["error"].(map[string]interface{})
	if errObj["code"] != "MissingApiVersionParameter" {
		t.Errorf("code = %v, want MissingApiVersionParameter", errObj["code"])
	}
}

func TestLB_PUT_NilTags_NormalisedToEmptyObject(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, lbURL(srv.URL, "sub1", "rg1", "lb1"), lbBodyStandard)
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

func TestLB_AzureHeaders_Present(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, lbURL(srv.URL, "sub1", "rg1", "lb1"), lbBodyStandard)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	if resp.Header.Get("x-ms-request-id") == "" {
		t.Error("x-ms-request-id header missing")
	}
	if resp.Header.Get("x-ms-correlation-request-id") == "" {
		t.Error("x-ms-correlation-request-id header missing")
	}
}

// ==========================================================================
// Backend Address Pool tests
// ==========================================================================

func TestLBBackendPool_PUT_Creates_Returns201(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, lbURL(srv.URL, "sub1", "rg1", "lb1"), lbBodyStandard)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = httpPut(t, lbBackendPoolURL(srv.URL, "sub1", "rg1", "lb1", "pool1"), backendPoolBody)
	assertStatus(t, resp, http.StatusCreated)

	body := decodeJSON(t, resp)
	if body["name"] != "pool1" {
		t.Errorf("name = %v, want pool1", body["name"])
	}
	if body["type"] != lbBackendPoolType {
		t.Errorf("type = %v, want %s", body["type"], lbBackendPoolType)
	}
	props := body["properties"].(map[string]interface{})
	if props["provisioningState"] != "Succeeded" {
		t.Errorf("provisioningState = %v, want Succeeded", props["provisioningState"])
	}
}

func TestLBBackendPool_PUT_Updates_Returns200(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, lbURL(srv.URL, "sub1", "rg1", "lb1"), lbBodyStandard)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	url := lbBackendPoolURL(srv.URL, "sub1", "rg1", "lb1", "pool1")
	resp = httpPut(t, url, backendPoolBody)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = httpPut(t, url, backendPoolBody)
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()
}

func TestLBBackendPool_PUT_ParentNotFound_Returns404(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, lbBackendPoolURL(srv.URL, "sub1", "rg1", "lb-ghost", "pool1"), backendPoolBody)
	assertStatus(t, resp, http.StatusNotFound)

	errBody := decodeJSON(t, resp)
	errObj := errBody["error"].(map[string]interface{})
	if errObj["code"] != "ParentResourceNotFound" {
		t.Errorf("code = %v, want ParentResourceNotFound", errObj["code"])
	}
}

func TestLBBackendPool_GET_Exists_Returns200(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, lbURL(srv.URL, "sub1", "rg1", "lb1"), lbBodyStandard)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	url := lbBackendPoolURL(srv.URL, "sub1", "rg1", "lb1", "pool1")
	resp = httpPut(t, url, backendPoolBody)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = httpGet(t, url)
	assertStatus(t, resp, http.StatusOK)
	body := decodeJSON(t, resp)
	if body["name"] != "pool1" {
		t.Errorf("name = %v, want pool1", body["name"])
	}
}

func TestLBBackendPool_GET_NotFound_Returns404(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, lbURL(srv.URL, "sub1", "rg1", "lb1"), lbBodyStandard)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = httpGet(t, lbBackendPoolURL(srv.URL, "sub1", "rg1", "lb1", "ghost"))
	assertStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
}

func TestLBBackendPool_HEAD_Exists_Returns204(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, lbURL(srv.URL, "sub1", "rg1", "lb1"), lbBodyStandard)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	url := lbBackendPoolURL(srv.URL, "sub1", "rg1", "lb1", "pool1")
	resp = httpPut(t, url, backendPoolBody)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = httpHead(t, url)
	assertStatus(t, resp, http.StatusNoContent)
	if body := readBody(t, resp); body != "" {
		t.Errorf("HEAD body = %q, want empty", body)
	}
}

func TestLBBackendPool_DELETE_Returns202(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, lbURL(srv.URL, "sub1", "rg1", "lb1"), lbBodyStandard)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	url := lbBackendPoolURL(srv.URL, "sub1", "rg1", "lb1", "pool1")
	resp = httpPut(t, url, backendPoolBody)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = httpDelete(t, url)
	assertStatus(t, resp, http.StatusAccepted)
	if loc := resp.Header.Get("Location"); loc == "" {
		t.Errorf("Location header missing on DELETE")
	}
	resp.Body.Close()

	resp = httpGet(t, url)
	assertStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
}

func TestLBBackendPool_LIST_ReturnsValueWrapper(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, lbURL(srv.URL, "sub1", "rg1", "lb1"), lbBodyStandard)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = httpPut(t, lbBackendPoolURL(srv.URL, "sub1", "rg1", "lb1", "pool1"), backendPoolBody)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()
	resp = httpPut(t, lbBackendPoolURL(srv.URL, "sub1", "rg1", "lb1", "pool2"), backendPoolBody)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = httpGet(t, lbBackendPoolListURL(srv.URL, "sub1", "rg1", "lb1"))
	assertStatus(t, resp, http.StatusOK)
	body := decodeJSON(t, resp)
	items := body["value"].([]interface{})
	if len(items) != 2 {
		t.Errorf("len(items) = %d, want 2", len(items))
	}
}

// ==========================================================================
// Load Balancing Rule tests
// ==========================================================================

func TestLBRule_PUT_Creates_Returns201(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, lbURL(srv.URL, "sub1", "rg1", "lb1"), lbBodyStandard)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = httpPut(t, lbRuleURL(srv.URL, "sub1", "rg1", "lb1", "rule1"), lbRuleBody)
	assertStatus(t, resp, http.StatusCreated)

	body := decodeJSON(t, resp)
	if body["name"] != "rule1" {
		t.Errorf("name = %v, want rule1", body["name"])
	}
	if body["type"] != lbRuleType {
		t.Errorf("type = %v, want %s", body["type"], lbRuleType)
	}
}

func TestLBRule_PUT_ParentNotFound_Returns404(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, lbRuleURL(srv.URL, "sub1", "rg1", "lb-ghost", "rule1"), lbRuleBody)
	assertStatus(t, resp, http.StatusNotFound)

	errBody := decodeJSON(t, resp)
	errObj := errBody["error"].(map[string]interface{})
	if errObj["code"] != "ParentResourceNotFound" {
		t.Errorf("code = %v, want ParentResourceNotFound", errObj["code"])
	}
}

func TestLBRule_GET_Exists_Returns200(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, lbURL(srv.URL, "sub1", "rg1", "lb1"), lbBodyStandard)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	url := lbRuleURL(srv.URL, "sub1", "rg1", "lb1", "rule1")
	resp = httpPut(t, url, lbRuleBody)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = httpGet(t, url)
	assertStatus(t, resp, http.StatusOK)
	body := decodeJSON(t, resp)
	if body["name"] != "rule1" {
		t.Errorf("name = %v, want rule1", body["name"])
	}
}

func TestLBRule_HEAD_Exists_Returns204(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, lbURL(srv.URL, "sub1", "rg1", "lb1"), lbBodyStandard)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	url := lbRuleURL(srv.URL, "sub1", "rg1", "lb1", "rule1")
	resp = httpPut(t, url, lbRuleBody)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = httpHead(t, url)
	assertStatus(t, resp, http.StatusNoContent)
	if body := readBody(t, resp); body != "" {
		t.Errorf("HEAD body = %q, want empty", body)
	}
}

func TestLBRule_DELETE_Returns202(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, lbURL(srv.URL, "sub1", "rg1", "lb1"), lbBodyStandard)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	url := lbRuleURL(srv.URL, "sub1", "rg1", "lb1", "rule1")
	resp = httpPut(t, url, lbRuleBody)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = httpDelete(t, url)
	assertStatus(t, resp, http.StatusAccepted)
	resp.Body.Close()

	resp = httpGet(t, url)
	assertStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
}

func TestLBRule_LIST_ReturnsValueWrapper(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, lbURL(srv.URL, "sub1", "rg1", "lb1"), lbBodyStandard)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = httpPut(t, lbRuleURL(srv.URL, "sub1", "rg1", "lb1", "rule1"), lbRuleBody)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = httpGet(t, lbRuleListURL(srv.URL, "sub1", "rg1", "lb1"))
	assertStatus(t, resp, http.StatusOK)
	body := decodeJSON(t, resp)
	items := body["value"].([]interface{})
	if len(items) != 1 {
		t.Errorf("len(items) = %d, want 1", len(items))
	}
}

// ==========================================================================
// Probe tests
// ==========================================================================

func TestLBProbe_PUT_Creates_Returns201(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, lbURL(srv.URL, "sub1", "rg1", "lb1"), lbBodyStandard)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = httpPut(t, lbProbeURL(srv.URL, "sub1", "rg1", "lb1", "probe1"), lbProbeBody)
	assertStatus(t, resp, http.StatusCreated)

	body := decodeJSON(t, resp)
	if body["name"] != "probe1" {
		t.Errorf("name = %v, want probe1", body["name"])
	}
	if body["type"] != lbProbeType {
		t.Errorf("type = %v, want %s", body["type"], lbProbeType)
	}
	props := body["properties"].(map[string]interface{})
	if props["protocol"] != "Http" {
		t.Errorf("protocol = %v, want Http", props["protocol"])
	}
}

func TestLBProbe_PUT_ParentNotFound_Returns404(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, lbProbeURL(srv.URL, "sub1", "rg1", "lb-ghost", "probe1"), lbProbeBody)
	assertStatus(t, resp, http.StatusNotFound)

	errBody := decodeJSON(t, resp)
	errObj := errBody["error"].(map[string]interface{})
	if errObj["code"] != "ParentResourceNotFound" {
		t.Errorf("code = %v, want ParentResourceNotFound", errObj["code"])
	}
}

func TestLBProbe_GET_Exists_Returns200(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, lbURL(srv.URL, "sub1", "rg1", "lb1"), lbBodyStandard)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	url := lbProbeURL(srv.URL, "sub1", "rg1", "lb1", "probe1")
	resp = httpPut(t, url, lbProbeBody)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = httpGet(t, url)
	assertStatus(t, resp, http.StatusOK)
	body := decodeJSON(t, resp)
	if body["name"] != "probe1" {
		t.Errorf("name = %v, want probe1", body["name"])
	}
}

func TestLBProbe_HEAD_Exists_Returns204(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, lbURL(srv.URL, "sub1", "rg1", "lb1"), lbBodyStandard)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	url := lbProbeURL(srv.URL, "sub1", "rg1", "lb1", "probe1")
	resp = httpPut(t, url, lbProbeBody)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = httpHead(t, url)
	assertStatus(t, resp, http.StatusNoContent)
	if body := readBody(t, resp); body != "" {
		t.Errorf("HEAD body = %q, want empty", body)
	}
}

func TestLBProbe_DELETE_Returns202(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, lbURL(srv.URL, "sub1", "rg1", "lb1"), lbBodyStandard)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	url := lbProbeURL(srv.URL, "sub1", "rg1", "lb1", "probe1")
	resp = httpPut(t, url, lbProbeBody)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = httpDelete(t, url)
	assertStatus(t, resp, http.StatusAccepted)
	resp.Body.Close()

	resp = httpGet(t, url)
	assertStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
}

func TestLBProbe_LIST_ReturnsValueWrapper(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, lbURL(srv.URL, "sub1", "rg1", "lb1"), lbBodyStandard)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = httpPut(t, lbProbeURL(srv.URL, "sub1", "rg1", "lb1", "probe1"), lbProbeBody)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()
	resp = httpPut(t, lbProbeURL(srv.URL, "sub1", "rg1", "lb1", "probe2"), lbProbeBody)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = httpGet(t, lbProbeListURL(srv.URL, "sub1", "rg1", "lb1"))
	assertStatus(t, resp, http.StatusOK)
	body := decodeJSON(t, resp)
	items := body["value"].([]interface{})
	if len(items) != 2 {
		t.Errorf("len(items) = %d, want 2", len(items))
	}
}

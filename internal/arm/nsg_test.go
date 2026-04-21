package arm

import (
	"fmt"
	"net/http"
	"testing"
)

func nsgURL(srvURL, sub, rg, nsg string) string {
	return fmt.Sprintf(
		"%s/subscriptions/%s/resourcegroups/%s/providers/microsoft.network/networksecuritygroups/%s",
		srvURL, sub, rg, nsg,
	)
}

func nsgListByRGURL(srvURL, sub, rg string) string {
	return fmt.Sprintf(
		"%s/subscriptions/%s/resourcegroups/%s/providers/microsoft.network/networksecuritygroups",
		srvURL, sub, rg,
	)
}

func nsgListBySubURL(srvURL, sub string) string {
	return fmt.Sprintf(
		"%s/subscriptions/%s/providers/microsoft.network/networksecuritygroups",
		srvURL, sub,
	)
}

func ruleURL(srvURL, sub, rg, nsg, rule string) string {
	return fmt.Sprintf(
		"%s/subscriptions/%s/resourcegroups/%s/providers/microsoft.network/networksecuritygroups/%s/securityrules/%s",
		srvURL, sub, rg, nsg, rule,
	)
}

func ruleListURL(srvURL, sub, rg, nsg string) string {
	return fmt.Sprintf(
		"%s/subscriptions/%s/resourcegroups/%s/providers/microsoft.network/networksecuritygroups/%s/securityrules",
		srvURL, sub, rg, nsg,
	)
}

const nsgBodyMinimal = `{"location":"uksouth"}`

const ruleBodyAllow = `{
  "properties": {
    "priority": 100,
    "protocol": "Tcp",
    "access": "Allow",
    "direction": "Inbound",
    "sourceAddressPrefix": "*",
    "sourcePortRange": "*",
    "destinationAddressPrefix": "*",
    "destinationPortRange": "80"
  }
}`

// --- NSG tests ---

func TestNSG_PUT_Creates_Returns201(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, nsgURL(srv.URL, "sub1", "rg1", "nsg1"), nsgBodyMinimal)
	assertStatus(t, resp, http.StatusCreated)

	body := decodeJSON(t, resp)
	if body["name"] != "nsg1" {
		t.Errorf("name = %v, want nsg1", body["name"])
	}
	if body["type"] != nsgTypeString {
		t.Errorf("type = %v, want %s", body["type"], nsgTypeString)
	}
	if body["location"] != "uksouth" {
		t.Errorf("location = %v, want uksouth", body["location"])
	}
	wantID := "/subscriptions/sub1/resourceGroups/rg1/providers/Microsoft.Network/networkSecurityGroups/nsg1"
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
	// securityRules must be an empty array, not null or missing.
	rules, ok := props["securityRules"].([]interface{})
	if !ok {
		t.Fatalf("securityRules missing or wrong type: %T", props["securityRules"])
	}
	if len(rules) != 0 {
		t.Errorf("securityRules = %v, want []", rules)
	}
}

func TestNSG_PUT_Updates_Returns200(t *testing.T) {
	srv := newTestServer(t)
	url := nsgURL(srv.URL, "sub1", "rg1", "nsg1")

	resp := httpPut(t, url, nsgBodyMinimal)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = httpPut(t, url, nsgBodyMinimal)
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()
}

func TestNSG_PUT_MissingLocation_Returns400(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, nsgURL(srv.URL, "sub1", "rg1", "nsg1"), `{}`)
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

func TestNSG_PUT_InvalidJSON_Returns400(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, nsgURL(srv.URL, "sub1", "rg1", "nsg1"), `{not json`)
	assertStatus(t, resp, http.StatusBadRequest)
	resp.Body.Close()
}

func TestNSG_GET_Exists_Returns200(t *testing.T) {
	srv := newTestServer(t)
	url := nsgURL(srv.URL, "sub1", "rg1", "nsg1")

	resp := httpPut(t, url, nsgBodyMinimal)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = httpGet(t, url)
	assertStatus(t, resp, http.StatusOK)
	body := decodeJSON(t, resp)
	if body["name"] != "nsg1" {
		t.Errorf("name = %v, want nsg1", body["name"])
	}
}

func TestNSG_GET_NotFound_Returns404(t *testing.T) {
	srv := newTestServer(t)
	resp := httpGet(t, nsgURL(srv.URL, "sub1", "rg1", "ghost"))
	assertStatus(t, resp, http.StatusNotFound)

	errBody := decodeJSON(t, resp)
	errObj := errBody["error"].(map[string]interface{})
	if errObj["code"] != "ResourceNotFound" {
		t.Errorf("code = %v, want ResourceNotFound", errObj["code"])
	}
}

func TestNSG_GET_EmbedsChildRules(t *testing.T) {
	srv := newTestServer(t)
	nsgU := nsgURL(srv.URL, "sub1", "rg1", "nsg1")

	resp := httpPut(t, nsgU, nsgBodyMinimal)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = httpPut(t, ruleURL(srv.URL, "sub1", "rg1", "nsg1", "allow-http"), ruleBodyAllow)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = httpGet(t, nsgU)
	assertStatus(t, resp, http.StatusOK)
	body := decodeJSON(t, resp)
	props := body["properties"].(map[string]interface{})
	rules, ok := props["securityRules"].([]interface{})
	if !ok {
		t.Fatalf("securityRules missing: %v", props)
	}
	if len(rules) != 1 {
		t.Fatalf("len(securityRules) = %d, want 1", len(rules))
	}
	rule := rules[0].(map[string]interface{})
	if rule["name"] != "allow-http" {
		t.Errorf("rule name = %v, want allow-http", rule["name"])
	}
}

func TestNSG_HEAD_Exists_Returns204_NoBody(t *testing.T) {
	srv := newTestServer(t)
	url := nsgURL(srv.URL, "sub1", "rg1", "nsg1")
	resp := httpPut(t, url, nsgBodyMinimal)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = httpHead(t, url)
	assertStatus(t, resp, http.StatusNoContent)
	if body := readBody(t, resp); body != "" {
		t.Errorf("HEAD body = %q, want empty", body)
	}
}

func TestNSG_HEAD_NotFound_Returns404_NoBody(t *testing.T) {
	srv := newTestServer(t)
	resp := httpHead(t, nsgURL(srv.URL, "sub1", "rg1", "ghost"))
	assertStatus(t, resp, http.StatusNotFound)
	if body := readBody(t, resp); body != "" {
		t.Errorf("HEAD body = %q, want empty", body)
	}
}

func TestNSG_DELETE_Returns202_WithLocationHeader(t *testing.T) {
	srv := newTestServer(t)
	url := nsgURL(srv.URL, "sub1", "rg1", "nsg1")
	resp := httpPut(t, url, nsgBodyMinimal)
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

func TestNSG_DELETE_NotFound_Returns404(t *testing.T) {
	srv := newTestServer(t)
	resp := httpDelete(t, nsgURL(srv.URL, "sub1", "rg1", "ghost"))
	assertStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
}

func TestNSG_DELETE_CascadesRules(t *testing.T) {
	srv := newTestServer(t)
	nsgU := nsgURL(srv.URL, "sub1", "rg1", "nsg1")

	resp := httpPut(t, nsgU, nsgBodyMinimal)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	ruleU := ruleURL(srv.URL, "sub1", "rg1", "nsg1", "rule1")
	resp = httpPut(t, ruleU, ruleBodyAllow)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = httpDelete(t, nsgU)
	assertStatus(t, resp, http.StatusAccepted)
	resp.Body.Close()

	// Rule must also be gone after cascade.
	resp = httpGet(t, ruleU)
	assertStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
}

func TestNSG_LIST_ByRG_ReturnsValueWrapper(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, nsgURL(srv.URL, "sub1", "rg1", "nsg-a"), nsgBodyMinimal)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()
	resp = httpPut(t, nsgURL(srv.URL, "sub1", "rg1", "nsg-b"), nsgBodyMinimal)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = httpGet(t, nsgListByRGURL(srv.URL, "sub1", "rg1"))
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

func TestNSG_LIST_ByRG_FiltersOutRules(t *testing.T) {
	srv := newTestServer(t)
	nsgU := nsgURL(srv.URL, "sub1", "rg1", "nsg-with-rule")
	resp := httpPut(t, nsgU, nsgBodyMinimal)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = httpPut(t, ruleURL(srv.URL, "sub1", "rg1", "nsg-with-rule", "rule1"), ruleBodyAllow)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = httpGet(t, nsgListByRGURL(srv.URL, "sub1", "rg1"))
	assertStatus(t, resp, http.StatusOK)
	body := decodeJSON(t, resp)
	items := body["value"].([]interface{})
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1 (rule must be filtered out)", len(items))
	}
	item := items[0].(map[string]interface{})
	if item["type"] != nsgTypeString {
		t.Errorf("type = %v, want %s", item["type"], nsgTypeString)
	}
}

func TestNSG_LIST_BySubscription_ReturnsAll(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, nsgURL(srv.URL, "sub1", "rg1", "nsg-a"), nsgBodyMinimal)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()
	resp = httpPut(t, nsgURL(srv.URL, "sub1", "rg2", "nsg-b"), nsgBodyMinimal)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = httpGet(t, nsgListBySubURL(srv.URL, "sub1"))
	assertStatus(t, resp, http.StatusOK)
	body := decodeJSON(t, resp)
	items := body["value"].([]interface{})
	if len(items) != 2 {
		t.Errorf("len(items) = %d, want 2 (across rg1 and rg2)", len(items))
	}
}

func TestNSG_LIST_Empty_ReturnsEmptyArray(t *testing.T) {
	srv := newTestServer(t)
	resp := httpGet(t, nsgListByRGURL(srv.URL, "sub1", "rg-empty"))
	assertStatus(t, resp, http.StatusOK)
	body := decodeJSON(t, resp)
	items := body["value"].([]interface{})
	if len(items) != 0 {
		t.Errorf("len(items) = %d, want 0", len(items))
	}
}

func TestNSG_MissingAPIVersion_Returns400(t *testing.T) {
	srv := newTestServer(t)
	resp := httpGetRaw(t, nsgURL(srv.URL, "sub1", "rg1", "nsg1"))
	assertStatus(t, resp, http.StatusBadRequest)
	body := decodeJSON(t, resp)
	errObj := body["error"].(map[string]interface{})
	if errObj["code"] != "MissingApiVersionParameter" {
		t.Errorf("code = %v, want MissingApiVersionParameter", errObj["code"])
	}
}

func TestNSG_PUT_NilTags_NormalisedToEmptyObject(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, nsgURL(srv.URL, "sub1", "rg1", "nsg1"), nsgBodyMinimal)
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

// --- Security Rule tests ---

func TestRule_PUT_Creates_Returns201(t *testing.T) {
	srv := newTestServer(t)
	nsgU := nsgURL(srv.URL, "sub1", "rg1", "nsg1")
	resp := httpPut(t, nsgU, nsgBodyMinimal)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	ruleU := ruleURL(srv.URL, "sub1", "rg1", "nsg1", "allow-http")
	resp = httpPut(t, ruleU, ruleBodyAllow)
	assertStatus(t, resp, http.StatusCreated)

	body := decodeJSON(t, resp)
	if body["name"] != "allow-http" {
		t.Errorf("name = %v, want allow-http", body["name"])
	}
	if body["type"] != ruleTypeString {
		t.Errorf("type = %v, want %s", body["type"], ruleTypeString)
	}
	wantID := "/subscriptions/sub1/resourceGroups/rg1/providers/Microsoft.Network/networkSecurityGroups/nsg1/securityRules/allow-http"
	if body["id"] != wantID {
		t.Errorf("id = %v, want %s", body["id"], wantID)
	}
	props := body["properties"].(map[string]interface{})
	if props["provisioningState"] != "Succeeded" {
		t.Errorf("provisioningState = %v, want Succeeded", props["provisioningState"])
	}
	if props["priority"] != float64(100) {
		t.Errorf("priority = %v, want 100", props["priority"])
	}
}

func TestRule_PUT_Updates_Returns200(t *testing.T) {
	srv := newTestServer(t)
	nsgU := nsgURL(srv.URL, "sub1", "rg1", "nsg1")
	resp := httpPut(t, nsgU, nsgBodyMinimal)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	ruleU := ruleURL(srv.URL, "sub1", "rg1", "nsg1", "rule1")
	resp = httpPut(t, ruleU, ruleBodyAllow)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = httpPut(t, ruleU, ruleBodyAllow)
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()
}

func TestRule_PUT_ParentNotFound_Returns404(t *testing.T) {
	srv := newTestServer(t)
	// No NSG created — should return ParentResourceNotFound.
	resp := httpPut(t, ruleURL(srv.URL, "sub1", "rg1", "ghost-nsg", "rule1"), ruleBodyAllow)
	assertStatus(t, resp, http.StatusNotFound)

	errBody := decodeJSON(t, resp)
	errObj := errBody["error"].(map[string]interface{})
	if errObj["code"] != "ParentResourceNotFound" {
		t.Errorf("code = %v, want ParentResourceNotFound", errObj["code"])
	}
}

func TestRule_PUT_InvalidJSON_Returns400(t *testing.T) {
	srv := newTestServer(t)
	nsgU := nsgURL(srv.URL, "sub1", "rg1", "nsg1")
	resp := httpPut(t, nsgU, nsgBodyMinimal)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = httpPut(t, ruleURL(srv.URL, "sub1", "rg1", "nsg1", "rule1"), `{bad`)
	assertStatus(t, resp, http.StatusBadRequest)
	resp.Body.Close()
}

func TestRule_GET_Exists_Returns200(t *testing.T) {
	srv := newTestServer(t)
	nsgU := nsgURL(srv.URL, "sub1", "rg1", "nsg1")
	resp := httpPut(t, nsgU, nsgBodyMinimal)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	ruleU := ruleURL(srv.URL, "sub1", "rg1", "nsg1", "rule1")
	resp = httpPut(t, ruleU, ruleBodyAllow)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = httpGet(t, ruleU)
	assertStatus(t, resp, http.StatusOK)
	body := decodeJSON(t, resp)
	if body["name"] != "rule1" {
		t.Errorf("name = %v, want rule1", body["name"])
	}
}

func TestRule_GET_NotFound_Returns404(t *testing.T) {
	srv := newTestServer(t)
	resp := httpGet(t, ruleURL(srv.URL, "sub1", "rg1", "nsg1", "ghost"))
	assertStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
}

func TestRule_HEAD_Exists_Returns204_NoBody(t *testing.T) {
	srv := newTestServer(t)
	nsgU := nsgURL(srv.URL, "sub1", "rg1", "nsg1")
	resp := httpPut(t, nsgU, nsgBodyMinimal)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	ruleU := ruleURL(srv.URL, "sub1", "rg1", "nsg1", "rule1")
	resp = httpPut(t, ruleU, ruleBodyAllow)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = httpHead(t, ruleU)
	assertStatus(t, resp, http.StatusNoContent)
	if body := readBody(t, resp); body != "" {
		t.Errorf("HEAD body = %q, want empty", body)
	}
}

func TestRule_HEAD_NotFound_Returns404_NoBody(t *testing.T) {
	srv := newTestServer(t)
	resp := httpHead(t, ruleURL(srv.URL, "sub1", "rg1", "nsg1", "ghost"))
	assertStatus(t, resp, http.StatusNotFound)
	if body := readBody(t, resp); body != "" {
		t.Errorf("HEAD body = %q, want empty", body)
	}
}

func TestRule_DELETE_Returns202_WithLocationHeader(t *testing.T) {
	srv := newTestServer(t)
	nsgU := nsgURL(srv.URL, "sub1", "rg1", "nsg1")
	resp := httpPut(t, nsgU, nsgBodyMinimal)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	ruleU := ruleURL(srv.URL, "sub1", "rg1", "nsg1", "rule1")
	resp = httpPut(t, ruleU, ruleBodyAllow)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = httpDelete(t, ruleU)
	assertStatus(t, resp, http.StatusAccepted)
	if loc := resp.Header.Get("Location"); loc == "" {
		t.Errorf("Location header missing on rule DELETE response")
	}
	resp.Body.Close()

	resp = httpGet(t, ruleU)
	assertStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
}

func TestRule_DELETE_NotFound_Returns404(t *testing.T) {
	srv := newTestServer(t)
	resp := httpDelete(t, ruleURL(srv.URL, "sub1", "rg1", "nsg1", "ghost"))
	assertStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
}

func TestRule_LIST_ReturnsValueWrapper(t *testing.T) {
	srv := newTestServer(t)
	nsgU := nsgURL(srv.URL, "sub1", "rg1", "nsg1")
	resp := httpPut(t, nsgU, nsgBodyMinimal)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = httpPut(t, ruleURL(srv.URL, "sub1", "rg1", "nsg1", "rule-a"), ruleBodyAllow)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()
	resp = httpPut(t, ruleURL(srv.URL, "sub1", "rg1", "nsg1", "rule-b"), ruleBodyAllow)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = httpGet(t, ruleListURL(srv.URL, "sub1", "rg1", "nsg1"))
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

func TestRule_LIST_Empty_ReturnsEmptyArray(t *testing.T) {
	srv := newTestServer(t)
	nsgU := nsgURL(srv.URL, "sub1", "rg1", "nsg1")
	resp := httpPut(t, nsgU, nsgBodyMinimal)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = httpGet(t, ruleListURL(srv.URL, "sub1", "rg1", "nsg1"))
	assertStatus(t, resp, http.StatusOK)
	body := decodeJSON(t, resp)
	items := body["value"].([]interface{})
	if len(items) != 0 {
		t.Errorf("len(items) = %d, want 0", len(items))
	}
}

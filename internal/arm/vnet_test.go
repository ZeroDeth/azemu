package arm

import (
	"fmt"
	"net/http"
	"testing"
)

// All vnet URLs share the same prefix; a single helper keeps test bodies
// focused on behaviour rather than path construction.
func vnetURL(srvURL, sub, rg, vnet string) string {
	return fmt.Sprintf(
		"%s/subscriptions/%s/resourcegroups/%s/providers/microsoft.network/virtualnetworks/%s",
		srvURL, sub, rg, vnet,
	)
}

func vnetListByRGURL(srvURL, sub, rg string) string {
	return fmt.Sprintf(
		"%s/subscriptions/%s/resourcegroups/%s/providers/microsoft.network/virtualnetworks",
		srvURL, sub, rg,
	)
}

func vnetListBySubURL(srvURL, sub string) string {
	return fmt.Sprintf(
		"%s/subscriptions/%s/providers/microsoft.network/virtualnetworks",
		srvURL, sub,
	)
}

const vnetBodyMinimal = `{
  "location": "uksouth",
  "properties": {
    "addressSpace": { "addressPrefixes": ["10.0.0.0/16"] }
  }
}`

func TestVNet_PUT_Creates_Returns201(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, vnetURL(srv.URL, "sub1", "rg1", "vnet1"), vnetBodyMinimal)
	assertStatus(t, resp, http.StatusCreated)

	body := decodeJSON(t, resp)
	if body["name"] != "vnet1" {
		t.Errorf("name = %v, want vnet1", body["name"])
	}
	if body["type"] != vnetTypeString {
		t.Errorf("type = %v, want %s", body["type"], vnetTypeString)
	}
	if body["location"] != "uksouth" {
		t.Errorf("location = %v, want uksouth", body["location"])
	}
	wantID := "/subscriptions/sub1/resourceGroups/rg1/providers/Microsoft.Network/virtualNetworks/vnet1"
	if body["id"] != wantID {
		t.Errorf("id = %v, want %s", body["id"], wantID)
	}

	// provisioningState must be embedded in the properties map.
	props, ok := body["properties"].(map[string]interface{})
	if !ok {
		t.Fatalf("properties missing or wrong type: %T", body["properties"])
	}
	if props["provisioningState"] != "Succeeded" {
		t.Errorf("provisioningState = %v, want Succeeded", props["provisioningState"])
	}
	// Even on create, subnets is an empty array, not null or missing.
	subnets, ok := props["subnets"].([]interface{})
	if !ok {
		t.Fatalf("subnets missing or wrong type: %T", props["subnets"])
	}
	if len(subnets) != 0 {
		t.Errorf("subnets = %v, want []", subnets)
	}
}

func TestVNet_PUT_Updates_Returns200(t *testing.T) {
	srv := newTestServer(t)
	url := vnetURL(srv.URL, "sub1", "rg1", "vnet1")

	resp := httpPut(t, url, vnetBodyMinimal)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = httpPut(t, url, vnetBodyMinimal)
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()
}

func TestVNet_PUT_MissingLocation_Returns400(t *testing.T) {
	srv := newTestServer(t)
	body := `{"properties": {"addressSpace": {"addressPrefixes": ["10.0.0.0/16"]}}}`
	resp := httpPut(t, vnetURL(srv.URL, "sub1", "rg1", "vnet1"), body)
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

func TestVNet_PUT_InvalidJSON_Returns400(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, vnetURL(srv.URL, "sub1", "rg1", "vnet1"), `{not json`)
	assertStatus(t, resp, http.StatusBadRequest)
	resp.Body.Close()
}

func TestVNet_GET_Exists_Returns200_WithCorrectShape(t *testing.T) {
	srv := newTestServer(t)
	url := vnetURL(srv.URL, "sub1", "rg1", "vnet1")

	resp := httpPut(t, url, vnetBodyMinimal)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = httpGet(t, url)
	assertStatus(t, resp, http.StatusOK)
	body := decodeJSON(t, resp)

	// addressSpace must be passed through from the original PUT.
	props := body["properties"].(map[string]interface{})
	addrSpace, ok := props["addressSpace"].(map[string]interface{})
	if !ok {
		t.Fatalf("addressSpace missing: %v", props)
	}
	prefixes, ok := addrSpace["addressPrefixes"].([]interface{})
	if !ok || len(prefixes) != 1 || prefixes[0] != "10.0.0.0/16" {
		t.Errorf("addressPrefixes = %v, want [10.0.0.0/16]", prefixes)
	}
}

func TestVNet_GET_NotFound_Returns404(t *testing.T) {
	srv := newTestServer(t)
	resp := httpGet(t, vnetURL(srv.URL, "sub1", "rg1", "ghost"))
	assertStatus(t, resp, http.StatusNotFound)

	errBody := decodeJSON(t, resp)
	errObj := errBody["error"].(map[string]interface{})
	if errObj["code"] != "ResourceNotFound" {
		t.Errorf("code = %v, want ResourceNotFound", errObj["code"])
	}
}

func TestVNet_GET_EmbedsChildSubnets(t *testing.T) {
	srv := newTestServer(t)
	vnetU := vnetURL(srv.URL, "sub1", "rg1", "vnet1")

	resp := httpPut(t, vnetU, vnetBodyMinimal)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	// Create two subnets under the vnet.
	sub1Body := `{"properties":{"addressPrefix":"10.0.1.0/24"}}`
	sub2Body := `{"properties":{"addressPrefix":"10.0.2.0/24"}}`
	resp = httpPut(t, vnetU+"/subnets/sub-a", sub1Body)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()
	resp = httpPut(t, vnetU+"/subnets/sub-b", sub2Body)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	// GET the parent and confirm both subnets are embedded.
	resp = httpGet(t, vnetU)
	assertStatus(t, resp, http.StatusOK)
	body := decodeJSON(t, resp)
	props := body["properties"].(map[string]interface{})
	subnets, ok := props["subnets"].([]interface{})
	if !ok {
		t.Fatalf("subnets missing: %v", props)
	}
	if len(subnets) != 2 {
		t.Fatalf("len(subnets) = %d, want 2", len(subnets))
	}

	names := map[string]bool{}
	for _, s := range subnets {
		sm := s.(map[string]interface{})
		names[sm["name"].(string)] = true
	}
	if !names["sub-a"] || !names["sub-b"] {
		t.Errorf("subnet names = %v, want sub-a and sub-b", names)
	}
}

func TestVNet_HEAD_Exists_Returns204_NoBody(t *testing.T) {
	srv := newTestServer(t)
	url := vnetURL(srv.URL, "sub1", "rg1", "vnet1")
	resp := httpPut(t, url, vnetBodyMinimal)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = httpHead(t, url)
	assertStatus(t, resp, http.StatusNoContent)
	if body := readBody(t, resp); body != "" {
		t.Errorf("HEAD body = %q, want empty", body)
	}
}

func TestVNet_HEAD_NotFound_Returns404_NoBody(t *testing.T) {
	srv := newTestServer(t)
	resp := httpHead(t, vnetURL(srv.URL, "sub1", "rg1", "ghost"))
	assertStatus(t, resp, http.StatusNotFound)
	if body := readBody(t, resp); body != "" {
		t.Errorf("HEAD body = %q, want empty", body)
	}
}

func TestVNet_DELETE_Returns202_WithLocationHeader(t *testing.T) {
	srv := newTestServer(t)
	url := vnetURL(srv.URL, "sub1", "rg1", "vnet1")
	resp := httpPut(t, url, vnetBodyMinimal)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = httpDelete(t, url)
	assertStatus(t, resp, http.StatusAccepted)
	if loc := resp.Header.Get("Location"); loc == "" {
		t.Errorf("Location header missing on DELETE response")
	}
	resp.Body.Close()

	// Follow-up GET must be 404.
	resp = httpGet(t, url)
	assertStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
}

func TestVNet_DELETE_NotFound_Returns404(t *testing.T) {
	srv := newTestServer(t)
	resp := httpDelete(t, vnetURL(srv.URL, "sub1", "rg1", "ghost"))
	assertStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
}

func TestVNet_DELETE_CascadesSubnets(t *testing.T) {
	srv := newTestServer(t)
	vnetU := vnetURL(srv.URL, "sub1", "rg1", "vnet1")

	resp := httpPut(t, vnetU, vnetBodyMinimal)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = httpPut(t, vnetU+"/subnets/child", `{"properties":{"addressPrefix":"10.0.1.0/24"}}`)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = httpDelete(t, vnetU)
	assertStatus(t, resp, http.StatusAccepted)
	resp.Body.Close()

	// Subnet must also be gone, confirming cascade via prefix-match in
	// MemoryStore.Delete.
	resp = httpGet(t, vnetU+"/subnets/child")
	assertStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
}

func TestVNet_LIST_ByRG_ReturnsValueWrapper(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, vnetURL(srv.URL, "sub1", "rg1", "a"), vnetBodyMinimal)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()
	resp = httpPut(t, vnetURL(srv.URL, "sub1", "rg1", "b"), vnetBodyMinimal)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = httpGet(t, vnetListByRGURL(srv.URL, "sub1", "rg1"))
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

func TestVNet_LIST_BySubscription_ReturnsAll(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, vnetURL(srv.URL, "sub1", "rg1", "a"), vnetBodyMinimal)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()
	resp = httpPut(t, vnetURL(srv.URL, "sub1", "rg2", "b"), vnetBodyMinimal)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = httpGet(t, vnetListBySubURL(srv.URL, "sub1"))
	assertStatus(t, resp, http.StatusOK)
	body := decodeJSON(t, resp)
	items := body["value"].([]interface{})
	if len(items) != 2 {
		t.Errorf("len(items) = %d, want 2 (across rg1 and rg2)", len(items))
	}
}

// TestVNet_LIST_ByRG_FiltersOutSubnets covers the "non-vnet type" continue
// branch of writeVNetList that was missing from the Phase 2 initial slice
// (TODO.md coverage gap: writeVNetList 85.7%). The subscription-scope list
// uses a prefix that picks up subnets too, so the type filter must drop
// them. Creating a vnet and a subnet beneath it and then listing vnets
// exercises exactly that branch.
func TestVNet_LIST_ByRG_FiltersOutSubnets(t *testing.T) {
	srv := newTestServer(t)
	vnetU := vnetURL(srv.URL, "sub1", "rg1", "vnet-with-child")
	resp := httpPut(t, vnetU, vnetBodyMinimal)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	// Add a subnet so store.List(prefix) returns both the vnet and the
	// subnet. Without the type filter, writeVNetList would incorrectly
	// render the subnet as a vnet item.
	resp = httpPut(t, vnetU+"/subnets/child", `{"properties":{"addressPrefix":"10.0.1.0/24"}}`)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = httpGet(t, vnetListByRGURL(srv.URL, "sub1", "rg1"))
	assertStatus(t, resp, http.StatusOK)
	body := decodeJSON(t, resp)
	items, ok := body["value"].([]interface{})
	if !ok {
		t.Fatalf("value missing or wrong type: %T", body["value"])
	}
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1 (subnet must be filtered out)", len(items))
	}
	item := items[0].(map[string]interface{})
	if item["type"] != vnetTypeString {
		t.Errorf("type = %v, want %s", item["type"], vnetTypeString)
	}
	if item["name"] != "vnet-with-child" {
		t.Errorf("name = %v, want vnet-with-child", item["name"])
	}
}

func TestVNet_LIST_Empty_ReturnsEmptyArray(t *testing.T) {
	srv := newTestServer(t)
	resp := httpGet(t, vnetListByRGURL(srv.URL, "sub1", "rg-empty"))
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

func TestVNet_MissingAPIVersion_Returns400(t *testing.T) {
	srv := newTestServer(t)
	// Use the raw helper so withAPIVersion does not inject the default.
	resp := httpGetRaw(t, vnetURL(srv.URL, "sub1", "rg1", "vnet1"))
	assertStatus(t, resp, http.StatusBadRequest)
	body := decodeJSON(t, resp)
	errObj := body["error"].(map[string]interface{})
	if errObj["code"] != "MissingApiVersionParameter" {
		t.Errorf("code = %v, want MissingApiVersionParameter", errObj["code"])
	}
}

// TestVNet_PUT_NilTags_NormalisedToEmptyObject verifies that when a client
// sends no tags field, the response contains "tags": {} (not null), matching
// real Azure behaviour.
func TestVNet_PUT_NilTags_NormalisedToEmptyObject(t *testing.T) {
	srv := newTestServer(t)
	// First create the parent RG.
	httpPut(t, rgURL(srv.URL, "sub1", "rg1"), rgBodyMinimal)
	// vnetBodyMinimal has no "tags" key, so body.Tags will be nil.
	resp := httpPut(t, vnetURL(srv.URL, "sub1", "rg1", "vnet1"), vnetBodyMinimal)
	assertStatus(t, resp, http.StatusCreated)

	body := decodeJSON(t, resp)
	tags, ok := body["tags"]
	if !ok {
		t.Fatal("tags field missing from response")
	}
	tagsMap, ok := tags.(map[string]interface{})
	if !ok {
		t.Fatalf("tags is %T, want map (JSON object); got %v", tags, tags)
	}
	if len(tagsMap) != 0 {
		t.Errorf("tags should be empty object, got %v", tagsMap)
	}
}

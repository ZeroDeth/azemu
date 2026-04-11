package arm

import (
	"fmt"
	"net/http"
	"testing"
)

func subnetURL(srvURL, sub, rg, vnet, subnet string) string {
	return fmt.Sprintf("%s/subnets/%s", vnetURL(srvURL, sub, rg, vnet), subnet)
}

func subnetListURL(srvURL, sub, rg, vnet string) string {
	return fmt.Sprintf("%s/subnets", vnetURL(srvURL, sub, rg, vnet))
}

const subnetBodyMinimal = `{"properties":{"addressPrefix":"10.0.1.0/24"}}`

// createParentVNet is a fixture used by almost every subnet test. Subnet
// handlers require the parent to exist so we PUT one up-front.
func createParentVNet(t *testing.T, srvURL string) {
	t.Helper()
	resp := httpPut(t, vnetURL(srvURL, "sub1", "rg1", "vnet1"), vnetBodyMinimal)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()
}

func TestSubnet_PUT_ParentMissing_Returns404_ParentResourceNotFound(t *testing.T) {
	srv := newTestServer(t)
	// Deliberately skip the parent vnet.
	resp := httpPut(t, subnetURL(srv.URL, "sub1", "rg1", "ghost", "s1"), subnetBodyMinimal)
	assertStatus(t, resp, http.StatusNotFound)

	body := decodeJSON(t, resp)
	errObj := body["error"].(map[string]interface{})
	if errObj["code"] != "ParentResourceNotFound" {
		t.Errorf("code = %v, want ParentResourceNotFound", errObj["code"])
	}
}

func TestSubnet_PUT_Creates_Returns201(t *testing.T) {
	srv := newTestServer(t)
	createParentVNet(t, srv.URL)

	resp := httpPut(t, subnetURL(srv.URL, "sub1", "rg1", "vnet1", "s1"), subnetBodyMinimal)
	assertStatus(t, resp, http.StatusCreated)

	body := decodeJSON(t, resp)
	if body["name"] != "s1" {
		t.Errorf("name = %v, want s1", body["name"])
	}
	if body["type"] != subnetTypeString {
		t.Errorf("type = %v, want %s", body["type"], subnetTypeString)
	}

	wantID := "/subscriptions/sub1/resourceGroups/rg1/providers/Microsoft.Network/virtualNetworks/vnet1/subnets/s1"
	if body["id"] != wantID {
		t.Errorf("id = %v, want %s", body["id"], wantID)
	}

	props := body["properties"].(map[string]interface{})
	if props["addressPrefix"] != "10.0.1.0/24" {
		t.Errorf("addressPrefix = %v, want 10.0.1.0/24", props["addressPrefix"])
	}
	if props["provisioningState"] != "Succeeded" {
		t.Errorf("provisioningState = %v, want Succeeded", props["provisioningState"])
	}
}

func TestSubnet_PUT_Updates_Returns200(t *testing.T) {
	srv := newTestServer(t)
	createParentVNet(t, srv.URL)

	url := subnetURL(srv.URL, "sub1", "rg1", "vnet1", "s1")
	resp := httpPut(t, url, subnetBodyMinimal)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = httpPut(t, url, `{"properties":{"addressPrefix":"10.0.9.0/24"}}`)
	assertStatus(t, resp, http.StatusOK)
	body := decodeJSON(t, resp)
	props := body["properties"].(map[string]interface{})
	if props["addressPrefix"] != "10.0.9.0/24" {
		t.Errorf("addressPrefix = %v, want 10.0.9.0/24 (update should replace)", props["addressPrefix"])
	}
}

func TestSubnet_GET_Exists_Returns200(t *testing.T) {
	srv := newTestServer(t)
	createParentVNet(t, srv.URL)
	url := subnetURL(srv.URL, "sub1", "rg1", "vnet1", "s1")
	resp := httpPut(t, url, subnetBodyMinimal)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = httpGet(t, url)
	assertStatus(t, resp, http.StatusOK)
	body := decodeJSON(t, resp)
	if body["name"] != "s1" {
		t.Errorf("name = %v, want s1", body["name"])
	}
}

func TestSubnet_GET_NotFound_Returns404(t *testing.T) {
	srv := newTestServer(t)
	createParentVNet(t, srv.URL)
	resp := httpGet(t, subnetURL(srv.URL, "sub1", "rg1", "vnet1", "ghost"))
	assertStatus(t, resp, http.StatusNotFound)
	body := decodeJSON(t, resp)
	errObj := body["error"].(map[string]interface{})
	if errObj["code"] != "ResourceNotFound" {
		t.Errorf("code = %v, want ResourceNotFound", errObj["code"])
	}
}

func TestSubnet_HEAD_Exists_Returns204(t *testing.T) {
	srv := newTestServer(t)
	createParentVNet(t, srv.URL)
	url := subnetURL(srv.URL, "sub1", "rg1", "vnet1", "s1")
	resp := httpPut(t, url, subnetBodyMinimal)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = httpHead(t, url)
	assertStatus(t, resp, http.StatusNoContent)
}

// TestSubnet_HEAD_NotFound_Returns404_EmptyBody covers the !ok branch of
// headSubnet that was missing from the Phase 2 initial slice (TODO.md
// coverage gap: headSubnet 77.8%). HEAD semantics require an empty body
// even on 404.
func TestSubnet_HEAD_NotFound_Returns404_EmptyBody(t *testing.T) {
	srv := newTestServer(t)
	createParentVNet(t, srv.URL)
	resp := httpHead(t, subnetURL(srv.URL, "sub1", "rg1", "vnet1", "ghost"))
	assertStatus(t, resp, http.StatusNotFound)
	if body := readBody(t, resp); body != "" {
		t.Errorf("HEAD body = %q, want empty", body)
	}
}

func TestSubnet_DELETE_Returns202(t *testing.T) {
	srv := newTestServer(t)
	createParentVNet(t, srv.URL)
	url := subnetURL(srv.URL, "sub1", "rg1", "vnet1", "s1")
	resp := httpPut(t, url, subnetBodyMinimal)
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

// TestSubnet_DELETE_NotFound_Returns404 covers the store.Delete-returns-false
// branch of deleteSubnet that was missing from the Phase 2 initial slice
// (TODO.md coverage gap: deleteSubnet 81.8%).
func TestSubnet_DELETE_NotFound_Returns404(t *testing.T) {
	srv := newTestServer(t)
	createParentVNet(t, srv.URL)
	resp := httpDelete(t, subnetURL(srv.URL, "sub1", "rg1", "vnet1", "ghost"))
	assertStatus(t, resp, http.StatusNotFound)

	errBody := decodeJSON(t, resp)
	errObj, ok := errBody["error"].(map[string]interface{})
	if !ok {
		t.Fatalf("error object missing: %v", errBody)
	}
	if errObj["code"] != "ResourceNotFound" {
		t.Errorf("code = %v, want ResourceNotFound", errObj["code"])
	}
}

func TestSubnet_DELETE_DoesNotAffectParentVNet(t *testing.T) {
	srv := newTestServer(t)
	createParentVNet(t, srv.URL)
	subU := subnetURL(srv.URL, "sub1", "rg1", "vnet1", "s1")
	resp := httpPut(t, subU, subnetBodyMinimal)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = httpDelete(t, subU)
	assertStatus(t, resp, http.StatusAccepted)
	resp.Body.Close()

	// Parent vnet must still be reachable.
	resp = httpGet(t, vnetURL(srv.URL, "sub1", "rg1", "vnet1"))
	assertStatus(t, resp, http.StatusOK)
	body := decodeJSON(t, resp)
	props := body["properties"].(map[string]interface{})
	subnets := props["subnets"].([]interface{})
	if len(subnets) != 0 {
		t.Errorf("len(subnets) = %d, want 0 after child deletion", len(subnets))
	}
}

func TestSubnet_LIST_ReturnsValueWrapper(t *testing.T) {
	srv := newTestServer(t)
	createParentVNet(t, srv.URL)

	for _, name := range []string{"a", "b", "c"} {
		resp := httpPut(t, subnetURL(srv.URL, "sub1", "rg1", "vnet1", name), subnetBodyMinimal)
		assertStatus(t, resp, http.StatusCreated)
		resp.Body.Close()
	}

	resp := httpGet(t, subnetListURL(srv.URL, "sub1", "rg1", "vnet1"))
	assertStatus(t, resp, http.StatusOK)
	body := decodeJSON(t, resp)
	items, ok := body["value"].([]interface{})
	if !ok {
		t.Fatalf("value missing: %v", body)
	}
	if len(items) != 3 {
		t.Errorf("len(items) = %d, want 3", len(items))
	}
}

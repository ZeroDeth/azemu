package arm

import (
	"fmt"
	"net/http"
	"testing"
)

func rgResourcesURL(srvURL, sub, rg string) string {
	return fmt.Sprintf("%s/subscriptions/%s/resourcegroups/%s/resources",
		srvURL, sub, rg)
}

// TestRGResources_EmptyRG_ReturnsEmptyValueArray verifies the destroy-time
// safety check the azurerm provider performs before deleting a resource
// group. An empty RG must return {"value": []}, not 501 (which surfaces
// as a cryptic "polling status of Failed should be surfaced as a
// PollingFailedError" provider-side error).
func TestRGResources_EmptyRG_ReturnsEmptyValueArray(t *testing.T) {
	srv := newTestServer(t)
	// Create an RG with no children.
	resp := httpPut(t,
		srv.URL+"/subscriptions/sub1/resourcegroups/rg1",
		`{"location":"uksouth"}`)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = httpGet(t, rgResourcesURL(srv.URL, "sub1", "rg1"))
	assertStatus(t, resp, http.StatusOK)
	body := decodeJSON(t, resp)
	items, ok := body["value"].([]interface{})
	if !ok {
		t.Fatalf("value missing or wrong type: %v", body)
	}
	if len(items) != 0 {
		t.Errorf("len(items) = %d, want 0 (RG has no children)", len(items))
	}
}

// TestRGResources_WithChildren_ReturnsAll covers the populated case so a
// future contributor doesn't accidentally hard-code the empty path.
func TestRGResources_WithChildren_ReturnsAll(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t,
		srv.URL+"/subscriptions/sub1/resourcegroups/rg1",
		`{"location":"uksouth"}`)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	// Create two vnets inside the RG.
	for _, name := range []string{"a", "b"} {
		resp = httpPut(t, vnetURL(srv.URL, "sub1", "rg1", name), vnetBodyMinimal)
		assertStatus(t, resp, http.StatusCreated)
		resp.Body.Close()
	}

	resp = httpGet(t, rgResourcesURL(srv.URL, "sub1", "rg1"))
	assertStatus(t, resp, http.StatusOK)
	body := decodeJSON(t, resp)
	items := body["value"].([]interface{})
	if len(items) != 2 {
		t.Errorf("len(items) = %d, want 2", len(items))
	}
	for _, item := range items {
		m := item.(map[string]interface{})
		if m["type"] != "Microsoft.Network/virtualNetworks" {
			t.Errorf("unexpected child type: %v", m["type"])
		}
	}
}

// TestRGResources_ExcludesParentRG ensures the prefix-match doesn't
// accidentally include the resource group itself in its children list.
func TestRGResources_ExcludesParentRG(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t,
		srv.URL+"/subscriptions/sub1/resourcegroups/rg1",
		`{"location":"uksouth"}`)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = httpGet(t, rgResourcesURL(srv.URL, "sub1", "rg1"))
	assertStatus(t, resp, http.StatusOK)
	body := decodeJSON(t, resp)
	items := body["value"].([]interface{})
	for _, item := range items {
		m := item.(map[string]interface{})
		if m["type"] == "Microsoft.Resources/resourceGroups" {
			t.Errorf("parent RG leaked into children list: %v", m)
		}
	}
}

// TestRGResources_AcceptsAndIgnoresODataParams verifies that $expand,
// $top, and other OData query params are accepted (not 400'd) but ignored.
func TestRGResources_AcceptsAndIgnoresODataParams(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t,
		srv.URL+"/subscriptions/sub1/resourcegroups/rg1",
		`{"location":"uksouth"}`)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	url := rgResourcesURL(srv.URL, "sub1", "rg1") +
		"?%24expand=provisioningState&%24top=10&api-version=2023-07-01"
	resp = httpGet(t, url)
	assertStatus(t, resp, http.StatusOK)
}

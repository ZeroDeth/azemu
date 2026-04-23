package arm

import (
	"fmt"
	"net/http"
	"testing"
)

func cdnProfileURL(srvURL, sub, rg, name string) string {
	return fmt.Sprintf(
		"%s/subscriptions/%s/resourcegroups/%s/providers/microsoft.cdn/profiles/%s",
		srvURL, sub, rg, name,
	)
}

func cdnProfileListByRGURL(srvURL, sub, rg string) string {
	return fmt.Sprintf(
		"%s/subscriptions/%s/resourcegroups/%s/providers/microsoft.cdn/profiles",
		srvURL, sub, rg,
	)
}

func cdnProfileListBySubURL(srvURL, sub string) string {
	return fmt.Sprintf(
		"%s/subscriptions/%s/providers/microsoft.cdn/profiles",
		srvURL, sub,
	)
}

func cdnEndpointURL(srvURL, sub, rg, profile, endpoint string) string {
	return fmt.Sprintf(
		"%s/subscriptions/%s/resourcegroups/%s/providers/microsoft.cdn/profiles/%s/endpoints/%s",
		srvURL, sub, rg, profile, endpoint,
	)
}

func cdnEndpointListURL(srvURL, sub, rg, profile string) string {
	return fmt.Sprintf(
		"%s/subscriptions/%s/resourcegroups/%s/providers/microsoft.cdn/profiles/%s/endpoints",
		srvURL, sub, rg, profile,
	)
}

const cdnProfileBodyStandard = `{
  "location": "global",
  "sku": {"name": "Standard_Microsoft"},
  "properties": {}
}`

const cdnProfileBodyPremium = `{
  "location": "global",
  "sku": {"name": "Premium_Verizon"},
  "properties": {}
}`

const cdnEndpointBody = `{
  "location": "global",
  "properties": {
    "origins": [{"name": "origin1", "properties": {"hostName": "example.com"}}],
    "isHttpAllowed": true,
    "isHttpsAllowed": true
  }
}`

// --- CDN Profile tests ---

func TestCDNProfile_PUT_Creates_Returns201(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, cdnProfileURL(srv.URL, "sub1", "rg1", "myprofile"), cdnProfileBodyStandard)
	assertStatus(t, resp, http.StatusCreated)

	body := decodeJSON(t, resp)
	if body["name"] != "myprofile" {
		t.Errorf("name = %v, want myprofile", body["name"])
	}
	if body["type"] != "Microsoft.Cdn/profiles" {
		t.Errorf("type = %v, want Microsoft.Cdn/profiles", body["type"])
	}
}

func TestCDNProfile_PUT_Idempotent_Returns200(t *testing.T) {
	srv := newTestServer(t)
	httpPut(t, cdnProfileURL(srv.URL, "sub1", "rg1", "myprofile"), cdnProfileBodyStandard)
	resp := httpPut(t, cdnProfileURL(srv.URL, "sub1", "rg1", "myprofile"), cdnProfileBodyStandard)
	assertStatus(t, resp, http.StatusOK)
}

func TestCDNProfile_PUT_MissingLocation_Returns400(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, cdnProfileURL(srv.URL, "sub1", "rg1", "myprofile"),
		`{"sku":{"name":"Standard_Microsoft"},"properties":{}}`)
	assertStatus(t, resp, http.StatusBadRequest)

	body := decodeJSON(t, resp)
	errBlock := body["error"].(map[string]interface{})
	if errBlock["code"] != "InvalidRequestContent" {
		t.Errorf("error.code = %v, want InvalidRequestContent", errBlock["code"])
	}
}

func TestCDNProfile_PUT_ProvisioningStateSucceeded(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, cdnProfileURL(srv.URL, "sub1", "rg1", "myprofile"), cdnProfileBodyStandard)
	assertStatus(t, resp, http.StatusCreated)

	body := decodeJSON(t, resp)
	props := body["properties"].(map[string]interface{})
	if props["provisioningState"] != "Succeeded" {
		t.Errorf("provisioningState = %v, want Succeeded", props["provisioningState"])
	}
}

func TestCDNProfile_PUT_SKUPreserved(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, cdnProfileURL(srv.URL, "sub1", "rg1", "myprofile"), cdnProfileBodyPremium)
	assertStatus(t, resp, http.StatusCreated)

	body := decodeJSON(t, resp)
	sku, ok := body["sku"].(map[string]interface{})
	if !ok {
		t.Fatalf("sku missing or wrong type")
	}
	if sku["name"] != "Premium_Verizon" {
		t.Errorf("sku.name = %v, want Premium_Verizon", sku["name"])
	}
}

func TestCDNProfile_GET_ReturnsStored(t *testing.T) {
	srv := newTestServer(t)
	httpPut(t, cdnProfileURL(srv.URL, "sub1", "rg1", "myprofile"), cdnProfileBodyStandard)

	resp := httpGet(t, cdnProfileURL(srv.URL, "sub1", "rg1", "myprofile"))
	assertStatus(t, resp, http.StatusOK)

	body := decodeJSON(t, resp)
	if body["name"] != "myprofile" {
		t.Errorf("name = %v, want myprofile", body["name"])
	}
}

func TestCDNProfile_GET_NotFound_Returns404(t *testing.T) {
	srv := newTestServer(t)
	resp := httpGet(t, cdnProfileURL(srv.URL, "sub1", "rg1", "doesnotexist"))
	assertStatus(t, resp, http.StatusNotFound)

	body := decodeJSON(t, resp)
	errBlock := body["error"].(map[string]interface{})
	if errBlock["code"] != "ResourceNotFound" {
		t.Errorf("error.code = %v, want ResourceNotFound", errBlock["code"])
	}
}

func TestCDNProfile_HEAD_Exists_Returns204(t *testing.T) {
	srv := newTestServer(t)
	httpPut(t, cdnProfileURL(srv.URL, "sub1", "rg1", "myprofile"), cdnProfileBodyStandard)

	resp := httpHead(t, cdnProfileURL(srv.URL, "sub1", "rg1", "myprofile"))
	assertStatus(t, resp, http.StatusNoContent)
}

func TestCDNProfile_HEAD_NotFound_Returns404(t *testing.T) {
	srv := newTestServer(t)
	resp := httpHead(t, cdnProfileURL(srv.URL, "sub1", "rg1", "doesnotexist"))
	assertStatus(t, resp, http.StatusNotFound)
}

func TestCDNProfile_DELETE_Returns202(t *testing.T) {
	srv := newTestServer(t)
	httpPut(t, cdnProfileURL(srv.URL, "sub1", "rg1", "myprofile"), cdnProfileBodyStandard)

	resp := httpDelete(t, cdnProfileURL(srv.URL, "sub1", "rg1", "myprofile"))
	assertStatus(t, resp, http.StatusAccepted)

	if resp.Header.Get("Location") == "" {
		t.Error("Location header missing on 202 Accepted")
	}
}

func TestCDNProfile_DELETE_NotFound_Returns404(t *testing.T) {
	srv := newTestServer(t)
	resp := httpDelete(t, cdnProfileURL(srv.URL, "sub1", "rg1", "doesnotexist"))
	assertStatus(t, resp, http.StatusNotFound)
}

func TestCDNProfile_DELETE_Then_GET_Returns404(t *testing.T) {
	srv := newTestServer(t)
	httpPut(t, cdnProfileURL(srv.URL, "sub1", "rg1", "myprofile"), cdnProfileBodyStandard)
	httpDelete(t, cdnProfileURL(srv.URL, "sub1", "rg1", "myprofile"))

	resp := httpGet(t, cdnProfileURL(srv.URL, "sub1", "rg1", "myprofile"))
	assertStatus(t, resp, http.StatusNotFound)
}

func TestCDNProfile_LIST_ByRG_ReturnsValueWrapper(t *testing.T) {
	srv := newTestServer(t)
	httpPut(t, cdnProfileURL(srv.URL, "sub1", "rg1", "profile1"), cdnProfileBodyStandard)
	httpPut(t, cdnProfileURL(srv.URL, "sub1", "rg1", "profile2"), cdnProfileBodyPremium)

	resp := httpGet(t, cdnProfileListByRGURL(srv.URL, "sub1", "rg1"))
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

func TestCDNProfile_LIST_ByRG_Empty_ReturnsEmptyArray(t *testing.T) {
	srv := newTestServer(t)
	resp := httpGet(t, cdnProfileListByRGURL(srv.URL, "sub1", "rg1"))
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

func TestCDNProfile_LIST_BySub_ReturnsAcrossRGs(t *testing.T) {
	srv := newTestServer(t)
	httpPut(t, cdnProfileURL(srv.URL, "sub1", "rg1", "profile1"), cdnProfileBodyStandard)
	httpPut(t, cdnProfileURL(srv.URL, "sub1", "rg2", "profile2"), cdnProfileBodyPremium)

	resp := httpGet(t, cdnProfileListBySubURL(srv.URL, "sub1"))
	assertStatus(t, resp, http.StatusOK)

	body := decodeJSON(t, resp)
	items := body["value"].([]interface{})
	if len(items) != 2 {
		t.Errorf("len(value) = %d, want 2 (across two RGs)", len(items))
	}
}

func TestCDNProfile_PUT_TagsNormalisedToEmptyObject(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, cdnProfileURL(srv.URL, "sub1", "rg1", "myprofile"),
		`{"location":"global","sku":{"name":"Standard_Microsoft"},"properties":{}}`)
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

func TestCDNProfile_DELETE_CascadesEndpoints(t *testing.T) {
	srv := newTestServer(t)
	httpPut(t, cdnProfileURL(srv.URL, "sub1", "rg1", "myprofile"), cdnProfileBodyStandard)
	httpPut(t, cdnEndpointURL(srv.URL, "sub1", "rg1", "myprofile", "ep1"), cdnEndpointBody)

	httpDelete(t, cdnProfileURL(srv.URL, "sub1", "rg1", "myprofile"))

	// Endpoint should be gone after profile delete (store cascade).
	resp := httpGet(t, cdnEndpointURL(srv.URL, "sub1", "rg1", "myprofile", "ep1"))
	assertStatus(t, resp, http.StatusNotFound)
}

// --- CDN Endpoint tests ---

func TestCDNEndpoint_PUT_Creates_Returns201(t *testing.T) {
	srv := newTestServer(t)
	httpPut(t, cdnProfileURL(srv.URL, "sub1", "rg1", "myprofile"), cdnProfileBodyStandard)

	resp := httpPut(t, cdnEndpointURL(srv.URL, "sub1", "rg1", "myprofile", "myendpoint"), cdnEndpointBody)
	assertStatus(t, resp, http.StatusCreated)

	body := decodeJSON(t, resp)
	if body["name"] != "myendpoint" {
		t.Errorf("name = %v, want myendpoint", body["name"])
	}
	if body["type"] != "Microsoft.Cdn/profiles/endpoints" {
		t.Errorf("type = %v, want Microsoft.Cdn/profiles/endpoints", body["type"])
	}
}

func TestCDNEndpoint_PUT_Idempotent_Returns200(t *testing.T) {
	srv := newTestServer(t)
	httpPut(t, cdnProfileURL(srv.URL, "sub1", "rg1", "myprofile"), cdnProfileBodyStandard)
	httpPut(t, cdnEndpointURL(srv.URL, "sub1", "rg1", "myprofile", "myendpoint"), cdnEndpointBody)
	resp := httpPut(t, cdnEndpointURL(srv.URL, "sub1", "rg1", "myprofile", "myendpoint"), cdnEndpointBody)
	assertStatus(t, resp, http.StatusOK)
}

func TestCDNEndpoint_PUT_ParentMissing_Returns404(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, cdnEndpointURL(srv.URL, "sub1", "rg1", "nosuchprofile", "myendpoint"), cdnEndpointBody)
	assertStatus(t, resp, http.StatusNotFound)

	body := decodeJSON(t, resp)
	errBlock := body["error"].(map[string]interface{})
	if errBlock["code"] != "ParentResourceNotFound" {
		t.Errorf("error.code = %v, want ParentResourceNotFound", errBlock["code"])
	}
}

func TestCDNEndpoint_PUT_HostnameComputed(t *testing.T) {
	srv := newTestServer(t)
	httpPut(t, cdnProfileURL(srv.URL, "sub1", "rg1", "myprofile"), cdnProfileBodyStandard)
	resp := httpPut(t, cdnEndpointURL(srv.URL, "sub1", "rg1", "myprofile", "myendpoint"), cdnEndpointBody)
	assertStatus(t, resp, http.StatusCreated)

	body := decodeJSON(t, resp)
	props := body["properties"].(map[string]interface{})
	if props["hostName"] != "myendpoint.azureedge.net" {
		t.Errorf("hostName = %v, want myendpoint.azureedge.net", props["hostName"])
	}
}

func TestCDNEndpoint_GET_ReturnsStored(t *testing.T) {
	srv := newTestServer(t)
	httpPut(t, cdnProfileURL(srv.URL, "sub1", "rg1", "myprofile"), cdnProfileBodyStandard)
	httpPut(t, cdnEndpointURL(srv.URL, "sub1", "rg1", "myprofile", "myendpoint"), cdnEndpointBody)

	resp := httpGet(t, cdnEndpointURL(srv.URL, "sub1", "rg1", "myprofile", "myendpoint"))
	assertStatus(t, resp, http.StatusOK)

	body := decodeJSON(t, resp)
	if body["name"] != "myendpoint" {
		t.Errorf("name = %v, want myendpoint", body["name"])
	}
}

func TestCDNEndpoint_GET_NotFound_Returns404(t *testing.T) {
	srv := newTestServer(t)
	httpPut(t, cdnProfileURL(srv.URL, "sub1", "rg1", "myprofile"), cdnProfileBodyStandard)
	resp := httpGet(t, cdnEndpointURL(srv.URL, "sub1", "rg1", "myprofile", "doesnotexist"))
	assertStatus(t, resp, http.StatusNotFound)
}

func TestCDNEndpoint_HEAD_Exists_Returns204(t *testing.T) {
	srv := newTestServer(t)
	httpPut(t, cdnProfileURL(srv.URL, "sub1", "rg1", "myprofile"), cdnProfileBodyStandard)
	httpPut(t, cdnEndpointURL(srv.URL, "sub1", "rg1", "myprofile", "myendpoint"), cdnEndpointBody)

	resp := httpHead(t, cdnEndpointURL(srv.URL, "sub1", "rg1", "myprofile", "myendpoint"))
	assertStatus(t, resp, http.StatusNoContent)
}

func TestCDNEndpoint_DELETE_Returns202(t *testing.T) {
	srv := newTestServer(t)
	httpPut(t, cdnProfileURL(srv.URL, "sub1", "rg1", "myprofile"), cdnProfileBodyStandard)
	httpPut(t, cdnEndpointURL(srv.URL, "sub1", "rg1", "myprofile", "myendpoint"), cdnEndpointBody)

	resp := httpDelete(t, cdnEndpointURL(srv.URL, "sub1", "rg1", "myprofile", "myendpoint"))
	assertStatus(t, resp, http.StatusAccepted)

	if resp.Header.Get("Location") == "" {
		t.Error("Location header missing on 202 Accepted")
	}
}

func TestCDNEndpoint_LIST_ReturnsValueWrapper(t *testing.T) {
	srv := newTestServer(t)
	httpPut(t, cdnProfileURL(srv.URL, "sub1", "rg1", "myprofile"), cdnProfileBodyStandard)
	httpPut(t, cdnEndpointURL(srv.URL, "sub1", "rg1", "myprofile", "ep1"), cdnEndpointBody)
	httpPut(t, cdnEndpointURL(srv.URL, "sub1", "rg1", "myprofile", "ep2"), cdnEndpointBody)

	resp := httpGet(t, cdnEndpointListURL(srv.URL, "sub1", "rg1", "myprofile"))
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

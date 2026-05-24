package arm

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

const (
	testSubID  = "00000000-0000-0000-0000-000000000000"
	testRGName = "my-rg"
)

func uaiURL(srv *httptest.Server, name string) string {
	return fmt.Sprintf("%s/subscriptions/%s/resourcegroups/%s/providers/microsoft.managedidentity/userassignedidentities/%s",
		srv.URL, testSubID, testRGName, name)
}

func TestUAI_PUT_Creates_Returns201(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, uaiURL(srv, "my-identity"), `{"location":"uksouth"}`)
	assertStatus(t, resp, http.StatusCreated)
}

func TestUAI_PUT_Update_Returns200(t *testing.T) {
	srv := newTestServer(t)
	httpPut(t, uaiURL(srv, "my-identity"), `{"location":"uksouth"}`)
	resp := httpPut(t, uaiURL(srv, "my-identity"), `{"location":"uksouth"}`)
	assertStatus(t, resp, http.StatusOK)
}

func TestUAI_PUT_ResponseShape(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, uaiURL(srv, "my-identity"), `{"location":"uksouth","tags":{"env":"test"}}`)
	assertStatus(t, resp, http.StatusCreated)
	body := decodeJSON(t, resp)

	if body["id"] == nil {
		t.Error("id missing from response")
	}
	if body["name"] != "my-identity" {
		t.Errorf("name = %v, want my-identity", body["name"])
	}
	if body["type"] != "Microsoft.ManagedIdentity/userAssignedIdentities" {
		t.Errorf("type = %v", body["type"])
	}
	if body["location"] != "uksouth" {
		t.Errorf("location = %v, want uksouth", body["location"])
	}

	props, ok := body["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("properties missing or not a map")
	}
	if props["provisioningState"] != "Succeeded" {
		t.Errorf("provisioningState = %v", props["provisioningState"])
	}
	if props["principalId"] == nil || props["principalId"] == "" {
		t.Error("principalId missing")
	}
	if props["clientId"] == nil || props["clientId"] == "" {
		t.Error("clientId missing")
	}
}

func TestUAI_PUT_PrincipalIDStableAcrossPuts(t *testing.T) {
	srv := newTestServer(t)

	resp1 := httpPut(t, uaiURL(srv, "stable-identity"), `{"location":"uksouth"}`)
	body1 := decodeJSON(t, resp1)
	props1 := body1["properties"].(map[string]interface{})
	principalID1 := props1["principalId"]

	resp2 := httpPut(t, uaiURL(srv, "stable-identity"), `{"location":"uksouth"}`)
	body2 := decodeJSON(t, resp2)
	props2 := body2["properties"].(map[string]interface{})
	principalID2 := props2["principalId"]

	if principalID1 != principalID2 {
		t.Errorf("principalId changed across puts: %v != %v", principalID1, principalID2)
	}
}

func TestUAI_PUT_MissingLocation_Returns400(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, uaiURL(srv, "my-identity"), `{"tags":{"env":"test"}}`)
	assertStatus(t, resp, http.StatusBadRequest)
}

func TestUAI_GET_Returns200(t *testing.T) {
	srv := newTestServer(t)
	httpPut(t, uaiURL(srv, "my-identity"), `{"location":"uksouth"}`)
	resp := httpGet(t, uaiURL(srv, "my-identity"))
	assertStatus(t, resp, http.StatusOK)
}

func TestUAI_GET_NotFound_Returns404(t *testing.T) {
	srv := newTestServer(t)
	resp := httpGet(t, uaiURL(srv, "nonexistent"))
	assertStatus(t, resp, http.StatusNotFound)
}

func TestUAI_HEAD_Exists_Returns204(t *testing.T) {
	srv := newTestServer(t)
	httpPut(t, uaiURL(srv, "my-identity"), `{"location":"uksouth"}`)
	resp := httpHead(t, uaiURL(srv, "my-identity"))
	assertStatus(t, resp, http.StatusNoContent)
	body := readBody(t, resp)
	if body != "" {
		t.Errorf("HEAD body should be empty, got %q", body)
	}
}

func TestUAI_HEAD_NotFound_Returns404(t *testing.T) {
	srv := newTestServer(t)
	resp := httpHead(t, uaiURL(srv, "nonexistent"))
	assertStatus(t, resp, http.StatusNotFound)
}

func TestUAI_DELETE_Returns200(t *testing.T) {
	srv := newTestServer(t)
	httpPut(t, uaiURL(srv, "my-identity"), `{"location":"uksouth"}`)
	resp := httpDelete(t, uaiURL(srv, "my-identity"))
	// azurerm's userassignedidentities.Client#Delete expects 200 OK or 204.
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()
}

func TestUAI_DELETE_NotFound_Returns404(t *testing.T) {
	srv := newTestServer(t)
	resp := httpDelete(t, uaiURL(srv, "nonexistent"))
	assertStatus(t, resp, http.StatusNotFound)
}

func TestUAI_DELETE_ThenGet_Returns404(t *testing.T) {
	srv := newTestServer(t)
	httpPut(t, uaiURL(srv, "my-identity"), `{"location":"uksouth"}`)
	httpDelete(t, uaiURL(srv, "my-identity"))
	resp := httpGet(t, uaiURL(srv, "my-identity"))
	assertStatus(t, resp, http.StatusNotFound)
}

func TestUAI_ListByRG(t *testing.T) {
	srv := newTestServer(t)
	httpPut(t, uaiURL(srv, "identity-a"), `{"location":"uksouth"}`)
	httpPut(t, uaiURL(srv, "identity-b"), `{"location":"uksouth"}`)

	listURL := fmt.Sprintf("%s/subscriptions/%s/resourcegroups/%s/providers/microsoft.managedidentity/userassignedidentities",
		srv.URL, testSubID, testRGName)
	resp := httpGet(t, listURL)
	assertStatus(t, resp, http.StatusOK)
	body := decodeJSON(t, resp)
	items, ok := body["value"].([]interface{})
	if !ok {
		t.Fatal("value field missing or not an array")
	}
	if len(items) < 2 {
		t.Errorf("expected at least 2 items, got %d", len(items))
	}
}

func TestUAI_ListBySub(t *testing.T) {
	srv := newTestServer(t)
	httpPut(t, uaiURL(srv, "identity-a"), `{"location":"uksouth"}`)

	listURL := fmt.Sprintf("%s/subscriptions/%s/providers/microsoft.managedidentity/userassignedidentities",
		srv.URL, testSubID)
	resp := httpGet(t, listURL)
	assertStatus(t, resp, http.StatusOK)
	body := decodeJSON(t, resp)
	if body["value"] == nil {
		t.Error("value field missing")
	}
}

func TestUAI_DifferentIdentities_HaveDifferentPrincipalIDs(t *testing.T) {
	srv := newTestServer(t)

	resp1 := httpPut(t, uaiURL(srv, "identity-one"), `{"location":"uksouth"}`)
	body1 := decodeJSON(t, resp1)
	props1 := body1["properties"].(map[string]interface{})

	resp2 := httpPut(t, uaiURL(srv, "identity-two"), `{"location":"uksouth"}`)
	body2 := decodeJSON(t, resp2)
	props2 := body2["properties"].(map[string]interface{})

	if props1["principalId"] == props2["principalId"] {
		t.Error("different identities should have different principalIds")
	}
}

func TestUAI_MissingAPIVersion_Returns400(t *testing.T) {
	srv := newTestServer(t)
	resp, err := http.Get(uaiURL(srv, "my-identity"))
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	assertStatus(t, resp, http.StatusBadRequest)
}

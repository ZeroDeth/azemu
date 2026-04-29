package arm

import (
	"fmt"
	"net/http"
	"testing"
)

func ficURL(srvURL, identityName, credentialName string) string {
	return fmt.Sprintf("%s/subscriptions/%s/resourcegroups/%s/providers/microsoft.managedidentity/userassignedidentities/%s/federatedidentitycredentials/%s",
		srvURL, testSubID, testRGName, identityName, credentialName)
}

func ficListURL(srvURL, identityName string) string {
	return fmt.Sprintf("%s/subscriptions/%s/resourcegroups/%s/providers/microsoft.managedidentity/userassignedidentities/%s/federatedidentitycredentials",
		srvURL, testSubID, testRGName, identityName)
}

func createTestIdentity(t *testing.T, srvURL, identityName string) {
	t.Helper()
	resp := httpPut(t, fmt.Sprintf("%s/subscriptions/%s/resourcegroups/%s/providers/microsoft.managedidentity/userassignedidentities/%s",
		srvURL, testSubID, testRGName, identityName), `{"location":"uksouth"}`)
	assertStatus(t, resp, http.StatusCreated)
}

const ficBody = `{"properties":{"issuer":"https://issuer.test/","subject":"system:serviceaccount:default:app","audiences":["api://AzureADTokenExchange"]}}`

func TestFIC_PUT_Creates_Returns201(t *testing.T) {
	srv := newTestServer(t)
	createTestIdentity(t, srv.URL, "my-identity")

	resp := httpPut(t, ficURL(srv.URL, "my-identity", "fic-one"), ficBody)
	assertStatus(t, resp, http.StatusCreated)

	body := decodeJSON(t, resp)
	if body["name"] != "fic-one" {
		t.Errorf("name = %v, want fic-one", body["name"])
	}
	if body["type"] != federatedIdentityCredentialTypeString {
		t.Errorf("type = %v, want %s", body["type"], federatedIdentityCredentialTypeString)
	}
	props := body["properties"].(map[string]interface{})
	if props["issuer"] != "https://issuer.test/" {
		t.Errorf("issuer = %v", props["issuer"])
	}
}

func TestFIC_PUT_Update_Returns200(t *testing.T) {
	srv := newTestServer(t)
	createTestIdentity(t, srv.URL, "my-identity")
	httpPut(t, ficURL(srv.URL, "my-identity", "fic-one"), ficBody)

	updated := `{"properties":{"issuer":"https://issuer.test/","subject":"system:serviceaccount:default:other","audiences":["api://AzureADTokenExchange"]}}`
	resp := httpPut(t, ficURL(srv.URL, "my-identity", "fic-one"), updated)
	assertStatus(t, resp, http.StatusOK)

	body := decodeJSON(t, resp)
	props := body["properties"].(map[string]interface{})
	if props["subject"] != "system:serviceaccount:default:other" {
		t.Errorf("subject = %v", props["subject"])
	}
}

func TestFIC_PUT_MissingParent_Returns404(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, ficURL(srv.URL, "missing-identity", "fic-one"), ficBody)
	assertStatus(t, resp, http.StatusNotFound)
}

func TestFIC_PUT_MissingRequiredProperties_Returns400(t *testing.T) {
	srv := newTestServer(t)
	createTestIdentity(t, srv.URL, "my-identity")

	resp := httpPut(t, ficURL(srv.URL, "my-identity", "fic-one"), `{"properties":{"issuer":"https://issuer.test/"}}`)
	assertStatus(t, resp, http.StatusBadRequest)
}

func TestFIC_GET_Returns200(t *testing.T) {
	srv := newTestServer(t)
	createTestIdentity(t, srv.URL, "my-identity")
	httpPut(t, ficURL(srv.URL, "my-identity", "fic-one"), ficBody)

	resp := httpGet(t, ficURL(srv.URL, "my-identity", "fic-one"))
	assertStatus(t, resp, http.StatusOK)
}

func TestFIC_HEAD_Exists_Returns204(t *testing.T) {
	srv := newTestServer(t)
	createTestIdentity(t, srv.URL, "my-identity")
	httpPut(t, ficURL(srv.URL, "my-identity", "fic-one"), ficBody)

	resp := httpHead(t, ficURL(srv.URL, "my-identity", "fic-one"))
	assertStatus(t, resp, http.StatusNoContent)
	if body := readBody(t, resp); body != "" {
		t.Errorf("HEAD body should be empty, got %q", body)
	}
}

func TestFIC_GET_NotFound_Returns404(t *testing.T) {
	srv := newTestServer(t)
	resp := httpGet(t, ficURL(srv.URL, "my-identity", "missing"))
	assertStatus(t, resp, http.StatusNotFound)
}

func TestFIC_LIST_ReturnsChildren(t *testing.T) {
	srv := newTestServer(t)
	createTestIdentity(t, srv.URL, "my-identity")
	httpPut(t, ficURL(srv.URL, "my-identity", "fic-one"), ficBody)
	httpPut(t, ficURL(srv.URL, "my-identity", "fic-two"), ficBody)

	resp := httpGet(t, ficListURL(srv.URL, "my-identity"))
	assertStatus(t, resp, http.StatusOK)
	body := decodeJSON(t, resp)
	items := body["value"].([]interface{})
	if len(items) != 2 {
		t.Errorf("len(value) = %d, want 2", len(items))
	}
}

func TestFIC_DELETE_Returns200Then204(t *testing.T) {
	srv := newTestServer(t)
	createTestIdentity(t, srv.URL, "my-identity")
	httpPut(t, ficURL(srv.URL, "my-identity", "fic-one"), ficBody)

	resp := httpDelete(t, ficURL(srv.URL, "my-identity", "fic-one"))
	assertStatus(t, resp, http.StatusOK)
	resp = httpDelete(t, ficURL(srv.URL, "my-identity", "fic-one"))
	assertStatus(t, resp, http.StatusNoContent)
}

package arm

import (
	"net/http"
	"strings"
	"testing"
)

const apimBasicBody = `{"location":"Australia East","sku":{"name":"Developer","capacity":1},"properties":{"publisherName":"Example","publisherEmail":"example@example.test","customProperties":{"Microsoft.WindowsAzure.ApiManagement.Gateway.Protocols.Server.Http2":"true"}}}`

func TestAPIM_BasicLifecycle(t *testing.T) {
	srv := newTestServer(t)
	base := srv.URL + "/subscriptions/sub1/resourceGroups/rg1/providers/Microsoft.ApiManagement/service/example-apim"
	rg := srv.URL + "/subscriptions/sub1/resourceGroups/rg1"
	deleted := srv.URL + "/subscriptions/sub1/providers/Microsoft.ApiManagement/locations/australiaeast/deletedservices/example-apim"
	resp := httpPut(t, base, apimBasicBody)
	assertStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
	resp = httpPut(t, rg, `{"location":"australiaeast"}`)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()
	for _, status := range []int{http.StatusCreated, http.StatusOK} {
		resp = httpPut(t, base+"?api-version=2024-05-01", apimBasicBody)
		assertStatus(t, resp, status)
		body := decodeJSON(t, resp)
		if body["location"] != "australiaeast" || body["type"] != apimServiceType {
			t.Fatalf("unexpected service: %v", body)
		}
		if resp.Header.Get("x-ms-request-id") == "" {
			t.Fatal("missing Azure request ID")
		}
	}
	resp = httpHead(t, base)
	assertStatus(t, resp, http.StatusNoContent)
	resp.Body.Close()
	resp = httpGet(t, strings.TrimSuffix(base, "/example-apim"))
	assertStatus(t, resp, http.StatusOK)
	if len(decodeJSON(t, resp)["value"].([]any)) != 1 {
		t.Fatal("service missing from collection")
	}
	resp = httpPatch(t, base, `{"tags":{"updated":"yes"}}`)
	assertStatus(t, resp, http.StatusOK)
	body := decodeJSON(t, resp)
	if body["tags"].(map[string]any)["updated"] != "yes" {
		t.Fatal("PATCH did not update tags")
	}
	if body["properties"].(map[string]any)["publisherName"] != "Example" {
		t.Fatal("PATCH lost properties")
	}
	resp = httpDelete(t, base)
	assertStatus(t, resp, http.StatusAccepted)
	pollURL := resp.Header.Get("Azure-AsyncOperation")
	resp.Body.Close()
	resp = httpGet(t, pollURL)
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()
	resp = httpGet(t, base)
	assertStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
	resp = httpGet(t, deleted)
	assertStatus(t, resp, http.StatusOK)
	if decodeJSON(t, resp)["properties"].(map[string]any)["serviceId"] == nil {
		t.Fatal("deleted service missing original ID")
	}
	resp = httpPut(t, base, apimBasicBody)
	assertStatus(t, resp, http.StatusConflict)
	resp.Body.Close()
	resp = httpDelete(t, deleted)
	assertStatus(t, resp, http.StatusNoContent)
	resp.Body.Close()
	resp = httpGet(t, deleted)
	assertStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
	resp = httpPut(t, base, apimBasicBody)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()
}

func TestAPIM_SettingsAndMissingVersion(t *testing.T) {
	srv := newTestServer(t)
	rg := srv.URL + "/subscriptions/sub1/resourceGroups/rg1"
	base := rg + "/providers/Microsoft.ApiManagement/service/apim"
	resp := httpPut(t, rg, `{"location":"uksouth"}`)
	resp.Body.Close()
	resp = httpPut(t, base, apimBasicBody)
	resp.Body.Close()
	for _, path := range []string{"/portalsettings/signin", "/portalsettings/signup", "/portalsettings/delegation"} {
		resp = httpGet(t, base+path+"?api-version=2022-08-01")
		assertStatus(t, resp, http.StatusOK)
		resp.Body.Close()
	}
	resp = httpPut(t, base+"/portalsettings/signin", `{"properties":{"enabled":true}}`)
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()
	resp = httpGet(t, base+"/portalsettings/signin")
	if decodeJSON(t, resp)["properties"].(map[string]any)["enabled"] != true {
		t.Fatal("setting not persisted")
	}
	resp = httpPut(t, base+"/portalsettings/signin", `{"properties":{"enabled":"invalid"}}`)
	assertStatus(t, resp, http.StatusBadRequest)
	resp.Body.Close()
	resp = httpGet(t, base+"/apis")
	if len(decodeJSON(t, resp)["value"].([]any)) != 0 {
		t.Fatal("unexpected sample APIs")
	}
	resp = httpGet(t, base+"/policies/policy")
	assertStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
	resp, err := http.Get(base)
	if err != nil {
		t.Fatal(err)
	}
	assertStatus(t, resp, http.StatusBadRequest)
	resp.Body.Close()
	resp = httpDelete(t, rg)
	resp.Body.Close()
	resp = httpGet(t, base+"/portalsettings/signin")
	assertStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
}

func TestAPIM_InvalidBody(t *testing.T) {
	srv := newTestServer(t)
	rg := srv.URL + "/subscriptions/sub1/resourcegroups/rg"
	resp := httpPut(t, rg, `{"location":"uksouth"}`)
	resp.Body.Close()
	for _, body := range []string{`{`, `{}`, `{"location":"uksouth","sku":{"name":"Premium","capacity":1},"properties":{"publisherName":"a","publisherEmail":"a@b.test"}}`} {
		resp := httpPut(t, rg+"/providers/microsoft.apimanagement/service/apim", body)
		assertStatus(t, resp, http.StatusBadRequest)
		resp.Body.Close()
	}
}

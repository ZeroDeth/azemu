package arm

import (
	"net/http"
	"strings"
	"testing"
)

const apimBackendBody = `{"properties":{"protocol":"http","url":"https://backend","title":"CAF104","tls":{"validateCertificateChain":true,"validateCertificateName":false}}}`
const apimBackendVersion = "?api-version=2024-05-01"

func TestAPIMBackend_Lifecycle(t *testing.T) {
	base, rg := newAPIMAPIParent(t)
	endpoint := base + "/backends/backend1" + apimBackendVersion
	for _, status := range []int{201, 200} {
		resp := httpPut(t, endpoint, apimBackendBody)
		assertStatus(t, resp, status)
		body := decodeJSON(t, resp)
		if body["type"] != apimBackendType || body["name"] != "backend1" {
			t.Fatalf("unexpected backend: %v", body)
		}
		for _, header := range []string{"x-ms-request-id", "x-ms-correlation-request-id"} {
			if resp.Header.Get(header) == "" {
				t.Fatalf("missing %s", header)
			}
		}
	}
	resp := httpPatch(t, endpoint, `{"properties":{"url":"https://changed.example.test"}}`)
	assertStatus(t, resp, 200)
	resp.Body.Close()
	resp = httpGet(t, endpoint)
	props := decodeJSON(t, resp)["properties"].(map[string]any)
	if props["url"] != "https://changed.example.test" || props["protocol"] != "http" {
		t.Fatal("PATCH did not preserve backend settings")
	}
	if props["tls"].(map[string]any)["validateCertificateName"] != false {
		t.Fatal("TLS settings were lost")
	}
	resp = httpHead(t, endpoint)
	assertStatus(t, resp, 204)
	if readBody(t, resp) != "" {
		t.Fatal("HEAD returned a body")
	}
	resp = httpGet(t, base+"/backends"+apimBackendVersion)
	if len(decodeJSON(t, resp)["value"].([]any)) != 1 {
		t.Fatal("backend missing from list")
	}
	resp = httpDelete(t, endpoint)
	assertStatus(t, resp, 204)
	resp.Body.Close()
	resp = httpGet(t, endpoint)
	assertStatus(t, resp, 404)
	resp.Body.Close()
	resp = httpDelete(t, endpoint)
	assertStatus(t, resp, 204)
	resp.Body.Close()
	resp = httpPut(t, endpoint, apimBackendBody)
	resp.Body.Close()
	resp = httpDelete(t, rg)
	resp.Body.Close()
	resp = httpGet(t, endpoint)
	assertStatus(t, resp, 404)
	resp.Body.Close()
}

func TestAPIMBackend_Validation(t *testing.T) {
	base, _ := newAPIMAPIParent(t)
	for _, tc := range []struct {
		name, body string
		status     int
	}{
		{"malformed", `{`, 400},
		{"missing properties", `{}`, 400},
		{"missing protocol", `{"properties":{"url":"https://backend"}}`, 400},
		{"relative URL", `{"properties":{"protocol":"http","url":"/backend"}}`, 400},
		{"non HTTP URL", `{"properties":{"protocol":"http","url":"file:///tmp/backend"}}`, 400},
		{"invalid TLS", `{"properties":{"protocol":"http","url":"https://backend","tls":{"validateCertificateChain":"yes"}}}`, 400},
		{"unsupported credentials", `{"properties":{"protocol":"http","url":"https://backend","credentials":{"header":{"key":["value"]}}}}`, 501},
		{"unsupported protocol", strings.Replace(apimBackendBody, `"http"`, `"soap"`, 1), 501},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp := httpPut(t, base+"/backends/b"+apimBackendVersion, tc.body)
			assertStatus(t, resp, tc.status)
			if decodeJSON(t, resp)["error"] == nil {
				t.Fatal("missing Azure error")
			}
		})
	}
	resp := httpGetRaw(t, base+"/backends/b")
	assertStatus(t, resp, 400)
	resp.Body.Close()
	srv := newTestServer(t)
	resp = httpPut(t, srv.URL+"/subscriptions/sub1/resourceGroups/rg/providers/Microsoft.ApiManagement/service/missing/backends/b"+apimBackendVersion, apimBackendBody)
	assertStatus(t, resp, http.StatusNotFound)
	if decodeJSON(t, resp)["error"].(map[string]any)["code"] != "ParentResourceNotFound" {
		t.Fatal("missing parent accepted")
	}
}

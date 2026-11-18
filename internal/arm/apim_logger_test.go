package arm

import (
	"net/http"
	"strings"
	"testing"
)

const apimLoggerBody = `{"properties":{"loggerType":"applicationInsights","description":"CAF102","isBuffered":true,"credentials":{"instrumentationKey":"local-test-key","connectionString":"InstrumentationKey=local-test-key"}}}`

func TestAPIMLogger_Lifecycle(t *testing.T) {
	base, _ := newAPIMAPIParent(t)
	url := base + "/loggers/logger1"
	for _, status := range []int{201, 200} {
		resp := httpPut(t, url+apimAPIVersion, apimLoggerBody)
		assertStatus(t, resp, status)
		if resp.Header.Get("x-ms-request-id") == "" || resp.Header.Get("x-ms-correlation-request-id") == "" {
			t.Fatal("missing Azure headers")
		}
		body := decodeJSON(t, resp)
		if body["name"] != "logger1" || body["type"] != apimLoggerType || body["properties"].(map[string]any)["isBuffered"] != true {
			t.Fatalf("invalid logger: %v", body)
		}
	}
	resp := httpPatch(t, url+apimAPIVersion, `{"properties":{"description":"updated","isBuffered":false}}`)
	assertStatus(t, resp, 200)
	resp.Body.Close()
	resp = httpGet(t, url+apimAPIVersion)
	props := decodeJSON(t, resp)["properties"].(map[string]any)
	if props["description"] != "updated" || props["isBuffered"] != false || props["credentials"] == nil {
		t.Fatal("PATCH did not merge logger")
	}
	resp = httpHead(t, url+apimAPIVersion)
	assertStatus(t, resp, 204)
	if readBody(t, resp) != "" {
		t.Fatal("HEAD body")
	}
	resp = httpGet(t, base+"/loggers"+apimAPIVersion)
	if len(decodeJSON(t, resp)["value"].([]any)) != 1 {
		t.Fatal("missing logger in list")
	}
	resp = httpDelete(t, url+apimAPIVersion)
	assertStatus(t, resp, 204)
	resp.Body.Close()
	resp = httpGet(t, url+apimAPIVersion)
	assertStatus(t, resp, 404)
	resp.Body.Close()
	resp = httpHead(t, url+apimAPIVersion)
	assertStatus(t, resp, 404)
	if readBody(t, resp) != "" {
		t.Fatal("missing HEAD body")
	}
	resp = httpDelete(t, url+apimAPIVersion)
	assertStatus(t, resp, 204)
	resp.Body.Close()
}

func TestAPIMLogger_Validation(t *testing.T) {
	base, _ := newAPIMAPIParent(t)
	url := base + "/loggers/logger1"
	for _, tc := range []struct {
		name, body string
		status     int
	}{
		{"malformed", `{`, 400},
		{"missing properties", `{}`, 400},
		{"credentials", strings.Replace(apimLoggerBody, `"credentials":{`, `"invalid":{`, 1), 501},
		{"missing credentials", `{"properties":{"loggerType":"applicationInsights"}}`, 400},
		{"buffer type", strings.Replace(apimLoggerBody, `"isBuffered":true`, `"isBuffered":"yes"`, 1), 400},
		{"eventhub", strings.Replace(apimLoggerBody, `"applicationInsights"`, `"azureEventHub"`, 1), 501},
		{"identity", `{"properties":{"loggerType":"applicationInsights","credentials":{"identityClientId":"unsupported"}}}`, 501},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp := httpPut(t, url+apimAPIVersion, tc.body)
			assertStatus(t, resp, tc.status)
			if decodeJSON(t, resp)["error"] == nil {
				t.Fatal("missing Azure error")
			}
		})
	}
	resp := httpGetRaw(t, url)
	assertStatus(t, resp, 400)
	if decodeJSON(t, resp)["error"].(map[string]any)["code"] != "MissingApiVersionParameter" {
		t.Fatal("missing API version accepted")
	}
	srv := newTestServer(t)
	resp = httpPut(t, srv.URL+"/subscriptions/sub1/resourceGroups/rg/providers/Microsoft.ApiManagement/service/missing/loggers/logger"+apimAPIVersion, apimLoggerBody)
	assertStatus(t, resp, http.StatusNotFound)
	if decodeJSON(t, resp)["error"].(map[string]any)["code"] != "ParentResourceNotFound" {
		t.Fatal("missing parent accepted")
	}
}

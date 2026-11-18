package arm

import (
	"encoding/json"
	"strings"
	"testing"
)

func newAPIMDiagnosticParent(t *testing.T) (string, string, string, string) {
	t.Helper()
	base, rg := newAPIMAPIParent(t)
	api := base + "/apis/example;rev=1"
	resp := httpPut(t, api+apimAPIVersion, apimAPIBody)
	assertStatus(t, resp, 201)
	resp.Body.Close()
	logger := base + "/loggers/logger1"
	resp = httpPut(t, logger+apimAPIVersion, apimLoggerBody)
	assertStatus(t, resp, 201)
	resp.Body.Close()
	// Strip the httptest origin: loggerId is an ARM ID, not an absolute URL.
	id := logger[strings.Index(logger, "/subscriptions/"):]
	body := `{"properties":{"loggerId":"` + id + `","sampling":{"samplingType":"fixed","percentage":5},"alwaysLog":"allErrors","logClientIp":true,"verbosity":"verbose","httpCorrelationProtocol":"W3C","operationNameFormat":"Name","frontend":{"request":{"body":{"bytes":32},"headers":["content-type","accept","origin"]},"response":{"body":{"bytes":32},"headers":["content-type","content-length","origin"]}},"backend":{"request":{"body":{"bytes":32},"headers":["content-type","accept","origin"]},"response":{"body":{"bytes":32},"headers":["content-type","content-length","origin"]}}}}`
	return base, rg, logger, body
}

func TestAPIMAPIDiagnostic_Lifecycle(t *testing.T) {
	base, _, logger, body := newAPIMDiagnosticParent(t)
	api := base + "/apis/example;rev=1"
	url := api + "/diagnostics/applicationinsights"
	for _, status := range []int{201, 200} {
		resp := httpPut(t, url+apimAPIVersion, body)
		assertStatus(t, resp, status)
		if resp.Header.Get("x-ms-request-id") == "" || resp.Header.Get("x-ms-correlation-request-id") == "" {
			t.Fatal("missing Azure headers")
		}
		result := decodeJSON(t, resp)
		if result["type"] != apimAPIDiagnosticType || result["name"] != "applicationinsights" {
			t.Fatalf("invalid diagnostic shape %v", result)
		}
		var expected map[string]any
		if err := json.Unmarshal([]byte(body), &expected); err != nil {
			t.Fatal(err)
		}
		want, _ := json.Marshal(expected["properties"])
		got, _ := json.Marshal(result["properties"])
		if string(got) != string(want) {
			t.Fatalf("configured fields lost: %s != %s", got, want)
		}
	}
	for _, path := range []string{url, base + "/apis/example/diagnostics/applicationinsights"} {
		resp := httpGet(t, path+apimAPIVersion)
		assertStatus(t, resp, 200)
		resp.Body.Close()
		resp = httpHead(t, path+apimAPIVersion)
		assertStatus(t, resp, 204)
		if readBody(t, resp) != "" {
			t.Fatal("HEAD body")
		}
	}
	resp := httpGet(t, api+"/diagnostics"+apimAPIVersion)
	if len(decodeJSON(t, resp)["value"].([]any)) != 1 {
		t.Fatal("missing diagnostic")
	}
	resp = httpPut(t, url+apimAPIVersion, strings.Replace(body, `"percentage":5`, `"percentage":10`, 1))
	assertStatus(t, resp, 200)
	resp.Body.Close()
	resp = httpGet(t, url+apimAPIVersion)
	if decodeJSON(t, resp)["properties"].(map[string]any)["sampling"].(map[string]any)["percentage"] != float64(10) {
		t.Fatal("sampling update lost")
	}
	resp = httpDelete(t, logger+apimAPIVersion)
	assertStatus(t, resp, 409)
	resp.Body.Close()
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
	// PUT replacement clears omitted optional logging settings.
	id := logger[strings.Index(logger, "/subscriptions/"):]
	resp = httpPut(t, url+apimAPIVersion, `{"properties":{"loggerId":"`+id+`"}}`)
	assertStatus(t, resp, 201)
	props := decodeJSON(t, resp)["properties"].(map[string]any)
	if props["frontend"] != nil || props["sampling"] != nil || props["alwaysLog"] != nil {
		t.Fatal("omitted optional properties retained")
	}
}

func TestAPIMAPIDiagnostic_Validation(t *testing.T) {
	base, _, _, body := newAPIMDiagnosticParent(t)
	url := base + "/apis/example/diagnostics/applicationinsights"
	for _, tc := range []struct {
		name, body string
		status     int
	}{
		{"malformed", `{`, 400},
		{"missing properties", `{}`, 400},
		{"logger", `{"properties":{"loggerId":"/missing"}}`, 400},
		{"sample", strings.Replace(body, `"percentage":5`, `"percentage":101`, 1), 400},
		{"body bytes", strings.Replace(body, `"bytes":32`, `"bytes":8193`, 1), 400},
		{"fractional bytes", strings.Replace(body, `"bytes":32`, `"bytes":1.5`, 1), 400},
		{"headers", strings.Replace(body, `"accept"`, `false`, 1), 400},
		{"correlation", strings.Replace(body, `"W3C"`, `"invalid"`, 1), 400},
		{"verbosity", strings.Replace(body, `"verbose"`, `"invalid"`, 1), 400},
		{"boolean", strings.Replace(body, `"logClientIp":true`, `"logClientIp":"yes"`, 1), 400},
		{"unknown", strings.Replace(body, `"verbosity":"verbose"`, `"unsupported":true`, 1), 501},
		{"masking", strings.Replace(body, `"body":{"bytes":32}`, `"dataMasking":{}`, 1), 400},
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
		t.Fatal("API version not enforced")
	}
	resp = httpPut(t, strings.Replace(url, "/apis/example/", "/apis/missing/", 1)+apimAPIVersion, body)
	assertStatus(t, resp, 404)
	if decodeJSON(t, resp)["error"].(map[string]any)["code"] != "ParentResourceNotFound" {
		t.Fatal("missing API parent accepted")
	}
}

func TestAPIMAPIDiagnostic_Cascade(t *testing.T) {
	for _, scope := range []string{"api", "service", "rg"} {
		t.Run(scope, func(t *testing.T) {
			base, rg, logger, body := newAPIMDiagnosticParent(t)
			api := base + "/apis/example;rev=1"
			url := api + "/diagnostics/applicationinsights"
			resp := httpPut(t, url+apimAPIVersion, body)
			assertStatus(t, resp, 201)
			resp.Body.Close()
			parent := map[string]string{"api": api, "service": base, "rg": rg}[scope]
			resp = httpDelete(t, parent+apimAPIVersion)
			resp.Body.Close()
			resp = httpGet(t, url+apimAPIVersion)
			assertStatus(t, resp, 404)
			resp.Body.Close()
			if scope != "api" {
				resp = httpGet(t, logger+apimAPIVersion)
				assertStatus(t, resp, 404)
				resp.Body.Close()
			}
		})
	}
}

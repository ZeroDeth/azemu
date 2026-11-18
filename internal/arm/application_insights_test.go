package arm

import (
	"strings"
	"testing"
)

const insightsVersion = "?api-version=2020-02-02"
const insightsBody = `{"location":"UK South","kind":"web","tags":{"test":"102"},"properties":{"Application_Type":"web","SamplingPercentage":100,"RetentionInDays":90}}`

func TestApplicationInsights_Lifecycle(t *testing.T) {
	srv := newTestServer(t)
	rg := srv.URL + "/subscriptions/sub1/resourceGroups/rg1"
	base := rg + "/providers/Microsoft.Insights/components"
	url := base + "/ai1"
	resp := httpPut(t, url+insightsVersion, insightsBody)
	assertStatus(t, resp, 404)
	if decodeJSON(t, resp)["error"].(map[string]any)["code"] != "ParentResourceNotFound" {
		t.Fatal("expected missing parent")
	}
	resp = httpPut(t, rg, `{"location":"uksouth"}`)
	resp.Body.Close()
	var key any
	for i := 0; i < 2; i++ {
		resp = httpPut(t, url+insightsVersion, insightsBody)
		// Exact SDK contract accepts 200 only, even on create.
		assertStatus(t, resp, 200)
		for _, h := range []string{"x-ms-request-id", "x-ms-correlation-request-id"} {
			if resp.Header.Get(h) == "" {
				t.Fatalf("missing %s", h)
			}
		}
		body := decodeJSON(t, resp)
		props := body["properties"].(map[string]any)
		if body["location"] != "uksouth" || body["type"] != applicationInsightsType || body["kind"] != "web" || props["InstrumentationKey"] == "" {
			t.Fatalf("invalid shape: %v", body)
		}
		if i == 1 && key != props["InstrumentationKey"] {
			t.Fatal("credential changed on PUT")
		}
		key = props["InstrumentationKey"]
	}
	resp = httpGet(t, url+insightsVersion)
	assertStatus(t, resp, 200)
	decodeJSON(t, resp)
	resp = httpHead(t, url+insightsVersion)
	assertStatus(t, resp, 204)
	if readBody(t, resp) != "" {
		t.Fatal("HEAD body")
	}
	for _, list := range []string{base, srv.URL + "/subscriptions/sub1/providers/Microsoft.Insights/components"} {
		resp = httpGet(t, list+insightsVersion)
		assertStatus(t, resp, 200)
		if len(decodeJSON(t, resp)["value"].([]any)) != 1 {
			t.Fatal("missing component")
		}
	}
	resp = httpPut(t, url+insightsVersion, strings.Replace(insightsBody, `"SamplingPercentage":100`, `"SamplingPercentage":25`, 1))
	assertStatus(t, resp, 200)
	if decodeJSON(t, resp)["properties"].(map[string]any)["SamplingPercentage"] != float64(25) {
		t.Fatal("update lost")
	}
	billing := url + "/currentBillingFeatures?api-version=2015-05-01"
	resp = httpGet(t, billing)
	assertStatus(t, resp, 200)
	if decodeJSON(t, resp)["DataVolumeCap"].(map[string]any)["Cap"] != float64(100) {
		t.Fatal("invalid cap default")
	}
	resp = httpPut(t, billing, `{"CurrentBillingFeatures":["Basic"],"DataVolumeCap":{"Cap":42,"StopSendNotificationWhenHitCap":true}}`)
	assertStatus(t, resp, 200)
	decodeJSON(t, resp)
	resp = httpGet(t, billing)
	if decodeJSON(t, resp)["DataVolumeCap"].(map[string]any)["Cap"] != float64(42) {
		t.Fatal("cap update lost")
	}
	resp = httpPut(t, url+insightsVersion, insightsBody)
	assertStatus(t, resp, 200)
	resp.Body.Close()
	resp = httpGet(t, billing)
	if decodeJSON(t, resp)["DataVolumeCap"].(map[string]any)["Cap"] != float64(42) {
		t.Fatal("component update reset billing")
	}
	resp = httpDelete(t, url+insightsVersion)
	assertStatus(t, resp, 204)
	resp.Body.Close()
	resp = httpGet(t, url+insightsVersion)
	assertStatus(t, resp, 404)
	resp.Body.Close()
	resp = httpHead(t, url+insightsVersion)
	assertStatus(t, resp, 404)
	if readBody(t, resp) != "" {
		t.Fatal("missing HEAD body")
	}
	resp = httpDelete(t, url+insightsVersion)
	assertStatus(t, resp, 204)
	resp.Body.Close()
	resp = httpPut(t, url+insightsVersion, insightsBody)
	resp.Body.Close()
	resp = httpDelete(t, rg)
	assertStatus(t, resp, 202)
	resp.Body.Close()
	resp = httpGet(t, url+insightsVersion)
	assertStatus(t, resp, 404)
	resp.Body.Close()
}

func TestApplicationInsights_Validation(t *testing.T) {
	srv := newTestServer(t)
	rg := srv.URL + "/subscriptions/sub1/resourceGroups/rg1"
	resp := httpPut(t, rg, `{"location":"uksouth"}`)
	resp.Body.Close()
	url := rg + "/providers/Microsoft.Insights/components/ai1"
	resp = httpGetRaw(t, url)
	assertStatus(t, resp, 400)
	if decodeJSON(t, resp)["error"].(map[string]any)["code"] != "MissingApiVersionParameter" {
		t.Fatal("missing API version accepted")
	}
	for _, tc := range []struct {
		name, body string
		status     int
	}{
		{"malformed", `{`, 400},
		{"null", `null`, 400},
		{"location", `{"properties":{"Application_Type":"web"}}`, 400},
		{"type", strings.Replace(insightsBody, `"Application_Type":"web"`, `"Application_Type":"invalid"`, 1), 400},
		{"sampling", strings.Replace(insightsBody, `"SamplingPercentage":100`, `"SamplingPercentage":101`, 1), 400},
		{"retention", strings.Replace(insightsBody, `"RetentionInDays":90`, `"RetentionInDays":-1`, 1), 400},
		{"workspace", strings.Replace(insightsBody, `"SamplingPercentage":100`, `"WorkspaceResourceId":"unsupported"`, 1), 501},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp := httpPut(t, url+insightsVersion, tc.body)
			assertStatus(t, resp, tc.status)
			if decodeJSON(t, resp)["error"] == nil {
				t.Fatal("missing Azure error")
			}
		})
	}
	resp = httpPut(t, url+insightsVersion, insightsBody)
	resp.Body.Close()
	for _, body := range []string{`{`, `null`, `{}`, `{"DataVolumeCap":{"Cap":-1}}`, `{"DataVolumeCap":{"Cap":"bad"}}`} {
		resp = httpPut(t, url+"/currentBillingFeatures?api-version=2015-05-01", body)
		assertStatus(t, resp, 400)
		resp.Body.Close()
	}
}

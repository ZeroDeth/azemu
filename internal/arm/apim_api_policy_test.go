package arm

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// Keep the CAF 105 document's whitespace, including its trailing newline.
const apimAPIPolicyXML = "<policies>\n  <inbound>\n    <find-and-replace from=\"xyz\" to=\"abc\" />\n  </inbound>\n</policies>\n"

func apimAPIPolicyBody(t *testing.T, format, value string) string {
	t.Helper()
	body, err := json.Marshal(map[string]any{"properties": map[string]any{"format": format, "value": value}})
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func TestAPIMAPIPolicy_Lifecycle(t *testing.T) {
	base, _ := newAPIMAPIParent(t)
	api := base + "/apis/Example;rev=1"
	alias := base + "/apis/Example"
	resp := httpPut(t, api+apimAPIVersion, apimAPIBody)
	assertStatus(t, resp, 201)
	resp.Body.Close()
	endpoint := alias + "/policies/policy" + apimAPIVersion
	resp = httpGet(t, endpoint)
	assertStatus(t, resp, 404)
	resp.Body.Close()
	resp = httpHead(t, endpoint)
	assertStatus(t, resp, 404)
	if readBody(t, resp) != "" {
		t.Fatal("missing HEAD has a body")
	}
	resp = httpGet(t, alias+"/policies"+apimAPIVersion)
	if len(decodeJSON(t, resp)["value"].([]any)) != 0 {
		t.Fatal("unexpected initial policy")
	}
	for _, status := range []int{201, 200} {
		resp = httpPut(t, endpoint, apimAPIPolicyBody(t, "rawxml", apimAPIPolicyXML))
		assertStatus(t, resp, status)
		for _, header := range []string{"x-ms-request-id", "x-ms-correlation-request-id", "ETag"} {
			if resp.Header.Get(header) == "" {
				t.Fatalf("missing %s", header)
			}
		}
		body := decodeJSON(t, resp)
		if body["type"] != apimAPIPolicyType || body["name"] != "policy" ||
			body["id"] != strings.TrimPrefix(api, strings.Split(base, "/subscriptions/")[0])+"/policies/policy" {
			t.Fatalf("unexpected policy shape: %v", body)
		}
		if body["location"] != nil || body["tags"] != nil || body["properties"].(map[string]any)["value"] != apimAPIPolicyXML {
			t.Fatalf("policy contract or XML changed: %v", body)
		}
	}
	for _, path := range []string{api, alias} {
		for _, format := range []string{"", "&format=xml", "&format=rawxml"} {
			resp = httpGet(t, path+"/policies/policy"+apimAPIVersion+format)
			assertStatus(t, resp, 200)
			if decodeJSON(t, resp)["properties"].(map[string]any)["value"] != apimAPIPolicyXML {
				t.Fatal("XML whitespace was not preserved")
			}
		}
		resp = httpHead(t, path+"/policies/policy"+apimAPIVersion)
		assertStatus(t, resp, 200) // SDK GetEntityTag, unlike generic ARM HEAD.
		if resp.Header.Get("ETag") == "" || readBody(t, resp) != "" {
			t.Fatal("HEAD must have an ETag and no body")
		}
	}
	updated := strings.ReplaceAll(apimAPIPolicyXML, `to="abc"`, `to="updated &amp; encoded"`)
	resp = httpPut(t, endpoint, apimAPIPolicyBody(t, "xml", updated))
	assertStatus(t, resp, 200)
	resp.Body.Close()
	resp = httpGet(t, alias+"/policies"+apimAPIVersion)
	items := decodeJSON(t, resp)["value"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["properties"].(map[string]any)["value"] != updated {
		t.Fatal("updated policy missing from list")
	}
	// A policy attached to an API must not create a service policy.
	resp = httpGet(t, base+"/policies/policy"+apimAPIVersion)
	assertStatus(t, resp, 404)
	resp.Body.Close()
	resp = httpDelete(t, endpoint)
	assertStatus(t, resp, 204)
	resp.Body.Close()
	resp = httpGet(t, api+"/policies/policy"+apimAPIVersion)
	assertStatus(t, resp, 404)
	resp.Body.Close()
	resp = httpDelete(t, endpoint)
	assertStatus(t, resp, 204)
	resp.Body.Close()
	resp = httpGet(t, alias+"/policies"+apimAPIVersion)
	if len(decodeJSON(t, resp)["value"].([]any)) != 0 {
		t.Fatal("deleted policy still listed")
	}
}

func TestAPIMAPIPolicy_Validation(t *testing.T) {
	base, _ := newAPIMAPIParent(t)
	api := base + "/apis/Example;rev=1"
	resp := httpPut(t, api+apimAPIVersion, apimAPIBody)
	resp.Body.Close()
	endpoint := api + "/policies/policy" + apimAPIVersion
	for _, tc := range []struct {
		name, body string
		status     int
	}{
		{"invalid JSON", `{`, 400},
		{"trailing JSON", `{"properties":{}} {}`, 400},
		{"missing properties", `{}`, 400},
		{"null properties", `{"properties":null}`, 400},
		{"unknown envelope", `{"properties":{},"location":"uksouth"}`, 400},
		{"missing value", `{"properties":{"format":"rawxml"}}`, 400},
		{"empty value", apimAPIPolicyBody(t, "xml", " "), 400},
		{"value type", `{"properties":{"value":true}}`, 400},
		{"format type", `{"properties":{"format":null,"value":"<policies/>"}}`, 400},
		{"format empty", apimAPIPolicyBody(t, "", "<policies/>"), 400},
		{"unknown format", apimAPIPolicyBody(t, "json", "<policies/>"), 400},
		{"raw link", apimAPIPolicyBody(t, "rawxml-link", "https://example.test/policy.xml"), 501},
		{"xml link", apimAPIPolicyBody(t, "xml-link", "https://example.test/policy.xml"), 501},
		{"unknown property", `{"properties":{"value":"<policies/>","ignored":true}}`, 501},
		{"malformed XML", apimAPIPolicyBody(t, "rawxml", "<policies><inbound></policies>"), 400},
		{"escaped entire XML", apimAPIPolicyBody(t, "xml", "&lt;policies/&gt;"), 400},
		{"wrong root", apimAPIPolicyBody(t, "xml", "<inbound/>"), 400},
		{"two roots", apimAPIPolicyBody(t, "xml", "<policies/><policies/>"), 400},
		{"root attributes", apimAPIPolicyBody(t, "xml", `<policies foo="bar"/>`), 400},
		{"duplicate sections", apimAPIPolicyBody(t, "xml", "<policies><inbound/><inbound/></policies>"), 400},
		{"unknown section", apimAPIPolicyBody(t, "xml", "<policies><invalid/></policies>"), 400},
		{"section attributes", apimAPIPolicyBody(t, "xml", `<policies><inbound foo="bar"/></policies>`), 400},
		{"text content", apimAPIPolicyBody(t, "xml", "<policies>unexpected</policies>"), 400},
		{"DTD", apimAPIPolicyBody(t, "xml", "<!DOCTYPE policies><policies/>"), 400},
		{"PI", apimAPIPolicyBody(t, "xml", "<?unsupported value?><policies/>"), 400},
		{"namespace", apimAPIPolicyBody(t, "xml", `<policies xmlns="urn:unsupported"/>`), 501},
		{"missing from", apimAPIPolicyBody(t, "xml", `<policies><inbound><find-and-replace to=""/></inbound></policies>`), 400},
		{"missing to", apimAPIPolicyBody(t, "xml", `<policies><inbound><find-and-replace from="x"/></inbound></policies>`), 400},
		{"duplicate attribute", apimAPIPolicyBody(t, "xml", `<policies><inbound><find-and-replace from="x" from="y" to="z"/></inbound></policies>`), 400},
		{"invalid backend statement", apimAPIPolicyBody(t, "xml", `<policies><backend><find-and-replace from="x" to="y"/></backend></policies>`), 400},
		{"base attributes", apimAPIPolicyBody(t, "xml", `<policies><inbound><base foo="bar"/></inbound></policies>`), 400},
		{"nested statement", apimAPIPolicyBody(t, "xml", `<policies><inbound><base><base/></base></inbound></policies>`), 400},
		{"unsupported statement", apimAPIPolicyBody(t, "xml", `<policies><inbound><set-header name="x"/></inbound></policies>`), 501},
		{"expression", apimAPIPolicyBody(t, "rawxml", `<policies><inbound><find-and-replace from="@("a")" to="x"/></inbound></policies>`), 501},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp := httpPut(t, endpoint, tc.body)
			assertStatus(t, resp, tc.status)
			if decodeJSON(t, resp)["error"] == nil {
				t.Fatal("missing Azure error")
			}
		})
	}
	// Failed PUTs must not have created a policy.
	resp = httpGet(t, endpoint)
	assertStatus(t, resp, 404)
	resp.Body.Close()
	for _, body := range []string{
		`{"properties":{"value":"<policies/>"}}`, // ARM's default xml format.
		apimAPIPolicyBody(t, "xml", "<?xml version=\"1.0\"?>\r\n<!--keep--><policies><inbound><base/></inbound><backend><base/></backend><outbound/><on-error/></policies>\r\n"),
		apimAPIPolicyBody(t, "rawxml", `<policies><inbound><find-and-replace from="a &amp; b" to=""/></inbound></policies>`),
	} {
		resp = httpPut(t, endpoint, body)
		if resp.StatusCode != 201 && resp.StatusCode != 200 {
			t.Fatalf("valid document rejected: %s", readBody(t, resp))
		}
		resp.Body.Close()
	}
	for _, query := range []string{"&format=", "&format=xml-link", "&format=invalid", "&format=xml&format=rawxml", "&effective=true"} {
		resp = httpGet(t, endpoint+query)
		status := 400
		if strings.Contains(query, "effective") {
			status = 501
		}
		assertStatus(t, resp, status)
		if decodeJSON(t, resp)["error"] == nil {
			t.Fatal("missing Azure error")
		}
	}
	resp = httpGetRaw(t, api+"/policies/policy")
	assertStatus(t, resp, 400)
	if decodeJSON(t, resp)["error"].(map[string]any)["code"] != "MissingApiVersionParameter" {
		t.Fatal("missing API version was not rejected by middleware")
	}
}

func TestAPIMAPIPolicy_ParentsRevisionsAndCascade(t *testing.T) {
	for _, parent := range []string{"api", "service", "resource group"} {
		t.Run(parent, func(t *testing.T) {
			base, rg := newAPIMAPIParent(t)
			alias := base + "/apis/Example"
			policy := alias + "/policies/policy" + apimAPIVersion
			body := apimAPIPolicyBody(t, "rawxml", apimAPIPolicyXML)
			resp := httpPut(t, policy, body)
			assertStatus(t, resp, 404)
			if decodeJSON(t, resp)["error"].(map[string]any)["code"] != "ParentResourceNotFound" {
				t.Fatal("missing API parent accepted")
			}
			for _, revision := range []string{"1", "2"} {
				resp = httpPut(t, alias+";rev="+revision+apimAPIVersion, apimAPIBody)
				assertStatus(t, resp, 201)
				resp.Body.Close()
				resp = httpPut(t, alias+";rev="+revision+"/policies/policy"+apimAPIVersion, body)
				assertStatus(t, resp, 201)
				resp.Body.Close()
			}
			// Deleting via the unqualified name affects only the current revision.
			resp = httpDelete(t, policy)
			assertStatus(t, resp, 204)
			resp.Body.Close()
			resp = httpGet(t, alias+";rev=2/policies/policy"+apimAPIVersion)
			assertStatus(t, resp, 200)
			resp.Body.Close()
			resp = httpPut(t, policy, body)
			assertStatus(t, resp, 201)
			resp.Body.Close()
			target := alias + ";rev=1" + apimAPIVersion + "&deleteRevisions=true"
			switch parent {
			case "service":
				target = base + apimAPIVersion
			case "resource group":
				target = rg
			}
			resp = httpDelete(t, target)
			if resp.StatusCode != 202 && resp.StatusCode != 204 {
				t.Fatalf("delete parent: %s", readBody(t, resp))
			}
			resp.Body.Close()
			for _, revision := range []string{"", ";rev=1", ";rev=2"} {
				endpoint := alias + revision + "/policies/policy" + apimAPIVersion
				resp = httpGet(t, endpoint)
				assertStatus(t, resp, 404)
				resp.Body.Close()
				resp = httpHead(t, endpoint)
				assertStatus(t, resp, 404)
				if readBody(t, resp) != "" {
					t.Fatal("HEAD has a body")
				}
			}
		})
	}
	srv := newTestServer(t)
	resp := httpPut(t, srv.URL+"/subscriptions/sub1/resourceGroups/missing/providers/Microsoft.ApiManagement/service/missing/apis/missing/policies/policy"+apimAPIVersion, apimAPIPolicyBody(t, "xml", "<policies/>"))
	assertStatus(t, resp, 404)
	if decodeJSON(t, resp)["error"].(map[string]any)["code"] != "ParentResourceNotFound" {
		t.Fatal("missing ancestors accepted")
	}
}

func TestAPIMAPIPolicy_ETag(t *testing.T) {
	base, _ := newAPIMAPIParent(t)
	api := base + "/apis/Example;rev=1"
	resp := httpPut(t, api+apimAPIVersion, apimAPIBody)
	resp.Body.Close()
	endpoint := api + "/policies/policy" + apimAPIVersion
	resp = httpPut(t, endpoint, apimAPIPolicyBody(t, "xml", apimAPIPolicyXML))
	etag := resp.Header.Get("ETag")
	resp.Body.Close()
	for _, tc := range []struct {
		method, match string
		status        int
	}{
		{http.MethodPut, `"stale"`, 412},
		{http.MethodDelete, `"stale"`, 412},
		{http.MethodPut, etag, 200},
		{http.MethodPut, "*", 200},
		{http.MethodDelete, etag, 204},
	} {
		req, err := http.NewRequest(tc.method, endpoint, strings.NewReader(apimAPIPolicyBody(t, "xml", apimAPIPolicyXML)))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("If-Match", tc.match)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		assertStatus(t, resp, tc.status)
		resp.Body.Close()
	}
}

package arm

import (
	"net/http"
	"strings"
	"testing"
)

func TestAPIMAPIOperationPolicy_Lifecycle(t *testing.T) {
	api, service, _ := newAPIMOperationParent(t)
	operation := api + "/operations/sample"
	resp := httpPut(t, operation+apimAPIVersion, apimOperationBody)
	assertStatus(t, resp, 201)
	resp.Body.Close()
	policy := operation + "/policies/policy"
	alias := service + "/apis/Example/operations/sample/policies/policy"
	resp = httpGet(t, policy+apimAPIVersion)
	assertStatus(t, resp, 404)
	resp.Body.Close()
	resp = httpHead(t, policy+apimAPIVersion)
	assertStatus(t, resp, 404)
	if readBody(t, resp) != "" {
		t.Fatal("missing HEAD has body")
	}
	for _, status := range []int{201, 200} {
		resp = httpPut(t, alias+apimAPIVersion, apimAPIPolicyBody(t, "rawxml", apimAPIPolicyXML))
		assertStatus(t, resp, status)
		for _, header := range []string{"ETag", "x-ms-request-id", "x-ms-correlation-request-id"} {
			if resp.Header.Get(header) == "" {
				t.Fatalf("missing %s", header)
			}
		}
		body := decodeJSON(t, resp)
		if body["name"] != "policy" || body["type"] != apimAPIOperationPolicyType || len(body) != 4 ||
			!strings.HasSuffix(body["id"].(string), "/apis/Example;rev=1/operations/sample/policies/policy") ||
			body["properties"].(map[string]any)["value"] != apimAPIPolicyXML {
			t.Fatalf("invalid policy contract: %v", body)
		}
	}
	for _, address := range []string{policy, alias} {
		resp = httpGet(t, address+apimAPIVersion+"&format=xml")
		assertStatus(t, resp, 200)
		if decodeJSON(t, resp)["properties"].(map[string]any)["value"] != apimAPIPolicyXML {
			t.Fatal("XML bytes changed")
		}
		resp = httpHead(t, address+apimAPIVersion)
		assertStatus(t, resp, 200)
		if resp.Header.Get("ETag") == "" || readBody(t, resp) != "" {
			t.Fatal("HEAD needs ETag and no body")
		}
	}
	updated := "<policies>\r\n<inbound><base/><find-and-replace from=\"a\" to=\"b &amp; c\"/></inbound></policies>\r\n"
	resp = httpPut(t, policy+apimAPIVersion, apimAPIPolicyBody(t, "xml", updated))
	assertStatus(t, resp, 200)
	etag := resp.Header.Get("ETag")
	resp.Body.Close()
	resp = httpGet(t, operation+"/policies"+apimAPIVersion)
	items := decodeJSON(t, resp)["value"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["properties"].(map[string]any)["value"] != updated {
		t.Fatal("updated policy list changed")
	}
	revision2 := service + "/apis/Example;rev=2"
	resp = httpPut(t, revision2+apimAPIVersion, apimAPIBody)
	assertStatus(t, resp, 201)
	resp.Body.Close()
	resp = httpPut(t, revision2+"/operations/sample"+apimAPIVersion, apimOperationBody)
	assertStatus(t, resp, 201)
	resp.Body.Close()
	resp = httpGet(t, revision2+"/operations/sample/policies/policy"+apimAPIVersion)
	assertStatus(t, resp, 404)
	resp.Body.Close()
	for _, method := range []string{http.MethodPut, http.MethodDelete} {
		req, err := http.NewRequest(method, alias+apimAPIVersion, strings.NewReader(apimAPIPolicyBody(t, "xml", updated)))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("If-Match", `"stale"`)
		resp, err = http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		assertStatus(t, resp, 412)
		resp.Body.Close()
	}
	req, err := http.NewRequest(http.MethodDelete, alias+apimAPIVersion, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("If-Match", etag)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	assertStatus(t, resp, 204)
	resp.Body.Close()
	resp = httpDelete(t, policy+apimAPIVersion)
	assertStatus(t, resp, 204)
	resp.Body.Close()
	resp = httpGet(t, policy+apimAPIVersion)
	assertStatus(t, resp, 404)
	resp.Body.Close()
	resp = httpGet(t, operation+"/policies"+apimAPIVersion)
	if len(decodeJSON(t, resp)["value"].([]any)) != 0 {
		t.Fatal("policy survived delete")
	}
}

func TestAPIMAPIOperationPolicy_ValidationAndCascade(t *testing.T) {
	api, _, _ := newAPIMOperationParent(t)
	operation := api + "/operations/sample"
	policy := operation + "/policies/policy"
	resp := httpPut(t, policy+apimAPIVersion, apimAPIPolicyBody(t, "xml", apimAPIPolicyXML))
	assertStatus(t, resp, 404)
	if decodeJSON(t, resp)["error"].(map[string]any)["code"] != "ParentResourceNotFound" {
		t.Fatal("missing operation error")
	}
	resp = httpPut(t, operation+apimAPIVersion, apimOperationBody)
	assertStatus(t, resp, 201)
	resp.Body.Close()
	for _, test := range []struct {
		body   string
		status int
	}{
		{`{`, 400}, {`{}`, 400}, {`{"properties":null}`, 400}, {`{"properties":{}}`, 400},
		{`{"properties":{"value":true}}`, 400}, {`{"properties":{"value":"<policies/>","format":3}}`, 400},
		{`{"properties":{"value":"<policies/>"}} {}`, 400},
		{apimAPIPolicyBody(t, "unknown", "<policies/>"), 400},
		{apimAPIPolicyBody(t, "xml", "<policies>"), 400},
		{apimAPIPolicyBody(t, "xml-link", "https://example.test/policy.xml"), 501},
		{apimAPIPolicyBody(t, "xml", `<policies><inbound><set-header name="x" exists-action="override"/></inbound></policies>`), 501},
		{apimAPIPolicyBody(t, "rawxml", `<policies><inbound><find-and-replace from="x" to="@(true)"/></inbound></policies>`), 501},
	} {
		resp = httpPut(t, policy+apimAPIVersion, test.body)
		assertStatus(t, resp, test.status)
		if decodeJSON(t, resp)["error"] == nil {
			t.Fatal("expected Azure error")
		}
	}
	resp = httpGetRaw(t, policy)
	assertStatus(t, resp, 400)
	if decodeJSON(t, resp)["error"].(map[string]any)["code"] != "MissingApiVersionParameter" {
		t.Fatal("missing version error")
	}
	resp = httpPut(t, policy+apimAPIVersion, `{"properties":{"value":"<policies/>"}}`)
	assertStatus(t, resp, 201) // format defaults to xml.
	resp.Body.Close()
	for _, test := range []struct {
		query  string
		status int
	}{{"&format=json", 400}, {"&format=xml&format=rawxml", 400}, {"&effective=true", 501}} {
		resp = httpGet(t, policy+apimAPIVersion+test.query)
		assertStatus(t, resp, test.status)
		resp.Body.Close()
	}
	// Operation scope must not overwrite API-level policies.
	resp = httpPut(t, api+"/policies/policy"+apimAPIVersion, apimAPIPolicyBody(t, "xml", apimAPIPolicyXML))
	assertStatus(t, resp, 201)
	resp.Body.Close()
	resp = httpGet(t, api+"/policies/policy"+apimAPIVersion)
	if decodeJSON(t, resp)["properties"].(map[string]any)["value"] != apimAPIPolicyXML {
		t.Fatal("operation policy leaked into API scope")
	}
	for _, scope := range []string{"operation", "api", "service", "rg"} {
		t.Run(scope, func(t *testing.T) {
			api, service, rg := newAPIMOperationParent(t)
			operation := api + "/operations/sample"
			policy := operation + "/policies/policy"
			resp := httpPut(t, operation+apimAPIVersion, apimOperationBody)
			assertStatus(t, resp, 201)
			resp.Body.Close()
			resp = httpPut(t, policy+apimAPIVersion, apimAPIPolicyBody(t, "xml", apimAPIPolicyXML))
			assertStatus(t, resp, 201)
			resp.Body.Close()
			parent := map[string]string{"operation": operation, "api": api, "service": service, "rg": rg}[scope]
			resp = httpDelete(t, parent+apimAPIVersion)
			resp.Body.Close()
			resp = httpGet(t, policy+apimAPIVersion)
			assertStatus(t, resp, 404)
			resp.Body.Close()
		})
	}
}

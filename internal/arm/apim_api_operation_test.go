package arm

import (
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

const apimOperationBody = `{"properties":{"displayName":"Get WishLists","method":"GET","urlTemplate":"/wishlists","description":"Get WishList","request":{"description":"","headers":[{"name":"test","required":false,"type":"string","values":["true","false"],"examples":{"name":{}}}]}}}`

func newAPIMOperationParent(t *testing.T) (string, string, string) {
	t.Helper()
	service, rg := newAPIMAPIParent(t)
	api := service + "/apis/Example;rev=1"
	resp := httpPut(t, api+apimAPIVersion, apimAPIBody)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()
	return api, service, rg
}

func TestAPIMAPIOperation_Lifecycle(t *testing.T) {
	api, service, _ := newAPIMOperationParent(t)
	operation := api + "/operations/sample"
	for _, status := range []int{http.StatusCreated, http.StatusOK} {
		resp := httpPut(t, operation+apimAPIVersion, apimOperationBody)
		assertStatus(t, resp, status)
		for _, header := range []string{"x-ms-request-id", "x-ms-correlation-request-id"} {
			if resp.Header.Get(header) == "" {
				t.Fatalf("missing %s", header)
			}
		}
		body := decodeJSON(t, resp)
		if body["type"] != apimAPIOperationType || body["name"] != "sample" ||
			!strings.HasSuffix(body["id"].(string), "/apis/Example;rev=1/operations/sample") || len(body) != 4 {
			t.Fatalf("invalid OperationContract shape: %v", body)
		}
		var expected map[string]any
		if err := json.Unmarshal([]byte(apimOperationBody), &expected); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(body["properties"], expected["properties"]) {
			t.Fatalf("configured fields changed: %v", body)
		}
	}
	for _, address := range []string{api, service + "/apis/Example"} {
		resp := httpGet(t, address+"/operations/sample"+apimAPIVersion)
		assertStatus(t, resp, http.StatusOK)
		props := decodeJSON(t, resp)["properties"].(map[string]any)
		if props["displayName"] != "Get WishLists" {
			t.Fatal("alias did not resolve operation")
		}
		resp = httpHead(t, address+"/operations/sample"+apimAPIVersion)
		assertStatus(t, resp, http.StatusNoContent)
		if readBody(t, resp) != "" {
			t.Fatal("HEAD returned a body")
		}
		resp = httpGet(t, address+"/operations"+apimAPIVersion)
		assertStatus(t, resp, http.StatusOK)
		if len(decodeJSON(t, resp)["value"].([]any)) != 1 {
			t.Fatal("alias duplicated or lost operation")
		}
	}
	updated := `{"properties":{"displayName":"Updated","method":"POST","urlTemplate":"/wishlists"}}`
	resp := httpPut(t, service+"/apis/Example/operations/sample"+apimAPIVersion, updated)
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()
	resp = httpGet(t, operation+apimAPIVersion)
	props := decodeJSON(t, resp)["properties"].(map[string]any)
	if props["method"] != "POST" || props["request"] != nil || props["description"] != nil {
		t.Fatalf("operation PUT must replace optional fields: %v", props)
	}
	for i := 0; i < 2; i++ {
		resp = httpDelete(t, service+"/apis/Example/operations/sample"+apimAPIVersion)
		assertStatus(t, resp, http.StatusNoContent)
		resp.Body.Close()
	}
	resp = httpGet(t, operation+apimAPIVersion)
	assertStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
	resp = httpHead(t, operation+apimAPIVersion)
	assertStatus(t, resp, http.StatusNotFound)
	if readBody(t, resp) != "" {
		t.Fatal("missing HEAD returned a body")
	}
	resp = httpGet(t, api+"/operations"+apimAPIVersion)
	if len(decodeJSON(t, resp)["value"].([]any)) != 0 {
		t.Fatal("delete did not remove operation")
	}
}

func TestAPIMAPIOperation_NestedContracts(t *testing.T) {
	api, _, _ := newAPIMOperationParent(t)
	parameter := map[string]any{
		"name": "id", "type": "string", "required": true, "description": "parameter",
		"defaultValue": "one", "schemaId": "schema", "typeName": "Model", "values": []any{"one", "two"},
		"examples": map[string]any{"object": map[string]any{
			"summary": "summary", "description": "description", "externalValue": "https://example.test/example",
			"value": map[string]any{"nested": []any{true, false, nil, float64(42), "text"}},
		}},
	}
	representation := map[string]any{
		"contentType": "application/json", "schemaId": "schema", "typeName": "Model",
		"formParameters": []any{parameter}, "examples": parameter["examples"],
	}
	props := map[string]any{
		"displayName": "Nested", "method": "POST", "urlTemplate": "/items/{id}",
		"templateParameters": []any{parameter},
		"request": map[string]any{
			"description": "request", "headers": []any{parameter}, "queryParameters": []any{parameter},
			"representations": []any{representation},
		},
		"responses": []any{
			map[string]any{"statusCode": float64(200), "description": "response", "headers": []any{parameter}, "representations": []any{representation}},
			map[string]any{"statusCode": float64(0), "headers": []any{}},
		},
	}
	body, err := json.Marshal(map[string]any{"properties": props})
	if err != nil {
		t.Fatal(err)
	}
	for _, status := range []int{http.StatusCreated, http.StatusOK} {
		resp := httpPut(t, api+"/operations/nested"+apimAPIVersion, string(body))
		assertStatus(t, resp, status)
		resp.Body.Close()
		resp = httpGet(t, api+"/operations/nested"+apimAPIVersion)
		if got := decodeJSON(t, resp)["properties"]; !reflect.DeepEqual(got, props) {
			t.Fatalf("nested fields were changed:\ngot: %#v\nwant: %#v", got, props)
		}
	}
}

func TestAPIMAPIOperation_Validation(t *testing.T) {
	api, _, _ := newAPIMOperationParent(t)
	for _, tc := range []struct {
		name, body string
	}{
		{"invalid JSON", `{`},
		{"missing properties", `{}`},
		{"null properties", `{"properties":null}`},
		{"missing fields", `{"properties":{}}`},
		{"trailing JSON", apimOperationBody + `{}`},
		{"unknown property", `{"properties":{"displayName":"x","method":"GET","urlTemplate":"/","unsupported":true}}`},
		{"unknown nested field", strings.Replace(apimOperationBody, `"required":false`, `"required":false,"unknown":true`, 1)},
		{"wrong parameter type", strings.Replace(apimOperationBody, `"required":false`, `"required":"false"`, 1)},
		{"empty parameter name", strings.Replace(apimOperationBody, `"name":"test"`, `"name":""`, 1)},
		{"missing parameter type", strings.Replace(apimOperationBody, `"type":"string",`, ``, 1)},
		{"invalid values", strings.Replace(apimOperationBody, `["true","false"]`, `[true,false]`, 1)},
		{"invalid example", strings.Replace(apimOperationBody, `"name":{}`, `"name":{"summary":true}`, 1)},
		{"missing method", `{"properties":{"displayName":"x","urlTemplate":"/"}}`},
		{"invalid method", `{"properties":{"displayName":"x","method":"GET bad","urlTemplate":"/"}}`},
		{"invalid URL", `{"properties":{"displayName":"x","method":"GET","urlTemplate":"not-relative"}}`},
		{"missing template parameter", `{"properties":{"displayName":"x","method":"GET","urlTemplate":"/{id}"}}`},
		{"unused template parameter", `{"properties":{"displayName":"x","method":"GET","urlTemplate":"/","templateParameters":[{"name":"id","type":"string"}]}}`},
		{"invalid status", `{"properties":{"displayName":"x","method":"GET","urlTemplate":"/","responses":[{"statusCode":600}]}}`},
		{"missing status", `{"properties":{"displayName":"x","method":"GET","urlTemplate":"/","responses":[{}]}}`},
		{"invalid response header", `{"properties":{"displayName":"x","method":"GET","urlTemplate":"/","responses":[{"statusCode":200,"headers":[{}]}]}}`},
		{"invalid representation", `{"properties":{"displayName":"x","method":"GET","urlTemplate":"/","request":{"representations":[{}]}}}`},
		{"invalid form parameter", `{"properties":{"displayName":"x","method":"GET","urlTemplate":"/","request":{"representations":[{"contentType":"multipart/form-data","formParameters":[{}]}]}}}`},
		{"policies unsupported", `{"properties":{"displayName":"x","method":"GET","urlTemplate":"/","policies":"<policies/>"}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp := httpPut(t, api+"/operations/invalid"+apimAPIVersion, tc.body)
			assertStatus(t, resp, http.StatusBadRequest)
			if decodeJSON(t, resp)["error"].(map[string]any)["code"] == "" {
				t.Fatal("missing Azure error code")
			}
		})
	}
	resp := httpGetRaw(t, api+"/operations/sample")
	assertStatus(t, resp, http.StatusBadRequest)
	if decodeJSON(t, resp)["error"].(map[string]any)["code"] != "MissingApiVersionParameter" {
		t.Fatal("api-version middleware not exercised")
	}
	resp = httpPut(t, api+"/operations/"+strings.Repeat("x", 81)+apimAPIVersion, apimOperationBody)
	assertStatus(t, resp, http.StatusBadRequest)
	resp.Body.Close()
	resp = httpGet(t, api+"/operations"+apimAPIVersion)
	if len(decodeJSON(t, resp)["value"].([]any)) != 0 {
		t.Fatal("invalid operation was persisted")
	}
}

func TestAPIMAPIOperation_ParentAndCascade(t *testing.T) {
	for _, scope := range []string{"api", "service", "resource group"} {
		t.Run(scope, func(t *testing.T) {
			api, service, rg := newAPIMOperationParent(t)
			resp := httpPut(t, api+"/operations/sample"+apimAPIVersion, apimOperationBody)
			assertStatus(t, resp, http.StatusCreated)
			resp.Body.Close()
			parent, status := api, http.StatusNoContent
			switch scope {
			case "service":
				parent, status = service, http.StatusAccepted
			case "resource group":
				parent, status = rg, http.StatusAccepted
			}
			resp = httpDelete(t, parent+apimAPIVersion)
			assertStatus(t, resp, status)
			resp.Body.Close()
			resp = httpGet(t, api+"/operations/sample"+apimAPIVersion)
			assertStatus(t, resp, http.StatusNotFound)
			resp.Body.Close()
			resp = httpPut(t, api+"/operations/sample"+apimAPIVersion, apimOperationBody)
			assertStatus(t, resp, http.StatusNotFound)
			if decodeJSON(t, resp)["error"].(map[string]any)["code"] != "ParentResourceNotFound" {
				t.Fatal("missing parent did not return ParentResourceNotFound")
			}
		})
	}
}

func TestAPIMAPIOperation_ImportAndRevisionAliases(t *testing.T) {
	api, service, _ := newAPIMOperationParent(t)
	importBody, err := json.Marshal(map[string]any{"properties": map[string]any{
		"path": "example", "format": "swagger-json", "value": apimSwaggerTest,
	}})
	if err != nil {
		t.Fatal(err)
	}

	resp := httpPut(t, api+apimAPIVersion, string(importBody))
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()
	// Explicit PUT can take ownership of an imported operation, and can
	// also add an operation unrelated to the Swagger document.
	for _, name := range []string{"sample", "getPet"} {
		resp = httpPut(t, service+"/apis/Example/operations/"+name+apimAPIVersion, apimOperationBody)
		status := http.StatusCreated
		if name == "getPet" {
			status = http.StatusOK
		}
		assertStatus(t, resp, status)
		resp.Body.Close()
	}
	for _, body := range []string{apimAPIBody, string(importBody)} {
		resp = httpPut(t, api+apimAPIVersion, body)
		assertStatus(t, resp, http.StatusOK)
		resp.Body.Close()
		for _, name := range []string{"sample", "getPet"} {
			resp = httpGet(t, api+"/operations/"+name+apimAPIVersion)
			assertStatus(t, resp, http.StatusOK)
			if decodeJSON(t, resp)["properties"].(map[string]any)["displayName"] != "Get WishLists" {
				t.Fatal("API update overwrote an explicitly managed operation")
			}
		}
		resp = httpGet(t, service+"/apis/Example/operations"+apimAPIVersion)
		if len(decodeJSON(t, resp)["value"].([]any)) != 2 {
			t.Fatal("API import duplicated/lost an operation")
		}
	}
	resp = httpPut(t, service+"/apis/Example;rev=2"+apimAPIVersion, apimAPIBody)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()
	resp = httpGet(t, service+"/apis/Example;rev=2/operations/sample"+apimAPIVersion)
	assertStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
	resp = httpDelete(t, api+apimAPIVersion)
	assertStatus(t, resp, http.StatusNoContent)
	resp.Body.Close()
	// Recreating the revision must not resurrect its deleted children.
	resp = httpPut(t, api+apimAPIVersion, apimAPIBody)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()
	resp = httpGet(t, api+"/operations"+apimAPIVersion)
	if len(decodeJSON(t, resp)["value"].([]any)) != 0 {
		t.Fatal("cascaded operations survived API recreation")
	}
}

func TestAPIMAPIOperation_ImportedRefreshAndDelete(t *testing.T) {
	api, _, _ := newAPIMOperationParent(t)
	importDocument := func(document string) {
		t.Helper()
		body, err := json.Marshal(map[string]any{"properties": map[string]any{
			"path": "example", "format": "swagger-json", "value": document,
		}})
		if err != nil {
			t.Fatal(err)
		}
		resp := httpPut(t, api+apimAPIVersion, string(body))
		assertStatus(t, resp, http.StatusOK)
		resp.Body.Close()
	}
	importDocument(apimSwaggerTest)
	resp := httpHead(t, api+"/operations/getPet"+apimAPIVersion)
	assertStatus(t, resp, http.StatusNoContent)
	if readBody(t, resp) != "" {
		t.Fatal("imported operation HEAD returned a body")
	}
	resp = httpGet(t, api+"/operations/getPet"+apimAPIVersion)
	assertStatus(t, resp, http.StatusOK)
	if decodeJSON(t, resp)["properties"].(map[string]any)["displayName"] != "Get a pet" {
		t.Fatal("projected operation reads regressed")
	}
	resp = httpDelete(t, api+"/operations/getPet"+apimAPIVersion)
	assertStatus(t, resp, http.StatusNoContent)
	resp.Body.Close()
	resp = httpGet(t, api+"/operations/getPet"+apimAPIVersion)
	assertStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
	// Reads do not recreate projections. An explicit re-import does.
	importDocument(apimSwaggerTest)
	resp = httpGet(t, api+"/operations/getPet"+apimAPIVersion)
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()
	resp = httpPut(t, api+"/operations/sample"+apimAPIVersion, apimOperationBody)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()
	importDocument(`{"swagger":"2.0","info":{"title":"Empty","version":"1"},"paths":{}}`)
	resp = httpGet(t, api+"/operations"+apimAPIVersion)
	assertStatus(t, resp, http.StatusOK)
	items := decodeJSON(t, resp)["value"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["name"] != "sample" {
		t.Fatalf("re-import must remove stale projections but retain explicit operations: %v", items)
	}
}

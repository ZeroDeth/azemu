package arm

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const apimAPIVersion = "?api-version=2022-08-01"
const apimAPIBody = `{"properties":{"path":"example","displayName":"Example API","protocols":["https"],"subscriptionRequired":true,"authenticationSettings":{},"apiRevisionDescription":"","apiVersionDescription":"","type":"http","apiType":"http"}}`
const apimSwaggerTest = `{"swagger":"2.0","info":{"title":"Imported API","version":"1.0","description":"Imported description","contact":{"email":"api@example.test"},"license":{"name":"Example license","url":"https://example.test/license"},"termsOfService":"https://example.test/terms"},"host":"api.example.test","basePath":"/v2","schemes":["https","http"],"paths":{"/pets/{id}":{"get":{"operationId":"getPet","summary":"Get a pet","parameters":[{"in":"path","name":"id","required":true,"type":"integer"}],"responses":{"200":{"description":"A pet","schema":{"$ref":"#/definitions/Pet"}}}}}},"definitions":{"Pet":{"type":"object","properties":{"id":{"type":"integer"}}}}}`

func newAPIMAPIParent(t *testing.T) (string, string) {
	t.Helper()
	srv := newTestServer(t)
	rg := srv.URL + "/subscriptions/sub1/resourceGroups/rg1"
	base := rg + "/providers/Microsoft.ApiManagement/service/example-apim"
	resp := httpPut(t, rg, `{"location":"australiaeast"}`)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()
	resp = httpPut(t, base, apimBasicBody)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()
	return base, rg
}

func TestAPIMAPI_Lifecycle(t *testing.T) {
	base, _ := newAPIMAPIParent(t)
	api := base + "/apis/Example;rev=1"
	for _, status := range []int{http.StatusCreated, http.StatusOK} {
		resp := httpPut(t, api+apimAPIVersion, apimAPIBody)
		assertStatus(t, resp, status)
		for _, header := range []string{"x-ms-request-id", "x-ms-correlation-request-id"} {
			if resp.Header.Get(header) == "" {
				t.Fatalf("missing %s", header)
			}
		}
		body := decodeJSON(t, resp)
		props := body["properties"].(map[string]any)
		if body["type"] != apimAPIType || body["name"] != "Example;rev=1" ||
			props["apiRevision"] != "1" || props["isCurrent"] != true ||
			props["isOnline"] != true || props["subscriptionRequired"] != true {
			t.Fatalf("invalid API shape/defaults: %v", body)
		}
		keys := props["subscriptionKeyParameterNames"].(map[string]any)
		if keys["header"] != "Ocp-Apim-Subscription-Key" || keys["query"] != "subscription-key" {
			t.Fatalf("invalid subscription key defaults: %v", keys)
		}
	}
	for _, path := range []string{api, base + "/apis/Example"} {
		resp := httpGet(t, path+apimAPIVersion)
		assertStatus(t, resp, http.StatusOK)
		if decodeJSON(t, resp)["properties"].(map[string]any)["displayName"] != "Example API" {
			t.Fatal("API read did not return properties")
		}
		resp = httpHead(t, path+apimAPIVersion)
		assertStatus(t, resp, http.StatusNoContent)
		if readBody(t, resp) != "" {
			t.Fatal("HEAD returned a body")
		}
	}
	resp := httpGet(t, base+"/apis"+apimAPIVersion)
	assertStatus(t, resp, http.StatusOK)
	if len(decodeJSON(t, resp)["value"].([]any)) != 1 {
		t.Fatal("API not in list")
	}
	updated := strings.ReplaceAll(apimAPIBody, `"Example API"`, `"Updated API"`)
	resp = httpPut(t, api+apimAPIVersion, updated)
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()
	resp = httpGet(t, api+apimAPIVersion)
	if decodeJSON(t, resp)["properties"].(map[string]any)["displayName"] != "Updated API" {
		t.Fatal("update not persisted")
	}
	// Unlike generic ARM resources, the exact APIM SDK Delete contract is
	// synchronous and accepts only 200/204.
	resp = httpDelete(t, api+apimAPIVersion)
	assertStatus(t, resp, http.StatusNoContent)
	resp.Body.Close()
	resp = httpGet(t, api+apimAPIVersion)
	assertStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
	resp = httpHead(t, api+apimAPIVersion)
	assertStatus(t, resp, http.StatusNotFound)
	if readBody(t, resp) != "" {
		t.Fatal("missing HEAD returned a body")
	}
	resp = httpDelete(t, api+apimAPIVersion)
	assertStatus(t, resp, http.StatusNoContent)
	resp.Body.Close()
}

func TestAPIMAPI_ParentAndVersion(t *testing.T) {
	srv := newTestServer(t)
	url := srv.URL + "/subscriptions/sub1/resourceGroups/rg/providers/Microsoft.ApiManagement/service/missing/apis/api;rev=1"
	resp := httpPut(t, url+apimAPIVersion, apimAPIBody)
	assertStatus(t, resp, http.StatusNotFound)
	if decodeJSON(t, resp)["error"].(map[string]any)["code"] != "ParentResourceNotFound" {
		t.Fatal("incorrect missing parent error")
	}
	resp = httpGetRaw(t, url)
	assertStatus(t, resp, http.StatusBadRequest)
	if decodeJSON(t, resp)["error"].(map[string]any)["code"] != "MissingApiVersionParameter" {
		t.Fatal("missing API version was not rejected by middleware")
	}
}

func TestAPIMAPI_ProviderServiceIdentityNone(t *testing.T) {
	base, _ := newAPIMAPIParent(t)
	body := strings.Replace(apimBasicBody, `{"location"`, `{"identity":{"type":"None"},"location"`, 1)
	resp := httpPut(t, base, body)
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()
	for _, tc := range []struct {
		identity string
		status   int
	}{
		{`{"type":"SystemAssigned"}`, http.StatusNotImplemented},
		{`{"type":"UserAssigned","userAssignedIdentities":{"id":{}}}`, http.StatusBadRequest},
	} {
		body := strings.Replace(apimBasicBody, `{"location"`, `{"identity":`+tc.identity+`,"location"`, 1)
		resp := httpPut(t, base, body)
		assertStatus(t, resp, tc.status)
		resp.Body.Close()
	}
}

func TestAPIMAPI_InvalidAndUnsupported(t *testing.T) {
	base, _ := newAPIMAPIParent(t)
	for _, tc := range []struct {
		name   string
		body   string
		status int
	}{
		{"malformed", `{`, 400},
		{"missing properties", `{}`, 400},
		{"missing path", `{"properties":{"displayName":"API","protocols":["https"]}}`, 400},
		{"missing display", `{"properties":{"path":"x","protocols":["https"]}}`, 400},
		{"invalid protocols", `{"properties":{"path":"x","protocols":["ftp"]}}`, 400},
		{"invalid subscription", `{"properties":{"path":"x","subscriptionRequired":"true"}}`, 400},
		{"revision mismatch", `{"properties":{"path":"x","apiRevision":"2"}}`, 400},
		{"invalid contact", `{"properties":{"path":"x","contact":{"email":3}}}`, 400},
		{"private key", `{"properties":{"path":"x","_importDocument":{}}}`, 501},
		{"unsupported type", `{"properties":{"path":"x","type":"soap"}}`, 501},
		{"unsupported clone", `{"properties":{"path":"x","sourceApiId":"other"}}`, 501},
		{"unsupported version set", `{"properties":{"path":"x","apiVersionSetId":"other"}}`, 501},
		{"unsupported auth", `{"properties":{"path":"x","authenticationSettings":{"oAuth2":{}}}}`, 501},
		{"value without format", `{"properties":{"path":"x","value":"{}"}}`, 400},
		{"unsupported import", `{"properties":{"path":"x","format":"wsdl","value":"<x/>"}}`, 501},
		{"bad Swagger", `{"properties":{"path":"x","format":"swagger-json","value":"{}"}}`, 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			url := base + "/apis/invalid;rev=1" + apimAPIVersion
			resp := httpPut(t, url, tc.body)
			assertStatus(t, resp, tc.status)
			if decodeJSON(t, resp)["error"].(map[string]any)["code"] == "" {
				t.Fatal("missing Azure error code")
			}
			resp = httpGet(t, url)
			assertStatus(t, resp, http.StatusNotFound)
			resp.Body.Close()
		})
	}
}

func TestAPIMAPI_SwaggerLinkAndTwoStagePUT(t *testing.T) {
	base, _ := newAPIMAPIParent(t)
	requests := 0
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		fmt.Fprint(w, apimSwaggerTest)
	}))
	defer source.Close()
	api := base + "/apis/example;rev=1" + apimAPIVersion
	body, err := json.Marshal(map[string]any{"properties": map[string]any{
		"path": "example", "type": "http", "apiType": "http",
		"format": "swagger-link-json", "value": source.URL,
	}})
	if err != nil {
		t.Fatal(err)
	}
	resp := httpPut(t, api, string(body))
	assertStatus(t, resp, http.StatusCreated)
	props := decodeJSON(t, resp)["properties"].(map[string]any)
	if props["displayName"] != "Imported API" || props["serviceUrl"] != "https://api.example.test/v2" ||
		props["description"] != "Imported description" || props["termsOfServiceUrl"] != "https://example.test/terms" ||
		props["contact"].(map[string]any)["email"] != "api@example.test" ||
		props["license"].(map[string]any)["name"] != "Example license" {
		t.Fatalf("missing imported metadata: %v", props)
	}
	for _, key := range []string{"value", "format", "_importDocument"} {
		if _, ok := props[key]; ok {
			t.Fatalf("import internals leaked: %s", key)
		}
	}
	// This is the second request made by AzureRM 5.14 after import.
	resp = httpPut(t, api, apimAPIBody)
	assertStatus(t, resp, http.StatusOK)
	props = decodeJSON(t, resp)["properties"].(map[string]any)
	if props["displayName"] != "Example API" || props["description"] != "Imported description" ||
		props["termsOfServiceUrl"] != "https://example.test/terms" ||
		props["serviceUrl"] != "https://api.example.test/v2" {
		t.Fatalf("second PUT lost import metadata: %v", props)
	}
	for _, key := range []string{"contact", "license"} {
		if _, exists := props[key]; exists {
			t.Fatalf("ordinary PUT retained omitted %s: %v", key, props[key])
		}
	}
	// Exact AzureRM 5.14 UPDATE body observed on the following apply:
	// changed optional strings become non-nil pointers to "", while
	// removed contact/license blocks become nil pointers and are omitted.
	update := `{"properties":{"apiRevisionDescription":"","apiType":"http","apiVersionDescription":"","description":"","displayName":"Example API","path":"example","protocols":["https"],"serviceUrl":"https://api.example.test/v2","subscriptionKeyParameterNames":{"header":"Ocp-Apim-Subscription-Key","query":"subscription-key"},"subscriptionRequired":true,"termsOfServiceUrl":"","type":"http"}}`
	for _, body := range []string{update, apimAPIBody} {
		resp = httpPut(t, api, body)
		assertStatus(t, resp, http.StatusOK)
		resp.Body.Close()
		resp = httpGet(t, api)
		assertStatus(t, resp, http.StatusOK)
		props = decodeJSON(t, resp)["properties"].(map[string]any)
		if props["description"] != "" || props["termsOfServiceUrl"] != "" {
			t.Fatalf("cleared metadata was resurrected from import: %v", props)
		}
		for _, key := range []string{"contact", "license"} {
			if _, exists := props[key]; exists {
				t.Fatalf("cleared %s was resurrected from import", key)
			}
		}
	}
	if requests != 1 {
		t.Fatalf("import fetched %d times", requests)
	}
	resp = httpGet(t, base+"/apis/example/operations"+apimAPIVersion)
	assertStatus(t, resp, http.StatusOK)
	if len(decodeJSON(t, resp)["value"].([]any)) != 1 {
		t.Fatal("import did not materialize operations")
	}
	resp = httpGet(t, base+"/apis/example/operations/getPet"+apimAPIVersion)
	op := decodeJSON(t, resp)["properties"].(map[string]any)
	if op["method"] != "GET" || op["urlTemplate"] != "/pets/{id}" {
		t.Fatalf("invalid imported operation: %v", op)
	}
	resp = httpGet(t, base+"/apis/example/schemas/swagger-definitions"+apimAPIVersion)
	assertStatus(t, resp, http.StatusOK)
	if !strings.Contains(decodeJSON(t, resp)["properties"].(map[string]any)["document"].(map[string]any)["value"].(string), `"Pet"`) {
		t.Fatal("imported schemas not retained")
	}
}

func TestAPIMAPI_SwaggerDefinitionReferences(t *testing.T) {
	for _, tc := range []struct {
		name, reference string
	}{
		{"Pet/Details", "Pet~1Details"},
		{"Pet~Details", "Pet~0Details"},
		{"Pet~1Details", "Pet~01Details"},
	} {
		t.Run(tc.reference, func(t *testing.T) {
			base, _ := newAPIMAPIParent(t)
			swagger := strings.NewReplacer(
				`"Pet":`, fmt.Sprintf("%q:", tc.name),
				"#/definitions/Pet", "#/definitions/"+tc.reference,
			).Replace(apimSwaggerTest)
			body, err := json.Marshal(map[string]any{"properties": map[string]any{
				"path": "example", "format": "swagger-json", "value": swagger,
			}})
			if err != nil {
				t.Fatal(err)
			}
			resp := httpPut(t, base+"/apis/example;rev=1"+apimAPIVersion, string(body))
			assertStatus(t, resp, http.StatusCreated)
			resp.Body.Close()
			resp = httpGet(t, base+"/apis/example/operations/getPet"+apimAPIVersion)
			assertStatus(t, resp, http.StatusOK)
			operation := decodeJSON(t, resp)["properties"].(map[string]any)
			response := operation["responses"].([]any)[0].(map[string]any)
			representation := response["representations"].([]any)[0].(map[string]any)
			if representation["typeName"] != tc.name {
				t.Fatalf("typeName = %q, want %q", representation["typeName"], tc.name)
			}
			resp = httpGet(t, base+"/apis/example/schemas/swagger-definitions"+apimAPIVersion)
			assertStatus(t, resp, http.StatusOK)
			props := decodeJSON(t, resp)["properties"].(map[string]any)
			var document struct {
				Definitions map[string]any `json:"definitions"`
			}
			if err := json.Unmarshal([]byte(props["document"].(map[string]any)["value"].(string)), &document); err != nil {
				t.Fatal(err)
			}
			if _, exists := document.Definitions[representation["typeName"].(string)]; !exists {
				t.Fatalf("projected typeName does not identify a stored definition: %v", representation)
			}
		})
	}
}

func TestAPIMAPI_ExplicitImportMetadataPrecedence(t *testing.T) {
	base, _ := newAPIMAPIParent(t)
	api := base + "/apis/example;rev=1" + apimAPIVersion
	for _, tc := range []struct {
		name        string
		description string
		terms       string
		contact     any
		license     any
	}{
		{"configured", "Explicit description", "https://example.test/explicit-terms",
			map[string]any{"name": "Explicit contact"}, map[string]any{"name": "Explicit license"}},
		{"cleared", "", "", nil, nil},
		{"empty objects", "", "", map[string]any{}, map[string]any{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body, err := json.Marshal(map[string]any{"properties": map[string]any{
				"path": "explicit", "format": "swagger-json", "value": apimSwaggerTest,
				"description": tc.description, "termsOfServiceUrl": tc.terms,
				"contact": tc.contact, "license": tc.license,
			}})
			if err != nil {
				t.Fatal(err)
			}
			resp := httpPut(t, api, string(body))
			if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
				t.Fatalf("PUT: %d %s", resp.StatusCode, readBody(t, resp))
			}
			resp.Body.Close()
			resp = httpGet(t, api)
			assertStatus(t, resp, http.StatusOK)
			props := decodeJSON(t, resp)["properties"].(map[string]any)
			if props["description"] != tc.description || props["termsOfServiceUrl"] != tc.terms {
				t.Fatalf("explicit strings did not override import: %v", props)
			}
			for key, want := range map[string]any{"contact": tc.contact, "license": tc.license} {
				if want == nil {
					if _, exists := props[key]; exists {
						t.Fatalf("explicit null did not clear imported %s", key)
					}
				} else {
					gotJSON, err := json.Marshal(props[key])
					if err != nil {
						t.Fatal(err)
					}
					wantJSON, err := json.Marshal(want)
					if err != nil {
						t.Fatal(err)
					}
					if string(gotJSON) != string(wantJSON) {
						t.Fatalf("%s = %s, want %s", key, gotJSON, wantJSON)
					}
				}
			}
		})
	}
}

func TestAPIMAPI_RevisionsAndCascade(t *testing.T) {
	for _, parent := range []string{"service", "rg"} {
		t.Run(parent, func(t *testing.T) {
			base, rg := newAPIMAPIParent(t)
			for _, revision := range []string{"1", "2"} {
				resp := httpPut(t, base+"/apis/example;rev="+revision+apimAPIVersion, apimAPIBody)
				assertStatus(t, resp, http.StatusCreated)
				props := decodeJSON(t, resp)["properties"].(map[string]any)
				if props["apiRevision"] != revision || props["isCurrent"] != (revision == "1") {
					t.Fatalf("incorrect revision semantics: %v", props)
				}
			}
			resp := httpGet(t, base+"/apis/example/revisions"+apimAPIVersion)
			assertStatus(t, resp, http.StatusOK)
			if len(decodeJSON(t, resp)["value"].([]any)) != 2 {
				t.Fatal("missing revision")
			}
			resp = httpDelete(t, base+"/apis/example;rev=2"+apimAPIVersion)
			assertStatus(t, resp, http.StatusNoContent)
			resp.Body.Close()
			resp = httpGet(t, base+"/apis/example"+apimAPIVersion)
			assertStatus(t, resp, http.StatusOK)
			resp.Body.Close()
			target := base
			if parent == "rg" {
				target = rg
			}
			resp = httpDelete(t, target)
			assertStatus(t, resp, http.StatusAccepted)
			resp.Body.Close()
			resp = httpGet(t, base+"/apis/example;rev=1"+apimAPIVersion)
			assertStatus(t, resp, http.StatusNotFound)
			resp.Body.Close()
		})
	}
}

func TestAPIMAPI_ImportFailures(t *testing.T) {
	base, _ := newAPIMAPIParent(t)
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer source.Close()
	for _, tc := range []struct{ format, value string }{
		{"swagger-link-json", source.URL},
		{"swagger-link-json", "file:///etc/passwd"},
		{"swagger-json", `{`},
		{"swagger-json", strings.Replace(apimSwaggerTest, "#/definitions/Pet", "https://example.test/schema", 1)},
		{"swagger-json", strings.Replace(apimSwaggerTest, "#/definitions/Pet", "#/definitions/Unknown", 1)},
		{"swagger-json", strings.Repeat("x", apimImportLimit+1)},
	} {
		body, err := json.Marshal(map[string]any{"properties": map[string]any{"path": "x", "format": tc.format, "value": tc.value}})
		if err != nil {
			t.Fatal(err)
		}
		resp := httpPut(t, base+"/apis/failure;rev=1"+apimAPIVersion, string(body))
		assertStatus(t, resp, http.StatusBadRequest)
		resp.Body.Close()
		resp = httpGet(t, base+"/apis/failure;rev=1"+apimAPIVersion)
		assertStatus(t, resp, http.StatusNotFound)
		resp.Body.Close()
	}
}

func TestAPIMAPI_ReimportReplacesChildren(t *testing.T) {
	base, _ := newAPIMAPIParent(t)
	api := base + "/apis/example;rev=1" + apimAPIVersion
	for i, swagger := range []string{
		apimSwaggerTest,
		strings.ReplaceAll(apimSwaggerTest, `"getPet"`, `"readPet"`),
	} {
		body, err := json.Marshal(map[string]any{"properties": map[string]any{
			"path": "", "format": "swagger-json", "value": swagger,
		}})
		if err != nil {
			t.Fatal(err)
		}
		resp := httpPut(t, api, string(body))
		status := http.StatusCreated
		if i != 0 {
			status = http.StatusOK
		}
		assertStatus(t, resp, status)
		resp.Body.Close()
	}
	resp := httpGet(t, base+"/apis/example/operations/getPet"+apimAPIVersion)
	assertStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
	resp = httpGet(t, base+"/apis/example/operations/readPet"+apimAPIVersion)
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()
	resp = httpDelete(t, api)
	assertStatus(t, resp, http.StatusNoContent)
	resp.Body.Close()
	for _, path := range []string{"/operations/readPet", "/schemas/swagger-definitions"} {
		resp := httpGet(t, base+"/apis/example"+path+apimAPIVersion)
		assertStatus(t, resp, http.StatusNotFound)
		resp.Body.Close()
	}
}

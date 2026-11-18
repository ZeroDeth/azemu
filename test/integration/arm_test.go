//go:build integration

// Package integration exercises the ARM router through the same middleware
// stack used by cmd/azemu/main.go, but in-process via httptest. This is the
// closest we get to end-to-end coverage without requiring a real TCP listener,
// TLS trust, or a running Terraform CLI.
//
// Tests in this package are guarded by the `integration` build tag so they
// are not picked up by a bare `go test ./...`. Run them with:
//
//	go test ./test/integration/... -tags=integration -race -count=1
package integration

import (
	"bytes"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/zerodeth/azemu/internal/arm"
	mw "github.com/zerodeth/azemu/internal/middleware"
	"github.com/zerodeth/azemu/internal/store"
)

// buildFullServer assembles a chi router with the same middleware stack as
// cmd/azemu/main.go (minus TLS and OAuth, which are orthogonal to ARM
// routing) and wraps it in an httptest.Server. Each call produces a fresh
// MemoryStore so tests are isolated.
func buildFullServer(t *testing.T) *httptest.Server {
	t.Helper()
	s := store.NewMemoryStore()
	ar := arm.NewRouter(s, "http://azurite-test:10000", "https://kv-test", "redis://redis-test:6379")
	r := chi.NewRouter()
	r.Use(mw.NormalizePath)
	r.Use(mw.AzureHeaders)
	r.Use(mw.RequireAPIVersion)
	r.Route("/subscriptions", ar.Routes)
	r.Route("/keyvault", ar.KeyVaultDataPlaneRoutes)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv
}

const apiVersionQ = "?api-version=2023-09-01"

func TestARM_APIMCustomDomainCertificateLifecycle(t *testing.T) {
	srv := buildFullServer(t)
	version := "?api-version=2024-05-01"
	rg := srv.URL + "/subscriptions/sub1/resourceGroups/custom-domain-rg"
	apim := rg + "/providers/Microsoft.ApiManagement/service/example"
	mi := rg + "/providers/Microsoft.ManagedIdentity/userAssignedIdentities/mi1"
	vault := rg + "/providers/Microsoft.KeyVault/vaults/domain-vault"
	certURL := srv.URL + "/keyvault/domain-vault/certificates/example"
	for _, step := range []struct{ url, body string }{
		{rg, `{"location":"uksouth"}`},
		{mi, `{"location":"uksouth"}`},
		{vault, `{"location":"uksouth","properties":{"tenantId":"00000000-0000-0000-0000-000000000001"}}`},
	} {
		resp := doJSON(t, http.MethodPut, step.url+version, step.body)
		mustStatus(t, resp, 201)
		resp.Body.Close()
	}
	identity := decode(t, doJSON(t, http.MethodGet, mi+version, ""))
	identityProps := identity["properties"].(map[string]any)
	body, _ := json.Marshal(map[string]any{
		"location": "uksouth", "sku": map[string]any{"name": "Developer", "capacity": 1},
		"identity": map[string]any{"type": "UserAssigned", "userAssignedIdentities": map[string]any{identity["id"].(string): map[string]any{}}},
		"properties": map[string]any{"publisherName": "Example", "publisherEmail": "example@example.test",
			"customProperties": map[string]any{"Microsoft.WindowsAzure.ApiManagement.Gateway.Protocols.Server.Http2": "true"}},
	})
	resp := doJSON(t, http.MethodPut, apim+version, string(body))
	mustStatus(t, resp, 201)
	resp.Body.Close()
	body, _ = json.Marshal(map[string]any{"properties": map[string]any{"accessPolicies": []any{
		map[string]any{"objectId": identityProps["principalId"], "tenantId": identityProps["tenantId"],
			"permissions": map[string]any{"secrets": []string{"Get", "List"}}},
	}}})
	resp = doJSON(t, http.MethodPut, vault+"/accessPolicies/add"+version, string(body))
	mustStatus(t, resp, 200)
	resp.Body.Close()
	resp = doJSON(t, http.MethodPost, certURL+"/create?api-version=7.4", `{"policy":{
		"issuer":{"name":"Self"},"key_props":{"kty":"RSA","key_size":2048,"exportable":true,"reuse_key":true},
		"secret_props":{"contentType":"application/x-pkcs12"},
		"x509_props":{"subject":"CN=api.example.com","validity_months":12,"key_usage":["digitalSignature","keyEncipherment"],
		"sans":{"dns_names":["api.example.com","portal.example.com"]}}}}`)
	mustStatus(t, resp, 202)
	resp.Body.Close()
	cert := decode(t, doJSON(t, http.MethodGet, certURL+"?api-version=7.4", ""))
	der, err := base64.StdEncoding.DecodeString(cert["cer"].(string))
	if err != nil {
		t.Fatal(err)
	}
	if len(der) == 0 || cert["sid"] == nil {
		t.Fatal("certificate was not generated with a backing secret")
	}
	configs := []any{}
	for _, endpoint := range []struct{ kind, host string }{{"Proxy", "api.example.com"}, {"DeveloperPortal", "portal.example.com"}} {
		configs = append(configs, map[string]any{"type": endpoint.kind, "hostName": endpoint.host,
			"keyVaultId": cert["sid"], "identityClientId": identityProps["clientId"]})
	}
	body, _ = json.Marshal(map[string]any{"properties": map[string]any{"hostnameConfigurations": configs}})
	resp = doJSON(t, http.MethodPatch, apim+version, string(body))
	mustStatus(t, resp, 200)
	configured := decode(t, resp)
	hosts := configured["properties"].(map[string]any)["hostnameConfigurations"].([]any)
	if len(hosts) != 3 || hosts[0].(map[string]any)["certificateSource"] != "KeyVault" {
		t.Fatal("hostnames did not round-trip")
	}
	configured["properties"].(map[string]any)["hostnameConfigurations"] = nil
	body, _ = json.Marshal(configured)
	resp = doJSON(t, http.MethodPut, apim+version, string(body))
	mustStatus(t, resp, 200)
	cleared := decode(t, resp)
	props := cleared["properties"].(map[string]any)
	if len(props["hostnameConfigurations"].([]any)) != 1 || props["publisherName"] != "Example" ||
		cleared["identity"].(map[string]any)["type"] != "UserAssigned" {
		t.Fatal("clearing domains damaged the shared APIM resource")
	}
	resp = doJSON(t, http.MethodDelete, certURL+"?api-version=7.4", "")
	mustStatus(t, resp, 200)
	resp.Body.Close()
	resp = doJSON(t, http.MethodGet, certURL+"?api-version=7.4", "")
	mustStatus(t, resp, 404)
	resp.Body.Close()
	resp = doJSON(t, http.MethodDelete, rg+version, "")
	mustStatus(t, resp, 202)
	resp.Body.Close()
	resp = doJSON(t, http.MethodGet, apim+version, "")
	mustStatus(t, resp, 404)
	resp.Body.Close()
}

func TestARM_APIMAPIPolicyLifecycle(t *testing.T) {
	srv := buildFullServer(t)
	rg := srv.URL + "/subscriptions/sub1/resourceGroups/api-policy-rg"
	service := rg + "/providers/Microsoft.ApiManagement/service/apim-test"
	api := service + "/apis/example;rev=1"
	alias := service + "/apis/example"
	version := "?api-version=2022-08-01"
	policy := alias + "/policies/policy" + version
	document := "<policies>\n  <inbound>\n    <find-and-replace from=\"xyz\" to=\"abc\" />\n  </inbound>\n</policies>\n"
	for _, step := range []struct {
		url, body string
	}{
		{rg, `{"location":"australiaeast"}`},
		{service, `{"location":"australiaeast","sku":{"name":"Developer","capacity":1},"properties":{"publisherName":"Example","publisherEmail":"example@example.test"}}`},
		{api, `{"properties":{"path":"example","displayName":"Example","protocols":["https"]}}`},
	} {
		resp := doJSON(t, http.MethodPut, step.url+version, step.body)
		mustStatus(t, resp, 201)
		resp.Body.Close()
	}
	for i, value := range []string{document, strings.ReplaceAll(document, `to="abc"`, `to="updated"`)} {
		body, err := json.Marshal(map[string]any{"properties": map[string]any{"format": "rawxml", "value": value}})
		if err != nil {
			t.Fatal(err)
		}
		resp := doJSON(t, http.MethodPut, policy, string(body))
		status := 201
		if i == 1 {
			status = 200
		}
		mustStatus(t, resp, status)
		resp.Body.Close()
		for _, path := range []string{alias, api} {
			resp = doJSON(t, http.MethodGet, path+"/policies/policy"+version+"&format=xml", "")
			mustStatus(t, resp, 200)
			if decode(t, resp)["properties"].(map[string]any)["value"] != value {
				t.Fatal("API policy XML did not round-trip")
			}
		}
	}
	resp := doJSON(t, http.MethodGet, service+"/policies/policy"+version, "")
	mustStatus(t, resp, 404)
	resp.Body.Close()
	resp = doJSON(t, http.MethodDelete, policy, "")
	mustStatus(t, resp, 204)
	resp.Body.Close()
	resp = doJSON(t, http.MethodGet, policy+"&format=xml", "")
	mustStatus(t, resp, 404)
	resp.Body.Close()
}

func TestARM_APIMDiagnosticsLifecycle(t *testing.T) {
	srv := buildFullServer(t)
	rg := srv.URL + "/subscriptions/sub1/resourceGroups/diagnostics-rg"
	service := rg + "/providers/Microsoft.ApiManagement/service/apim-test"
	component := rg + "/providers/Microsoft.Insights/components/ai1"
	logger := service + "/loggers/logger1"
	api := service + "/apis/example;rev=1"
	diagnostic := service + "/apis/example/diagnostics/applicationinsights"
	for _, step := range []struct {
		url, body, version string
		status             int
	}{
		{rg, `{"location":"uksouth"}`, apiVersionQ, 201},
		{service, `{"location":"uksouth","sku":{"name":"Developer","capacity":1},"properties":{"publisherName":"Example","publisherEmail":"example@example.test"}}`, "?api-version=2024-05-01", 201},
		{component, `{"location":"UK South","kind":"web","properties":{"Application_Type":"web"}}`, "?api-version=2020-02-02", 200},
		{api, `{"properties":{"path":"example","displayName":"Example","protocols":["https"]}}`, "?api-version=2022-08-01", 201},
	} {
		resp := doJSON(t, http.MethodPut, step.url+step.version, step.body)
		mustStatus(t, resp, step.status)
		resp.Body.Close()
	}
	resp := doJSON(t, http.MethodGet, component+"?api-version=2020-02-02", "")
	mustStatus(t, resp, 200)
	key := decode(t, resp)["properties"].(map[string]any)["InstrumentationKey"].(string)
	loggerBody := `{"properties":{"loggerType":"applicationInsights","credentials":{"instrumentationKey":"` + key + `"},"isBuffered":true}}`
	resp = doJSON(t, http.MethodPut, logger+"?api-version=2022-08-01", loggerBody)
	mustStatus(t, resp, 201)
	resp.Body.Close()
	loggerID := strings.TrimPrefix(logger, srv.URL)
	diagnosticBody := `{"properties":{"loggerId":"` + loggerID + `","sampling":{"samplingType":"fixed","percentage":5},"alwaysLog":"allErrors","logClientIp":true,"httpCorrelationProtocol":"W3C","verbosity":"verbose","frontend":{"request":{"body":{"bytes":32},"headers":["content-type"]}}}}`
	resp = doJSON(t, http.MethodPut, diagnostic+"?api-version=2022-08-01", diagnosticBody)
	mustStatus(t, resp, 201)
	resp.Body.Close()
	for _, step := range []struct {
		method, url, body string
	}{
		{http.MethodPut, component + "?api-version=2020-02-02", `{"location":"uksouth","kind":"web","tags":{"updated":"yes"},"properties":{"Application_Type":"web","SamplingPercentage":25}}`},
		{http.MethodPatch, logger + "?api-version=2022-08-01", `{"properties":{"isBuffered":false,"description":"updated"}}`},
		{http.MethodPut, diagnostic + "?api-version=2022-08-01", strings.Replace(diagnosticBody, `"percentage":5`, `"percentage":10`, 1)},
		{http.MethodPut, component + "/currentBillingFeatures?api-version=2015-05-01", `{"CurrentBillingFeatures":["Basic"],"DataVolumeCap":{"Cap":42,"StopSendNotificationWhenHitCap":true}}`},
	} {
		resp = doJSON(t, step.method, step.url, step.body)
		mustStatus(t, resp, 200)
		resp.Body.Close()
	}
	resp = doJSON(t, http.MethodGet, component+"?api-version=2020-02-02", "")
	result := decode(t, resp)
	if result["location"] != "uksouth" || result["properties"].(map[string]any)["SamplingPercentage"] != float64(25) {
		t.Fatal("Application Insights update lost")
	}
	resp = doJSON(t, http.MethodGet, logger+"?api-version=2022-08-01", "")
	if decode(t, resp)["properties"].(map[string]any)["isBuffered"] != false {
		t.Fatal("logger update lost")
	}
	resp = doJSON(t, http.MethodGet, diagnostic+"?api-version=2022-08-01", "")
	if decode(t, resp)["properties"].(map[string]any)["sampling"].(map[string]any)["percentage"] != float64(10) {
		t.Fatal("diagnostic update lost")
	}
	for _, url := range []string{diagnostic + "?api-version=2022-08-01", logger + "?api-version=2022-08-01", component + "?api-version=2020-02-02"} {
		resp = doJSON(t, http.MethodDelete, url, "")
		mustStatus(t, resp, 204)
		resp.Body.Close()
		resp = doJSON(t, http.MethodGet, url, "")
		mustStatus(t, resp, 404)
		resp.Body.Close()
	}
}

func doJSON(t *testing.T, method, url, body string) *http.Response {
	t.Helper()
	var rdr io.Reader
	if body != "" {
		rdr = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, url, rdr)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do %s %s: %v", method, url, err)
	}
	return resp
}

func mustStatus(t *testing.T, resp *http.Response, want int) {
	t.Helper()
	if resp.StatusCode != want {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("status = %d, want %d. body=%s", resp.StatusCode, want, string(b))
	}
}

func decode(t *testing.T, resp *http.Response) map[string]interface{} {
	t.Helper()
	defer resp.Body.Close()
	var m map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		buf := &bytes.Buffer{}
		buf.ReadFrom(resp.Body)
		t.Fatalf("decode json: %v", err)
	}
	return m
}

func TestARM_APIMBackendLifecycle(t *testing.T) {
	srv := buildFullServer(t)
	rg := srv.URL + "/subscriptions/sub1/resourceGroups/apim-backend-rg"
	service := rg + "/providers/Microsoft.ApiManagement/service/apim-test"
	resp := doJSON(t, http.MethodPut, rg+apiVersionQ, `{"location":"uksouth"}`)
	mustStatus(t, resp, http.StatusCreated)
	resp.Body.Close()
	resp = doJSON(t, http.MethodPut, service+"?api-version=2024-05-01",
		`{"location":"uksouth","sku":{"name":"Developer","capacity":1},"properties":{"publisherName":"Example","publisherEmail":"example@example.test"}}`)
	mustStatus(t, resp, http.StatusCreated)
	resp.Body.Close()
	backend := service + "/backends/example?api-version=2024-05-01"
	resp = doJSON(t, http.MethodPut, backend, `{"properties":{"protocol":"http","url":"https://backend"}}`)
	mustStatus(t, resp, http.StatusCreated)
	resp.Body.Close()
	resp = doJSON(t, http.MethodPatch, backend, `{"properties":{"url":"https://changed.example.test"}}`)
	mustStatus(t, resp, http.StatusOK)
	resp.Body.Close()
	resp = doJSON(t, http.MethodGet, backend, "")
	mustStatus(t, resp, http.StatusOK)
	if decode(t, resp)["properties"].(map[string]any)["url"] != "https://changed.example.test" {
		t.Fatal("backend update was not persisted")
	}
	resp = doJSON(t, http.MethodDelete, backend, "")
	mustStatus(t, resp, http.StatusNoContent)
	resp.Body.Close()
	resp = doJSON(t, http.MethodGet, backend, "")
	mustStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
}

func TestARM_APIMBasicLifecycle(t *testing.T) {
	srv := buildFullServer(t)
	rg := srv.URL + "/subscriptions/sub1/resourceGroups/apim-rg"
	service := rg + "/providers/Microsoft.ApiManagement/service/apim-test"
	resp := doJSON(t, http.MethodPut, rg+apiVersionQ, `{"location":"uksouth"}`)
	mustStatus(t, resp, http.StatusCreated)
	resp.Body.Close()
	resp = doJSON(t, http.MethodPut, service+"?api-version=2024-05-01",
		`{"location":"uksouth","sku":{"name":"Developer","capacity":1},"properties":{"publisherName":"Example","publisherEmail":"example@example.test"}}`)
	mustStatus(t, resp, http.StatusCreated)
	resp.Body.Close()
	resp = doJSON(t, http.MethodPatch, service+"?api-version=2024-05-01", `{"tags":{"updated":"yes"}}`)
	mustStatus(t, resp, http.StatusOK)
	resp.Body.Close()
	resp = doJSON(t, http.MethodGet, service+"?api-version=2024-05-01", "")
	mustStatus(t, resp, http.StatusOK)
	if decode(t, resp)["tags"].(map[string]any)["updated"] != "yes" {
		t.Fatal("API Management update was not persisted")
	}
	resp = doJSON(t, http.MethodDelete, service+"?api-version=2024-05-01", "")
	mustStatus(t, resp, http.StatusAccepted)
	pollURL := resp.Header.Get("Azure-AsyncOperation")
	resp.Body.Close()
	resp = doJSON(t, http.MethodGet, pollURL, "")
	mustStatus(t, resp, http.StatusOK)
	if decode(t, resp)["status"] != "Succeeded" {
		t.Fatal("API Management delete polling did not complete")
	}
	deleted := srv.URL + "/subscriptions/sub1/providers/Microsoft.ApiManagement/locations/uksouth/deletedservices/apim-test?api-version=2022-08-01"
	resp = doJSON(t, http.MethodGet, deleted, "")
	mustStatus(t, resp, http.StatusOK)
	resp.Body.Close()
	resp = doJSON(t, http.MethodDelete, deleted, "")
	mustStatus(t, resp, http.StatusNoContent)
	resp.Body.Close()
	resp = doJSON(t, http.MethodGet, deleted, "")
	mustStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
}

func TestARM_APIMUserLifecycle(t *testing.T) {
	srv := buildFullServer(t)
	rg := srv.URL + "/subscriptions/sub1/resourceGroups/apim-user-rg"
	service := rg + "/providers/Microsoft.ApiManagement/service/apim-test"
	user := service + "/users/5931a75ae4bbd512288c680b"
	version := "?api-version=2022-08-01"
	for _, resource := range []struct{ url, body string }{
		{rg, `{"location":"uksouth"}`},
		{service, `{"location":"uksouth","sku":{"name":"Developer","capacity":1},"properties":{"publisherName":"Example","publisherEmail":"example@example.test"}}`},
		{user, `{"properties":{"firstName":"Example","lastName":"User","email":"tom+tfdev@example.test","state":"active"}}`},
	} {
		resp := doJSON(t, http.MethodPut, resource.url+version, resource.body)
		mustStatus(t, resp, 201)
		resp.Body.Close()
	}
	resp := doJSON(t, http.MethodGet, user+version, "")
	mustStatus(t, resp, 200)
	props := decode(t, resp)["properties"].(map[string]any)
	registered := props["registrationDate"]
	if props["firstName"] != "Example" || props["state"] != "active" || props["password"] != nil {
		t.Fatal("invalid user response")
	}
	resp = doJSON(t, http.MethodPut, user+version, `{"properties":{"firstName":"Updated","lastName":"User","email":"updated@example.test","state":"blocked","note":"managed locally"}}`)
	mustStatus(t, resp, 200)
	resp.Body.Close()
	resp = doJSON(t, http.MethodGet, user+version, "")
	props = decode(t, resp)["properties"].(map[string]any)
	if props["firstName"] != "Updated" || props["state"] != "blocked" || props["note"] != "managed locally" || props["registrationDate"] != registered {
		t.Fatal("user update did not round-trip")
	}
	resp = doJSON(t, http.MethodDelete, user+version+"&deleteSubscriptions=true&notify=false&appType=developerPortal", "")
	mustStatus(t, resp, 204)
	resp.Body.Close()
	resp = doJSON(t, http.MethodGet, user+version, "")
	mustStatus(t, resp, 404)
	resp.Body.Close()
}

func TestARM_APIMAPIOperationPolicyLifecycle(t *testing.T) {
	srv := buildFullServer(t)
	rg := srv.URL + "/subscriptions/sub1/resourceGroups/apim-operation-policy-rg"
	service := rg + "/providers/Microsoft.ApiManagement/service/apim-test"
	api := service + "/apis/Example;rev=1"
	operation := service + "/apis/Example/operations/sample"
	policy := operation + "/policies/policy"
	version := "?api-version=2022-08-01"
	for _, resource := range []struct{ url, body string }{
		{rg, `{"location":"uksouth"}`},
		{service, `{"location":"uksouth","sku":{"name":"Developer","capacity":1},"properties":{"publisherName":"Example","publisherEmail":"example@example.test"}}`},
		{api, `{"properties":{"path":"example","displayName":"Example API","protocols":["https"]}}`},
		{operation, `{"properties":{"displayName":"Sample","method":"GET","urlTemplate":"/wishlists"}}`},
		{policy, `{"properties":{"format":"rawxml","value":"<policies>\n  <inbound><find-and-replace from=\"xyz\" to=\"abc\" /></inbound>\n</policies>\n"}}`},
	} {
		resp := doJSON(t, http.MethodPut, resource.url+version, resource.body)
		mustStatus(t, resp, 201)
		resp.Body.Close()
	}
	resp := doJSON(t, http.MethodGet, api+"/operations/sample/policies/policy"+version+"&format=xml", "")
	mustStatus(t, resp, 200)
	if decode(t, resp)["properties"].(map[string]any)["value"] != "<policies>\n  <inbound><find-and-replace from=\"xyz\" to=\"abc\" /></inbound>\n</policies>\n" {
		t.Fatal("policy XML did not round-trip")
	}
	resp = doJSON(t, http.MethodPut, policy+version, `{"properties":{"format":"xml","value":"<policies><inbound><base/></inbound></policies>"}}`)
	mustStatus(t, resp, 200)
	resp.Body.Close()
	resp = doJSON(t, http.MethodDelete, policy+version, "")
	mustStatus(t, resp, 204)
	resp.Body.Close()
	resp = doJSON(t, http.MethodGet, policy+version, "")
	mustStatus(t, resp, 404)
	resp.Body.Close()
}

func TestARM_APIMAPIOperationTagLifecycle(t *testing.T) {
	srv := buildFullServer(t)
	rg := srv.URL + "/subscriptions/sub1/resourceGroups/apim-tag-rg"
	service := rg + "/providers/Microsoft.ApiManagement/service/apim-test"
	api := service + "/apis/Example;rev=1"
	operation := service + "/apis/Example/operations/sample"
	tag := service + "/tags/example-Tag"
	assignment := operation + "/tags/example-Tag"
	version := "?api-version=2022-08-01"
	for _, resource := range []struct{ url, body string }{
		{rg, `{"location":"uksouth"}`},
		{service, `{"location":"uksouth","sku":{"name":"Developer","capacity":1},"properties":{"publisherName":"Example","publisherEmail":"example@example.test"}}`},
		{api, `{"properties":{"path":"example","displayName":"Example API","protocols":["https"]}}`},
		{operation, `{"properties":{"displayName":"Sample","method":"GET","urlTemplate":"/wishlists"}}`},
		{tag, `{"properties":{"displayName":"display-name"}}`}, {assignment, ""},
	} {
		resp := doJSON(t, http.MethodPut, resource.url+version, resource.body)
		mustStatus(t, resp, 201)
		resp.Body.Close()
	}
	resp := doJSON(t, http.MethodPut, tag+version, `{"properties":{"displayName":"renamed"}}`)
	mustStatus(t, resp, 200)
	resp.Body.Close()
	resp = doJSON(t, http.MethodGet, api+"/operations/sample/tags/example-Tag"+version, "")
	mustStatus(t, resp, 200)
	body := decode(t, resp)
	if body["type"] != "Microsoft.ApiManagement/service/tags" || body["id"] != strings.TrimPrefix(tag, srv.URL) ||
		body["properties"].(map[string]any)["displayName"] != "renamed" {
		t.Fatal("assignment did not resolve updated service tag")
	}
	resp = doJSON(t, http.MethodDelete, assignment+version, "")
	mustStatus(t, resp, 204)
	resp.Body.Close()
	resp = doJSON(t, http.MethodGet, assignment+version, "")
	mustStatus(t, resp, 404)
	resp.Body.Close()
	resp = doJSON(t, http.MethodGet, tag+version, "")
	mustStatus(t, resp, 200)
	resp.Body.Close()
}

func TestARM_APIMAPIOperationLifecycle(t *testing.T) {
	srv := buildFullServer(t)
	rg := srv.URL + "/subscriptions/sub1/resourceGroups/apim-operation-rg"
	service := rg + "/providers/Microsoft.ApiManagement/service/apim-test"
	api := service + "/apis/Example;rev=1"
	version := "?api-version=2022-08-01"
	operation := service + "/apis/Example/operations/sample"
	for _, resource := range []struct{ url, body string }{
		{rg, `{"location":"uksouth"}`},
		{service, `{"location":"uksouth","sku":{"name":"Developer","capacity":1},"properties":{"publisherName":"Example","publisherEmail":"example@example.test"}}`},
		{api, `{"properties":{"path":"example","displayName":"Example API","protocols":["https"]}}`},
		{operation, `{"properties":{"displayName":"Get WishLists","method":"GET","urlTemplate":"/wishlists","request":{"headers":[{"name":"test","required":false,"type":"string","values":["true","false"],"examples":{"name":{}}}]}}}`},
	} {
		resp := doJSON(t, http.MethodPut, resource.url+version, resource.body)
		mustStatus(t, resp, http.StatusCreated)
		resp.Body.Close()
	}
	resp := doJSON(t, http.MethodGet, api+"/operations/sample"+version, "")
	mustStatus(t, resp, http.StatusOK)
	props := decode(t, resp)["properties"].(map[string]any)
	header := props["request"].(map[string]any)["headers"].([]any)[0].(map[string]any)
	if header["required"] != false || len(header["values"].([]any)) != 2 ||
		header["examples"].(map[string]any)["name"] == nil {
		t.Fatalf("nested operation contract was not preserved: %v", props)
	}
	resp = doJSON(t, http.MethodPut, operation+version, `{"properties":{"displayName":"Updated","method":"POST","urlTemplate":"/wishlists","responses":[{"statusCode":201,"description":"Created"}]}}`)
	mustStatus(t, resp, http.StatusOK)
	resp.Body.Close()
	resp = doJSON(t, http.MethodGet, operation+version, "")
	mustStatus(t, resp, http.StatusOK)
	props = decode(t, resp)["properties"].(map[string]any)
	if props["method"] != "POST" || props["request"] != nil ||
		props["responses"].([]any)[0].(map[string]any)["statusCode"] != float64(201) {
		t.Fatalf("operation update was not persisted: %v", props)
	}
	resp = doJSON(t, http.MethodDelete, operation+version, "")
	mustStatus(t, resp, http.StatusNoContent)
	resp.Body.Close()
	resp = doJSON(t, http.MethodGet, operation+version, "")
	mustStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
}

func TestARM_APIMAPILifecycle(t *testing.T) {
	srv := buildFullServer(t)
	rg := srv.URL + "/subscriptions/sub1/resourceGroups/apim-api-rg"
	service := rg + "/providers/Microsoft.ApiManagement/service/apim-test"
	api := service + "/apis/Example;rev=1?api-version=2022-08-01"
	for _, resource := range []struct{ url, body string }{
		{rg + apiVersionQ, `{"location":"uksouth"}`},
		{service + "?api-version=2024-05-01", `{"location":"uksouth","sku":{"name":"Developer","capacity":1},"properties":{"publisherName":"Example","publisherEmail":"example@example.test"}}`},
		{api, `{"properties":{"path":"example","displayName":"Example API","protocols":["https"]}}`},
	} {
		resp := doJSON(t, http.MethodPut, resource.url, resource.body)
		mustStatus(t, resp, http.StatusCreated)
		resp.Body.Close()
	}
	resp := doJSON(t, http.MethodPut, api, `{"properties":{"path":"updated","displayName":"Updated API","protocols":["https"],"subscriptionRequired":false}}`)
	mustStatus(t, resp, http.StatusOK)
	resp.Body.Close()
	resp = doJSON(t, http.MethodGet, api, "")
	mustStatus(t, resp, http.StatusOK)
	props := decode(t, resp)["properties"].(map[string]any)
	if props["path"] != "updated" || props["subscriptionRequired"] != false || props["apiRevision"] != "1" {
		t.Fatalf("API update/defaults not persisted: %v", props)
	}
	// AzureRM 5.14 imports first, PUTs its explicit create settings, then
	// sends empty strings when a later apply removes imported string
	// metadata. Removed contact/license blocks serialize as omitted fields.
	swagger := `{"swagger":"2.0","info":{"title":"Imported API","version":"1","description":"Imported description","termsOfService":"https://example.test/terms","contact":{"email":"api@example.test"},"license":{"name":"Imported license"}},"host":"api.example.test","basePath":"/v2","schemes":["https"],"paths":{}}`
	importBody, err := json.Marshal(map[string]any{"properties": map[string]any{
		"path": "example", "format": "swagger-json", "value": swagger,
	}})
	if err != nil {
		t.Fatal(err)
	}
	resp = doJSON(t, http.MethodPut, api, string(importBody))
	mustStatus(t, resp, http.StatusOK)
	props = decode(t, resp)["properties"].(map[string]any)
	if props["contact"].(map[string]any)["email"] != "api@example.test" ||
		props["license"].(map[string]any)["name"] != "Imported license" {
		t.Fatalf("import discarded valid metadata: %v", props)
	}
	for _, step := range []struct {
		body, description, terms string
	}{
		{`{"properties":{"path":"example","displayName":"Explicit API","protocols":["https"]}}`, "Imported description", "https://example.test/terms"},
		{`{"properties":{"path":"example","displayName":"Explicit API","protocols":["https"],"description":"","termsOfServiceUrl":""}}`, "", ""},
		{`{"properties":{"path":"example","displayName":"Explicit API","protocols":["https"]}}`, "", ""},
	} {
		resp = doJSON(t, http.MethodPut, api, step.body)
		mustStatus(t, resp, http.StatusOK)
		resp.Body.Close()
		resp = doJSON(t, http.MethodGet, api, "")
		mustStatus(t, resp, http.StatusOK)
		props = decode(t, resp)["properties"].(map[string]any)
		if props["displayName"] != "Explicit API" || props["description"] != step.description ||
			props["termsOfServiceUrl"] != step.terms || props["serviceUrl"] != "https://api.example.test/v2" {
			t.Fatalf("explicit PUT/import precedence is incorrect: %v", props)
		}
		for _, key := range []string{"contact", "license"} {
			if _, exists := props[key]; exists {
				t.Fatalf("omitted %s retained by non-import PUT", key)
			}
		}
	}
	resp = doJSON(t, http.MethodDelete, api, "")
	mustStatus(t, resp, http.StatusNoContent)
	resp.Body.Close()
	resp = doJSON(t, http.MethodGet, api, "")
	mustStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
	resp = doJSON(t, http.MethodDelete, service+"?api-version=2024-05-01", "")
	mustStatus(t, resp, http.StatusAccepted)
	resp.Body.Close()
}

// TestARM_VNetSubnetFullFlow exercises the canonical Terraform lifecycle:
// create an RG, create a vnet, create subnets, read everything back, delete
// the vnet, and confirm the subnets cascaded away while the RG remains.
func TestARM_VNetSubnetFullFlow(t *testing.T) {
	srv := buildFullServer(t)
	base := srv.URL

	rgURL := base + "/subscriptions/sub1/resourcegroups/rg1" + apiVersionQ
	vnetURL := base + "/subscriptions/sub1/resourcegroups/rg1/providers/microsoft.network/virtualnetworks/vnet1" + apiVersionQ
	sub1URL := base + "/subscriptions/sub1/resourcegroups/rg1/providers/microsoft.network/virtualnetworks/vnet1/subnets/sub-a" + apiVersionQ
	sub2URL := base + "/subscriptions/sub1/resourcegroups/rg1/providers/microsoft.network/virtualnetworks/vnet1/subnets/sub-b" + apiVersionQ

	// 1. Create resource group.
	resp := doJSON(t, http.MethodPut, rgURL, `{"location":"uksouth"}`)
	mustStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	// 2. Create vnet.
	resp = doJSON(t, http.MethodPut, vnetURL,
		`{"location":"uksouth","properties":{"addressSpace":{"addressPrefixes":["10.0.0.0/16"]}}}`)
	mustStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	// 3. Create two subnets.
	resp = doJSON(t, http.MethodPut, sub1URL, `{"properties":{"addressPrefix":"10.0.1.0/24"}}`)
	mustStatus(t, resp, http.StatusCreated)
	resp.Body.Close()
	resp = doJSON(t, http.MethodPut, sub2URL, `{"properties":{"addressPrefix":"10.0.2.0/24"}}`)
	mustStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	// 4. GET vnet and confirm both subnets are embedded in the response.
	resp = doJSON(t, http.MethodGet, vnetURL, "")
	mustStatus(t, resp, http.StatusOK)
	body := decode(t, resp)
	props, ok := body["properties"].(map[string]interface{})
	if !ok {
		t.Fatalf("properties missing: %v", body)
	}
	subnets, ok := props["subnets"].([]interface{})
	if !ok {
		t.Fatalf("subnets missing: %v", props)
	}
	if len(subnets) != 2 {
		t.Fatalf("len(subnets) = %d, want 2", len(subnets))
	}

	// 5. Delete vnet (cascade semantics).
	resp = doJSON(t, http.MethodDelete, vnetURL, "")
	mustStatus(t, resp, http.StatusAccepted)
	resp.Body.Close()

	// 6. Child subnet must be 404 after cascade.
	resp = doJSON(t, http.MethodGet, sub1URL, "")
	mustStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()

	// 7. Parent RG must still be intact.
	resp = doJSON(t, http.MethodGet, rgURL, "")
	mustStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	// 8. Azure request headers must be stamped on every response (middleware).
	if got := resp.Header.Get("x-ms-request-id"); got == "" {
		// Header assertion is intentionally on the last captured response;
		// any earlier one would do just as well.
		t.Errorf("x-ms-request-id header missing")
	}
}

// TestARM_PublicIPFullFlow exercises the Public IP lifecycle: create, read,
// verify SKU and fake ipAddress are present, update (idempotent PUT), delete,
// confirm 404.
func TestARM_PublicIPFullFlow(t *testing.T) {
	srv := buildFullServer(t)
	base := srv.URL

	pipURL := base + "/subscriptions/sub1/resourcegroups/rg1/providers/microsoft.network/publicipaddresses/pip1" + apiVersionQ
	listURL := base + "/subscriptions/sub1/resourcegroups/rg1/providers/microsoft.network/publicipaddresses" + apiVersionQ

	pipBody := `{
		"location": "uksouth",
		"sku": {"name": "Standard"},
		"properties": {
			"publicIPAllocationMethod": "Static",
			"publicIPAddressVersion": "IPv4"
		}
	}`

	// 1. Create.
	resp := doJSON(t, http.MethodPut, pipURL, pipBody)
	mustStatus(t, resp, http.StatusCreated)
	body := decode(t, resp)

	props, ok := body["properties"].(map[string]interface{})
	if !ok {
		t.Fatalf("properties missing: %v", body)
	}
	if props["provisioningState"] != "Succeeded" {
		t.Errorf("provisioningState = %v, want Succeeded", props["provisioningState"])
	}
	ip, ok := props["ipAddress"].(string)
	if !ok || ip == "" {
		t.Errorf("ipAddress missing or empty: %v", props["ipAddress"])
	}
	sku, ok := body["sku"].(map[string]interface{})
	if !ok {
		t.Fatalf("sku missing: %v", body)
	}
	if sku["name"] != "Standard" {
		t.Errorf("sku.name = %v, want Standard", sku["name"])
	}

	// 2. GET reads back the same IP.
	resp = doJSON(t, http.MethodGet, pipURL, "")
	mustStatus(t, resp, http.StatusOK)
	got := decode(t, resp)
	gotProps := got["properties"].(map[string]interface{})
	if gotProps["ipAddress"] != ip {
		t.Errorf("GET ipAddress = %v, want %v", gotProps["ipAddress"], ip)
	}

	// 3. Second PUT (update) preserves the assigned IP and returns 200.
	resp = doJSON(t, http.MethodPut, pipURL, pipBody)
	mustStatus(t, resp, http.StatusOK)
	updated := decode(t, resp)
	updatedProps := updated["properties"].(map[string]interface{})
	if updatedProps["ipAddress"] != ip {
		t.Errorf("update changed ipAddress: got %v, want %v", updatedProps["ipAddress"], ip)
	}

	// 4. LIST shows the resource.
	resp = doJSON(t, http.MethodGet, listURL, "")
	mustStatus(t, resp, http.StatusOK)
	list := decode(t, resp)
	items, ok := list["value"].([]interface{})
	if !ok || len(items) != 1 {
		t.Fatalf("list value = %v, want 1 item", list["value"])
	}

	// 5. DELETE is async (202 Accepted).
	resp = doJSON(t, http.MethodDelete, pipURL, "")
	mustStatus(t, resp, http.StatusAccepted)
	if resp.Header.Get("Location") == "" {
		t.Errorf("Location header missing on DELETE")
	}
	resp.Body.Close()

	// 6. Subsequent GET must return 404.
	resp = doJSON(t, http.MethodGet, pipURL, "")
	mustStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
}

// TestARM_LBFullFlow exercises the Load Balancer lifecycle: create an LB with
// Standard SKU, add a backend pool, a rule, and a probe; verify all three are
// embedded in the LB GET response; delete the LB and confirm children cascade.
func TestARM_LBFullFlow(t *testing.T) {
	srv := buildFullServer(t)
	base := srv.URL

	lbURL := base + "/subscriptions/sub1/resourcegroups/rg1/providers/microsoft.network/loadbalancers/lb1" + apiVersionQ
	poolURL := lbURL[:len(lbURL)-len(apiVersionQ)] + "/backendaddresspools/pool1" + apiVersionQ
	ruleURL := lbURL[:len(lbURL)-len(apiVersionQ)] + "/loadbalancingrules/rule1" + apiVersionQ
	probeURL := lbURL[:len(lbURL)-len(apiVersionQ)] + "/probes/probe1" + apiVersionQ
	listURL := base + "/subscriptions/sub1/resourcegroups/rg1/providers/microsoft.network/loadbalancers" + apiVersionQ

	lbBody := `{
		"location": "uksouth",
		"sku": {"name": "Standard"},
		"properties": {
			"frontendIPConfigurations": [
				{"name":"fe-config","properties":{"privateIPAllocationMethod":"Dynamic"}}
			]
		}
	}`

	// 1. Create LB.
	resp := doJSON(t, http.MethodPut, lbURL, lbBody)
	mustStatus(t, resp, http.StatusCreated)
	body := decode(t, resp)
	sku, ok := body["sku"].(map[string]interface{})
	if !ok {
		t.Fatalf("sku missing: %v", body)
	}
	if sku["name"] != "Standard" {
		t.Errorf("sku.name = %v, want Standard", sku["name"])
	}
	props := body["properties"].(map[string]interface{})
	if props["provisioningState"] != "Succeeded" {
		t.Errorf("provisioningState = %v, want Succeeded", props["provisioningState"])
	}

	// 2. Add backend pool, rule, probe.
	resp = doJSON(t, http.MethodPut, poolURL, `{"properties":{}}`)
	mustStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = doJSON(t, http.MethodPut, ruleURL, `{"properties":{"protocol":"Tcp","frontendPort":80,"backendPort":80}}`)
	mustStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = doJSON(t, http.MethodPut, probeURL, `{"properties":{"protocol":"Http","port":80,"requestPath":"/health"}}`)
	mustStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	// 3. GET LB: all three child types must be embedded.
	resp = doJSON(t, http.MethodGet, lbURL, "")
	mustStatus(t, resp, http.StatusOK)
	got := decode(t, resp)
	gotProps := got["properties"].(map[string]interface{})

	pools, ok := gotProps["backendAddressPools"].([]interface{})
	if !ok || len(pools) != 1 {
		t.Fatalf("backendAddressPools = %v, want 1 item", gotProps["backendAddressPools"])
	}
	rules, ok := gotProps["loadBalancingRules"].([]interface{})
	if !ok || len(rules) != 1 {
		t.Fatalf("loadBalancingRules = %v, want 1 item", gotProps["loadBalancingRules"])
	}
	probes, ok := gotProps["probes"].([]interface{})
	if !ok || len(probes) != 1 {
		t.Fatalf("probes = %v, want 1 item", gotProps["probes"])
	}

	// 4. Idempotent PUT returns 200.
	resp = doJSON(t, http.MethodPut, lbURL, lbBody)
	mustStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	// 5. LIST shows the LB.
	resp = doJSON(t, http.MethodGet, listURL, "")
	mustStatus(t, resp, http.StatusOK)
	list := decode(t, resp)
	items, ok := list["value"].([]interface{})
	if !ok || len(items) != 1 {
		t.Fatalf("list value = %v, want 1 item", list["value"])
	}

	// 6. DELETE LB is async (202 Accepted).
	resp = doJSON(t, http.MethodDelete, lbURL, "")
	mustStatus(t, resp, http.StatusAccepted)
	if resp.Header.Get("Location") == "" {
		t.Errorf("Location header missing on DELETE")
	}
	resp.Body.Close()

	// 7. Children must cascade away.
	resp = doJSON(t, http.MethodGet, poolURL, "")
	mustStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()

	resp = doJSON(t, http.MethodGet, ruleURL, "")
	mustStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()

	resp = doJSON(t, http.MethodGet, probeURL, "")
	mustStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()

	// 8. Subsequent GET on LB returns 404.
	resp = doJSON(t, http.MethodGet, lbURL, "")
	mustStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
}

// TestARM_NSGRuleFullFlow exercises the NSG + security rule lifecycle:
// create NSG, add a rule, verify it is embedded in the NSG response, delete
// the NSG, and confirm the rule cascaded away.
func TestARM_NSGRuleFullFlow(t *testing.T) {
	srv := buildFullServer(t)
	base := srv.URL

	nsgURL := base + "/subscriptions/sub1/resourcegroups/rg1/providers/microsoft.network/networksecuritygroups/nsg1" + apiVersionQ
	ruleURL := base + "/subscriptions/sub1/resourcegroups/rg1/providers/microsoft.network/networksecuritygroups/nsg1/securityrules/allow-http" + apiVersionQ
	listURL := base + "/subscriptions/sub1/resourcegroups/rg1/providers/microsoft.network/networksecuritygroups" + apiVersionQ

	nsgBody := `{"location":"uksouth"}`
	ruleBody := `{"properties":{"priority":100,"protocol":"Tcp","access":"Allow","direction":"Inbound","sourceAddressPrefix":"*","sourcePortRange":"*","destinationAddressPrefix":"*","destinationPortRange":"80"}}`

	// 1. Create NSG.
	resp := doJSON(t, http.MethodPut, nsgURL, nsgBody)
	mustStatus(t, resp, http.StatusCreated)
	body := decode(t, resp)
	props := body["properties"].(map[string]interface{})
	rules := props["securityRules"].([]interface{})
	if len(rules) != 0 {
		t.Fatalf("fresh NSG should have 0 rules, got %d", len(rules))
	}

	// 2. Add a security rule.
	resp = doJSON(t, http.MethodPut, ruleURL, ruleBody)
	mustStatus(t, resp, http.StatusCreated)
	ruleBody2 := decode(t, resp)
	if ruleBody2["name"] != "allow-http" {
		t.Errorf("rule name = %v, want allow-http", ruleBody2["name"])
	}

	// 3. GET NSG: rule must be embedded in securityRules array.
	resp = doJSON(t, http.MethodGet, nsgURL, "")
	mustStatus(t, resp, http.StatusOK)
	got := decode(t, resp)
	gotProps := got["properties"].(map[string]interface{})
	gotRules, ok := gotProps["securityRules"].([]interface{})
	if !ok || len(gotRules) != 1 {
		t.Fatalf("securityRules = %v, want 1 rule", gotProps["securityRules"])
	}
	gotRule := gotRules[0].(map[string]interface{})
	if gotRule["name"] != "allow-http" {
		t.Errorf("embedded rule name = %v, want allow-http", gotRule["name"])
	}

	// 4. LIST NSGs shows 1 entry; rules are not included as separate items.
	resp = doJSON(t, http.MethodGet, listURL, "")
	mustStatus(t, resp, http.StatusOK)
	list := decode(t, resp)
	items := list["value"].([]interface{})
	if len(items) != 1 {
		t.Fatalf("list count = %d, want 1", len(items))
	}

	// 5. DELETE NSG — rule cascades.
	resp = doJSON(t, http.MethodDelete, nsgURL, "")
	mustStatus(t, resp, http.StatusAccepted)
	resp.Body.Close()

	// 6. Rule GET must be 404 after cascade.
	resp = doJSON(t, http.MethodGet, ruleURL, "")
	mustStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
}

// TestARM_AppGWFullFlow exercises the Application Gateway lifecycle: create
// with a full Standard_v2 config, read back verifying inline properties are
// preserved, idempotent PUT returns 200, list shows 1 entry, delete returns
// 202 Accepted, subsequent GET returns 404.
func TestARM_AppGWFullFlow(t *testing.T) {
	srv := buildFullServer(t)
	base := srv.URL

	agwURL := base + "/subscriptions/sub1/resourcegroups/rg1/providers/microsoft.network/applicationgateways/agw1" + apiVersionQ
	listURL := base + "/subscriptions/sub1/resourcegroups/rg1/providers/microsoft.network/applicationgateways" + apiVersionQ

	agwBody := `{
		"location": "uksouth",
		"sku": {"name": "Standard_v2", "tier": "Standard_v2", "capacity": 2},
		"properties": {
			"gatewayIPConfigurations": [{"name":"gw-ip-cfg","properties":{"subnet":{"id":"/fake/subnet"}}}],
			"frontendIPConfigurations": [{"name":"fe-ip-cfg","properties":{"publicIPAddress":{"id":"/fake/pip"}}}],
			"frontendPorts": [{"name":"port-80","properties":{"port":80}}],
			"backendAddressPools": [{"name":"backend-pool","properties":{"backendAddresses":[]}}],
			"backendHttpSettingsCollection": [{"name":"http-settings","properties":{"port":80,"protocol":"Http","cookieBasedAffinity":"Disabled","requestTimeout":30}}],
			"httpListeners": [{"name":"http-listener","properties":{"frontendIPConfiguration":{"id":"fe-ip-cfg"},"frontendPort":{"id":"port-80"},"protocol":"Http"}}],
			"requestRoutingRules": [{"name":"routing-rule","properties":{"ruleType":"Basic","httpListener":{"id":"http-listener"},"backendAddressPool":{"id":"backend-pool"},"backendHttpSettings":{"id":"http-settings"},"priority":1}}]
		}
	}`

	// 1. Create.
	resp := doJSON(t, http.MethodPut, agwURL, agwBody)
	mustStatus(t, resp, http.StatusCreated)
	body := decode(t, resp)

	sku, ok := body["sku"].(map[string]interface{})
	if !ok {
		t.Fatalf("sku missing: %v", body)
	}
	if sku["name"] != "Standard_v2" {
		t.Errorf("sku.name = %v, want Standard_v2", sku["name"])
	}
	props := body["properties"].(map[string]interface{})
	if props["provisioningState"] != "Succeeded" {
		t.Errorf("provisioningState = %v, want Succeeded", props["provisioningState"])
	}
	if props["operationalState"] != "Running" {
		t.Errorf("operationalState = %v, want Running", props["operationalState"])
	}

	// 2. GET reads back inline properties intact.
	resp = doJSON(t, http.MethodGet, agwURL, "")
	mustStatus(t, resp, http.StatusOK)
	got := decode(t, resp)
	gotProps := got["properties"].(map[string]interface{})
	pools, ok := gotProps["backendAddressPools"].([]interface{})
	if !ok || len(pools) != 1 {
		t.Errorf("backendAddressPools = %v, want 1 item", gotProps["backendAddressPools"])
	}
	rules, ok := gotProps["requestRoutingRules"].([]interface{})
	if !ok || len(rules) != 1 {
		t.Errorf("requestRoutingRules = %v, want 1 item", gotProps["requestRoutingRules"])
	}

	// 3. Idempotent PUT returns 200.
	resp = doJSON(t, http.MethodPut, agwURL, agwBody)
	mustStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	// 4. LIST shows 1 entry.
	resp = doJSON(t, http.MethodGet, listURL, "")
	mustStatus(t, resp, http.StatusOK)
	list := decode(t, resp)
	items, ok := list["value"].([]interface{})
	if !ok || len(items) != 1 {
		t.Fatalf("list value = %v, want 1 item", list["value"])
	}

	// 5. DELETE is async (202 Accepted).
	resp = doJSON(t, http.MethodDelete, agwURL, "")
	mustStatus(t, resp, http.StatusAccepted)
	if resp.Header.Get("Location") == "" {
		t.Errorf("Location header missing on DELETE")
	}
	resp.Body.Close()

	// 6. Subsequent GET returns 404.
	resp = doJSON(t, http.MethodGet, agwURL, "")
	mustStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
}

func TestARM_DNSZoneFullFlow(t *testing.T) {
	srv := buildFullServer(t)
	base := srv.URL

	zoneURL := base + "/subscriptions/sub1/resourcegroups/rg1/providers/microsoft.network/dnszones/example.com" + apiVersionQ
	listURL := base + "/subscriptions/sub1/resourcegroups/rg1/providers/microsoft.network/dnszones" + apiVersionQ
	aRecordURL := base + "/subscriptions/sub1/resourcegroups/rg1/providers/microsoft.network/dnszones/example.com/A/www" + apiVersionQ
	txtRecordURL := base + "/subscriptions/sub1/resourcegroups/rg1/providers/microsoft.network/dnszones/example.com/TXT/verify" + apiVersionQ
	listAllURL := base + "/subscriptions/sub1/resourcegroups/rg1/providers/microsoft.network/dnszones/example.com/recordsets" + apiVersionQ

	// 1. Create zone — should auto-seed SOA and NS.
	resp := doJSON(t, http.MethodPut, zoneURL, `{"location":"global","tags":{}}`)
	mustStatus(t, resp, http.StatusCreated)
	body := decode(t, resp)
	if body["location"] != "global" {
		t.Errorf("location = %v, want global", body["location"])
	}
	props := body["properties"].(map[string]interface{})
	if props["provisioningState"] != "Succeeded" {
		t.Errorf("provisioningState = %v, want Succeeded", props["provisioningState"])
	}
	if props["zoneType"] != "Public" {
		t.Errorf("zoneType = %v, want Public", props["zoneType"])
	}
	ns, ok := props["nameServers"].([]interface{})
	if !ok || len(ns) == 0 {
		t.Errorf("nameServers missing or empty")
	}
	// Auto-seeded SOA + NS = numberOfRecordSets 2.
	if props["numberOfRecordSets"].(float64) != 2 {
		t.Errorf("numberOfRecordSets = %v, want 2 after create", props["numberOfRecordSets"])
	}

	// 2. Auto-seeded SOA is readable.
	soaURL := base + "/subscriptions/sub1/resourcegroups/rg1/providers/microsoft.network/dnszones/example.com/SOA/@" + apiVersionQ
	resp = doJSON(t, http.MethodGet, soaURL, "")
	mustStatus(t, resp, http.StatusOK)
	soaBody := decode(t, resp)
	if soaBody["type"] != "Microsoft.Network/dnsZones/SOA" {
		t.Errorf("SOA type = %v, want Microsoft.Network/dnsZones/SOA", soaBody["type"])
	}
	soaProps := soaBody["properties"].(map[string]interface{})
	if soaProps["fqdn"] != "example.com." {
		t.Errorf("SOA fqdn = %v, want example.com.", soaProps["fqdn"])
	}

	// 3. Add an A record.
	resp = doJSON(t, http.MethodPut, aRecordURL,
		`{"properties":{"TTL":300,"ARecords":[{"ipv4Address":"1.2.3.4"}]}}`)
	mustStatus(t, resp, http.StatusCreated)
	aBody := decode(t, resp)
	if aBody["type"] != "Microsoft.Network/dnsZones/A" {
		t.Errorf("A record type = %v, want Microsoft.Network/dnsZones/A", aBody["type"])
	}
	aProps := aBody["properties"].(map[string]interface{})
	if aProps["fqdn"] != "www.example.com." {
		t.Errorf("A fqdn = %v, want www.example.com.", aProps["fqdn"])
	}

	// 4. Add a TXT record.
	resp = doJSON(t, http.MethodPut, txtRecordURL,
		`{"properties":{"TTL":300,"TXTRecords":[{"value":["v=spf1 ~all"]}]}}`)
	mustStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	// 5. Zone GET shows numberOfRecordSets = 4 (SOA + NS + A + TXT).
	resp = doJSON(t, http.MethodGet, zoneURL, "")
	mustStatus(t, resp, http.StatusOK)
	body = decode(t, resp)
	props = body["properties"].(map[string]interface{})
	if props["numberOfRecordSets"].(float64) != 4 {
		t.Errorf("numberOfRecordSets = %v, want 4", props["numberOfRecordSets"])
	}

	// 6. Idempotent PUT on the zone returns 200.
	resp = doJSON(t, http.MethodPut, zoneURL, `{"location":"global","tags":{"env":"test"}}`)
	mustStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	// 7. LIST by RG shows 1 zone.
	resp = doJSON(t, http.MethodGet, listURL, "")
	mustStatus(t, resp, http.StatusOK)
	list := decode(t, resp)
	items, ok := list["value"].([]interface{})
	if !ok || len(items) != 1 {
		t.Fatalf("list value = %v, want 1 zone", list["value"])
	}

	// 8. list all record sets returns SOA + NS + A + TXT.
	resp = doJSON(t, http.MethodGet, listAllURL, "")
	mustStatus(t, resp, http.StatusOK)
	allList := decode(t, resp)
	allItems, ok := allList["value"].([]interface{})
	if !ok || len(allItems) != 4 {
		t.Errorf("list all returned %v items, want 4", len(allItems))
	}

	// 9. Delete the A record individually. Record set deletes are synchronous
	// (204 No Content); only zone deletes are async (see deleteDNSRecordSet in
	// internal/arm/dns.go).
	resp = doJSON(t, http.MethodDelete, aRecordURL, "")
	mustStatus(t, resp, http.StatusNoContent)
	resp.Body.Close()
	resp = doJSON(t, http.MethodGet, aRecordURL, "")
	mustStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()

	// 10. DELETE zone cascades all remaining record sets.
	resp = doJSON(t, http.MethodDelete, zoneURL, "")
	mustStatus(t, resp, http.StatusAccepted)
	if resp.Header.Get("Location") == "" {
		t.Errorf("Location header missing on zone DELETE")
	}
	resp.Body.Close()

	// Zone gone.
	resp = doJSON(t, http.MethodGet, zoneURL, "")
	mustStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
	// Auto-seeded SOA and NS gone.
	resp = doJSON(t, http.MethodGet, soaURL, "")
	mustStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
	// TXT gone (cascaded).
	resp = doJSON(t, http.MethodGet, txtRecordURL, "")
	mustStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
}

// TestARM_StorageAccountFullFlow exercises the storage account and container
// lifecycle: create account, verify SKU and endpoints, create containers, list
// them, delete a container, delete the account and confirm cascade.
func TestARM_CompatibilityReads(t *testing.T) {
	for _, tc := range []struct {
		name, parent, child, version, body string
		putStatus, getStatus               int
	}{
		{"storage", "Microsoft.Storage/storageAccounts/testacct", "accountMigrations/default", "2025-08-01",
			`{"location":"uksouth","sku":{"name":"Standard_LRS"},"kind":"StorageV2"}`, http.StatusOK, http.StatusOK},
		{"aks", "Microsoft.ContainerService/managedClusters/testaks", "maintenanceConfigurations/aksManagedAutoUpgradeSchedule", "2026-07-01",
			`{"location":"uksouth"}`, http.StatusCreated, http.StatusNotFound},
		{"redis", "Microsoft.Cache/Redis/testredis", "patchSchedules/default", "2024-11-01",
			`{"location":"uksouth","properties":{"sku":{"name":"Standard","family":"C","capacity":1}}}`, http.StatusCreated, http.StatusNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := buildFullServer(t)
			parent := srv.URL + "/subscriptions/sub1/resourceGroups/rg1/providers/" + tc.parent
			child := parent + "/" + tc.child + "?api-version=" + tc.version

			checkError := func(code string) {
				t.Helper()
				resp := doJSON(t, http.MethodGet, child, "")
				mustStatus(t, resp, http.StatusNotFound)
				body := decode(t, resp)
				apiError, _ := body["error"].(map[string]any)
				if apiError["code"] != code {
					t.Fatalf("error = %v, want %s", apiError, code)
				}
			}
			checkError("ParentResourceNotFound")

			resp := doJSON(t, http.MethodPut, parent+apiVersionQ, tc.body)
			mustStatus(t, resp, tc.putStatus)
			resp.Body.Close()
			if tc.getStatus == http.StatusNotFound {
				checkError("ResourceNotFound")
				return
			}
			resp = doJSON(t, http.MethodGet, child, "")
			mustStatus(t, resp, tc.getStatus)
			body := decode(t, resp)
			properties, ok := body["properties"].(map[string]any)
			if body["name"] != "default" || !ok || len(properties) != 0 {
				t.Fatalf("unexpected inactive migration response: %v", body)
			}
		})
	}
}

func TestARM_StorageAccountFullFlow(t *testing.T) {
	srv := buildFullServer(t)
	base := srv.URL

	acctURL := base + "/subscriptions/sub1/resourcegroups/rg1/providers/microsoft.storage/storageaccounts/integrationacct" + apiVersionQ
	cont1URL := base + "/subscriptions/sub1/resourcegroups/rg1/providers/microsoft.storage/storageaccounts/integrationacct/blobservices/default/containers/cont1" + apiVersionQ
	cont2URL := base + "/subscriptions/sub1/resourcegroups/rg1/providers/microsoft.storage/storageaccounts/integrationacct/blobservices/default/containers/cont2" + apiVersionQ
	listURL := base + "/subscriptions/sub1/resourcegroups/rg1/providers/microsoft.storage/storageaccounts/integrationacct/blobservices/default/containers" + apiVersionQ

	// 1. Create storage account. azurerm's storageaccounts.Create accepts
	// 200 or 202 only, not 201 (see putStorageAccount in
	// internal/arm/storage_account.go).
	resp := doJSON(t, http.MethodPut, acctURL, `{
		"location": "uksouth",
		"sku": {"name": "Standard_LRS"},
		"kind": "StorageV2",
		"properties": {"accessTier": "Hot"}
	}`)
	mustStatus(t, resp, http.StatusOK)
	acctBody := decode(t, resp)

	// Verify SKU and kind are at top level.
	sku, ok := acctBody["sku"].(map[string]interface{})
	if !ok {
		t.Fatalf("sku missing in storage account response")
	}
	if sku["name"] != "Standard_LRS" {
		t.Errorf("sku.name = %v, want Standard_LRS", sku["name"])
	}
	if acctBody["kind"] != "StorageV2" {
		t.Errorf("kind = %v, want StorageV2", acctBody["kind"])
	}

	// Verify primary endpoints are populated.
	props, ok := acctBody["properties"].(map[string]interface{})
	if !ok {
		t.Fatalf("properties missing in storage account response")
	}
	endpoints, ok := props["primaryEndpoints"].(map[string]interface{})
	if !ok {
		t.Fatalf("primaryEndpoints missing")
	}
	// buildFullServer wires AZEMU_AZURITE_ENDPOINT="http://azurite-test:10000",
	// so blob endpoints are path-style under that host, NOT real-Azure
	// {acct}.blob.core.windows.net.
	wantBlob := "http://azurite-test:10000/integrationacct/"
	if endpoints["blob"] != wantBlob {
		t.Errorf("primaryEndpoints.blob = %v, want %s", endpoints["blob"], wantBlob)
	}

	// 2. Verify idempotent PUT returns 200.
	resp = doJSON(t, http.MethodPut, acctURL, `{
		"location": "uksouth",
		"sku": {"name": "Standard_LRS"},
		"kind": "StorageV2",
		"properties": {}
	}`)
	mustStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	// 3. GET the account.
	resp = doJSON(t, http.MethodGet, acctURL, "")
	mustStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	// 4. Create two containers.
	resp = doJSON(t, http.MethodPut, cont1URL, `{"properties":{"publicAccess":"None"}}`)
	mustStatus(t, resp, http.StatusCreated)
	cont1Body := decode(t, resp)
	if cont1Body["name"] != "cont1" {
		t.Errorf("container name = %v, want cont1", cont1Body["name"])
	}
	contProps, ok := cont1Body["properties"].(map[string]interface{})
	if !ok {
		t.Fatalf("container properties missing")
	}
	if contProps["publicAccess"] != "None" {
		t.Errorf("publicAccess = %v, want None", contProps["publicAccess"])
	}

	resp = doJSON(t, http.MethodPut, cont2URL, `{"properties":{"publicAccess":"None"}}`)
	mustStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	// 5. List containers — expect 2.
	resp = doJSON(t, http.MethodGet, listURL, "")
	mustStatus(t, resp, http.StatusOK)
	listBody := decode(t, resp)
	items, ok := listBody["value"].([]interface{})
	if !ok || len(items) != 2 {
		t.Fatalf("container list = %v, want 2 items", listBody["value"])
	}

	// 6. Delete cont1 individually. Container deletes are synchronous 200 OK
	// (see deleteStorageContainer in internal/arm/storage_container.go).
	resp = doJSON(t, http.MethodDelete, cont1URL, "")
	mustStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	resp = doJSON(t, http.MethodGet, cont1URL, "")
	mustStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()

	// cont2 still exists.
	resp = doJSON(t, http.MethodGet, cont2URL, "")
	mustStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	// 7. Delete the storage account — cascades cont2. Account deletes are
	// synchronous 200 OK; a bodyless 202 makes the SDK error (see
	// deleteStorageAccount in internal/arm/storage_account.go).
	resp = doJSON(t, http.MethodDelete, acctURL, "")
	mustStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	// Account gone.
	resp = doJSON(t, http.MethodGet, acctURL, "")
	mustStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()

	// cont2 cascaded away.
	resp = doJSON(t, http.MethodGet, cont2URL, "")
	mustStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()

	// 8. Azure request headers must be present (middleware).
	resp = doJSON(t, http.MethodPut,
		base+"/subscriptions/sub1/resourcegroups/rg1/providers/microsoft.storage/storageaccounts/headercheck"+apiVersionQ,
		`{"location":"uksouth","sku":{"name":"Standard_LRS"},"kind":"StorageV2","properties":{}}`)
	if got := resp.Header.Get("x-ms-request-id"); got == "" {
		t.Error("x-ms-request-id header missing on storage account PUT")
	}
	resp.Body.Close()
}

// TestARM_RedisCacheFullFlow exercises the Microsoft.Cache/Redis lifecycle:
// create with redisConfiguration, verify hostName/port/sslPort/sku and
// verbatim config pass-through, idempotent PUT, listKeys returns the two
// deterministic dev keys, list-by-RG returns 1 item, async DELETE 202 with
// Location header, post-delete GET returns 404.
func TestARM_RedisCacheFullFlow(t *testing.T) {
	srv := buildFullServer(t)
	base := srv.URL

	cacheURL := base + "/subscriptions/sub1/resourcegroups/rg1/providers/microsoft.cache/redis/integ-redis" + apiVersionQ
	listURL := base + "/subscriptions/sub1/resourcegroups/rg1/providers/microsoft.cache/redis" + apiVersionQ
	keysURL := base + "/subscriptions/sub1/resourcegroups/rg1/providers/microsoft.cache/redis/integ-redis/listkeys" + apiVersionQ

	cacheBody := `{
		"location": "uksouth",
		"properties": {
			"sku": {"name": "Standard", "family": "C", "capacity": 1},
			"redisConfiguration": {"maxmemory-policy": "allkeys-lru"}
		}
	}`

	// 1. Create.
	resp := doJSON(t, http.MethodPut, cacheURL, cacheBody)
	mustStatus(t, resp, http.StatusCreated)
	body := decode(t, resp)
	props := body["properties"].(map[string]interface{})
	if props["hostName"] != "redis-test" {
		t.Errorf("hostName = %v, want redis-test", props["hostName"])
	}
	if props["port"].(float64) != 6379 {
		t.Errorf("port = %v, want 6379", props["port"])
	}
	if props["sslPort"].(float64) != 6380 {
		t.Errorf("sslPort = %v, want 6380", props["sslPort"])
	}
	sku, ok := props["sku"].(map[string]interface{})
	if !ok {
		t.Fatalf("sku missing: %v", props)
	}
	if sku["name"] != "Standard" {
		t.Errorf("sku.name = %v, want Standard", sku["name"])
	}
	rc, ok := props["redisConfiguration"].(map[string]interface{})
	if !ok {
		t.Fatalf("redisConfiguration missing: %v", props)
	}
	if rc["maxmemory-policy"] != "allkeys-lru" {
		t.Errorf("redisConfiguration.maxmemory-policy = %v, want allkeys-lru (verbatim pass-through)", rc["maxmemory-policy"])
	}

	// 2. Idempotent PUT returns 200.
	resp = doJSON(t, http.MethodPut, cacheURL, cacheBody)
	mustStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	// 3. listKeys returns both deterministic dev keys.
	resp = doJSON(t, http.MethodPost, keysURL, "{}")
	mustStatus(t, resp, http.StatusOK)
	keys := decode(t, resp)
	if keys["primaryKey"] != "azemu-dev-primary-key" {
		t.Errorf("primaryKey = %v, want azemu-dev-primary-key", keys["primaryKey"])
	}
	if keys["secondaryKey"] != "azemu-dev-secondary-key" {
		t.Errorf("secondaryKey = %v, want azemu-dev-secondary-key", keys["secondaryKey"])
	}

	// 4. List by RG returns 1 item.
	resp = doJSON(t, http.MethodGet, listURL, "")
	mustStatus(t, resp, http.StatusOK)
	list := decode(t, resp)
	items, ok := list["value"].([]interface{})
	if !ok || len(items) != 1 {
		t.Fatalf("list value = %v, want 1 item", list["value"])
	}

	// 5. DELETE is async (202 Accepted) with Location header.
	resp = doJSON(t, http.MethodDelete, cacheURL, "")
	mustStatus(t, resp, http.StatusAccepted)
	if resp.Header.Get("Location") == "" {
		t.Error("Location header missing on DELETE")
	}
	resp.Body.Close()

	// 6. Post-delete GET returns 404.
	resp = doJSON(t, http.MethodGet, cacheURL, "")
	mustStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
}

// TestARM_KeyVaultFullFlow exercises the Key Vault management plane lifecycle:
// create, read, idempotent update, list, delete, confirm 404.
func TestARM_KeyVaultFullFlow(t *testing.T) {
	srv := buildFullServer(t)
	base := srv.URL

	vaultURL := base + "/subscriptions/sub1/resourcegroups/rg1/providers/microsoft.keyvault/vaults/mytestvault" + apiVersionQ
	listURL := base + "/subscriptions/sub1/resourcegroups/rg1/providers/microsoft.keyvault/vaults" + apiVersionQ

	vaultBody := `{
		"location": "uksouth",
		"properties": {
			"sku": {"family": "A", "name": "standard"},
			"tenantId": "00000000-0000-0000-0000-000000000001",
			"accessPolicies": [],
			"enableSoftDelete": true,
			"softDeleteRetentionInDays": 90,
			"enableRbacAuthorization": false
		}
	}`

	// 1. Create returns 201.
	resp := doJSON(t, http.MethodPut, vaultURL, vaultBody)
	mustStatus(t, resp, http.StatusCreated)
	created := decode(t, resp)
	if created["name"] != "mytestvault" {
		t.Errorf("name = %v, want mytestvault", created["name"])
	}
	props, ok := created["properties"].(map[string]interface{})
	if !ok {
		t.Fatalf("properties missing")
	}
	// vaultUri uses the per-vault host form {name}.vault.localhost that the
	// azurerm provider requires (KeyVaultIDFromBaseUrl), resolving to azemu
	// instead of real-Azure {name}.vault.azure.net.
	if props["vaultUri"] != "https://mytestvault.vault.localhost/" {
		t.Errorf("vaultUri = %v, want https://mytestvault.vault.localhost/", props["vaultUri"])
	}
	if props["provisioningState"] != "Succeeded" {
		t.Errorf("provisioningState = %v, want Succeeded", props["provisioningState"])
	}

	// 2. Idempotent PUT returns 200.
	resp = doJSON(t, http.MethodPut, vaultURL, vaultBody)
	mustStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	// 3. GET returns the vault.
	resp = doJSON(t, http.MethodGet, vaultURL, "")
	mustStatus(t, resp, http.StatusOK)
	got := decode(t, resp)
	if got["type"] != "Microsoft.KeyVault/vaults" {
		t.Errorf("type = %v, want Microsoft.KeyVault/vaults", got["type"])
	}

	// 4. LIST shows the vault.
	resp = doJSON(t, http.MethodGet, listURL, "")
	mustStatus(t, resp, http.StatusOK)
	list := decode(t, resp)
	items, ok := list["value"].([]interface{})
	if !ok || len(items) != 1 {
		t.Fatalf("list value = %v, want 1 item", list["value"])
	}

	// 5. DELETE returns 200 OK. azurerm's vaults.VaultsClient#Delete expects
	// 200 when soft-delete is disabled (see deleteKeyVault in
	// internal/arm/keyvault.go); azemu does not implement soft-delete.
	resp = doJSON(t, http.MethodDelete, vaultURL, "")
	mustStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	// 6. GET returns 404 after delete.
	resp = doJSON(t, http.MethodGet, vaultURL, "")
	mustStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
}

// TestARM_KeyVaultKeySignFlow exercises the OTA manifest-signing pipeline:
// provision a vault via ARM, create an RSA-2048 key via the data plane, sign
// a SHA-256 digest with RS256, and verify the signature against the public
// JWK returned by GET. Runs through the production-like TLS harness so the
// data-plane mount, middleware, and vaultUri rewrite are all exercised.
func TestARM_KeyVaultKeySignFlow(t *testing.T) {
	srv := buildProductionLikeServer(t)
	client := srv.Client()
	base := srv.URL

	do := func(method, url, body string) *http.Response {
		t.Helper()
		var rdr io.Reader
		if body != "" {
			rdr = strings.NewReader(body)
		}
		req, err := http.NewRequest(method, url, rdr)
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("do %s %s: %v", method, url, err)
		}
		return resp
	}

	// 1. Provision the vault via the ARM control plane.
	vaultURL := base + "/subscriptions/sub1/resourcegroups/rg1/providers/microsoft.keyvault/vaults/otavault" + apiVersionQ
	resp := do(http.MethodPut, vaultURL, `{
		"location": "uksouth",
		"properties": {
			"sku": {"family": "A", "name": "standard"},
			"tenantId": "00000000-0000-0000-0000-000000000001"
		}
	}`)
	mustStatus(t, resp, http.StatusCreated)
	created := decode(t, resp)
	props, _ := created["properties"].(map[string]interface{})
	if props["vaultUri"] != "https://otavault.vault.localhost/" {
		t.Fatalf("vaultUri = %v, want https://otavault.vault.localhost/", props["vaultUri"])
	}

	// 2. Create an RSA-2048 signing key via the data plane.
	resp = do(http.MethodPost, base+"/keyvault/otavault/keys/manifest-signer/create",
		`{"kty":"RSA","key_size":2048,"key_ops":["sign","verify"]}`)
	mustStatus(t, resp, http.StatusOK)
	bundle := decode(t, resp)
	jwk, _ := bundle["key"].(map[string]interface{})
	if jwk == nil {
		t.Fatalf("create response has no key object: %v", bundle)
	}
	kid, _ := jwk["kid"].(string)
	version := kid[strings.LastIndex(kid, "/")+1:]

	// 3. GET the key back; rebuild the public key from the JWK.
	resp = do(http.MethodGet, base+"/keyvault/otavault/keys/manifest-signer", "")
	mustStatus(t, resp, http.StatusOK)
	got := decode(t, resp)
	gotJWK, _ := got["key"].(map[string]interface{})
	nB, err := base64.RawURLEncoding.DecodeString(gotJWK["n"].(string))
	if err != nil {
		t.Fatalf("decode jwk n: %v", err)
	}
	eB, err := base64.RawURLEncoding.DecodeString(gotJWK["e"].(string))
	if err != nil {
		t.Fatalf("decode jwk e: %v", err)
	}
	pub := &rsa.PublicKey{
		N: new(big.Int).SetBytes(nB),
		E: int(new(big.Int).SetBytes(eB).Int64()),
	}

	// 4. Sign a manifest digest (RS256 = RSASSA-PKCS1-v1_5 over SHA-256).
	manifest := []byte(`{"id":"0001","launchAsset":{"url":"https://cdn.example/bundle.js"}}`)
	digest := sha256.Sum256(manifest)
	resp = do(http.MethodPost, base+"/keyvault/otavault/keys/manifest-signer/"+version+"/sign",
		`{"alg":"RS256","value":"`+base64.RawURLEncoding.EncodeToString(digest[:])+`"}`)
	mustStatus(t, resp, http.StatusOK)
	signResult := decode(t, resp)
	if signResult["kid"] != kid {
		t.Errorf("sign kid = %v, want %v", signResult["kid"], kid)
	}

	// 5. The signature must verify against the JWK's public key.
	sig, err := base64.RawURLEncoding.DecodeString(signResult["value"].(string))
	if err != nil {
		t.Fatalf("decode signature: %v", err)
	}
	if err := rsa.VerifyPKCS1v15(pub, crypto.SHA256, digest[:], sig); err != nil {
		t.Fatalf("signature does not verify against returned JWK: %v", err)
	}

	// 6. Delete the key; subsequent GET is 404.
	resp = do(http.MethodDelete, base+"/keyvault/otavault/keys/manifest-signer", "")
	mustStatus(t, resp, http.StatusOK)
	resp.Body.Close()
	resp = do(http.MethodGet, base+"/keyvault/otavault/keys/manifest-signer", "")
	mustStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()

	// 7. Vault delete succeeds with the key already gone.
	resp = do(http.MethodDelete, vaultURL, "")
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		t.Fatalf("vault delete status = %d, want 200 or 202", resp.StatusCode)
	}
	resp.Body.Close()
}

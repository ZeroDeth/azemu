package arm

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestAPIMCustomDomain_IdentityAndHostnameLifecycle(t *testing.T) {
	srv := newTestServer(t)
	rg := srv.URL + "/subscriptions/sub1/resourceGroups/domain-rg"
	identityURL := rg + "/providers/Microsoft.ManagedIdentity/userAssignedIdentities/mi1"
	base := rg + "/providers/Microsoft.ApiManagement/service/domain-apim"
	vault := keyVaultURL(srv.URL, "sub1", "domain-rg", "domain-vault")
	for _, step := range []struct{ url, body string }{
		{rg, `{"location":"uksouth"}`}, {identityURL, `{"location":"uksouth"}`},
		{vault, keyVaultBodyStandard},
	} {
		resp := httpPut(t, step.url, step.body)
		assertStatus(t, resp, 201)
		resp.Body.Close()
	}
	mi := decodeJSON(t, httpGet(t, identityURL))
	miProps := mi["properties"].(map[string]any)
	identity := map[string]any{"type": "UserAssigned", "userAssignedIdentities": map[string]any{mi["id"].(string): map[string]any{}}}
	var body map[string]any
	if err := json.Unmarshal([]byte(apimBasicBody), &body); err != nil {
		t.Fatal(err)
	}
	body["identity"] = identity
	encoded, _ := json.Marshal(body)
	resp := httpPut(t, base, string(encoded))
	assertStatus(t, resp, 201)
	initial := decodeJSON(t, resp)
	savedIdentity := initial["identity"].(map[string]any)
	details := savedIdentity["userAssignedIdentities"].(map[string]any)[mi["id"].(string)].(map[string]any)
	if details["clientId"] != miProps["clientId"] || details["principalId"] != miProps["principalId"] {
		t.Fatal("APIM minted identity details instead of resolving ARM identity")
	}
	policy, _ := json.Marshal(map[string]any{"properties": map[string]any{"accessPolicies": []any{
		map[string]any{"tenantId": miProps["tenantId"], "objectId": miProps["principalId"],
			"permissions": map[string]any{"secrets": []string{"Get", "List"}}},
	}}})
	resp = httpPut(t, vault+"/accessPolicies/add", string(policy))
	assertStatus(t, resp, 200)
	resp.Body.Close()
	resp = httpPost(t, srv.URL+"/keyvault/domain-vault/certificates/example/create", kvCertificateSelfBody)
	assertStatus(t, resp, 202)
	resp.Body.Close()
	cert := decodeJSON(t, httpGet(t, srv.URL+"/keyvault/domain-vault/certificates/example"))
	configs := []any{}
	for _, endpoint := range []struct{ kind, host string }{{"Proxy", "api.example.com"}, {"DeveloperPortal", "portal.example.com"}} {
		configs = append(configs, map[string]any{"type": endpoint.kind, "hostName": endpoint.host,
			"keyVaultId": cert["sid"], "identityClientId": miProps["clientId"], "negotiateClientCertificate": true})
	}
	for _, tc := range []struct {
		name, key string
		value     any
	}{
		{"wrong-hostname", "hostName", "not-covered.example.net"},
		{"missing-certificate-version", "keyVaultId", strings.TrimSuffix(cert["sid"].(string), cert["sid"].(string)[strings.LastIndex(cert["sid"].(string), "/")+1:]) + "missing"},
		{"unassigned-client", "identityClientId", "00000000-0000-0000-0000-000000000099"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			invalid := map[string]any{}
			for key, val := range configs[0].(map[string]any) {
				invalid[key] = val
			}
			invalid[tc.key] = tc.value
			encoded, _ := json.Marshal(map[string]any{"properties": map[string]any{"hostnameConfigurations": []any{invalid}}})
			resp := httpPatch(t, base, string(encoded))
			assertStatus(t, resp, 400)
			resp.Body.Close()
		})
	}
	resp = httpPut(t, vault+"/accessPolicies/remove", string(policy))
	resp.Body.Close()
	denied, _ := json.Marshal(map[string]any{"properties": map[string]any{"hostnameConfigurations": configs}})
	resp = httpPatch(t, base, string(denied))
	assertStatus(t, resp, 403)
	if decodeJSON(t, resp)["error"].(map[string]any)["code"] != "Forbidden" {
		t.Error("Key Vault permission denial must preserve the authorization error category")
	}
	resp = httpPut(t, vault+"/accessPolicies/add", string(policy))
	resp.Body.Close()
	patch, _ := json.Marshal(map[string]any{"properties": map[string]any{"hostnameConfigurations": configs}})
	resp = httpPatch(t, base, string(patch))
	assertStatus(t, resp, 200)
	configured := decodeJSON(t, resp)
	hosts := configured["properties"].(map[string]any)["hostnameConfigurations"].([]any)
	if len(hosts) != 3 || hosts[0].(map[string]any)["certificateSource"] != "KeyVault" {
		t.Fatal("custom hostnames or built-in hostname missing")
	}
	if hosts[0].(map[string]any)["defaultSslBinding"] != true ||
		hosts[2].(map[string]any)["defaultSslBinding"] != false {
		t.Fatal("a lone custom Proxy with omitted binding must be the sole default")
	}
	for _, raw := range hosts[:2] {
		h := raw.(map[string]any)
		if h["keyVaultId"] != cert["sid"] || h["identityClientId"] != miProps["clientId"] ||
			h["negotiateClientCertificate"] != true || h["certificate"].(map[string]any)["thumbprint"] == "" {
			t.Fatal("certificate configuration was not faithfully returned")
		}
	}
	t.Run("proxy-default-bindings", func(t *testing.T) {
		for _, tc := range []struct {
			name                   string
			first, second, builtIn any
			includeBuiltIn         bool
			want                   []bool
			status                 int
		}{
			{name: "both-omitted", want: []bool{false, false, true}, status: 200},
			{name: "first-explicit-default", first: true, want: []bool{true, false, false}, status: 200},
			{name: "second-explicit-default", second: true, want: []bool{false, true, false}, status: 200},
			{name: "both-explicit-false", first: false, second: false, want: []bool{false, false, true}, status: 200},
			{name: "explicit-built-in-default", includeBuiltIn: true, builtIn: true, want: []bool{false, false, true}, status: 200},
			{name: "two-explicit-defaults", first: true, second: true, status: 400},
			{name: "custom-and-built-in-default", first: true, includeBuiltIn: true, builtIn: true, status: 400},
		} {
			t.Run(tc.name, func(t *testing.T) {
				proxies := []any{}
				for i, flag := range []any{tc.first, tc.second} {
					proxy := map[string]any{}
					for key, value := range configs[i].(map[string]any) {
						proxy[key] = value
					}
					proxy["type"] = "Proxy"
					if flag != nil {
						proxy["defaultSslBinding"] = flag
					}
					proxies = append(proxies, proxy)
				}
				if tc.includeBuiltIn {
					proxies = append(proxies, map[string]any{"type": "Proxy",
						"hostName": "domain-apim.azure-api.net", "defaultSslBinding": tc.builtIn})
				}
				encoded, _ := json.Marshal(map[string]any{"properties": map[string]any{"hostnameConfigurations": proxies}})
				resp := httpPatch(t, base, string(encoded))
				assertStatus(t, resp, tc.status)
				result := decodeJSON(t, resp)
				if tc.status != 200 {
					if result["error"].(map[string]any)["code"] != "InvalidHostnameConfiguration" {
						t.Fatal("conflicting defaults must return hostname validation error")
					}
					return
				}
				hosts := result["properties"].(map[string]any)["hostnameConfigurations"].([]any)
				if len(hosts) != len(tc.want) {
					t.Fatalf("hostname count = %d, want %d", len(hosts), len(tc.want))
				}
				for i, flag := range tc.want {
					if hosts[i].(map[string]any)["defaultSslBinding"] != flag {
						t.Errorf("hostname %d defaultSslBinding = %v, want %v", i, hosts[i].(map[string]any)["defaultSslBinding"], flag)
					}
				}
				encoded, _ = json.Marshal(result)
				resp = httpPut(t, base, string(encoded))
				assertStatus(t, resp, 200)
				roundtrip := decodeJSON(t, resp)["properties"].(map[string]any)["hostnameConfigurations"].([]any)
				for i, flag := range tc.want {
					if roundtrip[i].(map[string]any)["defaultSslBinding"] != flag {
						t.Error("GET-model PUT changed default binding")
					}
				}
			})
		}
	})
	// AzureRM custom-domain create/update sends the existing GET model as PUT.
	for i := 0; i < 2; i++ {
		encoded, _ = json.Marshal(configured)
		resp = httpPut(t, base, string(encoded))
		assertStatus(t, resp, 200)
		configured = decodeJSON(t, resp)
	}

	resp = httpPatch(t, base, `{"tags":{"test":"retained"}}`)
	assertStatus(t, resp, 200)
	configured = decodeJSON(t, resp)
	if len(configured["properties"].(map[string]any)["hostnameConfigurations"].([]any)) != 3 {
		t.Fatal("unrelated PATCH lost hostnames")
	}
	// AzureRM clears the hostname configurations on its parent GET model.
	configured["properties"].(map[string]any)["hostnameConfigurations"] = nil
	encoded, _ = json.Marshal(configured)
	resp = httpPut(t, base, string(encoded))
	assertStatus(t, resp, 200)
	cleared := decodeJSON(t, resp)
	props := cleared["properties"].(map[string]any)
	if len(props["hostnameConfigurations"].([]any)) != 1 || props["publisherName"] != "Example" ||
		props["customProperties"].(map[string]any)["Microsoft.WindowsAzure.ApiManagement.Gateway.Protocols.Server.Http2"] != "true" ||
		cleared["identity"].(map[string]any)["type"] != "UserAssigned" || cleared["tags"].(map[string]any)["test"] != "retained" {
		t.Fatal("custom-domain delete lost shared APIM configuration")
	}
	if cleared["properties"].(map[string]any)["_identity"] != nil {
		t.Fatal("internal identity storage leaked")
	}
	resp = httpDelete(t, rg)
	resp.Body.Close()
	assertStatus(t, httpGet(t, base), 404)
}

func TestAPIMCustomDomain_BuiltInHostnameCaseInsensitive(t *testing.T) {
	srv := newTestServer(t)
	rg := srv.URL + "/subscriptions/sub1/resourceGroups/case-domain-rg"
	resp := httpPut(t, rg, `{"location":"uksouth"}`)
	assertStatus(t, resp, 201)
	resp.Body.Close()
	base := rg + "/providers/Microsoft.ApiManagement/service/case-apim"
	resp = httpPut(t, base, apimBasicBody)
	assertStatus(t, resp, 201)
	resp.Body.Close()
	resp = httpPatch(t, base, `{"properties":{"hostnameConfigurations":[{"type":"Proxy","hostName":"CASE-APIM.AZURE-API.NET"}]}}`)
	assertStatus(t, resp, 200)
	hosts := decodeJSON(t, resp)["properties"].(map[string]any)["hostnameConfigurations"].([]any)
	if len(hosts) != 1 || hosts[0].(map[string]any)["certificateSource"] != "BuiltIn" ||
		hosts[0].(map[string]any)["defaultSslBinding"] != true {
		t.Fatal("case-only built-in hostname must not require a custom certificate or create a duplicate")
	}
}

func TestAPIMCustomDomain_InvalidReferences(t *testing.T) {
	srv := newTestServer(t)
	rg := srv.URL + "/subscriptions/sub1/resourceGroups/invalid-domain-rg"
	resp := httpPut(t, rg, `{"location":"uksouth"}`)
	resp.Body.Close()
	base := rg + "/providers/Microsoft.ApiManagement/service/apim"
	resp = httpPut(t, base, apimBasicBody)
	resp.Body.Close()
	for _, tc := range []struct {
		name, body string
		status     int
	}{
		{"missing-identity", `{"identity":{"type":"UserAssigned","userAssignedIdentities":{"/subscriptions/sub1/resourceGroups/missing/providers/Microsoft.ManagedIdentity/userAssignedIdentities/mi":{}}}}`, 400},
		{"wrong-resource-type", `{"identity":{"type":"UserAssigned","userAssignedIdentities":{"/subscriptions/sub1/resourceGroups/invalid-domain-rg":{}}}}`, 400},
		{"system-identity-explicit-unsupported", `{"identity":{"type":"SystemAssigned"}}`, 501},
		{"no-certificate", `{"properties":{"hostnameConfigurations":[{"type":"DeveloperPortal","hostName":"portal.example.com"}]}}`, 400},
		{"arbitrary-url", `{"properties":{"hostnameConfigurations":[{"type":"Proxy","hostName":"api.example.com","keyVaultId":"https://attacker.test/secrets/example"}]}}`, 400},
		{"invalid-type", `{"properties":{"hostnameConfigurations":[{"type":"Unknown","hostName":"api.example.com"}]}}`, 400},
		{"not-array", `{"properties":{"hostnameConfigurations":{}}}`, 400},
		{"bad-bool", `{"properties":{"hostnameConfigurations":[{"type":"Proxy","hostName":"api.example.com","defaultSslBinding":"true"}]}}`, 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp := httpPatch(t, base, tc.body)
			assertStatus(t, resp, tc.status)
			if decodeJSON(t, resp)["error"] == nil {
				t.Fatal("missing Azure error")
			}
		})
	}
	resp = httpGet(t, base)
	if len(decodeJSON(t, resp)["properties"].(map[string]any)["hostnameConfigurations"].([]any)) != 1 {
		t.Fatal("failed writes mutated stored service")
	}
	resp = httpHead(t, base)
	assertStatus(t, resp, 204)
	if strings.TrimSpace(readBody(t, resp)) != "" {
		t.Fatal("HEAD returned a body")
	}
}

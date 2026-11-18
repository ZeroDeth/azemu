package arm

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/zerodeth/azemu/internal/store"
)

const kvPolicy1 = `{"tenantId":"00000000-0000-0000-0000-000000000001","objectId":"00000000-0000-0000-0000-000000000002","permissions":{"secrets":["Get"]}}`
const kvPolicy2 = `{"tenantId":"00000000-0000-0000-0000-000000000001","objectId":"00000000-0000-0000-0000-000000000003","permissions":{"certificates":["Create"]}}`

func TestKeyVaultAccessPolicy_AddReplaceRemove(t *testing.T) {
	srv := newTestServer(t)
	base := keyVaultURL(srv.URL, "sub1", "rg1", "policyvault")
	request := func(operation, policy string, status int) {
		t.Helper()
		resp := httpPut(t, base+"/accessPolicies/"+operation, `{"properties":{"accessPolicies":[`+policy+`]}}`)
		assertStatus(t, resp, status)
		resp.Body.Close()
	}
	request("add", kvPolicy1, 404)
	resp := httpPut(t, base, keyVaultBodyStandard)
	resp.Body.Close()
	for _, policy := range []string{kvPolicy1, kvPolicy1, kvPolicy2} {
		request("add", policy, 200)
	}
	policies := decodeJSON(t, httpGet(t, base))["properties"].(map[string]any)["accessPolicies"].([]any)
	if len(policies) != 2 {
		t.Fatal("add must preserve other principals and be idempotent")
	}
	request("remove", kvPolicy1, 200)
	policies = decodeJSON(t, httpGet(t, base))["properties"].(map[string]any)["accessPolicies"].([]any)
	if len(policies) != 1 || policies[0].(map[string]any)["objectId"] != "00000000-0000-0000-0000-000000000003" {
		t.Fatal("remove affected unrelated principal")
	}
	request("replace", kvPolicy1, 200)
	policies = decodeJSON(t, httpGet(t, base))["properties"].(map[string]any)["accessPolicies"].([]any)
	if len(policies) != 1 || policies[0].(map[string]any)["objectId"] != "00000000-0000-0000-0000-000000000002" {
		t.Fatal("replace did not replace access policies")
	}
	for _, body := range []string{`{`, `{}`, `{"properties":{"accessPolicies":[{}]}}`} {
		resp := httpPut(t, base+"/accessPolicies/add", body)
		assertStatus(t, resp, http.StatusBadRequest)
		resp.Body.Close()
	}
	req, err := http.NewRequest(http.MethodPut, base+"/accessPolicies/add", nil)
	if err != nil {
		t.Fatal(err)
	}
	missingVersion, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	assertStatus(t, missingVersion, 400)
	missingVersion.Body.Close()
}

func TestKeyVaultPurge_ExplicitOperationStatus(t *testing.T) {
	srv := newTestServer(t)
	path := srv.URL + "/subscriptions/sub1/providers/Microsoft.KeyVault/locations/uksouth/deletedVaults/vault/purge?api-version=2026-02-01"
	resp := httpPost(t, path, "{}")
	assertStatus(t, resp, 202)
	poll := resp.Header.Get("Azure-AsyncOperation")
	if poll == "" || poll != resp.Header.Get("Location") {
		t.Fatal("purge POST must advertise a dedicated status endpoint")
	}
	resp.Body.Close()
	resp = httpGet(t, poll)
	assertStatus(t, resp, 200)
	if decodeJSON(t, resp)["status"] != "Succeeded" {
		t.Fatal("purge operation did not complete")
	}
}

func TestKeyVaultAccessPolicy_CaseInsensitiveIDs(t *testing.T) {
	srv := newTestServer(t)
	base := keyVaultURL(srv.URL, "sub1", "rg1", "casevault")
	tenant := "aabbccdd-0000-0000-0000-000000000001"
	object := "bbccddee-0000-0000-0000-000000000002"
	app := "ccddeeff-0000-0000-0000-000000000003"
	var body map[string]any
	if err := json.Unmarshal([]byte(keyVaultBodyStandard), &body); err != nil {
		t.Fatal(err)
	}
	props := body["properties"].(map[string]any)
	props["tenantId"] = strings.ToUpper(tenant)
	props["accessPolicies"] = []any{map[string]any{
		"tenantId": strings.ToUpper(tenant), "objectId": strings.ToUpper(object),
		"applicationId": strings.ToUpper(app), "permissions": map[string]any{"secrets": []string{"Get"}},
	}}
	encoded, _ := json.Marshal(body)
	resp := httpPut(t, base, string(encoded))
	assertStatus(t, resp, 201)
	resp.Body.Close()
	t.Run("authorize-existing-uppercase-policy", func(t *testing.T) {
		props := decodeJSON(t, httpGet(t, base))["properties"].(map[string]any)
		// Policies can also originate in the parent vault PUT.
		if !keyVaultPolicyAllows(&store.Resource{Properties: props}, object, "secrets", "get") {
			t.Fatal("UUID case changed an existing policy's authorization")
		}
	})
	t.Run("merge-canonicalize-and-remove", func(t *testing.T) {
		policy := map[string]any{
			"tenantId": tenant, "objectId": object, "applicationId": app,
			"permissions": map[string]any{"secrets": []string{"List"}},
		}
		encoded, _ := json.Marshal(map[string]any{"properties": map[string]any{"accessPolicies": []any{policy}}})
		resp := httpPut(t, base+"/accessPolicies/add", string(encoded))
		assertStatus(t, resp, 200)
		resp.Body.Close()
		policies := decodeJSON(t, httpGet(t, base))["properties"].(map[string]any)["accessPolicies"].([]any)
		if len(policies) != 1 {
			t.Fatalf("case-only update created %d policies, want 1", len(policies))
		}
		saved := policies[0].(map[string]any)
		if saved["tenantId"] != tenant || saved["objectId"] != object || saved["applicationId"] != app {
			t.Fatal("updated policy UUIDs were not canonicalized")
		}
		if len(saved["permissions"].(map[string]any)["secrets"].([]any)) != 2 {
			t.Fatal("case-only update lost existing permissions")
		}
		props := decodeJSON(t, httpGet(t, base))["properties"].(map[string]any)
		if !keyVaultPolicyAllows(&store.Resource{Properties: props}, strings.ToUpper(object), "secrets", "get") {
			t.Fatal("canonical policy must authorize against an uppercase vault tenant and principal")
		}
		policy["tenantId"], policy["objectId"], policy["applicationId"] =
			strings.ToUpper(tenant), strings.ToUpper(object), strings.ToUpper(app)
		encoded, _ = json.Marshal(map[string]any{"properties": map[string]any{"accessPolicies": []any{policy}}})
		resp = httpPut(t, base+"/accessPolicies/remove", string(encoded))
		assertStatus(t, resp, 200)
		resp.Body.Close()
		policies = decodeJSON(t, httpGet(t, base))["properties"].(map[string]any)["accessPolicies"].([]any)
		if len(policies) != 0 {
			t.Fatal("case-only principal reference did not remove the policy")
		}
	})
}

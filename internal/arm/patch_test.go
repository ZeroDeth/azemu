package arm

import (
	"net/http"
	"testing"
)

// The three resources whose azurerm provider updates by PATCH. Each is created
// with PUT, then patched, so the test exercises the real route table through
// the production middleware stack rather than calling the handler directly.
var patchCases = []struct {
	name       string
	path       string
	createBody string
	// a property the create body sets, which a later PATCH must not disturb
	keepKey  string
	keepWant string
}{
	{
		name: "key vault",
		path: "/subscriptions/sub1/resourceGroups/rg1/providers/Microsoft.KeyVault/vaults/kv1",
		createBody: `{"location":"uksouth","tags":{"env":"dev"},"properties":{
			"tenantId":"00000000-0000-0000-0000-000000000001",
			"sku":{"family":"A","name":"standard"}}}`,
		keepKey:  "tenantId",
		keepWant: "00000000-0000-0000-0000-000000000001",
	},
	{
		name: "redis cache",
		path: "/subscriptions/sub1/resourceGroups/rg1/providers/Microsoft.Cache/Redis/redis1",
		createBody: `{"location":"uksouth","tags":{"env":"dev"},"properties":{
			"sku":{"name":"Standard","family":"C","capacity":1},
			"minimumTlsVersion":"1.2"}}`,
		keepKey:  "minimumTlsVersion",
		keepWant: "1.2",
	},
	{
		name: "user assigned identity",
		path: "/subscriptions/sub1/resourceGroups/rg1/providers/Microsoft.ManagedIdentity/" +
			"userAssignedIdentities/uai1",
		createBody: `{"location":"uksouth","tags":{"env":"dev"}}`,
	},
}

// TestPatch_updatesTagsAndKeepsProperties is the case that matters in practice:
// `terraform apply` a scenario, change a tag, apply again. Before PATCH existed
// this returned 405 and the apply failed.
func TestPatch_updatesTagsAndKeepsProperties(t *testing.T) {
	for _, tc := range patchCases {
		t.Run(tc.name, func(t *testing.T) {
			srv := newTestServer(t)
			url := srv.URL + tc.path

			if resp := httpPut(t, url, tc.createBody); resp.StatusCode != http.StatusCreated {
				t.Fatalf("create: status = %d, want 201", resp.StatusCode)
			}

			resp := httpPatch(t, url, `{"tags":{"env":"prod","owner":"platform"}}`)
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("patch: status = %d, want 200", resp.StatusCode)
			}
			body := decodeJSON(t, resp)

			tags, _ := body["tags"].(map[string]interface{})
			if tags["env"] != "prod" {
				t.Errorf("tags.env = %v, want prod", tags["env"])
			}
			if tags["owner"] != "platform" {
				t.Errorf("tags.owner = %v, want platform", tags["owner"])
			}

			// A property the PATCH never mentioned must survive.
			if tc.keepKey != "" {
				props, _ := body["properties"].(map[string]interface{})
				if got := props[tc.keepKey]; got != tc.keepWant {
					t.Errorf("properties.%s = %v after patch, want %v (absent keys must be preserved)",
						tc.keepKey, got, tc.keepWant)
				}
			}

			// The response must match what a subsequent GET returns, or the
			// provider's read-back disagrees with what it was just told.
			got := decodeJSON(t, httpGet(t, url))
			gotTags, _ := got["tags"].(map[string]interface{})
			if gotTags["env"] != "prod" || gotTags["owner"] != "platform" {
				t.Errorf("GET after patch returned tags %v, want the patched set", gotTags)
			}
		})
	}
}

// TestPatch_tagsReplaceWholesale pins the difference between ARM's PATCH and a
// deep merge. Terraform removes a tag by sending the set without it; a
// key-by-key merge would make removal impossible.
func TestPatch_tagsReplaceWholesale(t *testing.T) {
	srv := newTestServer(t)
	url := srv.URL + patchCases[0].path

	httpPut(t, url, patchCases[0].createBody)
	httpPatch(t, url, `{"tags":{"a":"1","b":"2"}}`)

	body := decodeJSON(t, httpPatch(t, url, `{"tags":{"a":"1"}}`))
	tags, _ := body["tags"].(map[string]interface{})

	if _, still := tags["b"]; still {
		t.Errorf("tags = %v; b survived, so tags were merged rather than replaced", tags)
	}
	if tags["a"] != "1" {
		t.Errorf("tags.a = %v, want 1", tags["a"])
	}
}

// TestPatch_mergesProperties covers a property update rather than a tag one.
func TestPatch_mergesProperties(t *testing.T) {
	srv := newTestServer(t)
	url := srv.URL + patchCases[1].path

	httpPut(t, url, patchCases[1].createBody)

	body := decodeJSON(t, httpPatch(t, url, `{"properties":{"enableNonSslPort":true}}`))
	props, _ := body["properties"].(map[string]interface{})

	if props["enableNonSslPort"] != true {
		t.Errorf("properties.enableNonSslPort = %v, want true", props["enableNonSslPort"])
	}
	if props["minimumTlsVersion"] != "1.2" {
		t.Errorf("properties.minimumTlsVersion = %v, want 1.2 (untouched key must survive)",
			props["minimumTlsVersion"])
	}
	if props["provisioningState"] != "Succeeded" {
		t.Errorf("provisioningState = %v, want Succeeded", props["provisioningState"])
	}
}

// TestPatch_missingResource_404 pins that PATCH never creates. ARM answers 404,
// and an implicit create would hide a Terraform state mismatch.
func TestPatch_missingResource_404(t *testing.T) {
	for _, tc := range patchCases {
		t.Run(tc.name, func(t *testing.T) {
			srv := newTestServer(t)
			url := srv.URL + tc.path

			resp := httpPatch(t, url, `{"tags":{"env":"prod"}}`)
			if resp.StatusCode != http.StatusNotFound {
				t.Fatalf("status = %d, want 404", resp.StatusCode)
			}

			// And it really did not create anything.
			if got := httpGet(t, url); got.StatusCode != http.StatusNotFound {
				t.Errorf("GET after failed patch = %d, want 404; PATCH created the resource",
					got.StatusCode)
			}
		})
	}
}

// TestPatch_badBody_leavesStoreUntouched guards the shallow-copy trap:
// store.Get returns a struct copy whose maps still alias stored state, so
// merging in place would mutate the resource even when the request is rejected.
func TestPatch_badBody_leavesStoreUntouched(t *testing.T) {
	srv := newTestServer(t)
	url := srv.URL + patchCases[0].path

	httpPut(t, url, patchCases[0].createBody)

	resp := httpPatch(t, url, `{"tags":"not-an-object"}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}

	body := decodeJSON(t, httpGet(t, url))
	tags, _ := body["tags"].(map[string]interface{})
	if tags["env"] != "dev" {
		t.Errorf("tags = %v after a rejected patch, want the original {env:dev}", tags)
	}
}

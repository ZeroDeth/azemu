package arm

import (
	"fmt"
	"net/http"
	"testing"
)

// rgURL builds the canonical resource group URL for a subscription + RG name.
func rgURL(srvURL, sub, rg string) string {
	return fmt.Sprintf("%s/subscriptions/%s/resourcegroups/%s", srvURL, sub, rg)
}

// rgListURL builds the URL for listing all resource groups in a subscription.
func rgListURL(srvURL, sub string) string {
	return fmt.Sprintf("%s/subscriptions/%s/resourcegroups", srvURL, sub)
}

const rgBodyMinimal = `{"location": "uksouth"}`

// TestRG_PUT_Creates_Returns201 verifies the first write to a new RG returns
// 201 and includes the required ARM response fields.
func TestRG_PUT_Creates_Returns201(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, rgURL(srv.URL, "sub1", "rg1"), rgBodyMinimal)
	assertStatus(t, resp, http.StatusCreated)

	body := decodeJSON(t, resp)
	if body["name"] != "rg1" {
		t.Errorf("name = %v, want rg1", body["name"])
	}
	if body["type"] != "Microsoft.Resources/resourceGroups" {
		t.Errorf("type = %v, want Microsoft.Resources/resourceGroups", body["type"])
	}
	if body["location"] != "uksouth" {
		t.Errorf("location = %v, want uksouth", body["location"])
	}
	wantID := "/subscriptions/sub1/resourceGroups/rg1"
	if body["id"] != wantID {
		t.Errorf("id = %v, want %s", body["id"], wantID)
	}

	props, ok := body["properties"].(map[string]interface{})
	if !ok {
		t.Fatalf("properties missing or wrong type: %T", body["properties"])
	}
	if props["provisioningState"] != "Succeeded" {
		t.Errorf("provisioningState = %v, want Succeeded", props["provisioningState"])
	}
}

// TestRG_PUT_Idempotent_Returns200 verifies a second PUT to the same resource
// group returns 200, not 201.
func TestRG_PUT_Idempotent_Returns200(t *testing.T) {
	srv := newTestServer(t)
	url := rgURL(srv.URL, "sub1", "rg1")

	resp := httpPut(t, url, rgBodyMinimal)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = httpPut(t, url, rgBodyMinimal)
	assertStatus(t, resp, http.StatusOK)

	body := decodeJSON(t, resp)
	if body["name"] != "rg1" {
		t.Errorf("name = %v, want rg1", body["name"])
	}
}

// TestRG_PUT_MissingLocation_Returns400 verifies that a PUT body with no
// location is rejected with 400 InvalidRequestContent. This matches the
// validation pattern used by vnet.go / subnet.go; resourcegroup.go used to
// accept `{}` silently and was pinned by an earlier
// TestRG_PUT_MissingLocation_CurrentlyAccepted stub. The handler was brought
// in line during the Phase 2 closeout batch (TODO.md "Known Gaps").
func TestRG_PUT_MissingLocation_Returns400(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, rgURL(srv.URL, "sub1", "rg-noloc"), `{}`)
	assertStatus(t, resp, http.StatusBadRequest)

	errBody := decodeJSON(t, resp)
	errObj, ok := errBody["error"].(map[string]interface{})
	if !ok {
		t.Fatalf("error object missing: %v", errBody)
	}
	if errObj["code"] != "InvalidRequestContent" {
		t.Errorf("code = %v, want InvalidRequestContent", errObj["code"])
	}
}

// TestRG_PUT_WhitespaceOnlyLocation_Returns400 ensures the location trim
// matches the vnet.go behaviour: a location that is only whitespace is the
// same as an empty location.
func TestRG_PUT_WhitespaceOnlyLocation_Returns400(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, rgURL(srv.URL, "sub1", "rg-ws"), `{"location": "   "}`)
	assertStatus(t, resp, http.StatusBadRequest)

	errBody := decodeJSON(t, resp)
	errObj := errBody["error"].(map[string]interface{})
	if errObj["code"] != "InvalidRequestContent" {
		t.Errorf("code = %v, want InvalidRequestContent", errObj["code"])
	}
}

// TestRG_PUT_InvalidJSON_Returns400 verifies that a malformed request body
// returns 400 with the Azure error code InvalidRequestContent.
func TestRG_PUT_InvalidJSON_Returns400(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, rgURL(srv.URL, "sub1", "rg1"), `{not json`)
	assertStatus(t, resp, http.StatusBadRequest)

	errBody := decodeJSON(t, resp)
	errObj, ok := errBody["error"].(map[string]interface{})
	if !ok {
		t.Fatalf("error object missing: %v", errBody)
	}
	if errObj["code"] != "InvalidRequestContent" {
		t.Errorf("code = %v, want InvalidRequestContent", errObj["code"])
	}
}

// TestRG_PUT_LocationNormalizedLowercase verifies that a location provided in
// mixed-case is stored and returned in lowercase.
func TestRG_PUT_LocationNormalizedLowercase(t *testing.T) {
	srv := newTestServer(t)
	url := rgURL(srv.URL, "sub1", "rg1")

	resp := httpPut(t, url, `{"location": "WestEurope"}`)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = httpGet(t, url)
	assertStatus(t, resp, http.StatusOK)
	body := decodeJSON(t, resp)
	if body["location"] != "westeurope" {
		t.Errorf("location = %v, want westeurope", body["location"])
	}
}

// TestRG_GET_Existing_Returns200 verifies that a GET on an existing RG returns
// 200 with the correct ARM response shape.
func TestRG_GET_Existing_Returns200(t *testing.T) {
	srv := newTestServer(t)
	url := rgURL(srv.URL, "sub1", "rg1")

	resp := httpPut(t, url, rgBodyMinimal)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = httpGet(t, url)
	assertStatus(t, resp, http.StatusOK)
	body := decodeJSON(t, resp)

	if body["name"] != "rg1" {
		t.Errorf("name = %v, want rg1", body["name"])
	}
	if body["type"] != "Microsoft.Resources/resourceGroups" {
		t.Errorf("type = %v, want Microsoft.Resources/resourceGroups", body["type"])
	}
	if body["location"] != "uksouth" {
		t.Errorf("location = %v, want uksouth", body["location"])
	}
	props, ok := body["properties"].(map[string]interface{})
	if !ok {
		t.Fatalf("properties missing: %T", body["properties"])
	}
	if props["provisioningState"] != "Succeeded" {
		t.Errorf("provisioningState = %v, want Succeeded", props["provisioningState"])
	}
}

// TestRG_GET_Missing_Returns404 verifies that a GET on a non-existent RG
// returns 404 with error code ResourceGroupNotFound.
func TestRG_GET_Missing_Returns404(t *testing.T) {
	srv := newTestServer(t)
	resp := httpGet(t, rgURL(srv.URL, "sub1", "ghost"))
	assertStatus(t, resp, http.StatusNotFound)

	errBody := decodeJSON(t, resp)
	errObj, ok := errBody["error"].(map[string]interface{})
	if !ok {
		t.Fatalf("error object missing: %v", errBody)
	}
	if errObj["code"] != "ResourceGroupNotFound" {
		t.Errorf("code = %v, want ResourceGroupNotFound", errObj["code"])
	}
}

// TestRG_GET_WithoutAPIVersion_Returns400 verifies that the RequireAPIVersion
// middleware rejects requests that omit ?api-version= with 400 and error
// code MissingApiVersionParameter.
func TestRG_GET_WithoutAPIVersion_Returns400(t *testing.T) {
	srv := newTestServer(t)
	// Use the raw helper so withAPIVersion does not inject the default.
	resp := httpGetRaw(t, rgURL(srv.URL, "sub1", "rg1"))
	assertStatus(t, resp, http.StatusBadRequest)

	body := decodeJSON(t, resp)
	errObj, ok := body["error"].(map[string]interface{})
	if !ok {
		t.Fatalf("error object missing: %v", body)
	}
	if errObj["code"] != "MissingApiVersionParameter" {
		t.Errorf("code = %v, want MissingApiVersionParameter", errObj["code"])
	}
}

// TestRG_HEAD_Existing_Returns204_EmptyBody verifies the HEAD handler returns
// 204 No Content with an empty body for an existing RG.
func TestRG_HEAD_Existing_Returns204_EmptyBody(t *testing.T) {
	srv := newTestServer(t)
	url := rgURL(srv.URL, "sub1", "rg1")

	resp := httpPut(t, url, rgBodyMinimal)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = httpHead(t, url)
	assertStatus(t, resp, http.StatusNoContent)
	if body := readBody(t, resp); body != "" {
		t.Errorf("HEAD body = %q, want empty", body)
	}
}

// TestRG_HEAD_Missing_Returns404_EmptyBody verifies the HEAD handler returns
// 404 with an empty body for a non-existent RG (HEAD semantics require no
// body even on error responses).
func TestRG_HEAD_Missing_Returns404_EmptyBody(t *testing.T) {
	srv := newTestServer(t)
	resp := httpHead(t, rgURL(srv.URL, "sub1", "ghost"))
	assertStatus(t, resp, http.StatusNotFound)
	if body := readBody(t, resp); body != "" {
		t.Errorf("HEAD body = %q, want empty", body)
	}
}

// TestRG_DELETE_Existing_Returns202_WithLocationHeader verifies that deleting
// an existing RG returns 202 Accepted with a non-empty Location header, as
// required by the ARM async delete contract.
func TestRG_DELETE_Existing_Returns202_WithLocationHeader(t *testing.T) {
	srv := newTestServer(t)
	url := rgURL(srv.URL, "sub1", "rg1")

	resp := httpPut(t, url, rgBodyMinimal)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = httpDelete(t, url)
	assertStatus(t, resp, http.StatusAccepted)
	if loc := resp.Header.Get("Location"); loc == "" {
		t.Errorf("Location header missing on DELETE response")
	}
	resp.Body.Close()

	// Follow-up GET must be 404.
	resp = httpGet(t, url)
	assertStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
}

// TestRG_DELETE_Missing_Returns404 verifies that deleting a non-existent RG
// returns 404 with error code ResourceGroupNotFound.
func TestRG_DELETE_Missing_Returns404(t *testing.T) {
	srv := newTestServer(t)
	resp := httpDelete(t, rgURL(srv.URL, "sub1", "ghost"))
	assertStatus(t, resp, http.StatusNotFound)

	errBody := decodeJSON(t, resp)
	errObj, ok := errBody["error"].(map[string]interface{})
	if !ok {
		t.Fatalf("error object missing: %v", errBody)
	}
	if errObj["code"] != "ResourceGroupNotFound" {
		t.Errorf("code = %v, want ResourceGroupNotFound", errObj["code"])
	}
}

// TestRG_DELETE_CascadesChildren verifies that deleting an RG also deletes
// all child resources (VNets, Subnets) via the store's prefix-match delete.
func TestRG_DELETE_CascadesChildren(t *testing.T) {
	srv := newTestServer(t)

	// Create the RG.
	resp := httpPut(t, rgURL(srv.URL, "sub1", "rg-cascade"), rgBodyMinimal)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	// Create a VNet inside the RG.
	vnetU := vnetURL(srv.URL, "sub1", "rg-cascade", "vnet1")
	resp = httpPut(t, vnetU, vnetBodyMinimal)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	// Create a Subnet inside the VNet.
	subnetU := subnetURL(srv.URL, "sub1", "rg-cascade", "vnet1", "sub1")
	resp = httpPut(t, subnetU, subnetBodyMinimal)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	// Delete the RG.
	resp = httpDelete(t, rgURL(srv.URL, "sub1", "rg-cascade"))
	assertStatus(t, resp, http.StatusAccepted)
	resp.Body.Close()

	// VNet must be gone.
	resp = httpGet(t, vnetU)
	assertStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()

	// Subnet must also be gone.
	resp = httpGet(t, subnetU)
	assertStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
}

// TestRG_LIST_ReturnsAllInSubscription seeds two resource groups and verifies
// the list endpoint returns both wrapped in {"value": [...]}.
func TestRG_LIST_ReturnsAllInSubscription(t *testing.T) {
	srv := newTestServer(t)

	resp := httpPut(t, rgURL(srv.URL, "sub1", "rg-a"), rgBodyMinimal)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = httpPut(t, rgURL(srv.URL, "sub1", "rg-b"), rgBodyMinimal)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = httpGet(t, rgListURL(srv.URL, "sub1"))
	assertStatus(t, resp, http.StatusOK)
	body := decodeJSON(t, resp)
	items, ok := body["value"].([]interface{})
	if !ok {
		t.Fatalf("value missing or wrong type: %T", body["value"])
	}
	if len(items) != 2 {
		t.Errorf("len(items) = %d, want 2", len(items))
	}
}

// TestRG_LIST_EmptySubscription_ReturnsEmptyArray verifies that listing
// resource groups for a subscription with none returns {"value": []}, not
// null and not a missing key.
func TestRG_LIST_EmptySubscription_ReturnsEmptyArray(t *testing.T) {
	srv := newTestServer(t)
	resp := httpGet(t, rgListURL(srv.URL, "sub-empty"))
	assertStatus(t, resp, http.StatusOK)

	body := decodeJSON(t, resp)
	items, ok := body["value"].([]interface{})
	if !ok {
		t.Fatalf("value missing or wrong type: %T", body["value"])
	}
	if len(items) != 0 {
		t.Errorf("len(items) = %d, want 0", len(items))
	}
}

// TestRG_PUT_NilTags_NormalisedToEmptyObject verifies that when a client
// sends no tags field, the response contains "tags": {} (not null), matching
// real Azure behaviour.
func TestRG_PUT_NilTags_NormalisedToEmptyObject(t *testing.T) {
	srv := newTestServer(t)
	// rgBodyMinimal has no "tags" key, so body.Tags will be nil.
	resp := httpPut(t, rgURL(srv.URL, "sub1", "rg1"), rgBodyMinimal)
	assertStatus(t, resp, http.StatusCreated)

	body := decodeJSON(t, resp)
	tags, ok := body["tags"]
	if !ok {
		t.Fatal("tags field missing from response")
	}
	tagsMap, ok := tags.(map[string]interface{})
	if !ok {
		t.Fatalf("tags is %T, want map (JSON object); got %v", tags, tags)
	}
	if len(tagsMap) != 0 {
		t.Errorf("tags should be empty object, got %v", tagsMap)
	}
}

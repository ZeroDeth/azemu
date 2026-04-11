//go:build integration

// metadata_test covers the /metadata/endpoints contract through the
// production mux. The canonical schema in internal/metadata/service.go was
// the root cause of three Phase 1 blockers (M1, M2, M3 in TODO.md) before
// it was rewritten against ground truth from
// https://management.azure.com/metadata/endpoints?api-version=2022-09-01.
// These assertions pin the classifier-sensitive fields so a future refactor
// that reintroduces the hand-rolled schema gets caught here, not in
// terraform apply.

package integration

import (
	"net/http"
	"strings"
	"testing"
)

// TestMetadata_Endpoints_CanonicalShape hits /metadata/endpoints through
// the full middleware stack and verifies every field that M1, M2, M3 in
// TODO.md proved matter to go-azure-sdk. The cheapest regression net for
// the IsAzureStack classifier and the per-service authorizer builders.
func TestMetadata_Endpoints_CanonicalShape(t *testing.T) {
	srv := buildProductionLikeServer(t)

	resp, err := srv.Client().Get(srv.URL + "/metadata/endpoints")
	if err != nil {
		t.Fatalf("GET /metadata/endpoints: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body := decode(t, resp)

	// Top-level identification — provider loader keys on this.
	if body["name"] != "AzureCloud" {
		t.Errorf("name = %v, want AzureCloud", body["name"])
	}

	// Authentication block: the M2 classifier fix. tenant MUST be "common"
	// (the cloud-environment identifier) for go-azure-sdk's IsAzureStack
	// check to return false. If this regresses, the provider rejects the
	// whole environment with "does not support Azure Stack".
	authBlock, ok := body["authentication"].(map[string]interface{})
	if !ok {
		t.Fatalf("authentication block missing: %v", body)
	}
	if authBlock["tenant"] != "common" {
		t.Errorf("authentication.tenant = %v, want \"common\" (M2 classifier pin)", authBlock["tenant"])
	}
	loginEndpoint, ok := authBlock["loginEndpoint"].(string)
	if !ok || !strings.HasPrefix(loginEndpoint, "https://") {
		t.Errorf("authentication.loginEndpoint = %v, want https:// prefix (M1 dataPlane fix)", authBlock["loginEndpoint"])
	}

	// Required canonical top-level fields. Any missing entry regresses
	// an M3-class bug where a hand-rolled schema left go-azure-sdk unable
	// to construct a per-service authorizer.
	requiredFields := []string{
		"resourceManager",
		"activeDirectoryDataLake",
		"microsoftGraphResourceId",
		"appServiceResourceId",
		"appInsightsResourceId",
		"attestationResourceId",
		"synapseAnalyticsResourceId",
		"logAnalyticsResourceId",
		"ossrDbmsResourceId",
		"portal",
		"graph",
		"media",
		"batch",
		"sqlManagement",
	}
	for _, field := range requiredFields {
		if _, ok := body[field]; !ok {
			t.Errorf("top-level field %q missing — would break per-service authorizer construction", field)
		}
	}

	// Suffixes block. These are domain strings (not endpoints) but the
	// naming must match real Azure verbatim. `suffixes.storage` (not
	// `storageEndpoint`) was the M3 fix that unblocked the Storage
	// authorizer.
	suffixes, ok := body["suffixes"].(map[string]interface{})
	if !ok {
		t.Fatalf("suffixes block missing: %v", body)
	}
	requiredSuffixes := []string{
		"storage",
		"keyVaultDns",
		"sqlServerHostname",
		"acrLoginServer",
	}
	for _, s := range requiredSuffixes {
		if _, ok := suffixes[s]; !ok {
			t.Errorf("suffixes.%s missing — canonical schema drift", s)
		}
	}
	if suffixes["storage"] != "core.windows.net" {
		t.Errorf("suffixes.storage = %v, want core.windows.net", suffixes["storage"])
	}
}

// TestMetadata_Endpoints_AllRedirectedURLsAreHTTPS pins the M1 dataPlane
// fix: every azemu-redirected URL in the metadata response must use
// https://, never http://. Real Azure's dataPlane is HTTPS, and the
// IsAzureStack classifier in go-azure-sdk rejects http:// localhost as
// "not a valid public-cloud endpoint". M1 in TODO.md is the post-mortem.
func TestMetadata_Endpoints_AllRedirectedURLsAreHTTPS(t *testing.T) {
	srv := buildProductionLikeServer(t)

	resp, err := srv.Client().Get(srv.URL + "/metadata/endpoints")
	if err != nil {
		t.Fatalf("GET /metadata/endpoints: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body := decode(t, resp)

	// Walk the top-level fields that hold redirected URLs. Any string value
	// that starts with "http://localhost" is an M1-class regression.
	for key, v := range body {
		s, ok := v.(string)
		if !ok {
			continue
		}
		if strings.HasPrefix(s, "http://localhost") || strings.HasPrefix(s, "http://127.0.0.1") {
			t.Errorf("field %q = %q is http://, must be https:// (M1 dataPlane fix)", key, s)
		}
	}

	// Also check the authentication block.
	authBlock := body["authentication"].(map[string]interface{})
	if lep, ok := authBlock["loginEndpoint"].(string); ok {
		if strings.HasPrefix(lep, "http://localhost") || strings.HasPrefix(lep, "http://127.0.0.1") {
			t.Errorf("authentication.loginEndpoint = %q is http://, must be https://", lep)
		}
	}
}

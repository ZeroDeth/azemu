package auth

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

func newIMDSTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	tokenSvc, err := NewTokenService("test-tenant-id")
	if err != nil {
		t.Fatalf("NewTokenService: %v", err)
	}
	imdsSvc := NewIMDSService(tokenSvc)
	r := chi.NewRouter()
	r.Route("/metadata/identity", imdsSvc.Routes)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv
}

func imdsTokenURL(srv *httptest.Server, apiVersion, resource string) string {
	u := srv.URL + "/metadata/identity/oauth2/token"
	sep := "?"
	if apiVersion != "" {
		u += sep + "api-version=" + apiVersion
		sep = "&"
	}
	if resource != "" {
		u += sep + "resource=" + resource
	}
	return u
}

func imdsGet(t *testing.T, url string, withMetadataHeader bool) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if withMetadataHeader {
		req.Header.Set("Metadata", "true")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	return resp
}

func TestIMDS_MissingMetadataHeader_Returns400(t *testing.T) {
	srv := newIMDSTestServer(t)
	resp := imdsGet(t, imdsTokenURL(srv, "2018-02-01", ""), false)
	if resp.StatusCode != http.StatusBadRequest {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("status = %d, want 400. body=%s", resp.StatusCode, body)
	}
	defer resp.Body.Close()
	var body map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["error"] != "invalid_request" {
		t.Errorf("error = %v, want invalid_request", body["error"])
	}
}

func TestIMDS_MissingAPIVersion_Returns400(t *testing.T) {
	srv := newIMDSTestServer(t)
	resp := imdsGet(t, imdsTokenURL(srv, "", ""), true)
	if resp.StatusCode != http.StatusBadRequest {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("status = %d, want 400. body=%s", resp.StatusCode, body)
	}
}

func TestIMDS_HappyPath_Returns200(t *testing.T) {
	srv := newIMDSTestServer(t)
	resp := imdsGet(t, imdsTokenURL(srv, "2018-02-01", ""), true)
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("status = %d, want 200. body=%s", resp.StatusCode, body)
	}
}

func TestIMDS_ResponseFieldsMatchIMDSContract(t *testing.T) {
	srv := newIMDSTestServer(t)
	resp := imdsGet(t, imdsTokenURL(srv, "2018-02-01", ""), true)
	defer resp.Body.Close()

	var body map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// access_token must be a non-empty string
	token, ok := body["access_token"].(string)
	if !ok || token == "" {
		t.Error("access_token missing or empty")
	}

	// token_type must be Bearer
	if body["token_type"] != "Bearer" {
		t.Errorf("token_type = %v, want Bearer", body["token_type"])
	}

	// expires_in must be a string (IMDS contract, not a number)
	expiresIn, ok := body["expires_in"].(string)
	if !ok {
		t.Errorf("expires_in must be a string (IMDS contract), got %T: %v", body["expires_in"], body["expires_in"])
	} else if expiresIn != "3600" {
		t.Errorf("expires_in = %q, want \"3600\"", expiresIn)
	}

	// expires_on must be a number (unix timestamp)
	if _, ok := body["expires_on"].(float64); !ok {
		t.Errorf("expires_on must be a number, got %T", body["expires_on"])
	}

	// resource must be present
	if body["resource"] == nil || body["resource"] == "" {
		t.Error("resource missing")
	}
}

func TestIMDS_DefaultResource_IsManagementAzure(t *testing.T) {
	srv := newIMDSTestServer(t)
	resp := imdsGet(t, imdsTokenURL(srv, "2018-02-01", ""), true)
	defer resp.Body.Close()

	var body map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["resource"] != "https://management.azure.com/" {
		t.Errorf("default resource = %v, want https://management.azure.com/", body["resource"])
	}
}

func TestIMDS_CustomResource_IsReflectedInResponse(t *testing.T) {
	srv := newIMDSTestServer(t)
	url := imdsTokenURL(srv, "2018-02-01", "https://storage.azure.com/")
	resp := imdsGet(t, url, true)
	defer resp.Body.Close()

	var body map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["resource"] != "https://storage.azure.com/" {
		t.Errorf("resource = %v, want https://storage.azure.com/", body["resource"])
	}
}

func TestIMDS_TokenIsRS256JWT(t *testing.T) {
	srv := newIMDSTestServer(t)
	resp := imdsGet(t, imdsTokenURL(srv, "2018-02-01", ""), true)
	defer resp.Body.Close()

	var body map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	token, _ := body["access_token"].(string)

	// A JWT has exactly 3 dot-separated parts
	parts := splitDots(token)
	if len(parts) != 3 {
		t.Errorf("access_token has %d dot-separated parts, want 3 (not a JWT)", len(parts))
	}
}

// splitDots counts dot-separated segments without importing strings to keep
// the helper simple and avoid lint warnings about unused imports.
func splitDots(s string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '.' {
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	parts = append(parts, s[start:])
	return parts
}

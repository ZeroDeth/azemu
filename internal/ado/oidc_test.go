package ado

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

func newOIDCTestServer(t *testing.T) (*httptest.Server, *OIDCService) {
	t.Helper()
	svc, err := NewOIDCService()
	if err != nil {
		t.Fatalf("NewOIDCService: %v", err)
	}
	r := chi.NewRouter()
	svc.Routes(r)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv, svc
}

func TestOIDC_NewOIDCService_Succeeds(t *testing.T) {
	_, err := NewOIDCService()
	if err != nil {
		t.Fatalf("NewOIDCService error: %v", err)
	}
}

func TestOIDC_TokenEndpoint_Returns200(t *testing.T) {
	srv, _ := newOIDCTestServer(t)
	url := srv.URL + "/myorg/myproject/_apis/distributedtask/hubs/Gates/plans/plan-id-1/jobs/job-id-1/oidctoken"
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 200. body=%s", resp.StatusCode, body)
	}
}

func TestOIDC_TokenEndpoint_ReturnsOIDCToken(t *testing.T) {
	srv, _ := newOIDCTestServer(t)
	url := srv.URL + "/myorg/myproject/_apis/distributedtask/hubs/Gates/plans/plan-id-1/jobs/job-id-1/oidctoken"
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	var body map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	token, ok := body["oidcToken"].(string)
	if !ok || token == "" {
		t.Errorf("oidcToken missing or empty: %v", body)
	}
}

func TestOIDC_TokenIsRS256JWT(t *testing.T) {
	srv, _ := newOIDCTestServer(t)
	url := srv.URL + "/myorg/myproject/_apis/distributedtask/hubs/Gates/plans/plan-id-1/jobs/job-id-1/oidctoken"
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	var body map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	token, _ := body["oidcToken"].(string)

	// JWT = header.payload.signature (3 dot-separated parts)
	count := 0
	for _, c := range token {
		if c == '.' {
			count++
		}
	}
	if count != 2 {
		t.Errorf("oidcToken has %d dots, want 2 (not a JWT): %q", count, token)
	}
}

func TestOIDC_OpenIDConfig_Returns200(t *testing.T) {
	srv, _ := newOIDCTestServer(t)
	resp, err := http.Get(srv.URL + "/.well-known/openid-configuration")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 200. body=%s", resp.StatusCode, body)
	}
}

func TestOIDC_OpenIDConfig_RequiredFields(t *testing.T) {
	srv, _ := newOIDCTestServer(t)
	resp, err := http.Get(srv.URL + "/.well-known/openid-configuration")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	var body map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}

	for _, field := range []string{"issuer", "jwks_uri", "response_types_supported", "id_token_signing_alg_values_supported"} {
		if body[field] == nil {
			t.Errorf("openid-configuration missing field %q", field)
		}
	}
}

func TestOIDC_JWKS_Returns200(t *testing.T) {
	srv, _ := newOIDCTestServer(t)
	resp, err := http.Get(srv.URL + "/discovery/keys")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 200. body=%s", resp.StatusCode, body)
	}
}

func TestOIDC_JWKS_ContainsRSAKey(t *testing.T) {
	srv, _ := newOIDCTestServer(t)
	resp, err := http.Get(srv.URL + "/discovery/keys")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	var body map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	keys, ok := body["keys"].([]interface{})
	if !ok || len(keys) == 0 {
		t.Fatal("keys array missing or empty")
	}
	key, ok := keys[0].(map[string]interface{})
	if !ok {
		t.Fatal("first key is not an object")
	}
	if key["kty"] != "RSA" {
		t.Errorf("kty = %v, want RSA", key["kty"])
	}
	if key["alg"] != "RS256" {
		t.Errorf("alg = %v, want RS256", key["alg"])
	}
	for _, field := range []string{"n", "e", "kid"} {
		if key[field] == nil || key[field] == "" {
			t.Errorf("JWKS key missing field %q", field)
		}
	}
}

func TestOIDC_DifferentServices_HaveDifferentKeys(t *testing.T) {
	svc1, _ := NewOIDCService()
	svc2, _ := NewOIDCService()

	r1 := chi.NewRouter()
	svc1.Routes(r1)
	srv1 := httptest.NewServer(r1)
	defer srv1.Close()

	r2 := chi.NewRouter()
	svc2.Routes(r2)
	srv2 := httptest.NewServer(r2)
	defer srv2.Close()

	getN := func(t *testing.T, srv *httptest.Server) string {
		t.Helper()
		resp, err := http.Get(srv.URL + "/discovery/keys")
		if err != nil {
			t.Fatalf("GET keys: %v", err)
		}
		defer resp.Body.Close()
		var body map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		keys := body["keys"].([]interface{})
		key := keys[0].(map[string]interface{})
		return key["n"].(string)
	}

	n1 := getN(t, srv1)
	n2 := getN(t, srv2)
	if n1 == n2 {
		t.Error("two OIDCService instances should have different RSA keys")
	}
}

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
	"encoding/json"
	"io"
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
	ar := arm.NewRouter(s)
	r := chi.NewRouter()
	r.Use(mw.NormalizePath)
	r.Use(mw.AzureHeaders)
	r.Use(mw.RequireAPIVersion)
	r.Route("/subscriptions", ar.Routes)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv
}

const apiVersionQ = "?api-version=2023-09-01"

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

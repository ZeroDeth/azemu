//go:build integration

package integration

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/zerodeth/azemu/internal/arm"
	mw "github.com/zerodeth/azemu/internal/middleware"
	"github.com/zerodeth/azemu/internal/store"
)

// buildServerWithStore assembles a chi router backed by the given Store,
// using the same middleware stack as buildFullServer. This lets the persist
// test supply a FileStore while reusing the production routing.
func buildServerWithStore(t *testing.T, s store.Store) *httptest.Server {
	t.Helper()
	ar := arm.NewRouter(s, "http://azurite-test:10000", "https://kv-test", "redis://redis-test:6379")
	r := chi.NewRouter()
	r.Use(mw.NormalizePath)
	r.Use(mw.AzureHeaders)
	r.Use(mw.RequireAPIVersion)
	r.Route("/subscriptions", ar.Routes)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv
}

// TestPersist_StatesSurvivesRestart verifies that resources written through
// the ARM API are persisted to disk and survive a full server restart.
//
// Flow:
//  1. Create a FileStore at a temp path.
//  2. Build an HTTP server backed by that store.
//  3. Create a resource group and a vnet via PUT.
//  4. Shut down the server (close the httptest.Server).
//  5. Create a new FileStore at the same path (simulates restart).
//  6. Build a new HTTP server backed by the reloaded store.
//  7. GET the resource group and vnet; both must return 200 OK.
func TestPersist_StateSurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	persistPath := filepath.Join(dir, "state.json")

	// --- Phase 1: create resources, then shut down ---

	fs1, err := store.NewFileStore(persistPath)
	if err != nil {
		t.Fatalf("NewFileStore (phase 1): %v", err)
	}

	srv1 := buildServerWithStore(t, fs1)
	base1 := srv1.URL

	rgURL := "/subscriptions/sub1/resourcegroups/rg1" + apiVersionQ
	vnetURL := "/subscriptions/sub1/resourcegroups/rg1/providers/microsoft.network/virtualnetworks/vnet1" + apiVersionQ

	resp := doJSON(t, http.MethodPut, base1+rgURL, `{"location":"uksouth"}`)
	mustStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = doJSON(t, http.MethodPut, base1+vnetURL,
		`{"location":"uksouth","properties":{"addressSpace":{"addressPrefixes":["10.0.0.0/16"]}}}`)
	mustStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	// Verify the file was written to disk.
	if _, err := os.Stat(persistPath); err != nil {
		t.Fatalf("persist file not created: %v", err)
	}

	srv1.Close()

	// --- Phase 2: reload from disk, verify resources survived ---

	fs2, err := store.NewFileStore(persistPath)
	if err != nil {
		t.Fatalf("NewFileStore (phase 2): %v", err)
	}

	srv2 := buildServerWithStore(t, fs2)
	base2 := srv2.URL

	resp = doJSON(t, http.MethodGet, base2+rgURL, "")
	mustStatus(t, resp, http.StatusOK)
	body := decode(t, resp)
	if body["location"] != "uksouth" {
		t.Errorf("reloaded RG location = %v, want uksouth", body["location"])
	}

	resp = doJSON(t, http.MethodGet, base2+vnetURL, "")
	mustStatus(t, resp, http.StatusOK)
	body = decode(t, resp)
	if body["location"] != "uksouth" {
		t.Errorf("reloaded VNet location = %v, want uksouth", body["location"])
	}
}

// TestPersist_DeleteSurvivesRestart verifies that a delete operation is
// persisted: after deleting a resource and restarting, the resource must
// still be gone.
func TestPersist_DeleteSurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	persistPath := filepath.Join(dir, "state.json")

	// --- Phase 1: create, delete, shut down ---

	fs1, err := store.NewFileStore(persistPath)
	if err != nil {
		t.Fatalf("NewFileStore (phase 1): %v", err)
	}

	srv1 := buildServerWithStore(t, fs1)
	base1 := srv1.URL

	rgURL := "/subscriptions/sub1/resourcegroups/rg1" + apiVersionQ

	resp := doJSON(t, http.MethodPut, base1+rgURL, `{"location":"uksouth"}`)
	mustStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = doJSON(t, http.MethodDelete, base1+rgURL, "")
	mustStatus(t, resp, http.StatusAccepted)
	resp.Body.Close()

	srv1.Close()

	// --- Phase 2: reload, confirm resource is still gone ---

	fs2, err := store.NewFileStore(persistPath)
	if err != nil {
		t.Fatalf("NewFileStore (phase 2): %v", err)
	}

	srv2 := buildServerWithStore(t, fs2)
	base2 := srv2.URL

	resp = doJSON(t, http.MethodGet, base2+rgURL, "")
	mustStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
}

// TestPersist_ResetClearsPersistedState verifies that calling Reset on the
// store clears both in-memory and on-disk state.
func TestPersist_ResetClearsPersistedState(t *testing.T) {
	dir := t.TempDir()
	persistPath := filepath.Join(dir, "state.json")

	fs1, err := store.NewFileStore(persistPath)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}

	srv1 := buildServerWithStore(t, fs1)
	base1 := srv1.URL

	rgURL := "/subscriptions/sub1/resourcegroups/rg1" + apiVersionQ

	resp := doJSON(t, http.MethodPut, base1+rgURL, `{"location":"uksouth"}`)
	mustStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	fs1.Reset()
	srv1.Close()

	// Reload: resource must not exist.
	fs2, err := store.NewFileStore(persistPath)
	if err != nil {
		t.Fatalf("NewFileStore (reload): %v", err)
	}

	srv2 := buildServerWithStore(t, fs2)
	base2 := srv2.URL

	resp = doJSON(t, http.MethodGet, base2+rgURL, "")
	mustStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
}

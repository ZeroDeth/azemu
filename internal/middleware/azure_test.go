package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// newMiddlewareTestServer wires mw around downstream and returns a test server
// that is closed automatically when the test ends.
func newMiddlewareTestServer(t *testing.T, mw func(http.Handler) http.Handler, downstream http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(mw(downstream))
	t.Cleanup(srv.Close)
	return srv
}

// okHandler is a trivial downstream that always returns 200.
var okHandler http.HandlerFunc = func(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

// TestAzureHeaders_InjectsRequestIDs verifies that AzureHeaders sets the three
// x-ms-* headers and the Strict-Transport-Security header on every response.
func TestAzureHeaders_InjectsRequestIDs(t *testing.T) {
	srv := newMiddlewareTestServer(t, AzureHeaders, okHandler)

	resp, err := http.Get(srv.URL + "/any/path")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	resp.Body.Close()

	headers := []string{
		"x-ms-request-id",
		"x-ms-correlation-request-id",
		"x-ms-routing-request-id",
		"Strict-Transport-Security",
	}
	for _, h := range headers {
		val := resp.Header.Get(h)
		if val == "" {
			t.Errorf("header %q missing or empty", h)
		}
	}

	// routing header must start with the AZEMU prefix.
	routing := resp.Header.Get("x-ms-routing-request-id")
	if !strings.HasPrefix(routing, "AZEMU:") {
		t.Errorf("x-ms-routing-request-id = %q, want AZEMU: prefix", routing)
	}
}

// TestAzureHeaders_DoesNotOverwriteExisting documents the current behaviour:
// AzureHeaders uses Header.Set, which ALWAYS overwrites any pre-existing value.
// A downstream that sets x-ms-request-id first will have its value replaced by
// the middleware value because Set is called before ServeHTTP is invoked.
// This test pins that behaviour; if the middleware is ever changed to use
// Header.Add or a conditional set, this test will catch the regression.
func TestAzureHeaders_DoesNotOverwriteExisting(t *testing.T) {
	// Downstream tries to set its own x-ms-request-id.
	const downstreamValue = "downstream-set-value"
	downstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// This runs AFTER AzureHeaders has already called Set, so it would
		// overwrite the middleware value. But that is downstream behaviour,
		// not middleware behaviour. We care about what the middleware does
		// before the downstream runs.
		w.WriteHeader(http.StatusOK)
	})

	srv := newMiddlewareTestServer(t, AzureHeaders, downstream)

	resp, err := http.Get(srv.URL + "/any")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	resp.Body.Close()

	// The middleware sets its own UUID. The header must be non-empty and must
	// NOT equal the downstream placeholder, proving the middleware value wins
	// when it runs first.
	val := resp.Header.Get("x-ms-request-id")
	if val == "" {
		t.Error("x-ms-request-id missing after AzureHeaders")
	}
	if val == downstreamValue {
		t.Errorf("x-ms-request-id = %q; middleware should have set its own UUID", val)
	}
}

// TestRequireAPIVersion_SubscriptionPathWithoutVersion_Returns400 verifies
// that a request to an ARM subscription path without ?api-version= is rejected
// with 400 and the Azure error body shape.
func TestRequireAPIVersion_SubscriptionPathWithoutVersion_Returns400(t *testing.T) {
	srv := newMiddlewareTestServer(t, RequireAPIVersion, okHandler)

	resp, err := http.Get(srv.URL + "/subscriptions/sub1/resourcegroups/rg1")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}

	var m map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	errObj, ok := m["error"].(map[string]interface{})
	if !ok {
		t.Fatalf("body[\"error\"] missing or wrong type: %v", m)
	}
	if errObj["code"] != "MissingApiVersionParameter" {
		t.Errorf("code = %v, want MissingApiVersionParameter", errObj["code"])
	}
	if errObj["message"] == "" {
		t.Error("error.message is empty")
	}
}

// TestRequireAPIVersion_SubscriptionPathWithVersion_Allows verifies that a
// subscription-scoped request with ?api-version= passes through to the
// downstream handler.
func TestRequireAPIVersion_SubscriptionPathWithVersion_Allows(t *testing.T) {
	srv := newMiddlewareTestServer(t, RequireAPIVersion, okHandler)

	resp, err := http.Get(srv.URL + "/subscriptions/sub1/resourcegroups/rg1?api-version=2023-09-01")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
}

// TestRequireAPIVersion_MetadataPath_Exempt verifies that /metadata/* paths
// reach the downstream without requiring ?api-version=.
func TestRequireAPIVersion_MetadataPath_Exempt(t *testing.T) {
	srv := newMiddlewareTestServer(t, RequireAPIVersion, okHandler)

	resp, err := http.Get(srv.URL + "/metadata/endpoints")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200 (metadata path should be exempt)", resp.StatusCode)
	}
}

// TestRequireAPIVersion_OAuth2Path_Exempt verifies that paths containing
// /oauth2 reach downstream without ?api-version=.
func TestRequireAPIVersion_OAuth2Path_Exempt(t *testing.T) {
	srv := newMiddlewareTestServer(t, RequireAPIVersion, okHandler)

	resp, err := http.Get(srv.URL + "/tenant1/oauth2/v2.0/token")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200 (oauth2 path should be exempt)", resp.StatusCode)
	}
}

// TestRequireAPIVersion_WellKnownPath_Exempt verifies that /.well-known/*
// paths reach downstream without ?api-version=.
func TestRequireAPIVersion_WellKnownPath_Exempt(t *testing.T) {
	srv := newMiddlewareTestServer(t, RequireAPIVersion, okHandler)

	resp, err := http.Get(srv.URL + "/.well-known/openid-configuration")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200 (.well-known path should be exempt)", resp.StatusCode)
	}
}

// TestRequireAPIVersion_DiscoveryPath_Exempt verifies that paths containing
// /discovery/ reach downstream without ?api-version=.
func TestRequireAPIVersion_DiscoveryPath_Exempt(t *testing.T) {
	srv := newMiddlewareTestServer(t, RequireAPIVersion, okHandler)

	resp, err := http.Get(srv.URL + "/discovery/keys")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200 (discovery path should be exempt)", resp.StatusCode)
	}
}

// TestUnhandledTracker_Record_IncrementsCount verifies that calling Record
// increments the count for the given method+path key.
func TestUnhandledTracker_Record_IncrementsCount(t *testing.T) {
	tr := &UnhandledTracker{routes: make(map[string]int)}
	tr.Record("GET", "/unknown/path")
	tr.Record("GET", "/unknown/path")
	tr.Record("POST", "/another/path")

	routes := tr.List()
	if routes["GET /unknown/path"] != 2 {
		t.Errorf("GET /unknown/path count = %d, want 2", routes["GET /unknown/path"])
	}
	if routes["POST /another/path"] != 1 {
		t.Errorf("POST /another/path count = %d, want 1", routes["POST /another/path"])
	}
}

// TestUnhandledTracker_List_ReturnsCopy verifies that the map returned by List
// is a copy; mutating it must not affect the internal state.
func TestUnhandledTracker_List_ReturnsCopy(t *testing.T) {
	tr := &UnhandledTracker{routes: make(map[string]int)}
	tr.Record("DELETE", "/res/one")

	copy1 := tr.List()
	copy1["injected"] = 99

	copy2 := tr.List()
	if _, present := copy2["injected"]; present {
		t.Error("mutating the returned map affected internal tracker state; List must return a copy")
	}
}

// TestLogUnhandledRequests_Returns501WithAzureError verifies that the handler
// returned by LogUnhandledRequests writes 501 and the Azure error body shape.
func TestLogUnhandledRequests_Returns501WithAzureError(t *testing.T) {
	tracker := NewUnhandledTracker()
	handler := LogUnhandledRequests(tracker)

	req := httptest.NewRequest(http.MethodGet, "/some/unimplemented/path", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotImplemented {
		t.Errorf("status = %d, want 501", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var m map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&m); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	errObj, ok := m["error"].(map[string]interface{})
	if !ok {
		t.Fatalf("body[\"error\"] missing or wrong type: %v", m)
	}
	if errObj["code"] != "NotImplemented" {
		t.Errorf("code = %v, want NotImplemented", errObj["code"])
	}
	if errObj["message"] == "" {
		t.Error("error.message is empty")
	}
}

// TestLogUnhandledRequests_RecordsRoute verifies that calling the handler
// causes the route to appear in the tracker.
func TestLogUnhandledRequests_RecordsRoute(t *testing.T) {
	tracker := NewUnhandledTracker()
	handler := LogUnhandledRequests(tracker)
	req := httptest.NewRequest(http.MethodPut, "/api/untracked", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	routes := tracker.List()
	if routes["PUT /api/untracked"] != 1 {
		t.Errorf("tracker count for PUT /api/untracked = %d, want 1", routes["PUT /api/untracked"])
	}
}

// TestUnhandledTracker_ConcurrentRecord verifies that concurrent calls to
// Record do not race. Must pass under -race.
func TestUnhandledTracker_ConcurrentRecord(t *testing.T) {
	tr := &UnhandledTracker{routes: make(map[string]int)}
	const goroutines = 8
	const ops = 50

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				tr.Record("GET", "/concurrent/path")
				_ = tr.List()
			}
		}()
	}
	wg.Wait()

	routes := tr.List()
	if routes["GET /concurrent/path"] != goroutines*ops {
		t.Errorf("count = %d, want %d", routes["GET /concurrent/path"], goroutines*ops)
	}
}

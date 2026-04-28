package arm

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	mw "github.com/zerodeth/azemu/internal/middleware"
	"github.com/zerodeth/azemu/internal/store"
)

// defaultAPIVersion is injected by the http* helpers when the caller does not
// supply an api-version query parameter. Using a single constant keeps tests
// stable when the provider bumps its pinned ARM API version.
const defaultAPIVersion = "2023-09-01"

// newTestServer assembles a chi router that mirrors the production middleware
// stack in cmd/azemu/main.go (minus TLS/OAuth, which are orthogonal to ARM
// routing) and mounts the ARM routes at /subscriptions. Each test gets its
// own MemoryStore so tests are fully isolated.
//
// The returned server is registered for cleanup via t.Cleanup, so tests do
// not need a defer srv.Close().
func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	s := store.NewMemoryStore()
	ar := NewRouter(s, "http://azurite-test:10000", "https://kv-test", "redis://redis-test:6379")
	r := chi.NewRouter()
	// Mirror the production middleware order from cmd/azemu/main.go so
	// tests exercise the same path-normalization, header stamping, and
	// api-version enforcement as real traffic.
	r.Use(mw.NormalizePath)
	r.Use(mw.AzureHeaders)
	r.Use(mw.RequireAPIVersion)
	r.Route("/subscriptions", ar.Routes)
	r.Route("/keyvault", ar.KeyVaultDataPlaneRoutes)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv
}

// withAPIVersion appends ?api-version=<default> to a URL unless the caller
// has already specified one. Tests that want to exercise the middleware
// rejection path should use httpRaw* helpers instead.
func withAPIVersion(url string) string {
	if strings.Contains(url, "api-version=") {
		return url
	}
	sep := "?"
	if strings.Contains(url, "?") {
		sep = "&"
	}
	return url + sep + "api-version=" + defaultAPIVersion
}

func httpPut(t *testing.T, url, body string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPut, withAPIVersion(url), bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do PUT %s: %v", url, err)
	}
	return resp
}

func httpGet(t *testing.T, url string) *http.Response {
	t.Helper()
	resp, err := http.Get(withAPIVersion(url))
	if err != nil {
		t.Fatalf("do GET %s: %v", url, err)
	}
	return resp
}

func httpGetRaw(t *testing.T, url string) *http.Response {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("do raw GET %s: %v", url, err)
	}
	return resp
}

func httpHead(t *testing.T, url string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodHead, withAPIVersion(url), nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do HEAD %s: %v", url, err)
	}
	return resp
}

func httpPost(t *testing.T, url, body string) *http.Response {
	t.Helper()
	var bodyReader *bytes.Buffer
	if body != "" {
		bodyReader = bytes.NewBufferString(body)
	} else {
		bodyReader = bytes.NewBufferString("{}")
	}
	req, err := http.NewRequest(http.MethodPost, withAPIVersion(url), bodyReader)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do POST %s: %v", url, err)
	}
	return resp
}

func httpDelete(t *testing.T, url string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodDelete, withAPIVersion(url), nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do DELETE %s: %v", url, err)
	}
	return resp
}

func assertStatus(t *testing.T, resp *http.Response, want int) {
	t.Helper()
	if resp.StatusCode != want {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("status = %d, want %d. body=%s", resp.StatusCode, want, string(body))
	}
}

// decodeJSON reads the body and decodes it into a generic map, closing the
// body in the process.
func decodeJSON(t *testing.T, resp *http.Response) map[string]interface{} {
	t.Helper()
	defer resp.Body.Close()
	var m map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		t.Fatalf("decode json: %v", err)
	}
	return m
}

// readBody drains the response body and returns it as a string. Useful for
// asserting that HEAD responses are empty.
func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return string(b)
}

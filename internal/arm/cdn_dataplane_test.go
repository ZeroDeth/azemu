package arm

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zerodeth/azemu/internal/store"
)

func TestCDNEndpointNameFromHost(t *testing.T) {
	cases := []struct {
		host     string
		wantName string
		wantOK   bool
	}{
		{"otacdn.azureedge.net", "otacdn", true},
		{"otacdn.azureedge.net:4566", "otacdn", true},
		{"my-endpoint-1.azureedge.net", "my-endpoint-1", true},
		{"localhost", "", false},
		{"localhost:4566", "", false},
		{"vault1.vault.localhost", "", false},
		{"azureedge.net", "", false},
		{".azureedge.net", "", false},
		{"a.b.azureedge.net", "", false}, // multi-label endpoint name is not valid
	}
	for _, c := range cases {
		gotName, gotOK := cdnEndpointNameFromHost(c.host)
		if gotName != c.wantName || gotOK != c.wantOK {
			t.Errorf("cdnEndpointNameFromHost(%q) = (%q, %v), want (%q, %v)",
				c.host, gotName, gotOK, c.wantName, c.wantOK)
		}
	}
}

// seedCDNEndpoint stores a CDN endpoint whose origin points at the given
// storage account, returning a Router wired to the given Azurite origin base.
func seedCDNEndpoint(t *testing.T, originBase, endpointName, account string) *Router {
	t.Helper()
	s := store.NewMemoryStore()
	a := NewRouter(s, originBase, "https://kv-test", "redis://redis-test:6379")
	id := cdnEndpointID("sub1", "rg1", "prof1", endpointName)
	if err := s.Put(id, &store.Resource{
		ID:       id,
		Name:     endpointName,
		Type:     cdnEndpointTypeString,
		Location: "uksouth",
		Properties: map[string]interface{}{
			"hostName": endpointName + ".azureedge.net",
			"origins": []interface{}{
				map[string]interface{}{
					"name": "blob-origin",
					"properties": map[string]interface{}{
						"hostName": account + ".blob.core.windows.net",
					},
				},
			},
		},
	}); err != nil {
		t.Fatalf("seed endpoint: %v", err)
	}
	return a
}

func TestServeCDNContent_passthrough(t *testing.T) {
	// Origin stands in for Azurite: it serves a blob with the cache + content
	// headers the publish step would set, under the path-style /{account}/...
	var gotPath string
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "multipart/mixed; boundary=abc")
		w.Header().Set("Cache-Control", "max-age=30")
		w.Header().Set("ETag", `"v1"`)
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "manifest-bytes")
	}))
	defer origin.Close()

	a := seedCDNEndpoint(t, origin.URL, "otacdn", "otasa")

	req := httptest.NewRequest(http.MethodGet, "http://otacdn.azureedge.net/ota/1.0.0/manifest.json", nil)
	rec := httptest.NewRecorder()
	a.ServeCDNContent(rec, req)

	resp := rec.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if want := "/otasa/ota/1.0.0/manifest.json"; gotPath != want {
		t.Errorf("origin path = %q, want %q", gotPath, want)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "multipart/mixed; boundary=abc" {
		t.Errorf("Content-Type = %q, want multipart/mixed passthrough", ct)
	}
	if cc := resp.Header.Get("Cache-Control"); cc != "max-age=30" {
		t.Errorf("Cache-Control = %q, want max-age=30 passthrough", cc)
	}
	if xc := resp.Header.Get("X-Cache"); xc == "" {
		t.Error("X-Cache header not set; expected an edge marker")
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "manifest-bytes" {
		t.Errorf("body = %q, want origin bytes", string(body))
	}
}

func TestServeCDNContent_head_noBody(t *testing.T) {
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "should-not-be-forwarded-on-head")
	}))
	defer origin.Close()

	a := seedCDNEndpoint(t, origin.URL, "otacdn", "otasa")

	req := httptest.NewRequest(http.MethodHead, "http://otacdn.azureedge.net/ota/asset-deadbeef.png", nil)
	rec := httptest.NewRecorder()
	a.ServeCDNContent(rec, req)

	resp := rec.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if cc := resp.Header.Get("Cache-Control"); cc != "public, max-age=31536000, immutable" {
		t.Errorf("Cache-Control = %q, want immutable passthrough", cc)
	}
	body, _ := io.ReadAll(resp.Body)
	if len(body) != 0 {
		t.Errorf("HEAD returned a body of %d bytes, want empty", len(body))
	}
}

func TestServeCDNContent_originMiss_404(t *testing.T) {
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer origin.Close()

	a := seedCDNEndpoint(t, origin.URL, "otacdn", "otasa")

	req := httptest.NewRequest(http.MethodGet, "http://otacdn.azureedge.net/ota/missing.json", nil)
	rec := httptest.NewRecorder()
	a.ServeCDNContent(rec, req)

	if rec.Result().StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404 passthrough from origin", rec.Result().StatusCode)
	}
}

func TestServeCDNContent_unknownEndpoint_404(t *testing.T) {
	s := store.NewMemoryStore()
	a := NewRouter(s, "http://azurite-test:10000", "https://kv-test", "redis://redis-test:6379")

	req := httptest.NewRequest(http.MethodGet, "http://ghost.azureedge.net/x", nil)
	rec := httptest.NewRecorder()
	a.ServeCDNContent(rec, req)

	if rec.Result().StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404 for unknown endpoint", rec.Result().StatusCode)
	}
}

func TestServeCDNContent_methodNotAllowed(t *testing.T) {
	a := seedCDNEndpoint(t, "http://azurite-test:10000", "otacdn", "otasa")

	req := httptest.NewRequest(http.MethodPost, "http://otacdn.azureedge.net/ota/x", nil)
	rec := httptest.NewRecorder()
	a.ServeCDNContent(rec, req)

	if rec.Result().StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rec.Result().StatusCode)
	}
}

func TestBlobServiceBase(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"http://azurite:10000", "http://azurite:10000"},
		{"http://azurite", "http://azurite:10000"},
		{"https://127.0.0.1:54321", "https://127.0.0.1:54321"},
		{"http://azurite:10000/", "http://azurite:10000"},
	}
	for _, c := range cases {
		if got := blobServiceBase(c.in); got != c.want {
			t.Errorf("blobServiceBase(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

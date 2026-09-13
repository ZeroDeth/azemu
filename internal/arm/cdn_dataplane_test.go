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

// TestSafeBlobPath covers the traversal cases the data-plane mux exposes. The
// mux keys off the client-controlled Host header and runs before auth, so this
// path is entirely attacker-chosen.
func TestSafeBlobPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
		ok   bool
	}{
		{"plain blob", "/container/blob.txt", "/container/blob.txt", true},
		{"nested", "/c/a/b/c.bin", "/c/a/b/c.bin", true},
		{"root", "/", "/", true},
		{"empty", "", "/", true},
		{"no leading slash", "c/b.txt", "/c/b.txt", true},

		// Encoded characters in blob keys must survive untouched.
		{"encoded space", "/c/my%20blob.txt", "/c/my%20blob.txt", true},
		{"encoded hash", "/c/a%23b.txt", "/c/a%23b.txt", true},
		{"encoded slash in key", "/c/a%2Fb.txt", "/c/a%2Fb.txt", true},
		{"dots inside a name", "/c/..hidden.txt", "/c/..hidden.txt", true},
		{"name ending in dots", "/c/report..txt", "/c/report..txt", true},

		// Traversal, literal and encoded.
		{"literal dotdot", "/../other/secret.txt", "", false},
		{"dotdot mid path", "/c/../../other/secret", "", false},
		{"encoded dotdot lower", "/%2e%2e/other/x", "", false},
		{"encoded dotdot upper", "/%2E%2E/other/x", "", false},
		{"single dot", "/./c/b.txt", "", false},
		{"trailing dotdot", "/c/..", "", false},
		{"bad escape", "/c/%zz", "", false},

		// A traversal can hide inside a single segment: %2e%2e%2f decodes to
		// "../", so comparing escaped segments against ".." misses it. These
		// all bypassed an earlier version of this function.
		{"encoded separator", "/c/%2e%2e%2f/x", "", false},
		{"encoded separator twice", "/c/%2e%2e%2f%2e%2e%2f/x", "", false},
		{"encoded separator at root", "/%2e%2e%2fotheracct/secret.txt", "", false},
		{"mixed case encoded separator", "/c/%2e%2e%2F%2e%2e%2Fother/x", "", false},
		{"literal dots, encoded slash", "/c/..%2f..%2fotheracct/x", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := safeBlobPath(tt.in)
			if ok != tt.ok {
				t.Fatalf("safeBlobPath(%q) ok = %v, want %v", tt.in, ok, tt.ok)
			}
			if ok && got != tt.want {
				t.Errorf("safeBlobPath(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestServeCDNContent_traversalRejected proves the account prefix actually
// holds end to end. The origin records every path it is asked for, so the test
// fails if a traversal reaches it even when the response looks like a 404.
func TestServeCDNContent_traversalRejected(t *testing.T) {
	var reached []string
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = append(reached, r.URL.Path)
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "should-not-be-served")
	}))
	defer origin.Close()

	a := seedCDNEndpoint(t, origin.URL, "otacdn", "otasa")

	for _, target := range []string{
		"http://otacdn.azureedge.net/../otheracct/private/secret.txt",
		"http://otacdn.azureedge.net/%2e%2e/otheracct/private/secret.txt",
		"http://otacdn.azureedge.net/%2E%2E/otheracct/x",
		"http://otacdn.azureedge.net/ota/../../otheracct/x",
	} {
		rec := httptest.NewRecorder()
		a.ServeCDNContent(rec, httptest.NewRequest(http.MethodGet, target, nil))

		if got := rec.Result().StatusCode; got != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", target, got)
		}
	}

	if len(reached) != 0 {
		t.Errorf("origin was reached for %v; the account prefix was escaped", reached)
	}
}

// TestServeCDNContent_encodedKeyStillWorks guards the traversal fix against
// over-reach: a blob key may legitimately contain encoded characters, and %2F
// means a slash inside the key rather than a path separator.
func TestServeCDNContent_encodedKeyStillWorks(t *testing.T) {
	var gotPath string
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "ok")
	}))
	defer origin.Close()

	a := seedCDNEndpoint(t, origin.URL, "otacdn", "otasa")

	rec := httptest.NewRecorder()
	a.ServeCDNContent(rec, httptest.NewRequest(http.MethodGet,
		"http://otacdn.azureedge.net/ota/my%20blob..name%2Fpart.json", nil))

	if got := rec.Result().StatusCode; got != http.StatusOK {
		t.Fatalf("status = %d, want 200", got)
	}
	if want := "/otasa/ota/my%20blob..name%2Fpart.json"; gotPath != want {
		t.Errorf("origin path = %q, want %q", gotPath, want)
	}
}

package arm

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zerodeth/azemu/internal/store"
)

func TestAFDEndpointNameFromHost(t *testing.T) {
	cases := []struct {
		host     string
		wantName string
		wantOK   bool
	}{
		{"fdedge.azurefd.net", "fdedge", true},
		{"fdedge.azurefd.net:4566", "fdedge", true},
		{"my-endpoint-1.azurefd.net", "my-endpoint-1", true},
		{"localhost", "", false},
		{"vault1.vault.localhost", "", false},
		{"otacdn.azureedge.net", "", false}, // classic CDN host is not a Front Door host
		{"azurefd.net", "", false},
		{".azurefd.net", "", false},
		{"a.b.azurefd.net", "", false}, // multi-label endpoint name is not valid
	}
	for _, c := range cases {
		gotName, gotOK := afdEndpointNameFromHost(c.host)
		if gotName != c.wantName || gotOK != c.wantOK {
			t.Errorf("afdEndpointNameFromHost(%q) = (%q, %v), want (%q, %v)",
				c.host, gotName, gotOK, c.wantName, c.wantOK)
		}
	}
}

func TestBlobAccountFromHost(t *testing.T) {
	cases := []struct {
		host     string
		wantAcct string
		wantOK   bool
	}{
		{"otasa.blob.core.windows.net", "otasa", true},
		{"otasa.dfs.core.windows.net", "", false},
		{"", "", false},
		{"notblob", "", false},
	}
	for _, c := range cases {
		got, ok := blobAccountFromHost(c.host)
		if got != c.wantAcct || ok != c.wantOK {
			t.Errorf("blobAccountFromHost(%q) = (%q, %v), want (%q, %v)",
				c.host, got, ok, c.wantAcct, c.wantOK)
		}
	}
}

// seedAFDGraph stores a full Front Door resource graph (endpoint -> route ->
// originGroup -> origin) whose origin points at the given storage account, and
// returns a Router wired to the given Azurite origin base. The IDs use the same
// builders the handlers use so the data-plane resolution (prefix + originGroup
// reference) matches exactly.
func seedAFDGraph(t *testing.T, originBase, endpointName, account string) *Router {
	t.Helper()
	s := store.NewMemoryStore()
	a := NewRouter(s, originBase, "https://kv-test", "redis://redis-test:6379")

	const sub, rg, profile, group, origin, route = "sub1", "rg1", "fd1", "og1", "o1", "r1"
	ogID := afdOriginGroupID(sub, rg, profile, group)

	put := func(id, name, typ string, props map[string]interface{}) {
		if err := s.Put(id, &store.Resource{ID: id, Name: name, Type: typ, Properties: props}); err != nil {
			t.Fatalf("seed %s: %v", id, err)
		}
	}
	put(afdEndpointID(sub, rg, profile, endpointName), endpointName, afdEndpointTypeString,
		map[string]interface{}{"hostName": endpointName + ".azurefd.net"})
	put(ogID, group, afdOriginGroupTypeString, map[string]interface{}{})
	put(afdOriginID(sub, rg, profile, group, origin), origin, afdOriginTypeString,
		map[string]interface{}{
			"hostName": account + ".blob.core.windows.net",
			"priority": float64(1),
			"weight":   float64(1000),
		})
	put(afdRouteID(sub, rg, profile, endpointName, route), route, afdRouteTypeString,
		map[string]interface{}{"originGroup": map[string]interface{}{"id": ogID}})
	return a
}

func TestServeAFDContent_passthrough(t *testing.T) {
	var gotPath string
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "multipart/mixed; boundary=abc")
		w.Header().Set("Cache-Control", "max-age=30")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "manifest-bytes")
	}))
	defer origin.Close()

	a := seedAFDGraph(t, origin.URL, "fdedge", "otasa")

	req := httptest.NewRequest(http.MethodGet, "http://fdedge.azurefd.net/ota/1.0.0/manifest.json", nil)
	rec := httptest.NewRecorder()
	a.ServeAFDContent(rec, req)

	resp := rec.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if want := "/otasa/ota/1.0.0/manifest.json"; gotPath != want {
		t.Errorf("origin path = %q, want %q", gotPath, want)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "multipart/mixed; boundary=abc" {
		t.Errorf("Content-Type = %q, want multipart passthrough", ct)
	}
	if cc := resp.Header.Get("Cache-Control"); cc != "max-age=30" {
		t.Errorf("Cache-Control = %q, want max-age=30 passthrough", cc)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "manifest-bytes" {
		t.Errorf("body = %q, want origin bytes", string(body))
	}
}

func TestServeAFDContent_head_noBody(t *testing.T) {
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "should-not-be-forwarded-on-head")
	}))
	defer origin.Close()

	a := seedAFDGraph(t, origin.URL, "fdedge", "otasa")

	req := httptest.NewRequest(http.MethodHead, "http://fdedge.azurefd.net/ota/asset.png", nil)
	rec := httptest.NewRecorder()
	a.ServeAFDContent(rec, req)

	resp := rec.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if cc := resp.Header.Get("Cache-Control"); cc != "public, max-age=31536000, immutable" {
		t.Errorf("Cache-Control = %q, want immutable passthrough", cc)
	}
	body, _ := io.ReadAll(resp.Body)
	if len(body) != 0 {
		t.Errorf("HEAD returned %d body bytes, want empty", len(body))
	}
}

func TestServeAFDContent_unknownEndpoint_404(t *testing.T) {
	s := store.NewMemoryStore()
	a := NewRouter(s, "http://azurite-test:10000", "https://kv-test", "redis://redis-test:6379")

	req := httptest.NewRequest(http.MethodGet, "http://ghost.azurefd.net/x", nil)
	rec := httptest.NewRecorder()
	a.ServeAFDContent(rec, req)

	if rec.Result().StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404 for unknown endpoint", rec.Result().StatusCode)
	}
}

func TestServeAFDContent_unresolvedGraph_502(t *testing.T) {
	// Endpoint exists but has no route/origin graph, so the origin cannot be
	// resolved: the proxy reports a bad gateway rather than a 404.
	s := store.NewMemoryStore()
	a := NewRouter(s, "http://azurite-test:10000", "https://kv-test", "redis://redis-test:6379")
	id := afdEndpointID("sub1", "rg1", "fd1", "fdedge")
	if err := s.Put(id, &store.Resource{
		ID: id, Name: "fdedge", Type: afdEndpointTypeString,
		Properties: map[string]interface{}{"hostName": "fdedge.azurefd.net"},
	}); err != nil {
		t.Fatalf("seed endpoint: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "http://fdedge.azurefd.net/x", nil)
	rec := httptest.NewRecorder()
	a.ServeAFDContent(rec, req)

	if rec.Result().StatusCode != http.StatusBadGateway {
		t.Errorf("status = %d, want 502 for unresolved origin graph", rec.Result().StatusCode)
	}
}

func TestServeAFDContent_methodNotAllowed(t *testing.T) {
	a := seedAFDGraph(t, "http://azurite-test:10000", "fdedge", "otasa")

	req := httptest.NewRequest(http.MethodPost, "http://fdedge.azurefd.net/x", nil)
	rec := httptest.NewRecorder()
	a.ServeAFDContent(rec, req)

	if rec.Result().StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rec.Result().StatusCode)
	}
}

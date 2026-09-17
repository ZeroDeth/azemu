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

// TestServeAFDContent_upperCaseEndpointName_resolves guards the case-mismatch
// regression: an endpoint whose name contains uppercase letters (permitted by
// azurerm's naming validation) must still resolve on the data plane, because
// afdGeneratedHostName lowercases the host it advertises and the incoming
// request Host header is also lowercased before endpoint lookup.
func TestServeAFDContent_upperCaseEndpointName_resolves(t *testing.T) {
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "ok")
	}))
	defer origin.Close()

	a := seedAFDGraph(t, origin.URL, "MyEdge", "otasa")

	req := httptest.NewRequest(http.MethodGet, "http://myedge.azurefd.net/x", nil)
	rec := httptest.NewRecorder()
	a.ServeAFDContent(rec, req)

	if resp := rec.Result(); resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200 (uppercase endpoint name should still resolve)", resp.StatusCode)
	}
}

func TestIsAFDContentHost(t *testing.T) {
	cases := []struct {
		host string
		want bool
	}{
		{"fdedge.azurefd.net", true},
		{"fdedge.azurefd.net:4566", true},
		{"otacdn.azureedge.net", false},
		{"localhost", false},
	}
	for _, c := range cases {
		if got := IsAFDContentHost(c.host); got != c.want {
			t.Errorf("IsAFDContentHost(%q) = %v, want %v", c.host, got, c.want)
		}
	}
}

// TestServeAFDContent_traversalRejected proves the Front Door proxy inherits
// the account-prefix check rather than repeating the classic CDN proxy's old
// bug. Both call proxyBlobObject, so the check lives in blobOriginURL and
// neither call site can forget it.
//
// The encoded variants matter most: a traversal hides inside one segment,
// since %2e%2e%2f decodes to "../". curl collapses the literal form
// client-side, so only a test at this level actually exercises them.
func TestServeAFDContent_traversalRejected(t *testing.T) {
	var reached []string
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = append(reached, r.URL.Path)
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "should-not-be-served")
	}))
	defer origin.Close()

	a := seedAFDGraph(t, origin.URL, "fdedge", "otasa")

	for _, target := range []string{
		"http://fdedge.azurefd.net/../otheracct/private/secret.txt",
		"http://fdedge.azurefd.net/%2e%2e/otheracct/private/secret.txt",
		"http://fdedge.azurefd.net/ota/%2e%2e%2f%2e%2e%2fotheracct/x",
		"http://fdedge.azurefd.net/ota/..%2f..%2fotheracct/x",
	} {
		rec := httptest.NewRecorder()
		a.ServeAFDContent(rec, httptest.NewRequest(http.MethodGet, target, nil))

		if got := rec.Result().StatusCode; got != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", target, got)
		}
	}

	if len(reached) != 0 {
		t.Errorf("origin was reached for %v; the account prefix was escaped", reached)
	}
}

// TestPreferredOrigin_ordering exercises the comparator with more than one
// origin. With a single origin, as every other test here uses, the comparator
// could be inverted and nothing would notice.
func TestPreferredOrigin_ordering(t *testing.T) {
	const sub, rg, profile, group = "sub1", "rg1", "fd1", "og1"
	ogID := afdOriginGroupID(sub, rg, profile, group)

	newRouter := func(t *testing.T, origins map[string][2]float64) *Router {
		t.Helper()
		s := store.NewMemoryStore()
		a := NewRouter(s, "http://origin", "https://kv", "redis://r:6379")
		if err := s.Put(ogID, &store.Resource{
			ID: ogID, Name: group, Type: afdOriginGroupTypeString,
			Properties: map[string]interface{}{},
		}); err != nil {
			t.Fatalf("seed group: %v", err)
		}
		for name, pw := range origins {
			id := afdOriginID(sub, rg, profile, group, name)
			if err := s.Put(id, &store.Resource{
				ID: id, Name: name, Type: afdOriginTypeString,
				Properties: map[string]interface{}{
					"hostName": name + ".blob.core.windows.net",
					"priority": pw[0],
					"weight":   pw[1],
				},
			}); err != nil {
				t.Fatalf("seed origin %s: %v", name, err)
			}
		}
		return a
	}

	tests := []struct {
		name    string
		origins map[string][2]float64 // name -> {priority, weight}
		want    string
	}{
		{
			name:    "lower priority wins regardless of weight",
			origins: map[string][2]float64{"low": {1, 10}, "high": {5, 9000}},
			want:    "low",
		},
		{
			name:    "equal priority falls back to higher weight",
			origins: map[string][2]float64{"light": {1, 100}, "heavy": {1, 900}},
			want:    "heavy",
		},
		{
			name:    "priority beats weight even when weight is far larger",
			origins: map[string][2]float64{"p1": {1, 1}, "p2": {2, 100000}},
			want:    "p1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := newRouter(t, tt.origins)
			// Run repeatedly: the store iterates a map, so a comparator that
			// only happens to work would be caught by a differing pick.
			for i := 0; i < 20; i++ {
				got, ok := a.preferredOrigin(ogID)
				if !ok {
					t.Fatal("preferredOrigin found nothing")
				}
				if got.Name != tt.want {
					t.Fatalf("preferredOrigin = %q, want %q", got.Name, tt.want)
				}
			}
		})
	}
}

// TestFindAFDEndpoint_duplicateNamesAreStable pins the tie-break. Endpoint
// names resolve globally because the data plane has only the Host header, and
// the generated host derives from the name alone, so two endpoints sharing a
// name collide by construction. The pick must at least not vary per request.
func TestFindAFDEndpoint_duplicateNamesAreStable(t *testing.T) {
	s := store.NewMemoryStore()
	a := NewRouter(s, "http://origin", "https://kv", "redis://r:6379")

	for _, profile := range []string{"fdB", "fdA", "fdC"} {
		id := afdEndpointID("sub1", "rg1", profile, "edge")
		if err := s.Put(id, &store.Resource{
			ID: id, Name: "edge", Type: afdEndpointTypeString,
			Properties: map[string]interface{}{"hostName": "edge.azurefd.net"},
		}); err != nil {
			t.Fatalf("seed %s: %v", id, err)
		}
	}

	first, ok := a.findAFDEndpoint("edge")
	if !ok {
		t.Fatal("findAFDEndpoint found nothing")
	}
	for i := 0; i < 30; i++ {
		got, ok := a.findAFDEndpoint("edge")
		if !ok || got.ID != first.ID {
			t.Fatalf("resolution varies between calls: %q then %q", first.ID, got.ID)
		}
	}
}

package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// captureHandler is a tiny http.Handler that records the path it received
// so a test can assert what the middleware passed downstream.
type captureHandler struct {
	gotPath string
}

func (c *captureHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c.gotPath = r.URL.Path
	w.WriteHeader(http.StatusOK)
}

func runMiddleware(t *testing.T, inputPath string) string {
	t.Helper()
	cap := &captureHandler{}
	handler := NormalizePath(cap)
	req := httptest.NewRequest(http.MethodGet, "http://example.test"+inputPath, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return cap.gotPath
}

func TestNormalizePath_LowercasesCamelCaseARMLiterals(t *testing.T) {
	// The exact case azurerm v4.x sends.
	in := "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/azemu-test-rg"
	want := "/subscriptions/00000000-0000-0000-0000-000000000000/resourcegroups/azemu-test-rg"
	if got := runMiddleware(t, in); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestNormalizePath_LowercasesProviderNamespace(t *testing.T) {
	in := "/subscriptions/sub1/resourceGroups/rg1/providers/Microsoft.Network/virtualNetworks/vnet1"
	want := "/subscriptions/sub1/resourcegroups/rg1/providers/microsoft.network/virtualnetworks/vnet1"
	if got := runMiddleware(t, in); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestNormalizePath_PreservesParameterCasing(t *testing.T) {
	// Resource group name "MyMixedCaseRG" must NOT be lowercased — it's a
	// user-supplied parameter, not a known ARM literal.
	in := "/subscriptions/sub1/resourceGroups/MyMixedCaseRG"
	want := "/subscriptions/sub1/resourcegroups/MyMixedCaseRG"
	if got := runMiddleware(t, in); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestNormalizePath_PreservesAllLowercaseInput(t *testing.T) {
	// Curl/smoke-test traffic sends lowercase already; the middleware
	// must be a no-op for it (no allocation if possible).
	in := "/subscriptions/sub1/resourcegroups/rg1"
	want := "/subscriptions/sub1/resourcegroups/rg1"
	if got := runMiddleware(t, in); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestNormalizePath_CollapsesDoubleSlashes(t *testing.T) {
	// The provider concatenates metadata_host with /metadata/endpoints and
	// emits a leading "//" — chi treats this as a separate route.
	in := "//metadata/endpoints"
	want := "/metadata/endpoints"
	if got := runMiddleware(t, in); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestNormalizePath_CollapsesInternalSlashRuns(t *testing.T) {
	in := "/subscriptions///sub1/resourceGroups//rg1"
	want := "/subscriptions/sub1/resourcegroups/rg1"
	if got := runMiddleware(t, in); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestNormalizePath_OAuthPathUntouched(t *testing.T) {
	// OAuth tenant IDs include hyphens and zeros that must not be normalized
	// because they are parameter values, not known literals.
	in := "/00000000-0000-0000-0000-000000000001/oauth2/v2.0/token"
	want := "/00000000-0000-0000-0000-000000000001/oauth2/v2.0/token"
	if got := runMiddleware(t, in); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestNormalizePath_LowercaseLiteralsAreNoOp(t *testing.T) {
	// Existing handlers and curl smoke tests use lowercase already.
	in := "/subscriptions/sub1/resourcegroups/rg1/providers/microsoft.network/virtualnetworks/vnet1/subnets/sub-a"
	want := in
	if got := runMiddleware(t, in); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestNormalizePath_userNamesAreNotVocabulary guards the trade-off this map
// makes. It is applied to every segment of every request, including names the
// user chose, so any common English word added to it silently renames their
// resources. `routes` and `origins` were listed here for Front Door even
// though ARM already sends them lowercase, which made the entries useless for
// routing and harmful for anyone with a resource group called "Routes".
func TestNormalizePath_userNamesAreNotVocabulary(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "resource group named Routes keeps its case",
			path: "/subscriptions/s1/resourceGroups/Routes",
			want: "/subscriptions/s1/resourcegroups/Routes",
		},
		{
			name: "resource group named Origins keeps its case",
			path: "/subscriptions/s1/resourceGroups/Origins",
			want: "/subscriptions/s1/resourcegroups/Origins",
		},
		{
			name: "a vault secret named Routes keeps its case",
			path: "/subscriptions/s1/resourceGroups/rg/providers/Microsoft.KeyVault/vaults/Routes",
			want: "/subscriptions/s1/resourcegroups/rg/providers/microsoft.keyvault/vaults/Routes",
		},
		{
			// The literals that genuinely need normalising still do.
			name: "Front Door camelCase literals are still lowercased",
			path: "/subscriptions/s1/resourceGroups/rg/providers/Microsoft.Cdn/profiles/p1/afdEndpoints/e1/routes/r1",
			want: "/subscriptions/s1/resourcegroups/rg/providers/microsoft.cdn/profiles/p1/afdendpoints/e1/routes/r1",
		},
		{
			name: "originGroups is still lowercased",
			path: "/subscriptions/s1/resourceGroups/rg/providers/Microsoft.Cdn/profiles/p1/originGroups/og1/origins/o1",
			want: "/subscriptions/s1/resourcegroups/rg/providers/microsoft.cdn/profiles/p1/origingroups/og1/origins/o1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var got string
			h := NormalizePath(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				got = r.URL.Path
			}))
			h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, tt.path, nil))
			if got != tt.want {
				t.Errorf("NormalizePath(%q)\n got  %q\n want %q", tt.path, got, tt.want)
			}
		})
	}
}

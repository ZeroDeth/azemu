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

package metadata

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/zerodeth/azemu/pkg/config"
)

// newTestService wires the metadata service into a chi router exactly as
// cmd/azemu/main.go does, using a config that mirrors the defaults.
func newTestService(t *testing.T) *httptest.Server {
	t.Helper()
	cfg := &config.Config{
		HTTPPort:     4566,
		HTTPSPort:    4567,
		MetadataHost: "localhost:4567",
		TenantID:     "00000000-0000-0000-0000-000000000001",
	}
	svc := NewService(cfg)
	r := chi.NewRouter()
	r.Route("/metadata", svc.Routes)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv
}

func fetchMetadata(t *testing.T, srv *httptest.Server) map[string]interface{} {
	t.Helper()
	resp, err := http.Get(srv.URL + "/metadata/endpoints?api-version=2022-09-01")
	if err != nil {
		t.Fatalf("get metadata: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var body map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return body
}

// TestMetadata_RequiredFields ensures the metadata response has every field
// the azurerm provider needs to construct a working cloud environment.
// Missing any of these triggers a different failure mode (often a cryptic
// nil dereference deep in go-azure-sdk).
func TestMetadata_RequiredFields(t *testing.T) {
	srv := newTestService(t)
	body := fetchMetadata(t, srv)

	required := []string{
		"name",
		"resourceManager",
		"authentication",
		"graphEndpoint",
		"portalEndpoint",
		"activeDirectoryResourceId",
	}
	for _, k := range required {
		if _, ok := body[k]; !ok {
			t.Errorf("required field %q missing from metadata response", k)
		}
	}

	if body["name"] != "AzureCloud" {
		t.Errorf("name = %v, want AzureCloud (needed by classifier)", body["name"])
	}
}

// TestMetadata_AllLocalhostURLsUseHTTPS is the regression guard for the
// "Azure Stack rejection" bug. The azurerm provider's cloud classifier
// walks every URL-valued field in the metadata response and flips the
// environment classification to Azure Stack if ANY localhost URL uses
// http://. Earlier versions of this service declared a `dataPlane`
// variable as http:// and assigned it to `batch` and `sqlManagement`,
// which was enough to break the entire provider even though
// resourceManager itself was https.
//
// This test walks the entire JSON tree and asserts that every string that
// parses as a URL and has a localhost-like host uses the https scheme.
// It is intentionally strict: it would rather fail loudly on a new field
// than let another scheme mismatch ship.
func TestMetadata_AllLocalhostURLsUseHTTPS(t *testing.T) {
	srv := newTestService(t)
	body := fetchMetadata(t, srv)

	var problems []string
	walkURLs(body, "", func(path, value string) {
		u, err := url.Parse(value)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return // not a URL
		}
		host := u.Hostname()
		if host != "localhost" && host != "127.0.0.1" && host != "::1" {
			return // external URL, not our concern
		}
		if u.Scheme != "https" {
			problems = append(problems, path+" = "+value)
		}
	})
	if len(problems) > 0 {
		t.Errorf("metadata response has %d localhost URL(s) with non-https scheme:\n  %s",
			len(problems), strings.Join(problems, "\n  "))
	}
}

// TestMetadata_NotClassifiedAsAzureStack pins the three fields the
// azurerm provider's IsAzureStack() classifier inspects in
// go-azure-sdk/sdk/environments/azure_stack.go. The classifier returns
// true (and the provider rejects the environment with "does not support
// Azure Stack") if ANY of:
//
//   - name == "AzureStackCloud"                  (case-insensitive)
//   - authentication.identityProvider != "AAD"   (case-insensitive)
//   - authentication.tenant != "common"          (case-insensitive)
//
// Earlier versions of azemu populated authentication.tenant from the
// user's tenant UUID, which broke this check even though identityProvider
// was correct. This test catches any regression that re-breaks any of the
// three conditions.
func TestMetadata_NotClassifiedAsAzureStack(t *testing.T) {
	srv := newTestService(t)
	body := fetchMetadata(t, srv)

	if !strings.EqualFold(body["name"].(string), "AzureCloud") {
		t.Errorf("name = %q, want AzureCloud (else IsAzureStack returns true)", body["name"])
	}

	auth, ok := body["authentication"].(map[string]interface{})
	if !ok {
		t.Fatalf("authentication missing or wrong type: %v", body["authentication"])
	}
	if !strings.EqualFold(auth["identityProvider"].(string), "AAD") {
		t.Errorf("identityProvider = %q, want AAD", auth["identityProvider"])
	}
	if !strings.EqualFold(auth["tenant"].(string), "common") {
		t.Errorf("authentication.tenant = %q, want \"common\" "+
			"(NOT the user's tenant UUID — this is a cloud-environment identifier "+
			"and the IsAzureStack classifier rejects anything else)",
			auth["tenant"])
	}
}

// TestMetadata_DataPlaneFieldsAreHTTPS pins the specific fields that broke
// in the past so a future refactor that reintroduces the bug has a test
// name that clearly identifies the regression.
func TestMetadata_DataPlaneFieldsAreHTTPS(t *testing.T) {
	srv := newTestService(t)
	body := fetchMetadata(t, srv)

	for _, field := range []string{"batch", "sqlManagement", "resourceManager"} {
		v, ok := body[field].(string)
		if !ok {
			t.Errorf("%s missing or not a string: %v", field, body[field])
			continue
		}
		if !strings.HasPrefix(v, "https://") {
			t.Errorf("%s = %q, want https:// scheme (azure-stack classifier regression)", field, v)
		}
	}
}

// walkURLs invokes fn for every string value in a nested map/array tree,
// passing a dotted path for diagnostic messages.
func walkURLs(node interface{}, path string, fn func(path, value string)) {
	switch v := node.(type) {
	case map[string]interface{}:
		for k, child := range v {
			next := k
			if path != "" {
				next = path + "." + k
			}
			walkURLs(child, next, fn)
		}
	case []interface{}:
		for i, child := range v {
			walkURLs(child, path+"["+itoa(i)+"]", fn)
		}
	case string:
		fn(path, v)
	}
}

func itoa(i int) string {
	// local tiny helper so the test file has no strconv import clutter
	if i == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(buf[pos:])
}

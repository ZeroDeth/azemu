package auth

import (
	"crypto/rsa"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"

	"github.com/zerodeth/azemu/internal/store"
)

// newAuthTestServer creates an httptest.Server with the full token, OIDC, and
// JWKS routes wired the same way cmd/azemu/main.go wires them.
func newAuthTestServer(t *testing.T, tenantID string) (*httptest.Server, *TokenService) {
	t.Helper()
	svc, err := NewTokenService(tenantID)
	if err != nil {
		t.Fatalf("NewTokenService: %v", err)
	}
	r := chi.NewRouter()
	svc.Routes(r)
	r.Get("/{tenantID}/.well-known/openid-configuration", svc.OpenIDConfig)
	r.Get("/{tenantID}/discovery/v2.0/keys", svc.JWKS)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv, svc
}

// postToken sends a POST /token request and returns the raw access_token string.
func postToken(t *testing.T, srv *httptest.Server, clientID string) string {
	t.Helper()
	form := url.Values{}
	form.Set("client_id", clientID)
	form.Set("grant_type", "client_credentials")
	form.Set("client_secret", "ignored")
	resp, err := http.Post(srv.URL+"/token", "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatalf("POST /token: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST /token status %d: %s", resp.StatusCode, body)
	}
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode token response: %v", err)
	}
	token, ok := result["access_token"].(string)
	if !ok || token == "" {
		t.Fatalf("access_token missing or empty in response: %v", result)
	}
	return token
}

func postTokenForm(t *testing.T, srv *httptest.Server, form url.Values) (*http.Response, map[string]interface{}) {
	t.Helper()
	resp, err := http.Post(srv.URL+"/token", "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatalf("POST /token: %v", err)
	}
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		resp.Body.Close()
		t.Fatalf("decode token response: %v", err)
	}
	resp.Body.Close()
	return resp, result
}

func unsignedAssertion(t *testing.T, issuer, subject string, audiences interface{}) string {
	t.Helper()
	return unsignedAssertionWithClaims(t, jwt.MapClaims{
		"iss": issuer,
		"sub": subject,
		"aud": audiences,
	})
}

func unsignedAssertionWithClaims(t *testing.T, claims jwt.MapClaims) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodNone, claims)
	signed, err := token.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("sign unsigned assertion: %v", err)
	}
	return signed
}

// parseUnchecked parses a JWT without signature verification and returns the
// parsed token (header + claims accessible).
func parseUnchecked(t *testing.T, raw string) *jwt.Token {
	t.Helper()
	p := jwt.NewParser()
	token, _, err := p.ParseUnverified(raw, jwt.MapClaims{})
	if err != nil {
		t.Fatalf("ParseUnverified: %v", err)
	}
	return token
}

// getJWKS fetches the JWKS from /{tenantID}/discovery/v2.0/keys.
func getJWKS(t *testing.T, srv *httptest.Server, tenantID string) map[string]interface{} {
	t.Helper()
	resp, err := http.Get(srv.URL + "/" + tenantID + "/discovery/v2.0/keys")
	if err != nil {
		t.Fatalf("GET jwks: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET jwks status %d", resp.StatusCode)
	}
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode JWKS: %v", err)
	}
	return result
}

// extractFirstKey returns the first key map from a JWKS response.
func extractFirstKey(t *testing.T, jwks map[string]interface{}) map[string]interface{} {
	t.Helper()
	keys, ok := jwks["keys"].([]interface{})
	if !ok || len(keys) == 0 {
		t.Fatalf("jwks.keys missing or empty")
	}
	key, ok := keys[0].(map[string]interface{})
	if !ok {
		t.Fatalf("jwks.keys[0] is not an object")
	}
	return key
}

// TestToken_ReturnsJWTWithRS256Header verifies the JWT header carries alg=RS256
// and the expected key ID.
func TestToken_ReturnsJWTWithRS256Header(t *testing.T) {
	srv, _ := newAuthTestServer(t, "test-tenant-id")
	raw := postToken(t, srv, "test-client")
	tok := parseUnchecked(t, raw)

	if tok.Method.Alg() != "RS256" {
		t.Errorf("alg = %q, want RS256", tok.Method.Alg())
	}
	kid, _ := tok.Header["kid"].(string)
	if kid != "azemu-signing-key-1" {
		t.Errorf("kid = %q, want azemu-signing-key-1", kid)
	}
}

// TestToken_ClaimsContainRequiredFields checks that the token contains all
// required ARM/AAD claims.
func TestToken_ClaimsContainRequiredFields(t *testing.T) {
	srv, _ := newAuthTestServer(t, "test-tenant-id")
	raw := postToken(t, srv, "my-app-id")
	tok := parseUnchecked(t, raw)
	claims, ok := tok.Claims.(jwt.MapClaims)
	if !ok {
		t.Fatalf("claims are not MapClaims")
	}

	required := []string{"aud", "iss", "iat", "nbf", "exp", "tid", "oid", "appid", "sub"}
	for _, field := range required {
		if _, present := claims[field]; !present {
			t.Errorf("claim %q missing from token", field)
		}
	}
}

// TestToken_ExpiryIsOneHourFromIssue checks that exp - iat == 3600 within
// a two-second tolerance.
func TestToken_ExpiryIsOneHourFromIssue(t *testing.T) {
	srv, _ := newAuthTestServer(t, "test-tenant-id")
	raw := postToken(t, srv, "test-client")
	tok := parseUnchecked(t, raw)
	claims := tok.Claims.(jwt.MapClaims)

	iat, ok1 := claims["iat"].(float64)
	exp, ok2 := claims["exp"].(float64)
	if !ok1 || !ok2 {
		t.Fatalf("iat or exp is not a number: iat=%v exp=%v", claims["iat"], claims["exp"])
	}
	diff := exp - iat
	const want = 3600.0
	const tolerance = 2.0
	if diff < want-tolerance || diff > want+tolerance {
		t.Errorf("exp-iat = %.0f, want %d (±%.0f)", diff, int(want), tolerance)
	}
}

// TestToken_IssuerMatchesTenant constructs a TokenService with a known tenant
// ID and verifies the iss claim encodes that tenant ID.
func TestToken_IssuerMatchesTenant(t *testing.T) {
	const tenantID = "aaaabbbb-1111-2222-3333-ccccddddeeee"
	srv, _ := newAuthTestServer(t, tenantID)
	raw := postToken(t, srv, "test-client")
	tok := parseUnchecked(t, raw)
	claims := tok.Claims.(jwt.MapClaims)

	iss, _ := claims["iss"].(string)
	wantIss := "https://sts.windows.net/" + tenantID + "/"
	if iss != wantIss {
		t.Errorf("iss = %q, want %q", iss, wantIss)
	}
}

// TestToken_SignatureVerifiesWithJWKS performs an end-to-end round-trip: POST
// token, GET JWKS, reconstruct the RSA public key from the n/e parameters, and
// verify the token signature.
func TestToken_SignatureVerifiesWithJWKS(t *testing.T) {
	const tenantID = "sig-verify-tenant"
	srv, _ := newAuthTestServer(t, tenantID)
	raw := postToken(t, srv, "test-client")
	jwks := getJWKS(t, srv, tenantID)
	key := extractFirstKey(t, jwks)

	nStr, _ := key["n"].(string)
	eStr, _ := key["e"].(string)
	if nStr == "" || eStr == "" {
		t.Fatalf("n or e missing from JWKS key")
	}

	// base64url decode without padding (standard RawURLEncoding)
	nBytes, err := base64.RawURLEncoding.DecodeString(nStr)
	if err != nil {
		t.Fatalf("decode n: %v", err)
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(eStr)
	if err != nil {
		t.Fatalf("decode e: %v", err)
	}

	n := new(big.Int).SetBytes(nBytes)
	var eInt int
	for _, b := range eBytes {
		eInt = eInt<<8 | int(b)
	}
	pub := &rsa.PublicKey{N: n, E: eInt}

	_, err = jwt.Parse(raw, func(token *jwt.Token) (interface{}, error) {
		return pub, nil
	}, jwt.WithValidMethods([]string{"RS256"}))
	if err != nil {
		t.Errorf("signature verification failed: %v", err)
	}
}

func newFederatedTokenServer(t *testing.T, issuer, subject, audience string) (*httptest.Server, *TokenService, string) {
	t.Helper()
	s := store.NewMemoryStore()
	identityID := "/subscriptions/sub1/resourceGroups/rg1/providers/Microsoft.ManagedIdentity/userAssignedIdentities/my-identity"
	clientID := "11111111-1111-1111-1111-111111111111"
	if err := s.Put(identityID, &store.Resource{
		ID:   identityID,
		Name: "my-identity",
		Type: "Microsoft.ManagedIdentity/userAssignedIdentities",
		Properties: map[string]interface{}{
			"clientId":    clientID,
			"principalId": "22222222-2222-2222-2222-222222222222",
			"tenantId":    "test-tenant-id",
		},
	}); err != nil {
		t.Fatalf("put identity: %v", err)
	}
	if err := s.Put(identityID+"/federatedIdentityCredentials/fic-one", &store.Resource{
		ID:   identityID + "/federatedIdentityCredentials/fic-one",
		Name: "fic-one",
		Type: "Microsoft.ManagedIdentity/userAssignedIdentities/federatedIdentityCredentials",
		Properties: map[string]interface{}{
			"issuer":    issuer,
			"subject":   subject,
			"audiences": []string{audience},
		},
	}); err != nil {
		t.Fatalf("put federated identity credential: %v", err)
	}

	svc, err := NewTokenService("test-tenant-id", s)
	if err != nil {
		t.Fatalf("NewTokenService: %v", err)
	}
	r := chi.NewRouter()
	svc.Routes(r)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv, svc, clientID
}

func TestToken_WorkloadIdentityAssertion_ReturnsAccessToken(t *testing.T) {
	const issuer = "https://issuer.test/"
	const subject = "system:serviceaccount:default:app"
	const audience = "api://AzureADTokenExchange"
	srv, _, clientID := newFederatedTokenServer(t, issuer, subject, audience)

	form := url.Values{}
	form.Set("client_id", clientID)
	form.Set("client_assertion", unsignedAssertion(t, issuer, subject, audience))
	form.Set("grant_type", "client_credentials")
	form.Set("scope", "https://vault.azure.net/.default")

	resp, body := postTokenForm(t, srv, form)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200. body=%v", resp.StatusCode, body)
	}
	raw, _ := body["access_token"].(string)
	tok := parseUnchecked(t, raw)
	claims := tok.Claims.(jwt.MapClaims)
	if claims["aud"] != "https://vault.azure.net" {
		t.Errorf("aud = %v, want https://vault.azure.net", claims["aud"])
	}
	if claims["appid"] != clientID {
		t.Errorf("appid = %v, want %s", claims["appid"], clientID)
	}
	if claims["xms_mirid"] == nil {
		t.Error("xms_mirid missing")
	}
}

func TestToken_WorkloadIdentityAssertion_MismatchReturns400(t *testing.T) {
	const issuer = "https://issuer.test/"
	const subject = "system:serviceaccount:default:app"
	const audience = "api://AzureADTokenExchange"
	srv, _, clientID := newFederatedTokenServer(t, issuer, subject, audience)

	tests := []struct {
		name      string
		issuer    string
		subject   string
		audiences interface{}
	}{
		{name: "issuer", issuer: "https://other-issuer.test/", subject: subject, audiences: audience},
		{name: "subject", issuer: issuer, subject: "system:serviceaccount:default:other", audiences: audience},
		{name: "audience", issuer: issuer, subject: subject, audiences: "api://OtherAudience"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			form := url.Values{}
			form.Set("client_id", clientID)
			form.Set("client_assertion", unsignedAssertion(t, tt.issuer, tt.subject, tt.audiences))
			resp, body := postTokenForm(t, srv, form)
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400. body=%v", resp.StatusCode, body)
			}
			if body["error"] != "invalid_grant" {
				t.Errorf("error = %v, want invalid_grant", body["error"])
			}
		})
	}
}

func TestToken_WorkloadIdentityAssertion_ExpiredAssertionRejected(t *testing.T) {
	const issuer = "https://issuer.test/"
	const subject = "system:serviceaccount:default:app"
	const audience = "api://AzureADTokenExchange"
	srv, _, clientID := newFederatedTokenServer(t, issuer, subject, audience)

	expired := unsignedAssertionWithClaims(t, jwt.MapClaims{
		"iss": issuer,
		"sub": subject,
		"aud": audience,
		"exp": time.Now().Add(-1 * time.Minute).Unix(),
	})
	form := url.Values{}
	form.Set("client_id", clientID)
	form.Set("client_assertion", expired)
	resp, body := postTokenForm(t, srv, form)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400. body=%v", resp.StatusCode, body)
	}
	if body["error"] != "invalid_grant" {
		t.Errorf("error = %v, want invalid_grant", body["error"])
	}
}

func TestToken_WorkloadIdentityAssertion_NotYetValidRejected(t *testing.T) {
	const issuer = "https://issuer.test/"
	const subject = "system:serviceaccount:default:app"
	const audience = "api://AzureADTokenExchange"
	srv, _, clientID := newFederatedTokenServer(t, issuer, subject, audience)

	future := unsignedAssertionWithClaims(t, jwt.MapClaims{
		"iss": issuer,
		"sub": subject,
		"aud": audience,
		"nbf": time.Now().Add(5 * time.Minute).Unix(),
	})
	form := url.Values{}
	form.Set("client_id", clientID)
	form.Set("client_assertion", future)
	resp, body := postTokenForm(t, srv, form)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400. body=%v", resp.StatusCode, body)
	}
}

func TestToken_WorkloadIdentityAssertion_ArrayAudienceMatches(t *testing.T) {
	const issuer = "https://issuer.test/"
	const subject = "system:serviceaccount:default:app"
	const audience = "api://AzureADTokenExchange"
	srv, _, clientID := newFederatedTokenServer(t, issuer, subject, audience)

	form := url.Values{}
	form.Set("client_id", clientID)
	form.Set("client_assertion", unsignedAssertion(t, issuer, subject, []string{"api://Other", audience}))
	resp, body := postTokenForm(t, srv, form)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200. body=%v", resp.StatusCode, body)
	}
}

// TestOpenIDConfig_EmitsRequiredFields verifies all seven OIDC discovery fields
// are present and that id_token_signing_alg_values_supported contains RS256.
func TestOpenIDConfig_EmitsRequiredFields(t *testing.T) {
	const tenantID = "oidc-fields-tenant"
	srv, _ := newAuthTestServer(t, tenantID)

	resp, err := http.Get(srv.URL + "/" + tenantID + "/.well-known/openid-configuration")
	if err != nil {
		t.Fatalf("GET openid-configuration: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET openid-configuration status %d", resp.StatusCode)
	}

	var doc map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		t.Fatalf("decode openid-configuration: %v", err)
	}

	requiredFields := []string{
		"issuer",
		"authorization_endpoint",
		"token_endpoint",
		"jwks_uri",
		"response_types_supported",
		"subject_types_supported",
		"id_token_signing_alg_values_supported",
	}
	for _, field := range requiredFields {
		if _, ok := doc[field]; !ok {
			t.Errorf("field %q missing from openid-configuration", field)
		}
	}

	// RS256 must appear in id_token_signing_alg_values_supported.
	algs, _ := doc["id_token_signing_alg_values_supported"].([]interface{})
	found := false
	for _, alg := range algs {
		if alg.(string) == "RS256" {
			found = true
		}
	}
	if !found {
		t.Errorf("RS256 missing from id_token_signing_alg_values_supported: %v", algs)
	}
}

// TestOpenIDConfig_URLsUseRequestHost confirms that the URLs in the OIDC
// discovery document use the host from the incoming request (r.Host handling).
func TestOpenIDConfig_URLsUseRequestHost(t *testing.T) {
	const tenantID = "host-check-tenant"
	srv, _ := newAuthTestServer(t, tenantID)

	resp, err := http.Get(srv.URL + "/" + tenantID + "/.well-known/openid-configuration")
	if err != nil {
		t.Fatalf("GET openid-configuration: %v", err)
	}
	defer resp.Body.Close()

	var doc map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		t.Fatalf("decode openid-configuration: %v", err)
	}

	// httptest server URL is http://127.0.0.1:<port>. The handler prepends
	// "https://" + r.Host, so the host portion of the URL must match.
	parsed, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse test server URL: %v", err)
	}
	wantHost := parsed.Host

	urlFields := []string{"issuer", "authorization_endpoint", "token_endpoint", "jwks_uri"}
	for _, field := range urlFields {
		val, _ := doc[field].(string)
		if val == "" {
			t.Errorf("field %q is empty", field)
			continue
		}
		u, err := url.Parse(val)
		if err != nil {
			t.Errorf("parse %q value %q: %v", field, val, err)
			continue
		}
		if u.Host != wantHost {
			t.Errorf("%q host = %q, want %q", field, u.Host, wantHost)
		}
	}
}

// TestJWKS_ShapeMatchesContract verifies the JWKS response matches the
// expected shape: keys[0] has kty=RSA, use=sig, kid=azemu-signing-key-1,
// and non-empty n and e.
func TestJWKS_ShapeMatchesContract(t *testing.T) {
	const tenantID = "jwks-shape-tenant"
	srv, _ := newAuthTestServer(t, tenantID)
	jwks := getJWKS(t, srv, tenantID)
	key := extractFirstKey(t, jwks)

	cases := []struct {
		field string
		want  string
	}{
		{"kty", "RSA"},
		{"use", "sig"},
		{"kid", "azemu-signing-key-1"},
	}
	for _, c := range cases {
		got, _ := key[c.field].(string)
		if got != c.want {
			t.Errorf("keys[0].%s = %q, want %q", c.field, got, c.want)
		}
	}

	for _, field := range []string{"n", "e"} {
		val, _ := key[field].(string)
		if val == "" {
			t.Errorf("keys[0].%s is empty", field)
		}
	}
}

// TestJWKS_KidMatchesTokenHeader is a regression guard for key-rotation bugs:
// the kid in the JWT header must equal the kid in the JWKS.
func TestJWKS_KidMatchesTokenHeader(t *testing.T) {
	const tenantID = "kid-match-tenant"
	srv, _ := newAuthTestServer(t, tenantID)

	raw := postToken(t, srv, "test-client")
	tok := parseUnchecked(t, raw)
	headerKid, _ := tok.Header["kid"].(string)

	jwks := getJWKS(t, srv, tenantID)
	key := extractFirstKey(t, jwks)
	jwksKid, _ := key["kid"].(string)

	if headerKid != jwksKid {
		t.Errorf("JWT kid %q != JWKS kid %q", headerKid, jwksKid)
	}
}

// TestRoutesV2_TokenEndpointResponds confirms that RoutesV2 registers POST
// /token and issues a valid JWT, identical in shape to Routes.
func TestRoutesV2_TokenEndpointResponds(t *testing.T) {
	svc, err := NewTokenService("v2-tenant")
	if err != nil {
		t.Fatalf("NewTokenService: %v", err)
	}
	r := chi.NewRouter()
	svc.RoutesV2(r)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	raw := postToken(t, srv, "v2-client")
	tok := parseUnchecked(t, raw)
	if tok.Method.Alg() != "RS256" {
		t.Errorf("RoutesV2 token alg = %q, want RS256", tok.Method.Alg())
	}
}

// TestOpenIDConfig_FallsBackToServiceTenant verifies that when OpenIDConfig is
// called without a chi tenantID URL param, it falls back to the tenant ID
// stored in the TokenService.
func TestOpenIDConfig_FallsBackToServiceTenant(t *testing.T) {
	const tenantID = "fallback-tenant"
	svc, err := NewTokenService(tenantID)
	if err != nil {
		t.Fatalf("NewTokenService: %v", err)
	}
	// Wire OpenIDConfig directly without the {tenantID} URL param.
	r := chi.NewRouter()
	r.Get("/.well-known/openid-configuration", svc.OpenIDConfig)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/.well-known/openid-configuration")
	if err != nil {
		t.Fatalf("GET openid-configuration: %v", err)
	}
	defer resp.Body.Close()

	var doc map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		t.Fatalf("decode openid-configuration: %v", err)
	}

	issuer, _ := doc["issuer"].(string)
	if !strings.Contains(issuer, tenantID) {
		t.Errorf("issuer %q does not contain fallback tenantID %q", issuer, tenantID)
	}
}

// TestGenerateSelfSignedTLS_ReturnsUsableCert exercises the exported
// GenerateSelfSignedTLS function (no persistence path).
func TestGenerateSelfSignedTLS_ReturnsUsableCert(t *testing.T) {
	cert, err := GenerateSelfSignedTLS("localhost", "127.0.0.1")
	if err != nil {
		t.Fatalf("GenerateSelfSignedTLS: %v", err)
	}
	if len(cert.Certificate) == 0 {
		t.Error("returned certificate has no DER bytes")
	}
}

// TestWriteCertToTemp_WritesFile verifies that WriteCertToTemp persists a PEM
// file to the temp directory and returns a non-empty path.
func TestWriteCertToTemp_WritesFile(t *testing.T) {
	cert, err := GenerateSelfSignedTLS("localhost")
	if err != nil {
		t.Fatalf("GenerateSelfSignedTLS: %v", err)
	}
	path, err := WriteCertToTemp(cert)
	if err != nil {
		t.Fatalf("WriteCertToTemp: %v", err)
	}
	if path == "" {
		t.Fatal("WriteCertToTemp returned empty path")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat cert file: %v", err)
	}
	if info.Size() == 0 {
		t.Error("cert file is empty")
	}
}

// TestWriteCertToTemp_EmptyCertReturnsEmptyPath ensures WriteCertToTemp
// returns an empty path (and no error) when given a cert with no DER bytes.
func TestWriteCertToTemp_EmptyCertReturnsEmptyPath(t *testing.T) {
	path, err := WriteCertToTemp(tls.Certificate{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path != "" {
		t.Errorf("path = %q, want empty string for zero cert", path)
	}
}

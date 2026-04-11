//go:build integration

// auth_test covers the token + OIDC discovery + JWKS contract end-to-end
// through the production mux. The most valuable assertion here is the
// full kid-in-header round-trip: mint a token via POST /{tenant}/oauth2/v2.0/token,
// fetch the JWKS pointed at by OIDC discovery, pick the key out by its kid,
// and verify the token signature against that key. This is exactly the
// sequence the azurerm provider follows, so a regression in any of the
// three handlers surfaces here before it surfaces in `terraform apply`.

package integration

import (
	"crypto/rsa"
	"encoding/base64"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/golang-jwt/jwt/v5"
)

// postForm issues a POST with form-encoded body using the httptest TLS
// server's own client (which trusts the server's self-signed cert).
// Used for the OAuth token endpoint which reads credentials via r.FormValue.
func postForm(t *testing.T, client *http.Client, u string, values url.Values) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, u, strings.NewReader(values.Encode()))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("do POST %s: %v", u, err)
	}
	return resp
}

// getJSON is a thin wrapper that issues a GET via the server's own TLS
// client so the self-signed cert is trusted.
func getJSON(t *testing.T, client *http.Client, u string) *http.Response {
	t.Helper()
	resp, err := client.Get(u)
	if err != nil {
		t.Fatalf("do GET %s: %v", u, err)
	}
	return resp
}

// TestAuth_TokenEndpoint_ReturnsSignedJWT posts to the v2.0 token endpoint
// through the full middleware stack and verifies the response shape plus
// the JWT header contract. Every middleware in the production stack
// runs (NormalizePath, AzureHeaders, RequireAPIVersion) so a regression
// that breaks any of them for auth routes surfaces here.
func TestAuth_TokenEndpoint_ReturnsSignedJWT(t *testing.T) {
	srv := buildProductionLikeServer(t)
	client := srv.Client()
	tokenURL := srv.URL + "/" + testTenantID + "/oauth2/v2.0/token"

	resp := postForm(t, client, tokenURL, url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {"test-client-id"},
		"client_secret": {"test-secret"},
		"resource":      {"https://management.azure.com/"},
	})
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("status = %d, want 200. body=%s", resp.StatusCode, string(body))
	}
	body := decode(t, resp)

	// Response shape: the four fields the provider cares about.
	if body["token_type"] != "Bearer" {
		t.Errorf("token_type = %v, want Bearer", body["token_type"])
	}
	if body["resource"] != "https://management.azure.com/" {
		t.Errorf("resource = %v, want https://management.azure.com/", body["resource"])
	}
	if body["expires_in"] == nil {
		t.Errorf("expires_in missing")
	}
	rawToken, ok := body["access_token"].(string)
	if !ok || rawToken == "" {
		t.Fatalf("access_token missing or wrong type: %T", body["access_token"])
	}

	// Parse the token without verifying the signature yet — we want to read
	// the header's kid and the claims to pin the contract before we hit
	// JWKS. Signature verification is exercised end-to-end by
	// TestAuth_OIDCDiscoveryPlusJWKSVerifiesMintedToken below.
	parser := jwt.NewParser(jwt.WithoutClaimsValidation())
	tok, _, err := parser.ParseUnverified(rawToken, jwt.MapClaims{})
	if err != nil {
		t.Fatalf("parse token: %v", err)
	}
	if tok.Header["alg"] != "RS256" {
		t.Errorf("alg = %v, want RS256", tok.Header["alg"])
	}
	kid, ok := tok.Header["kid"].(string)
	if !ok || kid == "" {
		t.Errorf("kid header missing or wrong type: %v", tok.Header["kid"])
	}

	claims := tok.Claims.(jwt.MapClaims)
	if got := claims["tid"]; got != testTenantID {
		t.Errorf("tid claim = %v, want %s", got, testTenantID)
	}
	wantIss := "https://sts.windows.net/" + testTenantID + "/"
	if got := claims["iss"]; got != wantIss {
		t.Errorf("iss claim = %v, want %s", got, wantIss)
	}
	if got := claims["aud"]; got != "https://management.azure.com/" {
		t.Errorf("aud claim = %v, want https://management.azure.com/", got)
	}
	// appid must echo the form-posted client_id so the provider can confirm
	// the token was minted for the right identity.
	if got := claims["appid"]; got != "test-client-id" {
		t.Errorf("appid claim = %v, want test-client-id", got)
	}
}

// TestAuth_OIDCDiscoveryAdvertisesJWKS verifies the OIDC discovery document
// advertises the JWKS URL and signing algorithm the provider expects. The
// tenant is substituted into issuer and all endpoint URLs.
func TestAuth_OIDCDiscoveryAdvertisesJWKS(t *testing.T) {
	srv := buildProductionLikeServer(t)
	client := srv.Client()
	discoveryURL := srv.URL + "/" + testTenantID + "/.well-known/openid-configuration"

	resp := getJSON(t, client, discoveryURL)
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	doc := decode(t, resp)

	wantIssuer := srv.URL + "/" + testTenantID + "/"
	if doc["issuer"] != wantIssuer {
		t.Errorf("issuer = %v, want %s", doc["issuer"], wantIssuer)
	}
	wantJWKS := srv.URL + "/" + testTenantID + "/discovery/v2.0/keys"
	if doc["jwks_uri"] != wantJWKS {
		t.Errorf("jwks_uri = %v, want %s", doc["jwks_uri"], wantJWKS)
	}
	wantToken := srv.URL + "/" + testTenantID + "/oauth2/v2.0/token"
	if doc["token_endpoint"] != wantToken {
		t.Errorf("token_endpoint = %v, want %s", doc["token_endpoint"], wantToken)
	}

	algs, ok := doc["id_token_signing_alg_values_supported"].([]interface{})
	if !ok || len(algs) == 0 {
		t.Fatalf("id_token_signing_alg_values_supported missing: %v", doc)
	}
	if algs[0] != "RS256" {
		t.Errorf("signing alg = %v, want RS256", algs[0])
	}
}

// TestAuth_OIDCDiscoveryPlusJWKSVerifiesMintedToken is the contract test
// that catches the whole class of auth regressions: mint a token, discover
// the JWKS URL, fetch the JWKS, find the key by kid, reconstruct the RSA
// public key, and verify the token's signature with that key.
//
// This is the exact flow the azurerm provider follows for token
// verification. If any of the following break, this test catches it
// before `terraform apply` does:
//
//   - kid header in the JWT does not match a key in the JWKS
//   - JWKS n/e encoding differs from what jwt.Parse expects (base64url no
//     padding)
//   - Signing algorithm in the JWT diverges from the alg advertised by
//     OIDC discovery
//   - OIDC discovery advertises the wrong JWKS URL
func TestAuth_OIDCDiscoveryPlusJWKSVerifiesMintedToken(t *testing.T) {
	srv := buildProductionLikeServer(t)
	client := srv.Client()

	// 1. Mint a token.
	tokenURL := srv.URL + "/" + testTenantID + "/oauth2/v2.0/token"
	resp := postForm(t, client, tokenURL, url.Values{
		"grant_type": {"client_credentials"},
		"client_id":  {"azemu-provider"},
	})
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("token mint status = %d, want 200", resp.StatusCode)
	}
	tokenBody := decode(t, resp)
	rawToken := tokenBody["access_token"].(string)

	// 2. Fetch OIDC discovery and pull the JWKS URL.
	resp = getJSON(t, client, srv.URL+"/"+testTenantID+"/.well-known/openid-configuration")
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("discovery status = %d, want 200", resp.StatusCode)
	}
	doc := decode(t, resp)
	jwksURL, ok := doc["jwks_uri"].(string)
	if !ok || jwksURL == "" {
		t.Fatalf("jwks_uri missing from discovery doc: %v", doc)
	}

	// 3. Fetch the JWKS via the advertised URL — this is the exact path a
	// real OAuth client follows, so it proves discovery + JWKS + TLS all
	// work together.
	resp = getJSON(t, client, jwksURL)
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("jwks status = %d, want 200", resp.StatusCode)
	}
	jwks := decode(t, resp)
	keys, ok := jwks["keys"].([]interface{})
	if !ok || len(keys) == 0 {
		t.Fatalf("jwks keys missing or empty: %v", jwks)
	}

	// 4. Parse the token header to read kid, then find the matching JWKS
	// entry. jwt.Parse is invoked with a keyfunc that does the lookup so
	// the verification path mirrors what a real OAuth client does.
	verified, err := jwt.Parse(rawToken, func(tok *jwt.Token) (interface{}, error) {
		if _, ok := tok.Method.(*jwt.SigningMethodRSA); !ok {
			t.Fatalf("unexpected signing method: %v", tok.Header["alg"])
		}
		kid, ok := tok.Header["kid"].(string)
		if !ok || kid == "" {
			t.Fatalf("kid header missing: %v", tok.Header)
		}
		for _, k := range keys {
			km := k.(map[string]interface{})
			if km["kid"] != kid {
				continue
			}
			return rsaPubFromJWK(t, km), nil
		}
		t.Fatalf("no JWKS key matched kid=%q", kid)
		return nil, nil
	})
	if err != nil {
		t.Fatalf("jwt.Parse verify: %v", err)
	}
	if !verified.Valid {
		t.Fatalf("verified token marked invalid")
	}
}

// rsaPubFromJWK reconstructs an *rsa.PublicKey from a JWKS entry. The JWKS
// handler in internal/auth/token.go emits n and e as base64url without
// padding (the same encoding jwt.ParseRSAPublicKeyFromPEM does not handle),
// so this function replicates that decode path exactly. If the encoding
// side ever changes, this helper is the place to update.
func rsaPubFromJWK(t *testing.T, jwk map[string]interface{}) *rsa.PublicKey {
	t.Helper()
	nStr, _ := jwk["n"].(string)
	eStr, _ := jwk["e"].(string)
	if nStr == "" || eStr == "" {
		t.Fatalf("jwk missing n or e: %v", jwk)
	}
	nBytes, err := base64.RawURLEncoding.DecodeString(nStr)
	if err != nil {
		t.Fatalf("decode n: %v", err)
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(eStr)
	if err != nil {
		t.Fatalf("decode e: %v", err)
	}
	n := new(big.Int).SetBytes(nBytes)
	e := 0
	for _, b := range eBytes {
		e = e<<8 | int(b)
	}
	return &rsa.PublicKey{N: n, E: e}
}

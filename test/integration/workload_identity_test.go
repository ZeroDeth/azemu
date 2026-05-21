//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/golang-jwt/jwt/v5"
)

func unsignedWorkloadAssertion(t *testing.T, issuer, subject, audience string) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.MapClaims{
		"iss": issuer,
		"sub": subject,
		"aud": audience,
	})
	signed, err := token.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("sign unsigned assertion: %v", err)
	}
	return signed
}

func doJSONWithClient(t *testing.T, client *http.Client, method, url, body string) *http.Response {
	t.Helper()
	var rdr *strings.Reader
	if body != "" {
		rdr = strings.NewReader(body)
	} else {
		rdr = strings.NewReader("")
	}
	req, err := http.NewRequest(method, url, rdr)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("do %s %s: %v", method, url, err)
	}
	return resp
}

func doJSONWithBearer(t *testing.T, client *http.Client, method, url, body, token string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("do %s %s: %v", method, url, err)
	}
	return resp
}

func TestWorkloadIdentity_FederatedCredentialTokenHonouredByKeyVault(t *testing.T) {
	srv := buildProductionLikeServer(t)
	client := srv.Client()
	base := srv.URL

	const issuer = "https://issuer.test/"
	const subject = "system:serviceaccount:default:app"
	const audience = "api://AzureADTokenExchange"

	identityURL := base + "/subscriptions/sub1/resourcegroups/rg1/providers/microsoft.managedidentity/userassignedidentities/app-identity" + apiVersionQ
	resp := doJSONWithClient(t, client, http.MethodPut, identityURL, `{"location":"uksouth"}`)
	mustStatus(t, resp, http.StatusCreated)
	identityBody := decode(t, resp)
	clientID := identityBody["properties"].(map[string]interface{})["clientId"].(string)

	ficURL := base + "/subscriptions/sub1/resourcegroups/rg1/providers/microsoft.managedidentity/userassignedidentities/app-identity/federatedidentitycredentials/app-fic" + apiVersionQ
	resp = doJSONWithClient(t, client, http.MethodPut, ficURL,
		`{"properties":{"issuer":"`+issuer+`","subject":"`+subject+`","audiences":["`+audience+`"]}}`)
	mustStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	form := url.Values{}
	form.Set("client_id", clientID)
	form.Set("client_assertion", unsignedWorkloadAssertion(t, issuer, subject, audience))
	form.Set("grant_type", "client_credentials")
	form.Set("scope", "https://vault.azure.net/.default")
	resp, err := client.Post(base+"/"+testTenantID+"/oauth2/v2.0/token",
		"application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatalf("POST token: %v", err)
	}
	mustStatus(t, resp, http.StatusOK)
	var tokenBody map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&tokenBody); err != nil {
		t.Fatalf("decode token: %v", err)
	}
	resp.Body.Close()
	accessToken := tokenBody["access_token"].(string)

	secretURL := base + "/keyvault/myvault/secrets/bundle-signing-key" + apiVersionQ
	resp = doJSONWithBearer(t, client, http.MethodPut, secretURL, `{"value":"secret-value"}`, accessToken)
	mustStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = doJSONWithBearer(t, client, http.MethodGet, secretURL, "", accessToken)
	mustStatus(t, resp, http.StatusOK)
	secretBody := decode(t, resp)
	if secretBody["value"] != "secret-value" {
		t.Errorf("secret value = %v, want secret-value", secretBody["value"])
	}
}

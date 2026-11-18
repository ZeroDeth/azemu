package arm

import (
	"bytes"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"encoding/base64"
	"net/http"
	"strings"
	"testing"

	"software.sslmate.com/src/go-pkcs12"
)

const kvCertificateSelfBody = `{"policy":{
	"issuer":{"name":"Self"},
	"key_props":{"kty":"RSA","key_size":2048,"exportable":true,"reuse_key":true},
	"secret_props":{"contentType":"application/x-pkcs12"},
	"x509_props":{"subject":"CN=api.example.com","validity_months":12,
		"key_usage":["digitalSignature","keyEncipherment"],"ekus":["1.3.6.1.5.5.7.3.1"],
		"sans":{"dns_names":["api.example.com","portal.example.com"]}},
	"lifetime_actions":[{"action":{"action_type":"AutoRenew"},"trigger":{"lifetime_percentage":80}}]
},"tags":{"fixture":"109"}}`

func TestKeyVaultCertificate_LifecycleAndRealCrypto(t *testing.T) {
	srv := newTestServer(t)
	vault := keyVaultURL(srv.URL, "sub1", "rg1", "certvault")
	base := srv.URL + "/keyvault/certvault/certificates"
	assertStatus(t, httpPost(t, base+"/example/create", kvCertificateSelfBody), 404)
	resp := httpPut(t, vault, keyVaultBodyStandard)
	resp.Body.Close()
	resp = httpPost(t, base+"/example/create", kvCertificateSelfBody)
	assertStatus(t, resp, 202)
	if resp.Header.Get("x-ms-request-id") == "" || resp.Header.Get("x-ms-correlation-request-id") == "" {
		t.Fatal("missing Azure response headers")
	}
	resp.Body.Close()
	operation := decodeJSON(t, httpGet(t, base+"/example/pending"))
	if operation["status"] != "completed" {
		t.Fatal("certificate operation not complete")
	}
	bundle := decodeJSON(t, httpGet(t, base+"/example"))
	// Match the exact SDK's empty-version trailing slash, using a per-vault
	// Host header while still sending the HTTP request to newTestServer.
	for _, path := range []string{"/certificates/example/", "/certificates/example/pending", "/certificates/contacts"} {
		req, err := http.NewRequest(http.MethodGet, srv.URL+path+"?api-version=7.4", nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Host = "certvault.vault.localhost"
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		assertStatus(t, resp, 200)
		resp.Body.Close()
	}
	der, err := base64.StdEncoding.DecodeString(bundle["cer"].(string))
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	if err := cert.CheckSignature(cert.SignatureAlgorithm, cert.RawTBSCertificate, cert.Signature); err != nil {
		t.Fatal("self-signed certificate signature invalid:", err)
	}
	for _, host := range []string{"api.example.com", "portal.example.com"} {
		if err := cert.VerifyHostname(host); err != nil {
			t.Fatal(err)
		}
	}
	if cert.KeyUsage != x509.KeyUsageDigitalSignature|x509.KeyUsageKeyEncipherment || cert.Subject.CommonName != "api.example.com" ||
		len(cert.ExtKeyUsage) != 1 || cert.ExtKeyUsage[0] != x509.ExtKeyUsageServerAuth {
		t.Fatal("generated certificate did not implement policy")
	}
	hash := sha1.Sum(der)
	if bundle["x5t"] != base64.RawURLEncoding.EncodeToString(hash[:]) {
		t.Fatal("certificate thumbprint not derived from real DER")
	}
	secret := decodeJSON(t, httpGet(t, srv.URL+"/keyvault/certvault/secrets/example"))
	pfx, err := base64.StdEncoding.DecodeString(secret["value"].(string))
	if err != nil {
		t.Fatal(err)
	}
	private, exported, _, err := pkcs12.DecodeChain(pfx, "")
	if err != nil {
		t.Fatal(err)
	}
	key, ok := private.(*rsa.PrivateKey)
	if !ok || !key.PublicKey.Equal(cert.PublicKey) || !bytes.Equal(exported.Raw, der) {
		t.Fatal("certificate-backed secret does not contain matching private key/certificate")
	}
	if _, _, _, err := pkcs12.DecodeChain(pfx, "incorrect"); err == nil {
		t.Fatal("PKCS12 MAC accepted an incorrect password")
	}
	// A repeated create makes a new certificate version but reuses the actual
	// RSA key when reuse_key is true. Both prior versions remain readable.
	resp = httpPost(t, base+"/example/create", kvCertificateSelfBody)
	assertStatus(t, resp, 202)
	resp.Body.Close()
	newBundle := decodeJSON(t, httpGet(t, base+"/example"))
	if newBundle["id"] == bundle["id"] || newBundle["x5t"] == bundle["x5t"] {
		t.Fatal("new version reused fabricated certificate metadata")
	}
	newDER, _ := base64.StdEncoding.DecodeString(newBundle["cer"].(string))
	newCert, err := x509.ParseCertificate(newDER)
	if err != nil || !cert.PublicKey.(*rsa.PublicKey).Equal(newCert.PublicKey) {
		t.Fatal("reuse_key did not reuse the RSA key")
	}
	version := bundle["id"].(string)[strings.LastIndex(bundle["id"].(string), "/")+1:]
	resp = httpGet(t, base+"/example/"+version)
	assertStatus(t, resp, 200)
	resp.Body.Close()
	for _, tc := range []struct {
		path string
		size int
	}{{base, 1}, {base + "/example/versions", 2}} {
		items := decodeJSON(t, httpGet(t, tc.path))["value"].([]any)
		if len(items) != tc.size {
			t.Fatalf("list %s: got %d entries", tc.path, len(items))
		}
	}
	resp = httpHead(t, base+"/example")
	assertStatus(t, resp, 204)
	if readBody(t, resp) != "" {
		t.Fatal("HEAD returned a body")
	}
	resp = httpPatch(t, base+"/example", `{"tags":{"updated":"yes"}}`)
	assertStatus(t, resp, 200)
	if decodeJSON(t, resp)["tags"].(map[string]any)["updated"] != "yes" {
		t.Fatal("certificate update not returned")
	}
	resp = httpDelete(t, base+"/example")
	assertStatus(t, resp, 200) // SDK 7.4 certificate delete contract, not ARM 202.
	resp.Body.Close()
	for _, path := range []string{base + "/example", base + "/example/" + version,
		srv.URL + "/keyvault/certvault/secrets/example", srv.URL + "/keyvault/certvault/keys/example"} {
		resp = httpGet(t, path)
		assertStatus(t, resp, 404)
		resp.Body.Close()
	}
	resp = httpHead(t, base+"/example")
	assertStatus(t, resp, 404)
	resp.Body.Close()
	resp = httpDelete(t, srv.URL+"/keyvault/certvault/deletedcertificates/example")
	assertStatus(t, resp, 204)
	resp.Body.Close()
	// Vault deletion cascades certificate, key and secret data-plane entries.
	resp = httpPost(t, base+"/example/create", kvCertificateSelfBody)
	resp.Body.Close()
	resp = httpDelete(t, vault)
	resp.Body.Close()
	resp = httpGet(t, base+"/example")
	assertStatus(t, resp, 404)
	resp.Body.Close()
}

func TestKeyVaultCertificate_InvalidPolicies(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, keyVaultURL(srv.URL, "sub1", "rg1", "certvault"), keyVaultBodyStandard)
	resp.Body.Close()
	base := srv.URL + "/keyvault/certvault/certificates/example"
	for _, tc := range []struct {
		name, body string
		status     int
	}{
		{"invalid-json", "{", 400},
		{"missing-policy", "{}", 400},
		{"external-issuer", strings.Replace(kvCertificateSelfBody, `"Self"`, `"ExternalCA"`, 1), 501},
		{"unsupported-content-type", strings.Replace(kvCertificateSelfBody, "application/x-pkcs12", "application/x-pem-file", 1), 501},
		{"invalid-size", strings.Replace(kvCertificateSelfBody, "2048", "512", 1), 400},
		{"invalid-subject", strings.Replace(kvCertificateSelfBody, "CN=api.example.com", "invalid", 1), 400},
		{"invalid-usage", strings.Replace(kvCertificateSelfBody, "digitalSignature", "unknown", 1), 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp := httpPost(t, base+"/create", tc.body)
			assertStatus(t, resp, tc.status)
			if decodeJSON(t, resp)["error"] == nil {
				t.Fatal("missing error envelope")
			}
		})
	}
}

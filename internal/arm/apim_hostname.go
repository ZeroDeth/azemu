package arm

import (
	"bytes"
	"crypto"
	"crypto/sha1" // Azure's certificate thumbprint is SHA-1, not a signature.
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"software.sslmate.com/src/go-pkcs12"

	"github.com/zerodeth/azemu/internal/store"
)

// APIM user-assigned identities reference existing ARM identities. Never mint
// new client/principal IDs on an APIM read or echo caller-supplied identity IDs.
func (a *Router) apimIdentity(raw json.RawMessage, existing *store.Resource) (map[string]any, int, error) {
	if len(raw) == 0 || string(raw) == "null" {
		if existing != nil {
			identity, _ := existing.Properties["_identity"].(map[string]any)
			return identity, 0, nil
		}
		return nil, 0, nil
	}
	var body struct {
		Type string                     `json:"type"`
		IDs  map[string]json.RawMessage `json:"userAssignedIdentities"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, http.StatusBadRequest, fmt.Errorf("invalid identity: %w", err)
	}
	if body.Type == "None" && len(body.IDs) == 0 {
		return map[string]any{"type": "None"}, 0, nil
	}
	if body.Type == "SystemAssigned" || body.Type == "SystemAssigned, UserAssigned" {
		return nil, http.StatusNotImplemented, fmt.Errorf("APIM system-assigned identities are not implemented")
	}
	if body.Type != "UserAssigned" || len(body.IDs) == 0 {
		return nil, http.StatusBadRequest, fmt.Errorf("UserAssigned identity requires userAssignedIdentities")
	}
	ids := map[string]any{}
	for id := range body.IDs {
		res, ok := a.store.Get(id)
		if !ok {
			for _, candidate := range a.store.List("/subscriptions/") {
				if strings.EqualFold(candidate.ID, id) {
					res, ok = candidate, true
					break
				}
			}
		}
		if !ok || res.Type != userAssignedIdentityTypeString {
			return nil, http.StatusBadRequest, fmt.Errorf("user-assigned identity %q was not found", id)
		}
		ids[res.ID] = map[string]any{"clientId": res.Properties["clientId"], "principalId": res.Properties["principalId"]}
	}
	return map[string]any{"type": "UserAssigned", "userAssignedIdentities": ids}, 0, nil
}

func (a *Router) apimHostnames(raw any, identity map[string]any, serviceName string) ([]any, int, error) {
	configs, ok := raw.([]any)
	if raw != nil && !ok {
		return nil, http.StatusBadRequest, fmt.Errorf("hostnameConfigurations must be an array")
	}
	builtIn := strings.ToLower(serviceName) + ".azure-api.net"
	result := []any{}
	seen := map[string]bool{}
	var builtInConfig, defaultProxy map[string]any
	var customProxies []map[string]any
	for _, value := range configs {
		config, ok := value.(map[string]any)
		if !ok {
			return nil, http.StatusBadRequest, fmt.Errorf("hostname configuration must be an object")
		}
		host, _ := config["hostName"].(string)
		kind, _ := config["type"].(string)
		switch kind {
		case "Proxy", "DeveloperPortal", "Management", "Portal", "Scm":
		default:
			return nil, http.StatusBadRequest, fmt.Errorf("invalid hostname type")
		}
		if host == "" || seen[strings.ToLower(kind+"/"+host)] {
			return nil, http.StatusBadRequest, fmt.Errorf("hostname is missing or duplicated")
		}
		seen[strings.ToLower(kind+"/"+host)] = true
		out := map[string]any{}
		for key, val := range config {
			out[key] = val
		}
		for _, key := range []string{"negotiateClientCertificate", "defaultSslBinding"} {
			if v, exists := out[key]; exists && v != nil {
				if _, valid := v.(bool); !valid {
					return nil, http.StatusBadRequest, fmt.Errorf("%s must be a boolean", key)
				}
			}
		}
		for _, key := range []string{"keyVaultId", "identityClientId", "encodedCertificate", "certificatePassword"} {
			if value, present := config[key]; present && value != nil {
				if _, valid := value.(string); !valid {
					return nil, http.StatusBadRequest, fmt.Errorf("%s must be a string", key)
				}
			}
		}
		if encoded, _ := config["encodedCertificate"].(string); encoded != "" {
			return nil, http.StatusNotImplemented, fmt.Errorf("inline APIM certificates are not implemented; use a Key Vault certificate")
		}
		if strings.EqualFold(host, builtIn) && kind == "Proxy" {
			if ref, _ := config["keyVaultId"].(string); ref != "" {
				return nil, http.StatusBadRequest, fmt.Errorf("a Key Vault certificate cannot replace the built-in hostname certificate")
			}
			builtInConfig = out
			out["certificateSource"] = "BuiltIn"
		} else {
			cert, status, err := a.apimKeyVaultCertificate(config, identity)
			if err != nil {
				return nil, status, err
			}
			if err := cert.VerifyHostname(host); err != nil {
				return nil, http.StatusBadRequest, fmt.Errorf("certificate does not cover hostname %q", host)
			}
			sum := sha1.Sum(cert.Raw)
			out["certificateSource"] = "KeyVault"
			out["certificateStatus"] = "Completed"
			out["certificate"] = map[string]any{
				"expiry":  cert.NotAfter.UTC().Format(time.RFC3339),
				"subject": cert.Subject.String(), "thumbprint": strings.ToUpper(hex.EncodeToString(sum[:])),
			}
			if kind == "Proxy" {
				customProxies = append(customProxies, out)
			}
		}
		if kind == "Proxy" && out["defaultSslBinding"] == true {
			if defaultProxy != nil {
				return nil, http.StatusBadRequest, fmt.Errorf("only one gateway hostname can have defaultSslBinding set to true")
			}
			defaultProxy = out
		}
		if out["negotiateClientCertificate"] == nil {
			out["negotiateClientCertificate"] = false
		}
		result = append(result, out)
	}
	if builtInConfig == nil {
		builtInConfig = map[string]any{
			"type": "Proxy", "hostName": builtIn, "certificateSource": "BuiltIn",
			"defaultSslBinding": false, "negotiateClientCertificate": false,
		}
		result = append(result, builtInConfig)
	}
	if defaultProxy == nil {
		// With multiple custom gateways Azure falls back to *.azure-api.net
		// unless a default is selected. A lone custom gateway is the default
		// when its binding flag is omitted.
		defaultProxy = builtInConfig
		if len(customProxies) == 1 && customProxies[0]["defaultSslBinding"] == nil {
			defaultProxy = customProxies[0]
		}
	}
	for _, raw := range result {
		config := raw.(map[string]any)
		if config["type"] == "Proxy" && config["defaultSslBinding"] == nil {
			config["defaultSslBinding"] = false
		}
	}
	defaultProxy["defaultSslBinding"] = true
	return result, 0, nil
}

func (a *Router) apimKeyVaultCertificate(config, identity map[string]any) (*x509.Certificate, int, error) {
	ref, _ := config["keyVaultId"].(string)
	u, err := url.Parse(ref)
	if err != nil || u.Host == "" || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		return nil, http.StatusBadRequest, fmt.Errorf("keyVaultId must be a Key Vault certificate secret URL")
	}
	vaultName, ok := vaultNameFromHost(u.Host)
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if !ok || len(parts) < 2 || len(parts) > 3 || parts[0] != "secrets" || parts[1] == "" ||
		!strings.EqualFold(u.Scheme+"://"+u.Host, a.vaultBaseURL(vaultName)) {
		return nil, http.StatusBadRequest, fmt.Errorf("keyVaultId must reference a local Key Vault secret")
	}
	var vault *store.Resource
	for _, res := range a.store.List("/subscriptions/") {
		if res.Type == keyVaultTypeString && strings.EqualFold(res.Name, vaultName) {
			vault = res
		}
	}
	if vault == nil {
		return nil, http.StatusBadRequest, fmt.Errorf("certificate vault was not found")
	}
	clientID, _ := config["identityClientId"].(string)
	principalID := ""
	ids, _ := identity["userAssignedIdentities"].(map[string]any)
	for _, raw := range ids {
		details, _ := raw.(map[string]any)
		if clientID != "" && details["clientId"] == clientID {
			principalID, _ = details["principalId"].(string)
		}
	}
	if principalID == "" {
		return nil, http.StatusBadRequest, fmt.Errorf("identityClientId must identify an identity assigned to this APIM service")
	}
	if !keyVaultPolicyAllows(vault, principalID, "secrets", "get") {
		return nil, http.StatusForbidden, fmt.Errorf("APIM identity does not have Key Vault secrets Get permission")
	}
	version := ""
	if len(parts) == 3 {
		version = parts[2]
	}
	if version == "" {
		current, found := a.store.Get(secretCurrentKey(vaultName, parts[1]))
		if !found {
			return nil, http.StatusBadRequest, fmt.Errorf("certificate secret was not found")
		}
		version, _ = current.Properties["version"].(string)
	}
	secret, found := a.store.Get(secretStoreKey(vaultName, parts[1], version))
	certificate, certFound := a.store.Get(certificateStoreKey(vaultName, parts[1], version))
	if !found || !certFound || secret.Properties["contentType"] != "application/x-pkcs12" {
		return nil, http.StatusBadRequest, fmt.Errorf("keyVaultId must reference a PKCS12 certificate-backed secret")
	}
	attrs, _ := secret.Properties["attributes"].(map[string]any)
	if attrs["enabled"] == false {
		return nil, http.StatusBadRequest, fmt.Errorf("certificate secret is disabled")
	}
	value, _ := secret.Properties["value"].(string)
	blob, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return nil, http.StatusBadRequest, fmt.Errorf("invalid certificate secret encoding")
	}
	private, cert, _, err := pkcs12.DecodeChain(blob, "")
	if err != nil || cert == nil {
		return nil, http.StatusBadRequest, fmt.Errorf("certificate secret is not valid PKCS12")
	}
	key, ok := private.(crypto.Signer)
	if !ok {
		return nil, http.StatusBadRequest, fmt.Errorf("certificate secret has no private key")
	}
	publicDER, err := x509.MarshalPKIXPublicKey(key.Public())
	if err != nil {
		return nil, http.StatusBadRequest, fmt.Errorf("certificate secret has an invalid private key")
	}
	certPublicDER, err := x509.MarshalPKIXPublicKey(cert.PublicKey)
	if err != nil || !bytes.Equal(publicDER, certPublicDER) {
		return nil, http.StatusBadRequest, fmt.Errorf("certificate secret key does not match the certificate")
	}
	if certificate.Properties["cer"] != base64.StdEncoding.EncodeToString(cert.Raw) ||
		time.Now().Before(cert.NotBefore) || time.Now().After(cert.NotAfter) {
		return nil, http.StatusBadRequest, fmt.Errorf("certificate is invalid or outside its validity period")
	}
	return cert, 0, nil
}

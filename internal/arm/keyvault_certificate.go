package arm

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1" // Key Vault's x5t is the certificate's SHA-1 thumbprint.
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"software.sslmate.com/src/go-pkcs12"

	"github.com/zerodeth/azemu/internal/store"
)

func certificateStoreKey(vault, name, version string) string {
	return fmt.Sprintf("/keyvault/%s/certificates/%s/%s", vault, name, version)
}

func (a *Router) keyVaultCertificateRoutes(r chi.Router, prefix string) {
	wrap := func(h http.HandlerFunc) http.HandlerFunc {
		if prefix == "" {
			return a.withVaultFor("certificateName", "keyvault/certificate/current", "CertificateNotFound", h)
		}
		return h
	}
	r.Route(prefix+"/certificates", func(r chi.Router) {
		// The autorest SDK sends /certificates/{name}/ for an empty
		// version, just as it does for keys and secrets.
		r.Use(chimw.StripSlashes)
		r.Get("/contacts", a.getKeyVaultCertificateContacts)
		r.Post("/{certificateName}/create", wrap(a.createKeyVaultCertificate))
		r.Get("/{certificateName}/pending", wrap(a.getKeyVaultCertificateOperation))
		r.Get("/{certificateName}/versions", wrap(a.listKeyVaultCertificates))
		r.Get("/{certificateName}/{version}", wrap(a.getKeyVaultCertificate))
		r.Head("/{certificateName}/{version}", wrap(a.getKeyVaultCertificate))
		r.Patch("/{certificateName}/{version}", wrap(a.patchKeyVaultCertificate))
		r.Get("/{certificateName}", wrap(a.getKeyVaultCertificate))
		r.Head("/{certificateName}", wrap(a.getKeyVaultCertificate))
		r.Patch("/{certificateName}", wrap(a.patchKeyVaultCertificate))
		r.Delete("/{certificateName}", wrap(a.deleteKeyVaultCertificate))
		r.Get("/", wrap(a.listKeyVaultCertificates))
	})
	r.Get(prefix+"/deletedcertificates/{certificateName}", a.getDeletedKeyVaultItem)
	r.Delete(prefix+"/deletedcertificates/{certificateName}", a.purgeDeletedKeyVaultItem)
}

func (a *Router) resolveCertificate(vault, name, version string) (*store.Resource, bool) {
	if version == "" {
		current, ok := a.store.Get(certificateStoreKey(vault, name, "current"))
		if !ok {
			return nil, false
		}
		version, _ = current.Properties["version"].(string)
	}
	return a.store.Get(certificateStoreKey(vault, name, version))
}

type kvCertificatePolicy struct {
	Issuer struct {
		Name string `json:"name"`
	} `json:"issuer"`
	Key struct {
		Type       string `json:"kty"`
		Size       int    `json:"key_size"`
		Exportable bool   `json:"exportable"`
		Reuse      bool   `json:"reuse_key"`
	} `json:"key_props"`
	Secret struct {
		ContentType string `json:"contentType"`
	} `json:"secret_props"`
	X509 struct {
		Subject string   `json:"subject"`
		Months  int      `json:"validity_months"`
		Usage   []string `json:"key_usage"`
		EKUs    []string `json:"ekus"`
		SANs    struct {
			DNS    []string `json:"dns_names"`
			Emails []string `json:"emails"`
			UPNs   []string `json:"upns"`
		} `json:"sans"`
	} `json:"x509_props"`
}

func certificateTemplate(policy kvCertificatePolicy) (*x509.Certificate, int, error) {
	if policy.Issuer.Name != "Self" || policy.Key.Type != "RSA" || !policy.Key.Exportable ||
		policy.Secret.ContentType != "application/x-pkcs12" || len(policy.X509.SANs.UPNs) != 0 {
		return nil, http.StatusNotImplemented, fmt.Errorf("only Self-issued exportable RSA PKCS12 certificates with DNS/email SANs are implemented")
	}
	if policy.Key.Size != 2048 && policy.Key.Size != 3072 && policy.Key.Size != 4096 {
		return nil, http.StatusBadRequest, fmt.Errorf("RSA key_size must be 2048, 3072 or 4096")
	}
	if policy.X509.Months < 1 || policy.X509.Months > 120 || policy.X509.Subject == "" {
		return nil, http.StatusBadRequest, fmt.Errorf("subject and validity_months (1-120) are required")
	}
	subject := pkix.Name{}
	for _, field := range strings.Split(policy.X509.Subject, ",") {
		parts := strings.SplitN(strings.TrimSpace(field), "=", 2)
		if len(parts) != 2 || parts[1] == "" || strings.ContainsAny(parts[1], "\\+") {
			return nil, http.StatusBadRequest, fmt.Errorf("unsupported certificate subject syntax")
		}
		switch strings.ToUpper(parts[0]) {
		case "CN":
			subject.CommonName = parts[1]
		case "O":
			subject.Organization = append(subject.Organization, parts[1])
		case "OU":
			subject.OrganizationalUnit = append(subject.OrganizationalUnit, parts[1])
		case "C":
			subject.Country = append(subject.Country, parts[1])
		case "ST":
			subject.Province = append(subject.Province, parts[1])
		case "L":
			subject.Locality = append(subject.Locality, parts[1])
		default:
			return nil, http.StatusNotImplemented, fmt.Errorf("unsupported certificate subject attribute")
		}
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, http.StatusInternalServerError, fmt.Errorf("generate certificate serial: %w", err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	template := &x509.Certificate{
		SerialNumber: serial, Subject: subject, NotBefore: now.Add(-time.Minute),
		NotAfter: now.AddDate(0, policy.X509.Months, 0), DNSNames: policy.X509.SANs.DNS,
		EmailAddresses: policy.X509.SANs.Emails, BasicConstraintsValid: true,
	}
	usages := map[string]x509.KeyUsage{
		"digitalSignature": x509.KeyUsageDigitalSignature, "nonRepudiation": x509.KeyUsageContentCommitment,
		"keyEncipherment": x509.KeyUsageKeyEncipherment, "dataEncipherment": x509.KeyUsageDataEncipherment,
		"keyAgreement": x509.KeyUsageKeyAgreement, "keyCertSign": x509.KeyUsageCertSign,
		"cRLSign": x509.KeyUsageCRLSign, "encipherOnly": x509.KeyUsageEncipherOnly, "decipherOnly": x509.KeyUsageDecipherOnly,
	}
	for _, usage := range policy.X509.Usage {
		flag, ok := usages[usage]
		if !ok {
			return nil, http.StatusBadRequest, fmt.Errorf("invalid certificate key usage")
		}
		template.KeyUsage |= flag
	}
	for _, value := range policy.X509.EKUs {
		oid := asn1.ObjectIdentifier{}
		for _, part := range strings.Split(value, ".") {
			n, err := strconv.Atoi(part)
			if err != nil || n < 0 {
				return nil, http.StatusBadRequest, fmt.Errorf("invalid extended key usage OID")
			}
			oid = append(oid, n)
		}
		if len(oid) < 2 || oid[0] > 2 || (oid[0] < 2 && oid[1] > 39) {
			return nil, http.StatusBadRequest, fmt.Errorf("invalid extended key usage OID")
		}
		template.UnknownExtKeyUsage = append(template.UnknownExtKeyUsage, oid)
	}
	return template, 0, nil
}

func (a *Router) createKeyVaultCertificate(w http.ResponseWriter, r *http.Request) {
	vault, name := chi.URLParam(r, "vaultName"), chi.URLParam(r, "certificateName")
	if !a.vaultExists(vault) {
		writeAzureError(w, http.StatusNotFound, "ParentResourceNotFound", "Key Vault was not found.")
		return
	}
	var body struct {
		Policy map[string]any    `json:"policy"`
		Tags   map[string]string `json:"tags"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Policy == nil {
		writeAzureError(w, http.StatusBadRequest, "BadParameter", "A valid certificate policy is required.")
		return
	}
	raw, err := json.Marshal(body.Policy)
	if err != nil {
		writeAzureError(w, http.StatusBadRequest, "BadParameter", "Invalid certificate policy.")
		return
	}
	var policy kvCertificatePolicy
	if err := json.Unmarshal(raw, &policy); err != nil {
		writeAzureError(w, http.StatusBadRequest, "BadParameter", "Invalid certificate policy.")
		return
	}
	template, status, err := certificateTemplate(policy)
	if err != nil {
		writeAzureError(w, status, "BadParameter", err.Error())
		return
	}
	var key *rsa.PrivateKey
	if policy.Key.Reuse {
		if saved, ok := a.resolveKeyVersion(vault, name, ""); ok {
			encoded, _ := saved.Properties["privateKeyPkcs8"].(string)
			key, err = decodePrivateKeyPKCS8(encoded)
			if err != nil || key.N.BitLen() != policy.Key.Size {
				writeAzureError(w, http.StatusBadRequest, "BadParameter", "Existing key cannot be reused with this certificate policy.")
				return
			}
		}
	}
	if key == nil {
		key, err = rsa.GenerateKey(rand.Reader, policy.Key.Size)
		if err != nil {
			writeAzureError(w, http.StatusInternalServerError, "InternalServerError", "Failed to generate certificate key.")
			return
		}
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		writeAzureError(w, http.StatusInternalServerError, "InternalServerError", "Failed to create certificate.")
		return
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		writeAzureError(w, http.StatusInternalServerError, "InternalServerError", "Failed to parse generated certificate.")
		return
	}
	// Azure certificate-backed secrets are exported without a password. Modern
	// PBES2/AES-256 and SHA-256 integrity are still used; TLS/store isolation,
	// not an embedded password, protects local emulator key material.
	pfx, err := pkcs12.Modern2023.Encode(key, cert, nil, "")
	if err != nil {
		writeAzureError(w, http.StatusInternalServerError, "InternalServerError", "Failed to encode certificate.")
		return
	}
	encodedKey, err := encodePrivateKeyPKCS8(key)
	if err != nil {
		writeAzureError(w, http.StatusInternalServerError, "InternalServerError", "Failed to encode certificate key.")
		return
	}
	version := strings.ReplaceAll(uuid.NewString(), "-", "")
	id := a.vaultBaseURL(vault) + "/certificates/" + name + "/" + version
	now := time.Now().Unix()
	attrs := map[string]any{"enabled": true, "created": now, "updated": now,
		"nbf": cert.NotBefore.Unix(), "exp": cert.NotAfter.Unix(), "recoveryLevel": "Purgeable"}
	hash := sha1.Sum(der)
	props := map[string]any{
		"id": id, "sid": a.kvSecretID(vault, name, version),
		"kid": a.kvKeyID(vault, name, version), "cer": base64.StdEncoding.EncodeToString(der),
		"x5t": base64.RawURLEncoding.EncodeToString(hash[:]), "attributes": attrs,
		"policy": body.Policy, "tags": normaliseTags(body.Tags),
	}
	keyProps := map[string]any{"key": publicJWK(a.kvKeyID(vault, name, version), &key.PublicKey, []string{"sign", "verify"}),
		"attributes": attrs, "tags": normaliseTags(body.Tags), "privateKeyPkcs8": encodedKey}
	if err := a.storeKeyVersion(vault, name, version, keyProps); err != nil {
		writeAzureError(w, http.StatusInternalServerError, "InternalServerError", "Failed to store certificate key.")
		return
	}
	for _, entry := range []*store.Resource{
		{ID: secretStoreKey(vault, name, version), Name: name, Type: "keyvault/secret", Properties: map[string]any{
			"id": props["sid"], "value": base64.StdEncoding.EncodeToString(pfx), "contentType": "application/x-pkcs12",
			"attributes": attrs, "tags": normaliseTags(body.Tags), "version": version}},
		{ID: secretCurrentKey(vault, name), Name: name, Type: "keyvault/secret/current", Properties: map[string]any{"version": version}},
		{ID: certificateStoreKey(vault, name, version), Name: name, Type: "keyvault/certificate", Properties: props},
		{ID: certificateStoreKey(vault, name, "current"), Name: name, Type: "keyvault/certificate/current", Properties: map[string]any{"version": version}},
	} {
		if err := a.store.Put(entry.ID, entry); err != nil {
			writeAzureError(w, http.StatusInternalServerError, "InternalServerError", "Failed to store certificate.")
			return
		}
	}
	log.Info().Str("vault", vault).Str("certificate", name).Str("version", version).Msg("key vault self-signed certificate created")
	writeJSON(w, http.StatusAccepted, map[string]any{"id": a.vaultBaseURL(vault) + "/certificates/" + name + "/pending",
		"status": "completed", "issuer": body.Policy["issuer"], "target": id})
}

func (a *Router) getKeyVaultCertificate(w http.ResponseWriter, r *http.Request) {
	res, ok := a.resolveCertificate(chi.URLParam(r, "vaultName"), chi.URLParam(r, "certificateName"), chi.URLParam(r, "version"))
	if !ok {
		writeAzureError(w, http.StatusNotFound, "CertificateNotFound", "Certificate was not found.")
		return
	}
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	writeJSON(w, http.StatusOK, res.Properties)
}

func (a *Router) getKeyVaultCertificateOperation(w http.ResponseWriter, r *http.Request) {
	res, ok := a.resolveCertificate(chi.URLParam(r, "vaultName"), chi.URLParam(r, "certificateName"), "")
	if !ok {
		writeAzureError(w, http.StatusNotFound, "CertificateNotFound", "Certificate operation was not found.")
		return
	}
	policy, _ := res.Properties["policy"].(map[string]any)
	writeJSON(w, http.StatusOK, map[string]any{"status": "completed", "target": res.Properties["id"], "issuer": policy["issuer"]})
}

func (a *Router) listKeyVaultCertificates(w http.ResponseWriter, r *http.Request) {
	vault, name := chi.URLParam(r, "vaultName"), chi.URLParam(r, "certificateName")
	if !a.vaultExists(vault) {
		writeAzureError(w, http.StatusNotFound, "ParentResourceNotFound", "Key Vault was not found.")
		return
	}
	items := []any{}
	for _, res := range a.store.List("/keyvault/" + vault + "/certificates/") {
		if name == "" && res.Type == "keyvault/certificate/current" {
			res, _ = a.resolveCertificate(vault, res.Name, "")
		} else if name == "" || res.Name != name || res.Type != "keyvault/certificate" {
			continue
		}
		if res != nil {
			items = append(items, map[string]any{"id": res.Properties["id"], "x5t": res.Properties["x5t"],
				"attributes": res.Properties["attributes"], "tags": res.Properties["tags"]})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"value": items})
}

func (a *Router) patchKeyVaultCertificate(w http.ResponseWriter, r *http.Request) {
	res, ok := a.resolveCertificate(chi.URLParam(r, "vaultName"), chi.URLParam(r, "certificateName"), chi.URLParam(r, "version"))
	if !ok {
		writeAzureError(w, http.StatusNotFound, "CertificateNotFound", "Certificate was not found.")
		return
	}
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body == nil {
		writeAzureError(w, http.StatusBadRequest, "BadParameter", "Invalid certificate update.")
		return
	}
	props := map[string]any{}
	for key, val := range res.Properties {
		props[key] = val
	}
	for key, val := range body {
		if key != "tags" {
			writeAzureError(w, http.StatusNotImplemented, "NotImplemented", "Only certificate tag updates are implemented; create a new version for policy changes.")
			return
		}
		tags, ok := val.(map[string]any)
		if !ok {
			writeAzureError(w, http.StatusBadRequest, "BadParameter", "tags must be an object.")
			return
		}
		for _, value := range tags {
			if _, valid := value.(string); !valid {
				writeAzureError(w, http.StatusBadRequest, "BadParameter", "tags must contain strings.")
				return
			}
		}
		props[key] = val
	}
	res.Properties = props
	if err := a.store.Put(res.ID, res); err != nil {
		writeAzureError(w, http.StatusInternalServerError, "InternalServerError", "Failed to update certificate.")
		return
	}
	writeJSON(w, http.StatusOK, props)
}

func (a *Router) deleteKeyVaultCertificate(w http.ResponseWriter, r *http.Request) {
	vault, name := chi.URLParam(r, "vaultName"), chi.URLParam(r, "certificateName")
	res, ok := a.resolveCertificate(vault, name, "")
	if !ok {
		writeAzureError(w, http.StatusNotFound, "CertificateNotFound", "Certificate was not found.")
		return
	}
	for _, category := range []string{"certificates", "secrets", "keys"} {
		for _, item := range a.store.List("/keyvault/" + vault + "/" + category + "/" + name + "/") {
			a.store.Delete(item.ID)
		}
	}
	result := map[string]any{}
	for key, value := range res.Properties {
		result[key] = value
	}
	result["recoveryId"] = a.vaultBaseURL(vault) + "/deletedcertificates/" + name
	result["deletedDate"] = time.Now().Unix()
	result["scheduledPurgeDate"] = time.Now().Unix()
	log.Info().Str("vault", vault).Str("certificate", name).Msg("key vault certificate deleted")
	// Key Vault data-plane 7.4 DeleteCertificate requires 200 + bundle, not
	// ARM's 202 operation-status contract. No deleted material is retained.
	writeJSON(w, http.StatusOK, result)
}

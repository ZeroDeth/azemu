package auth

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

// generateSelfSignedPEM produces a self-signed cert and key as PEM blocks.
// It is the lower-level helper used by both GenerateSelfSignedTLS and
// LoadOrGenerateSelfSignedTLS so the persistence path can write the same
// PEM bytes that were used to construct the in-memory certificate.
func generateSelfSignedPEM(hosts ...string) (certPEM, keyPEM []byte, err error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}

	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{Organization: []string{"azemu"}, CommonName: "azemu localhost"},
		NotBefore:    time.Now().Add(-1 * time.Hour),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	for _, h := range hosts {
		if ip := net.ParseIP(h); ip != nil {
			tmpl.IPAddresses = append(tmpl.IPAddresses, ip)
		} else {
			tmpl.DNSNames = append(tmpl.DNSNames, h)
		}
	}

	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, nil, err
	}

	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyDER, _ := x509.MarshalECPrivateKey(key)
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	return certPEM, keyPEM, nil
}

// GenerateSelfSignedTLS creates an in-memory TLS certificate for localhost.
// It is preserved for callers that do not need persistence.
func GenerateSelfSignedTLS(hosts ...string) (tls.Certificate, error) {
	certPEM, keyPEM, err := generateSelfSignedPEM(hosts...)
	if err != nil {
		return tls.Certificate{}, err
	}
	return tls.X509KeyPair(certPEM, keyPEM)
}

// LoadOrGenerateSelfSignedTLS returns a tls.Certificate, loading it from a
// PEM bundle file at path if one already exists and is still valid, or
// generating a fresh pair and persisting it to path otherwise. When path
// is empty, this is equivalent to GenerateSelfSignedTLS with no persistence.
//
// The bundle file contains both the CERTIFICATE and EC PRIVATE KEY PEM
// blocks concatenated, written with mode 0600 because it includes a
// private key. An existing file is regenerated when:
//
//   - the file is unreadable (e.g., permissions changed),
//   - the PEM blocks are missing or malformed,
//   - the certificate is outside its NotBefore/NotAfter validity window.
//
// The boolean return value indicates whether a fresh pair was generated
// (true) or an existing file was loaded (false). Callers can use this to
// log the difference between "first start" and "warm start".
//
// This function exists to eliminate the per-restart TouchID friction in
// Phase 1 debugging: trust the cert in the system keychain once, set
// AZEMU_CERT_PATH, and every subsequent restart reuses the same cert.
func LoadOrGenerateSelfSignedTLS(path string, hosts ...string) (tls.Certificate, bool, error) {
	if path != "" {
		if cert, ok := tryLoadBundle(path); ok {
			return cert, false, nil
		}
	}
	certPEM, keyPEM, err := generateSelfSignedPEM(hosts...)
	if err != nil {
		return tls.Certificate{}, false, err
	}
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return tls.Certificate{}, false, err
	}
	if path != "" {
		// Write cert+key concatenated. Mode 0600 because the file contains
		// a private key — must not be world-readable.
		bundle := append(certPEM, keyPEM...)
		if err := os.WriteFile(path, bundle, 0600); err != nil {
			// Generation succeeded but persistence failed — return the cert
			// anyway so the server still starts; the caller can log the
			// path failure as a warning.
			return cert, true, err
		}
	}
	return cert, true, nil
}

// tryLoadBundle reads a PEM cert+key bundle from path and returns a usable
// tls.Certificate plus true on success. Returns false on any failure
// (missing file, parse error, expired cert, etc.) so the caller can
// transparently fall back to generating a fresh pair.
func tryLoadBundle(path string) (tls.Certificate, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return tls.Certificate{}, false
	}
	certBlock, rest := pem.Decode(data)
	if certBlock == nil || certBlock.Type != "CERTIFICATE" {
		return tls.Certificate{}, false
	}
	keyBlock, _ := pem.Decode(rest)
	if keyBlock == nil {
		return tls.Certificate{}, false
	}
	cert, err := tls.X509KeyPair(
		pem.EncodeToMemory(certBlock),
		pem.EncodeToMemory(keyBlock),
	)
	if err != nil {
		return tls.Certificate{}, false
	}
	parsed, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		return tls.Certificate{}, false
	}
	now := time.Now()
	if now.Before(parsed.NotBefore) || now.After(parsed.NotAfter) {
		return tls.Certificate{}, false
	}
	return cert, true
}

// WriteCertToTemp writes the certificate (without the private key) from a
// tls.Certificate to a temp file so users can set SSL_CERT_FILE to trust
// it. Used by the legacy code path when AZEMU_CERT_PATH is not set; new
// callers should prefer LoadOrGenerateSelfSignedTLS.
func WriteCertToTemp(cert tls.Certificate) (string, error) {
	if len(cert.Certificate) == 0 {
		return "", nil
	}
	dir := os.TempDir()
	path := filepath.Join(dir, "azemu-cert.pem")
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Certificate[0]})
	return path, os.WriteFile(path, certPEM, 0644)
}

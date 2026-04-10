package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
)

type TokenService struct {
	tenantID   string
	signingKey *rsa.PrivateKey
	keyID      string
}

func NewTokenService(tenantID string) *TokenService {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	return &TokenService{
		tenantID:   tenantID,
		signingKey: key,
		keyID:      "azemu-signing-key-1",
	}
}

func (t *TokenService) Routes(r chi.Router) {
	r.Post("/token", t.token)
}

func (t *TokenService) RoutesV2(r chi.Router) {
	r.Post("/token", t.token)
}

// token mints a mock access token. Accepts any credentials.
func (t *TokenService) token(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	claims := jwt.MapClaims{
		"aud":   "https://management.azure.com/",
		"iss":   "https://sts.windows.net/" + t.tenantID + "/",
		"iat":   now.Unix(),
		"nbf":   now.Unix(),
		"exp":   now.Add(1 * time.Hour).Unix(),
		"tid":   t.tenantID,
		"oid":   "00000000-0000-0000-0000-000000000002",
		"appid": r.FormValue("client_id"),
		"sub":   "azemu-mock-subject",
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = t.keyID
	signed, err := token.SignedString(t.signingKey)
	if err != nil {
		http.Error(w, `{"error":"token_signing_failed"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"access_token": signed,
		"token_type":   "Bearer",
		"expires_in":   3600,
		"resource":     "https://management.azure.com/",
	})
}

// OpenIDConfig returns OIDC discovery document.
func (t *TokenService) OpenIDConfig(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "tenantID")
	if tenantID == "" {
		tenantID = t.tenantID
	}
	host := r.Host
	base := "https://" + host

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"issuer":                 base + "/" + tenantID + "/",
		"authorization_endpoint": base + "/" + tenantID + "/oauth2/v2.0/authorize",
		"token_endpoint":         base + "/" + tenantID + "/oauth2/v2.0/token",
		"jwks_uri":               base + "/" + tenantID + "/discovery/v2.0/keys",
		"response_types_supported": []string{"code", "id_token", "token"},
		"subject_types_supported":  []string{"pairwise"},
		"id_token_signing_alg_values_supported": []string{"RS256"},
	})
}

// JWKS returns the public key set for token verification.
func (t *TokenService) JWKS(w http.ResponseWriter, r *http.Request) {
	pub := t.signingKey.PublicKey
	n := pub.N.Bytes()
	e := big.NewInt(int64(pub.E)).Bytes()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"keys": []map[string]interface{}{
			{
				"kty": "RSA",
				"use": "sig",
				"kid": t.keyID,
				"n":   base64URLEncode(n),
				"e":   base64URLEncode(e),
			},
		},
	})
}

func base64URLEncode(b []byte) string {
	// Standard base64url without padding
	const enc = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"
	result := make([]byte, 0, (len(b)*4+2)/3)
	for i := 0; i < len(b); i += 3 {
		val := uint(b[i]) << 16
		if i+1 < len(b) {
			val |= uint(b[i+1]) << 8
		}
		if i+2 < len(b) {
			val |= uint(b[i+2])
		}
		result = append(result, enc[(val>>18)&0x3F])
		result = append(result, enc[(val>>12)&0x3F])
		if i+1 < len(b) {
			result = append(result, enc[(val>>6)&0x3F])
		}
		if i+2 < len(b) {
			result = append(result, enc[val&0x3F])
		}
	}
	return string(result)
}

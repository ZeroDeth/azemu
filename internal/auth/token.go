package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"github.com/rs/zerolog/log"
)

type TokenService struct {
	tenantID   string
	signingKey *rsa.PrivateKey
	keyID      string
}

func NewTokenService(tenantID string) (*TokenService, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("generate RSA signing key: %w", err)
	}
	return &TokenService{
		tenantID:   tenantID,
		signingKey: key,
		keyID:      "azemu-signing-key-1",
	}, nil
}

// TenantRoutes mounts the full auth surface for a {tenantID} path group:
// OAuth2 token endpoints, OIDC discovery, and JWKS.
func (t *TokenService) TenantRoutes(r chi.Router) {
	r.Route("/oauth2", t.Routes)
	r.Route("/oauth2/v2.0", t.RoutesV2)
	r.Get("/.well-known/openid-configuration", t.OpenIDConfig)
	r.Get("/discovery/v2.0/keys", t.JWKS)
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
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"access_token": signed,
		"token_type":   "Bearer",
		"expires_in":   3600,
		"resource":     "https://management.azure.com/",
	}); err != nil {
		log.Error().Err(err).Msg("failed to write token response")
	}
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
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"issuer":                                base + "/" + tenantID + "/",
		"authorization_endpoint":                base + "/" + tenantID + "/oauth2/v2.0/authorize",
		"token_endpoint":                        base + "/" + tenantID + "/oauth2/v2.0/token",
		"jwks_uri":                              base + "/" + tenantID + "/discovery/v2.0/keys",
		"response_types_supported":              []string{"code", "id_token", "token"},
		"subject_types_supported":               []string{"pairwise"},
		"id_token_signing_alg_values_supported": []string{"RS256"},
	}); err != nil {
		log.Error().Err(err).Msg("failed to write OIDC config response")
	}
}

// JWKS returns the public key set for token verification.
func (t *TokenService) JWKS(w http.ResponseWriter, r *http.Request) {
	pub := t.signingKey.PublicKey
	n := pub.N.Bytes()
	e := big.NewInt(int64(pub.E)).Bytes()

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"keys": []map[string]interface{}{
			{
				"kty": "RSA",
				"use": "sig",
				"kid": t.keyID,
				"n":   base64.RawURLEncoding.EncodeToString(n),
				"e":   base64.RawURLEncoding.EncodeToString(e),
			},
		},
	}); err != nil {
		log.Error().Err(err).Msg("failed to write JWKS response")
	}
}

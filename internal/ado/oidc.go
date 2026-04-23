// Package ado emulates the Azure DevOps OIDC token endpoint and service
// endpoint (service connection) REST API. These surfaces are called by the
// hashicorp/azuredevops Terraform provider when configuring workload identity
// federation and service connections against azemu.
package ado

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

// OIDCService emulates the Azure DevOps OIDC token endpoint. During an ADO
// pipeline run the agent sets SYSTEM_OIDCREQUESTURI to a URL that returns a
// short-lived JWT; the azurerm provider exchanges that JWT for an Azure access
// token via the standard OAuth2 client-credentials flow.
type OIDCService struct {
	mu         sync.RWMutex
	signingKey *rsa.PrivateKey
	keyID      string
}

// NewOIDCService generates an in-process RSA-2048 signing key and returns a
// ready-to-mount OIDCService.
func NewOIDCService() (*OIDCService, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("generate ADO OIDC signing key: %w", err)
	}
	return &OIDCService{
		signingKey: key,
		keyID:      "azemu-ado-oidc-1",
	}, nil
}

// Routes mounts the ADO OIDC surface on the provided chi.Router.
func (s *OIDCService) Routes(r chi.Router) {
	r.Get("/{organization}/{project}/_apis/distributedtask/hubs/{hub}/plans/{planID}/jobs/{jobID}/oidctoken",
		s.oidcToken)
	r.Get("/.well-known/openid-configuration", s.openIDConfig)
	r.Get("/discovery/keys", s.jwks)
}

// oidcToken mints a mock OIDC token for the given job context.
func (s *OIDCService) oidcToken(w http.ResponseWriter, r *http.Request) {
	org := chi.URLParam(r, "organization")
	project := chi.URLParam(r, "project")
	planID := chi.URLParam(r, "planID")
	jobID := chi.URLParam(r, "jobID")

	now := time.Now()
	host := r.Host
	issuer := "http://" + host + "/ado"

	claims := map[string]interface{}{
		"iss":    issuer,
		"sub":    fmt.Sprintf("sc://%s/%s/azemu-service-connection", org, project),
		"aud":    "api://AzureADTokenExchange",
		"iat":    now.Unix(),
		"nbf":    now.Unix(),
		"exp":    now.Add(10 * time.Minute).Unix(),
		"jti":    uuid.New().String(),
		"planid": planID,
		"jobid":  jobID,
		"org":    org,
		"prj":    project,
	}

	s.mu.RLock()
	key := s.signingKey
	kid := s.keyID
	s.mu.RUnlock()

	signed, err := signRS256(key, kid, claims)
	if err != nil {
		log.Error().Err(err).Msg("ADO OIDC: sign token")
		http.Error(w, `{"error":"token_signing_failed"}`, http.StatusInternalServerError)
		return
	}

	log.Info().Str("org", org).Str("project", project).Str("job_id", jobID).Msg("ADO OIDC token issued")

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{"oidcToken": signed}); err != nil {
		log.Error().Err(err).Msg("ADO OIDC: write response")
	}
}

// openIDConfig returns the OIDC discovery document.
// The ADO server runs on plain HTTP (port 4569), so jwks_uri must use http://
// to avoid TLS errors when OIDC clients fetch the key set.
func (s *OIDCService) openIDConfig(w http.ResponseWriter, r *http.Request) {
	host := r.Host
	base := "http://" + host + "/ado"

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"issuer":                                base,
		"jwks_uri":                              base + "/discovery/keys",
		"response_types_supported":              []string{"id_token"},
		"subject_types_supported":               []string{"public"},
		"id_token_signing_alg_values_supported": []string{"RS256"},
		"claims_supported": []string{
			"iss", "sub", "aud", "iat", "nbf", "exp", "jti",
			"org", "prj", "planid", "jobid",
		},
	}); err != nil {
		log.Error().Err(err).Msg("ADO OIDC: write discovery document")
	}
}

// signRS256 mints an RS256 JWT with the given claims and key.
func signRS256(key *rsa.PrivateKey, kid string, claims map[string]interface{}) (string, error) {
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims(claims))
	tok.Header["kid"] = kid
	signed, err := tok.SignedString(key)
	if err != nil {
		return "", fmt.Errorf("sign RS256 token: %w", err)
	}
	return signed, nil
}

// jwks returns the public key set.
func (s *OIDCService) jwks(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	pub := s.signingKey.PublicKey
	kid := s.keyID
	s.mu.RUnlock()

	n := pub.N.Bytes()
	e := big.NewInt(int64(pub.E)).Bytes()

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"keys": []map[string]interface{}{
			{
				"kty": "RSA",
				"use": "sig",
				"alg": "RS256",
				"kid": kid,
				"n":   base64.RawURLEncoding.EncodeToString(n),
				"e":   base64.RawURLEncoding.EncodeToString(e),
			},
		},
	}); err != nil {
		log.Error().Err(err).Msg("ADO OIDC: write JWKS")
	}
}

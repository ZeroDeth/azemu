package auth

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"github.com/rs/zerolog/log"
)

// IMDSService emulates the Azure Instance Metadata Service token endpoint.
// Real IMDS is reachable only from inside an Azure VM at the link-local
// address 169.254.169.254. In azemu the handler is mounted on the normal
// HTTPS port so Terraform and SDK clients on the developer machine can
// exercise managed identity token flows without raw socket tricks.
//
// The Metadata: true header requirement is enforced as real IMDS does.
//
// Endpoint: GET /metadata/identity/oauth2/token
//   - Required header:  Metadata: true
//   - Required query:   api-version (any non-empty value accepted)
//   - Optional query:   resource (defaults to https://management.azure.com/)
type IMDSService struct {
	tokenSvc *TokenService
}

// NewIMDSService constructs an IMDSService backed by the same TokenService
// that serves the OAuth2 and OIDC routes, so the same RSA signing key is
// used for all token types.
func NewIMDSService(tokenSvc *TokenService) *IMDSService {
	return &IMDSService{tokenSvc: tokenSvc}
}

// Routes mounts the IMDS token endpoint on the provided chi.Router.
// Wire at /metadata/identity so the full path becomes
// /metadata/identity/oauth2/token.
func (s *IMDSService) Routes(r chi.Router) {
	r.Get("/oauth2/token", s.token)
}

// token handles GET /metadata/identity/oauth2/token.
func (s *IMDSService) token(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Metadata") != "true" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		if err := json.NewEncoder(w).Encode(map[string]interface{}{
			"error":             "invalid_request",
			"error_description": "Required metadata header not specified",
		}); err != nil {
			log.Error().Err(err).Msg("imds: write missing-header error")
		}
		return
	}

	if r.URL.Query().Get("api-version") == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		if err := json.NewEncoder(w).Encode(map[string]interface{}{
			"error":             "invalid_request",
			"error_description": "api-version query parameter is required",
		}); err != nil {
			log.Error().Err(err).Msg("imds: write missing-api-version error")
		}
		return
	}

	resource := r.URL.Query().Get("resource")
	if resource == "" {
		resource = "https://management.azure.com/"
	}

	now := time.Now()
	expiry := now.Add(1 * time.Hour)

	claims := jwt.MapClaims{
		"aud": resource,
		"iss": "https://sts.windows.net/" + s.tokenSvc.tenantID + "/",
		"iat": now.Unix(),
		"nbf": now.Unix(),
		"exp": expiry.Unix(),
		"tid": s.tokenSvc.tenantID,
		"oid": "00000000-0000-0000-0000-000000000003",
		// xms_mirid is the ARM resource ID of the managed identity; azemu uses
		// a synthetic value since no real identity resource has been created.
		"xms_mirid": "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/azemu/providers/Microsoft.ManagedIdentity/userAssignedIdentities/azemu-mock",
		"appid":     "00000000-0000-0000-0000-000000000004",
		"sub":       "azemu-mock-managed-identity",
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = s.tokenSvc.keyID
	signed, err := tok.SignedString(s.tokenSvc.signingKey)
	if err != nil {
		log.Error().Err(err).Msg("imds: sign token")
		http.Error(w, `{"error":"token_signing_failed"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"access_token":   signed,
		"client_id":      "00000000-0000-0000-0000-000000000004",
		"expires_in":     "3600",
		"expires_on":     expiry.Unix(),
		"ext_expires_in": "3600",
		"not_before":     now.Unix(),
		"resource":       resource,
		"token_type":     "Bearer",
	}); err != nil {
		log.Error().Err(err).Msg("imds: write token response")
	}
}

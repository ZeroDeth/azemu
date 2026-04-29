package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"github.com/rs/zerolog/log"

	"github.com/zerodeth/azemu/internal/store"
)

type TokenService struct {
	tenantID   string
	signingKey *rsa.PrivateKey
	keyID      string
	store      store.Store
}

func NewTokenService(tenantID string, stores ...store.Store) (*TokenService, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("generate RSA signing key: %w", err)
	}
	var s store.Store
	if len(stores) > 0 {
		s = stores[0]
	}
	return &TokenService{
		tenantID:   tenantID,
		signingKey: key,
		keyID:      "azemu-signing-key-1",
		store:      s,
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
	if assertion := r.FormValue("client_assertion"); assertion != "" {
		t.workloadIdentityToken(w, r, assertion)
		return
	}

	clientID := r.FormValue("client_id")
	audience := tokenAudience(r)
	now := time.Now()
	signed, err := t.signAccessToken(jwt.MapClaims{
		"aud":   audience,
		"iss":   "https://sts.windows.net/" + t.tenantID + "/",
		"iat":   now.Unix(),
		"nbf":   now.Unix(),
		"exp":   now.Add(1 * time.Hour).Unix(),
		"tid":   t.tenantID,
		"oid":   "00000000-0000-0000-0000-000000000002",
		"appid": clientID,
		"sub":   "azemu-mock-subject",
	})
	if err != nil {
		http.Error(w, `{"error":"token_signing_failed"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"access_token": signed,
		"token_type":   "Bearer",
		"expires_in":   3600,
		"resource":     audience,
	}); err != nil {
		log.Error().Err(err).Msg("failed to write token response")
	}
}

func (t *TokenService) workloadIdentityToken(w http.ResponseWriter, r *http.Request, assertion string) {
	match, assertionClaims, ok := t.matchFederatedIdentityCredential(r.FormValue("client_id"), assertion)
	if !ok {
		writeTokenError(w, http.StatusBadRequest, "invalid_grant", "client assertion does not match a federated identity credential")
		return
	}

	audience := tokenAudience(r)
	now := time.Now()
	subject, _ := assertionClaims["sub"].(string)
	signed, err := t.signAccessToken(jwt.MapClaims{
		"aud":       audience,
		"iss":       "https://sts.windows.net/" + t.tenantID + "/",
		"iat":       now.Unix(),
		"nbf":       now.Unix(),
		"exp":       now.Add(1 * time.Hour).Unix(),
		"tid":       t.tenantID,
		"oid":       match.principalID,
		"appid":     match.clientID,
		"sub":       subject,
		"xms_mirid": match.identityID,
	})
	if err != nil {
		http.Error(w, `{"error":"token_signing_failed"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"access_token": signed,
		"token_type":   "Bearer",
		"expires_in":   3600,
		"resource":     audience,
	}); err != nil {
		log.Error().Err(err).Msg("failed to write workload identity token response")
	}
}

type federatedIdentityMatch struct {
	clientID    string
	principalID string
	identityID  string
}

func (t *TokenService) matchFederatedIdentityCredential(clientID, assertion string) (federatedIdentityMatch, jwt.MapClaims, bool) {
	if t.store == nil || clientID == "" {
		return federatedIdentityMatch{}, nil, false
	}

	parser := jwt.NewParser()
	token, _, err := parser.ParseUnverified(assertion, jwt.MapClaims{})
	if err != nil {
		return federatedIdentityMatch{}, nil, false
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return federatedIdentityMatch{}, nil, false
	}

	issuer, _ := claims["iss"].(string)
	subject, _ := claims["sub"].(string)
	audiences := claimAudiences(claims["aud"])
	if issuer == "" || subject == "" || len(audiences) == 0 {
		return federatedIdentityMatch{}, claims, false
	}

	for _, identity := range t.store.List("/subscriptions/") {
		if identity.Type != "Microsoft.ManagedIdentity/userAssignedIdentities" {
			continue
		}
		props := identity.Properties
		if props == nil || props["clientId"] != clientID {
			continue
		}
		prefix := identity.ID + "/federatedIdentityCredentials/"
		for _, cred := range t.store.List(prefix) {
			if cred.Type != "Microsoft.ManagedIdentity/userAssignedIdentities/federatedIdentityCredentials" {
				continue
			}
			if credentialMatchesAssertion(cred.Properties, issuer, subject, audiences) {
				principalID, _ := props["principalId"].(string)
				return federatedIdentityMatch{
					clientID:    clientID,
					principalID: principalID,
					identityID:  identity.ID,
				}, claims, true
			}
		}
	}
	return federatedIdentityMatch{}, claims, false
}

func credentialMatchesAssertion(props map[string]interface{}, issuer, subject string, audiences []string) bool {
	if props["issuer"] != issuer || props["subject"] != subject {
		return false
	}
	allowed := stringSlice(props["audiences"])
	for _, want := range allowed {
		for _, got := range audiences {
			if want == got {
				return true
			}
		}
	}
	return false
}

func claimAudiences(value interface{}) []string {
	switch v := value.(type) {
	case string:
		return []string{v}
	case []string:
		return v
	case []interface{}:
		var out []string
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func stringSlice(value interface{}) []string {
	switch v := value.(type) {
	case []string:
		return v
	case []interface{}:
		var out []string
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func tokenAudience(r *http.Request) string {
	if resource := r.FormValue("resource"); resource != "" {
		return resource
	}
	if scope := r.FormValue("scope"); scope != "" {
		fields := strings.Fields(scope)
		if len(fields) > 0 {
			return strings.TrimSuffix(fields[0], "/.default")
		}
	}
	return "https://management.azure.com/"
}

func (t *TokenService) signAccessToken(claims jwt.MapClaims) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = t.keyID
	return token.SignedString(t.signingKey)
}

// ValidateBearerToken verifies that a data-plane bearer token was minted by
// this TokenService.
func (t *TokenService) ValidateBearerToken(raw string) bool {
	_, err := jwt.Parse(raw, func(token *jwt.Token) (interface{}, error) {
		return &t.signingKey.PublicKey, nil
	}, jwt.WithValidMethods([]string{"RS256"}))
	return err == nil
}

func writeTokenError(w http.ResponseWriter, status int, code, description string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"error":             code,
		"error_description": description,
	}); err != nil {
		log.Error().Err(err).Msg("failed to write token error")
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

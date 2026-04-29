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
)

// FICMatch describes the user-assigned identity that a workload-identity
// client assertion successfully resolved to.
type FICMatch struct {
	ClientID    string
	PrincipalID string
	IdentityID  string
}

// FICResolver looks up a stored federated identity credential whose trust
// configuration (issuer, subject, audiences) matches an incoming client
// assertion for the given clientID. The interface is defined here so that
// internal/auth has no dependency on internal/store; callers in cmd/azemu
// supply the concrete implementation that walks the store.
type FICResolver interface {
	ResolveFederatedIdentity(clientID, issuer, subject string, audiences []string) (FICMatch, bool)
}

type TokenService struct {
	tenantID   string
	signingKey *rsa.PrivateKey
	keyID      string
	resolver   FICResolver
}

func NewTokenService(tenantID string, resolvers ...FICResolver) (*TokenService, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("generate RSA signing key: %w", err)
	}
	var r FICResolver
	if len(resolvers) > 0 {
		r = resolvers[0]
	}
	return &TokenService{
		tenantID:   tenantID,
		signingKey: key,
		keyID:      "azemu-signing-key-1",
		resolver:   r,
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
		"oid":       match.PrincipalID,
		"appid":     match.ClientID,
		"sub":       subject,
		"xms_mirid": match.IdentityID,
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

func (t *TokenService) matchFederatedIdentityCredential(clientID, assertion string) (FICMatch, jwt.MapClaims, bool) {
	if t.resolver == nil || clientID == "" {
		return FICMatch{}, nil, false
	}

	// The client assertion is an OIDC token minted by an external issuer
	// (e.g. a Kubernetes service-account token, a GitHub Actions OIDC
	// token). azemu deliberately does not verify the signature: there is
	// no JWKS fetch against the issuer because azemu is a local dev
	// emulator with no network egress, and the federated trust is
	// expressed by the FIC issuer/subject/audiences match below. We do,
	// however, enforce temporal claims so an expired or not-yet-valid
	// token cannot be replayed indefinitely.
	parser := jwt.NewParser()
	token, _, err := parser.ParseUnverified(assertion, jwt.MapClaims{})
	if err != nil {
		return FICMatch{}, nil, false
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return FICMatch{}, nil, false
	}
	if !assertionTemporallyValid(claims) {
		return FICMatch{}, claims, false
	}

	issuer, _ := claims["iss"].(string)
	subject, _ := claims["sub"].(string)
	audiences := claimAudiences(claims["aud"])
	if issuer == "" || subject == "" || len(audiences) == 0 {
		return FICMatch{}, claims, false
	}

	match, ok := t.resolver.ResolveFederatedIdentity(clientID, issuer, subject, audiences)
	if !ok {
		return FICMatch{}, claims, false
	}
	return match, claims, true
}

// assertionTemporallyValid rejects assertions whose `exp` is in the past or
// whose `nbf` is in the future. Both claims are treated as optional: an
// assertion that omits them is accepted (signature verification is
// intentionally not performed; see matchFederatedIdentityCredential).
func assertionTemporallyValid(claims jwt.MapClaims) bool {
	now := time.Now().Unix()
	if exp, ok := claimUnixSeconds(claims["exp"]); ok && now > exp {
		return false
	}
	if nbf, ok := claimUnixSeconds(claims["nbf"]); ok && now < nbf {
		return false
	}
	return true
}

func claimUnixSeconds(v interface{}) (int64, bool) {
	switch n := v.(type) {
	case float64:
		return int64(n), true
	case int64:
		return n, true
	case json.Number:
		i, err := n.Int64()
		if err != nil {
			return 0, false
		}
		return i, true
	default:
		return 0, false
	}
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

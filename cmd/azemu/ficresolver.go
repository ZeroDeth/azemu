package main

import (
	"github.com/zerodeth/azemu/internal/auth"
	"github.com/zerodeth/azemu/internal/store"
)

// federatedIdentityResolver walks the in-memory store to satisfy
// auth.FICResolver. It lives in the cmd layer so internal/auth has no
// dependency on internal/store (see .claude/rules/go-style.md package
// boundaries).
type federatedIdentityResolver struct {
	store store.Store
}

func newFederatedIdentityResolver(s store.Store) *federatedIdentityResolver {
	return &federatedIdentityResolver{store: s}
}

func (r *federatedIdentityResolver) ResolveFederatedIdentity(clientID, issuer, subject string, audiences []string) (auth.FICMatch, bool) {
	for _, identity := range r.store.List("/subscriptions/") {
		if identity.Type != "Microsoft.ManagedIdentity/userAssignedIdentities" {
			continue
		}
		props := identity.Properties
		if props == nil || props["clientId"] != clientID {
			continue
		}
		prefix := identity.ID + "/federatedIdentityCredentials/"
		for _, cred := range r.store.List(prefix) {
			if cred.Type != "Microsoft.ManagedIdentity/userAssignedIdentities/federatedIdentityCredentials" {
				continue
			}
			if !credentialMatches(cred.Properties, issuer, subject, audiences) {
				continue
			}
			principalID, _ := props["principalId"].(string)
			return auth.FICMatch{
				ClientID:    clientID,
				PrincipalID: principalID,
				IdentityID:  identity.ID,
			}, true
		}
	}
	return auth.FICMatch{}, false
}

func credentialMatches(props map[string]interface{}, issuer, subject string, audiences []string) bool {
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

func stringSlice(value interface{}) []string {
	switch v := value.(type) {
	case []string:
		return v
	case []interface{}:
		out := make([]string, 0, len(v))
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

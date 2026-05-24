package main

import (
	"testing"

	"github.com/zerodeth/azemu/internal/store"
)

const (
	testIssuer      = "https://token.actions.githubusercontent.com"
	testSubject     = "repo:org/repo:ref:refs/heads/main"
	testAudience    = "api://AzureADTokenExchange"
	testClientID    = "test-client-id-1234"
	testPrincipalID = "test-principal-id-5678"

	identityID = "/subscriptions/sub1/resourceGroups/rg1/providers/Microsoft.ManagedIdentity/userAssignedIdentities/id1"
	credID     = identityID + "/federatedIdentityCredentials/cred1"
)

// seedStore populates a fresh store with one user-assigned identity and one
// federated identity credential attached to it, using the test constants above.
func seedStore(t *testing.T) store.Store {
	t.Helper()
	s := store.NewMemoryStore()

	if err := s.Put(identityID, &store.Resource{
		ID:   identityID,
		Name: "id1",
		Type: "Microsoft.ManagedIdentity/userAssignedIdentities",
		Properties: map[string]any{
			"clientId":    testClientID,
			"principalId": testPrincipalID,
		},
	}); err != nil {
		t.Fatalf("seed identity: %v", err)
	}

	if err := s.Put(credID, &store.Resource{
		ID:   credID,
		Name: "cred1",
		Type: "Microsoft.ManagedIdentity/userAssignedIdentities/federatedIdentityCredentials",
		Properties: map[string]any{
			"issuer":    testIssuer,
			"subject":   testSubject,
			"audiences": []any{testAudience},
		},
	}); err != nil {
		t.Fatalf("seed credential: %v", err)
	}

	return s
}

func TestResolveFederatedIdentity_found(t *testing.T) {
	r := newFederatedIdentityResolver(seedStore(t))
	match, ok := r.ResolveFederatedIdentity(testClientID, testIssuer, testSubject, []string{testAudience})
	if !ok {
		t.Fatal("want ok=true, got false")
	}
	if match.ClientID != testClientID {
		t.Errorf("ClientID: want %q, got %q", testClientID, match.ClientID)
	}
	if match.PrincipalID != testPrincipalID {
		t.Errorf("PrincipalID: want %q, got %q", testPrincipalID, match.PrincipalID)
	}
	if match.IdentityID != identityID {
		t.Errorf("IdentityID: want %q, got %q", identityID, match.IdentityID)
	}
}

func TestResolveFederatedIdentity_clientIDMismatch(t *testing.T) {
	r := newFederatedIdentityResolver(seedStore(t))
	_, ok := r.ResolveFederatedIdentity("wrong-client-id", testIssuer, testSubject, []string{testAudience})
	if ok {
		t.Fatal("want ok=false on clientID mismatch, got true")
	}
}

func TestResolveFederatedIdentity_issuerMismatch(t *testing.T) {
	r := newFederatedIdentityResolver(seedStore(t))
	_, ok := r.ResolveFederatedIdentity(testClientID, "https://wrong.issuer.com", testSubject, []string{testAudience})
	if ok {
		t.Fatal("want ok=false on issuer mismatch, got true")
	}
}

func TestResolveFederatedIdentity_subjectMismatch(t *testing.T) {
	r := newFederatedIdentityResolver(seedStore(t))
	_, ok := r.ResolveFederatedIdentity(testClientID, testIssuer, "wrong:subject", []string{testAudience})
	if ok {
		t.Fatal("want ok=false on subject mismatch, got true")
	}
}

func TestResolveFederatedIdentity_audienceMismatch(t *testing.T) {
	r := newFederatedIdentityResolver(seedStore(t))
	_, ok := r.ResolveFederatedIdentity(testClientID, testIssuer, testSubject, []string{"wrong-audience"})
	if ok {
		t.Fatal("want ok=false on audience mismatch, got true")
	}
}

func TestResolveFederatedIdentity_emptyStore(t *testing.T) {
	r := newFederatedIdentityResolver(store.NewMemoryStore())
	_, ok := r.ResolveFederatedIdentity(testClientID, testIssuer, testSubject, []string{testAudience})
	if ok {
		t.Fatal("want ok=false on empty store, got true")
	}
}

func TestResolveFederatedIdentity_multipleIdentities_secondMatches(t *testing.T) {
	s := seedStore(t) // has identity at identityID / testClientID

	// Add a second identity that does NOT match.
	id2 := "/subscriptions/sub1/resourceGroups/rg1/providers/Microsoft.ManagedIdentity/userAssignedIdentities/id2"
	if err := s.Put(id2, &store.Resource{
		ID:   id2,
		Name: "id2",
		Type: "Microsoft.ManagedIdentity/userAssignedIdentities",
		Properties: map[string]any{
			"clientId":    "other-client-id",
			"principalId": "other-principal-id",
		},
	}); err != nil {
		t.Fatalf("seed second identity: %v", err)
	}

	r := newFederatedIdentityResolver(s)
	match, ok := r.ResolveFederatedIdentity(testClientID, testIssuer, testSubject, []string{testAudience})
	if !ok {
		t.Fatal("want ok=true (first identity matches), got false")
	}
	if match.ClientID != testClientID {
		t.Errorf("ClientID: want %q, got %q", testClientID, match.ClientID)
	}
}

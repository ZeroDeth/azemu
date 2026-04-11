package store

import (
	"sync"
	"testing"
	"time"
)

// TestMemoryStore_Put_CreatesThenUpdates verifies that the first Put sets
// CreatedAt and that a second Put preserves CreatedAt while advancing UpdatedAt.
func TestMemoryStore_Put_CreatesThenUpdates(t *testing.T) {
	s := NewMemoryStore()

	r1 := &Resource{Name: "rg1", Type: "Microsoft.Resources/resourceGroups", Location: "uksouth"}
	if err := s.Put("/subscriptions/sub1/resourcegroups/rg1", r1); err != nil {
		t.Fatalf("first Put returned unexpected error: %v", err)
	}
	created := r1.CreatedAt
	updated1 := r1.UpdatedAt
	if created.IsZero() {
		t.Fatal("CreatedAt is zero after first Put")
	}
	if updated1.IsZero() {
		t.Fatal("UpdatedAt is zero after first Put")
	}

	// Wait long enough that a second Put will get a strictly later time.
	time.Sleep(2 * time.Millisecond)

	r2 := &Resource{Name: "rg1", Type: "Microsoft.Resources/resourceGroups", Location: "uksouth"}
	if err := s.Put("/subscriptions/sub1/resourcegroups/rg1", r2); err != nil {
		t.Fatalf("second Put returned unexpected error: %v", err)
	}
	if !r2.CreatedAt.Equal(created) {
		t.Errorf("CreatedAt changed on update: got %v, want %v", r2.CreatedAt, created)
	}
	if !r2.UpdatedAt.After(updated1) {
		t.Errorf("UpdatedAt did not advance on update: got %v, first was %v", r2.UpdatedAt, updated1)
	}
}

// TestMemoryStore_Put_ReturnsNilError_Today is a regression guard for the
// current contract. Today Put always returns nil. Phase 4's file-backed store
// will need this to start returning real errors; this test documents the
// current in-memory guarantee so a future file-store implementation knows it
// must not break the interface without updating callers.
func TestMemoryStore_Put_ReturnsNilError_Today(t *testing.T) {
	s := NewMemoryStore()
	r := &Resource{Name: "rg1", Location: "uksouth"}
	err := s.Put("/subscriptions/sub1/resourcegroups/rg1", r)
	if err != nil {
		t.Errorf("Put returned non-nil error: %v; current contract is always-nil", err)
	}
}

func TestMemoryStore_Get_MissingReturnsNilFalse(t *testing.T) {
	s := NewMemoryStore()
	r, ok := s.Get("/subscriptions/sub1/resourcegroups/does-not-exist")
	if ok {
		t.Error("ok = true for missing key, want false")
	}
	if r != nil {
		t.Errorf("resource = %v for missing key, want nil", r)
	}
}

func TestMemoryStore_Get_ExistingReturns(t *testing.T) {
	s := NewMemoryStore()
	id := "/subscriptions/sub1/resourcegroups/rg1"
	want := &Resource{Name: "rg1", Location: "eastus"}
	if err := s.Put(id, want); err != nil {
		t.Fatalf("Put: %v", err)
	}

	got, ok := s.Get(id)
	if !ok {
		t.Fatal("ok = false for existing key, want true")
	}
	if got == nil {
		t.Fatal("resource is nil for existing key")
	}
	if got.Name != "rg1" {
		t.Errorf("Name = %q, want rg1", got.Name)
	}
	if got.Location != "eastus" {
		t.Errorf("Location = %q, want eastus", got.Location)
	}
}

func TestMemoryStore_Delete_SingleKey_ReturnsTrue(t *testing.T) {
	s := NewMemoryStore()
	id := "/subscriptions/sub1/resourcegroups/rg1"
	if err := s.Put(id, &Resource{Name: "rg1"}); err != nil {
		t.Fatalf("Put: %v", err)
	}

	if deleted := s.Delete(id); !deleted {
		t.Error("Delete returned false for existing key, want true")
	}
	if _, ok := s.Get(id); ok {
		t.Error("Get found resource after Delete, want miss")
	}
}

func TestMemoryStore_Delete_Missing_ReturnsFalse(t *testing.T) {
	s := NewMemoryStore()
	if deleted := s.Delete("/subscriptions/sub1/resourcegroups/does-not-exist"); deleted {
		t.Error("Delete returned true for non-existent key, want false")
	}
}

// TestMemoryStore_Delete_CascadesByPrefix ensures that deleting a parent
// resource also deletes all children whose keys start with parentID + "/",
// while a sibling resource at the same level is left untouched.
func TestMemoryStore_Delete_CascadesByPrefix(t *testing.T) {
	s := NewMemoryStore()

	parent := "/subscriptions/sub1/resourcegroups/rg1"
	child := "/subscriptions/sub1/resourcegroups/rg1/providers/microsoft.network/virtualnetworks/vnet1/subnets/child"
	sibling := "/subscriptions/sub1/resourcegroups/rg2"

	for _, id := range []string{parent, child, sibling} {
		if err := s.Put(id, &Resource{Name: id}); err != nil {
			t.Fatalf("Put %q: %v", id, err)
		}
	}

	if deleted := s.Delete(parent); !deleted {
		t.Error("Delete returned false for existing parent, want true")
	}

	if _, ok := s.Get(parent); ok {
		t.Errorf("parent %q still present after Delete", parent)
	}
	if _, ok := s.Get(child); ok {
		t.Errorf("child %q still present after cascade Delete of parent", child)
	}
	if _, ok := s.Get(sibling); !ok {
		t.Errorf("sibling %q was deleted but should have survived", sibling)
	}
}

func TestMemoryStore_List_PrefixMatch(t *testing.T) {
	s := NewMemoryStore()

	rg1 := "/subscriptions/sub1/resourcegroups/rg1"
	vnet1 := "/subscriptions/sub1/resourcegroups/rg1/providers/microsoft.network/virtualnetworks/vnet1"
	vnet2 := "/subscriptions/sub1/resourcegroups/rg1/providers/microsoft.network/virtualnetworks/vnet2"
	other := "/subscriptions/sub1/resourcegroups/rg2/providers/microsoft.network/virtualnetworks/vnet3"

	for _, id := range []string{rg1, vnet1, vnet2, other} {
		if err := s.Put(id, &Resource{Name: id}); err != nil {
			t.Fatalf("Put %q: %v", id, err)
		}
	}

	results := s.List(rg1)
	// rg1 itself matches the prefix, plus vnet1 and vnet2 — three total.
	if len(results) != 3 {
		t.Errorf("List returned %d results, want 3", len(results))
	}

	// Verify the non-matching sibling is absent.
	for _, r := range results {
		if r.Name == other {
			t.Errorf("List returned sibling %q which should not match prefix %q", other, rg1)
		}
	}
}

func TestMemoryStore_ExportImport_RoundTrip(t *testing.T) {
	a := NewMemoryStore()
	id := "/subscriptions/sub1/resourcegroups/rg1"
	if err := a.Put(id, &Resource{Name: "rg1", Location: "uksouth", Type: "rg"}); err != nil {
		t.Fatalf("Put: %v", err)
	}

	data, err := a.Export()
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("Export returned empty bytes")
	}

	b := NewMemoryStore()
	if err := b.Import(data); err != nil {
		t.Fatalf("Import: %v", err)
	}

	got, ok := b.Get(id)
	if !ok {
		t.Fatalf("Get after Import returned miss for %q", id)
	}
	if got.Name != "rg1" {
		t.Errorf("Name = %q after round-trip, want rg1", got.Name)
	}
	if got.Location != "uksouth" {
		t.Errorf("Location = %q after round-trip, want uksouth", got.Location)
	}
}

func TestMemoryStore_Import_RejectsInvalidJSON(t *testing.T) {
	s := NewMemoryStore()
	err := s.Import([]byte("{not json"))
	if err == nil {
		t.Error("Import accepted invalid JSON without error, want non-nil error")
	}
}

// TestMemoryStore_ConcurrentAccess runs 8 goroutines doing Put/Get/Delete
// against non-overlapping key ranges. The test must pass under -race.
func TestMemoryStore_ConcurrentAccess(t *testing.T) {
	s := NewMemoryStore()
	const goroutines = 8
	const ops = 200

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := range goroutines {
		go func() {
			defer wg.Done()
			// Each goroutine operates on its own key prefix to keep keys
			// non-overlapping; this still exercises the shared lock.
			base := "/subscriptions/sub1/resourcegroups/rg-worker-"
			for i := range ops {
				id := base + string(rune('a'+g))
				_ = s.Put(id, &Resource{Name: id})
				_, _ = s.Get(id)
				if i%10 == 0 {
					s.Delete(id)
				}
			}
		}()
	}

	wg.Wait()
}

package store

import (
	"sync"
	"testing"
	"time"
)

// TestPut_CreatesThenUpdates verifies that the first Put sets CreatedAt and
// that a second Put preserves CreatedAt while advancing UpdatedAt.
func TestPut_CreatesThenUpdates(t *testing.T) {
	s := NewMemoryStore()
	id := "/subscriptions/sub1/resourcegroups/rg1"

	r1 := &Resource{Name: "rg1", Type: "Microsoft.Resources/resourceGroups", Location: "uksouth"}
	if err := s.Put(id, r1); err != nil {
		t.Fatalf("first Put returned unexpected error: %v", err)
	}
	got1, _ := s.Get(id)
	if got1.CreatedAt.IsZero() {
		t.Fatal("CreatedAt is zero after first Put")
	}
	if got1.UpdatedAt.IsZero() {
		t.Fatal("UpdatedAt is zero after first Put")
	}

	created := got1.CreatedAt
	updated1 := got1.UpdatedAt

	time.Sleep(2 * time.Millisecond)

	r2 := &Resource{Name: "rg1", Type: "Microsoft.Resources/resourceGroups", Location: "uksouth"}
	if err := s.Put(id, r2); err != nil {
		t.Fatalf("second Put returned unexpected error: %v", err)
	}
	got2, _ := s.Get(id)
	if !got2.CreatedAt.Equal(created) {
		t.Errorf("CreatedAt changed on update: got %v, want %v", got2.CreatedAt, created)
	}
	if !got2.UpdatedAt.After(updated1) {
		t.Errorf("UpdatedAt did not advance on update: got %v, first was %v", got2.UpdatedAt, updated1)
	}
}

// TestPut_ReturnsNilError_Today is a regression guard for the current
// contract. Today Put always returns nil. Phase 4's file-backed store will
// need this to start returning real errors; this test documents the current
// in-memory guarantee so a future file-store implementation knows it must not
// break the interface without updating callers.
func TestPut_ReturnsNilError_Today(t *testing.T) {
	s := NewMemoryStore()
	r := &Resource{Name: "rg1", Location: "uksouth"}
	err := s.Put("/subscriptions/sub1/resourcegroups/rg1", r)
	if err != nil {
		t.Errorf("Put returned non-nil error: %v; current contract is always-nil", err)
	}
}

func TestGet_MissingReturnsNilFalse(t *testing.T) {
	s := NewMemoryStore()
	r, ok := s.Get("/subscriptions/sub1/resourcegroups/does-not-exist")
	if ok {
		t.Error("ok = true for missing key, want false")
	}
	if r != nil {
		t.Errorf("resource = %v for missing key, want nil", r)
	}
}

func TestGet_ExistingReturns(t *testing.T) {
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

func TestDelete_SingleKey_ReturnsTrue(t *testing.T) {
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

func TestDelete_Missing_ReturnsFalse(t *testing.T) {
	s := NewMemoryStore()
	if deleted := s.Delete("/subscriptions/sub1/resourcegroups/does-not-exist"); deleted {
		t.Error("Delete returned true for non-existent key, want false")
	}
}

// TestMemoryStore_Delete_CascadesByPrefix ensures that deleting a parent
// resource also deletes all children whose keys start with parentID + "/",
// while a sibling resource at the same level is left untouched.
func TestDelete_CascadesByPrefix(t *testing.T) {
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

func TestList_PrefixMatch(t *testing.T) {
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

func TestExportImport_RoundTrip(t *testing.T) {
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

func TestImport_RejectsInvalidJSON(t *testing.T) {
	s := NewMemoryStore()
	err := s.Import([]byte("{not json"))
	if err == nil {
		t.Error("Import accepted invalid JSON without error, want non-nil error")
	}
}

// TestConcurrentAccess runs 8 goroutines doing Put/Get/Delete/List
// against non-overlapping key ranges. The test must pass under -race.
func TestConcurrentAccess(t *testing.T) {
	s := NewMemoryStore()
	const goroutines = 8
	const ops = 200

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := range goroutines {
		go func() {
			defer wg.Done()
			base := "/subscriptions/sub1/resourcegroups/rg-worker-"
			for i := range ops {
				id := base + string(rune('a'+g))
				_ = s.Put(id, &Resource{Name: id})
				_, _ = s.Get(id)
				_ = s.List(base)
				if i%10 == 0 {
					s.Delete(id)
				}
				if i%50 == 0 {
					_, _ = s.Export()
				}
			}
		}()
	}

	wg.Wait()
}

// TestPut_DoesNotAliasCaller verifies that Put stores a copy, so mutating
// the caller's pointer after Put does not affect the stored resource.
func TestPut_DoesNotAliasCaller(t *testing.T) {
	s := NewMemoryStore()
	id := "/subscriptions/sub1/resourcegroups/rg1"
	r := &Resource{Name: "rg1", Location: "uksouth"}
	if err := s.Put(id, r); err != nil {
		t.Fatalf("Put: %v", err)
	}

	// Mutate the caller's struct after Put.
	r.Location = "mutated"

	got, ok := s.Get(id)
	if !ok {
		t.Fatal("Get returned false after Put")
	}
	if got.Location != "uksouth" {
		t.Errorf("stored Location = %q after caller mutation, want uksouth", got.Location)
	}
}

// TestGet_ReturnsCopy verifies that mutating the returned resource does not
// affect the stored value.
func TestGet_ReturnsCopy(t *testing.T) {
	s := NewMemoryStore()
	id := "/subscriptions/sub1/resourcegroups/rg1"
	if err := s.Put(id, &Resource{Name: "rg1", Location: "uksouth"}); err != nil {
		t.Fatalf("Put: %v", err)
	}

	got, _ := s.Get(id)
	got.Location = "mutated"

	got2, _ := s.Get(id)
	if got2.Location != "uksouth" {
		t.Errorf("second Get Location = %q after mutation of first Get result, want uksouth", got2.Location)
	}
}

// TestExportImport_PreservesTimestamps verifies that CreatedAt and UpdatedAt
// survive an Export/Import round-trip (regression for json:"-" tag fix).
func TestExportImport_PreservesTimestamps(t *testing.T) {
	a := NewMemoryStore()
	id := "/subscriptions/sub1/resourcegroups/rg1"
	if err := a.Put(id, &Resource{Name: "rg1", Location: "uksouth"}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	orig, _ := a.Get(id)

	data, err := a.Export()
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	b := NewMemoryStore()
	if err := b.Import(data); err != nil {
		t.Fatalf("Import: %v", err)
	}
	got, ok := b.Get(id)
	if !ok {
		t.Fatal("Get after Import returned miss")
	}
	if got.CreatedAt.IsZero() {
		t.Error("CreatedAt is zero after round-trip")
	}
	if got.UpdatedAt.IsZero() {
		t.Error("UpdatedAt is zero after round-trip")
	}
	if !got.CreatedAt.Equal(orig.CreatedAt) {
		t.Errorf("CreatedAt changed: got %v, want %v", got.CreatedAt, orig.CreatedAt)
	}
	if !got.UpdatedAt.Equal(orig.UpdatedAt) {
		t.Errorf("UpdatedAt changed: got %v, want %v", got.UpdatedAt, orig.UpdatedAt)
	}
}

// TestImport_RejectsNilResource verifies that Import rejects JSON containing
// null resource values.
func TestImport_RejectsNilResource(t *testing.T) {
	s := NewMemoryStore()
	data := []byte(`{"/subscriptions/sub1/resourcegroups/rg1": null}`)
	err := s.Import(data)
	if err == nil {
		t.Error("Import accepted null resource value, want error")
	}
}

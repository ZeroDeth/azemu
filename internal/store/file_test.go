package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func newTestFileStore(t *testing.T) (*FileStore, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "state.json")
	fs, err := NewFileStore(path)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	return fs, path
}

// TestFileStore_Put_WritesThrough verifies that Put persists the resource
// to disk and the file contains valid JSON with the resource.
func TestFileStore_Put_WritesThrough(t *testing.T) {
	fs, path := newTestFileStore(t)
	id := "/subscriptions/sub1/resourcegroups/rg1"
	if err := fs.Put(id, &Resource{Name: "rg1", Location: "uksouth"}); err != nil {
		t.Fatalf("Put: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var m map[string]*Resource
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	r, ok := m[id]
	if !ok {
		t.Fatalf("resource %q not found in persisted file", id)
	}
	if r.Name != "rg1" {
		t.Errorf("Name = %q, want rg1", r.Name)
	}
}

// TestFileStore_Reload verifies that a new FileStore loaded from the same
// path sees resources written by the previous instance.
func TestFileStore_Reload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	// First instance: write a resource.
	fs1, err := NewFileStore(path)
	if err != nil {
		t.Fatalf("NewFileStore(1): %v", err)
	}
	id := "/subscriptions/sub1/resourcegroups/rg1"
	if err := fs1.Put(id, &Resource{Name: "rg1", Location: "uksouth"}); err != nil {
		t.Fatalf("Put: %v", err)
	}

	// Second instance: load from same path.
	fs2, err := NewFileStore(path)
	if err != nil {
		t.Fatalf("NewFileStore(2): %v", err)
	}
	got, ok := fs2.Get(id)
	if !ok {
		t.Fatal("Get returned false after reload, want true")
	}
	if got.Name != "rg1" {
		t.Errorf("Name = %q after reload, want rg1", got.Name)
	}
	if got.Location != "uksouth" {
		t.Errorf("Location = %q after reload, want uksouth", got.Location)
	}
}

// TestFileStore_Reload_PreservesTimestamps verifies that timestamps survive
// a persist-reload cycle.
func TestFileStore_Reload_PreservesTimestamps(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	fs1, err := NewFileStore(path)
	if err != nil {
		t.Fatalf("NewFileStore(1): %v", err)
	}
	id := "/subscriptions/sub1/resourcegroups/rg1"
	if err := fs1.Put(id, &Resource{Name: "rg1", Location: "uksouth"}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	orig, _ := fs1.Get(id)

	fs2, err := NewFileStore(path)
	if err != nil {
		t.Fatalf("NewFileStore(2): %v", err)
	}
	got, _ := fs2.Get(id)
	if !got.CreatedAt.Equal(orig.CreatedAt) {
		t.Errorf("CreatedAt changed after reload: %v vs %v", got.CreatedAt, orig.CreatedAt)
	}
	if !got.UpdatedAt.Equal(orig.UpdatedAt) {
		t.Errorf("UpdatedAt changed after reload: %v vs %v", got.UpdatedAt, orig.UpdatedAt)
	}
}

// TestFileStore_Delete_PersistsRemoval verifies that Delete writes through
// and the resource is absent on reload.
func TestFileStore_Delete_PersistsRemoval(t *testing.T) {
	fs, path := newTestFileStore(t)
	id := "/subscriptions/sub1/resourcegroups/rg1"
	if err := fs.Put(id, &Resource{Name: "rg1"}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if !fs.Delete(id) {
		t.Fatal("Delete returned false, want true")
	}

	fs2, err := NewFileStore(path)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	if _, ok := fs2.Get(id); ok {
		t.Error("Get returned true after delete + reload, want false")
	}
}

// TestFileStore_Reset_PersistsEmpty verifies that Reset writes an empty
// state to disk.
func TestFileStore_Reset_PersistsEmpty(t *testing.T) {
	fs, path := newTestFileStore(t)
	if err := fs.Put("/a", &Resource{Name: "a"}); err != nil {
		t.Fatalf("Put: %v", err)
	}

	fs.Reset()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var m map[string]*Resource
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(m) != 0 {
		t.Errorf("persisted state has %d resources after Reset, want 0", len(m))
	}
}

// TestFileStore_NoTmpFileLingers verifies that the .tmp file is cleaned up
// after a successful persist.
func TestFileStore_NoTmpFileLingers(t *testing.T) {
	fs, path := newTestFileStore(t)
	if err := fs.Put("/a", &Resource{Name: "a"}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Errorf(".tmp file exists after persist: err=%v", err)
	}
}

// TestFileStore_NewFileStore_MissingFile starts with no file on disk.
func TestFileStore_NewFileStore_MissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nonexistent.json")
	fs, err := NewFileStore(path)
	if err != nil {
		t.Fatalf("NewFileStore with missing file: %v", err)
	}
	// Store should be empty.
	if list := fs.List("/"); len(list) != 0 {
		t.Errorf("List returned %d items for missing file, want 0", len(list))
	}
}

// TestFileStore_NewFileStore_CorruptFile returns an error for invalid JSON.
func TestFileStore_NewFileStore_CorruptFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "corrupt.json")
	if err := os.WriteFile(path, []byte("{not json"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	_, err := NewFileStore(path)
	if err == nil {
		t.Error("NewFileStore accepted corrupt JSON, want error")
	}
}

// TestFileStore_Import_PersistsToDisk verifies that Import writes through.
func TestFileStore_Import_PersistsToDisk(t *testing.T) {
	fs, path := newTestFileStore(t)
	data := []byte(`{"/a":{"id":"/a","name":"a","type":"t","location":"l"}}`)
	if err := fs.Import(data); err != nil {
		t.Fatalf("Import: %v", err)
	}

	fs2, err := NewFileStore(path)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	if _, ok := fs2.Get("/a"); !ok {
		t.Error("imported resource not found after reload")
	}
}

// TestFileStore_ConcurrentAccess runs parallel Put/Get/Delete against a
// FileStore. Must pass under -race.
func TestFileStore_ConcurrentAccess(t *testing.T) {
	fs, _ := newTestFileStore(t)
	const goroutines = 4
	const ops = 50

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := range goroutines {
		go func() {
			defer wg.Done()
			base := "/subscriptions/sub1/resourcegroups/rg-worker-"
			for i := range ops {
				id := base + string(rune('a'+g))
				_ = fs.Put(id, &Resource{Name: id})
				_, _ = fs.Get(id)
				if i%10 == 0 {
					fs.Delete(id)
				}
			}
		}()
	}
	wg.Wait()
}

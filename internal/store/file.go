package store

import (
	"fmt"
	"os"
	"path/filepath"
)

// FileStore wraps MemoryStore and adds write-through persistence to a JSON
// file on disk. Every mutation (Put, Delete, Import, Reset) is followed by
// an atomic write: Export to a .tmp file, then os.Rename to the target path.
//
// Read operations (Get, List, Export) are inherited from MemoryStore and
// served entirely from memory with no disk I/O.
type FileStore struct {
	*MemoryStore
	path string
}

// NewFileStore creates a file-backed store at the given path. If the file
// already exists, its contents are loaded into memory. If the file does not
// exist, the store starts empty (and the file is created on the first write).
func NewFileStore(path string) (*FileStore, error) {
	f := &FileStore{
		MemoryStore: NewMemoryStore(),
		path:        path,
	}
	data, err := os.ReadFile(path)
	if err == nil {
		if err := f.MemoryStore.Import(data); err != nil {
			return nil, fmt.Errorf("load %s: %w", path, err)
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return f, nil
}

// Put stores a resource and persists the full state to disk.
func (f *FileStore) Put(id string, r *Resource) error {
	if err := f.MemoryStore.Put(id, r); err != nil {
		return err
	}
	if err := f.persist(); err != nil {
		return fmt.Errorf("persist after put %q: %w", id, err)
	}
	return nil
}

// Delete removes a resource (with cascade) and persists the state to disk.
func (f *FileStore) Delete(id string) bool {
	deleted := f.MemoryStore.Delete(id)
	if deleted {
		// Best-effort persist; log-worthy but not worth failing the delete.
		_ = f.persist()
	}
	return deleted
}

// Import replaces all state from the given JSON and persists to disk.
func (f *FileStore) Import(data []byte) error {
	if err := f.MemoryStore.Import(data); err != nil {
		return err
	}
	if err := f.persist(); err != nil {
		return fmt.Errorf("persist after import: %w", err)
	}
	return nil
}

// Reset clears all resources and persists the empty state to disk.
func (f *FileStore) Reset() {
	f.MemoryStore.Reset()
	_ = f.persist()
}

// persist writes the full store state to disk atomically. It exports the
// current state to JSON, writes it to a temporary file alongside the target,
// and renames the temp file to the target path. This prevents partial writes
// from corrupting the store on crash.
func (f *FileStore) persist() error {
	data, err := f.Export()
	if err != nil {
		return fmt.Errorf("export: %w", err)
	}
	tmp := f.path + ".tmp"
	if err := os.MkdirAll(filepath.Dir(f.path), 0755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(f.path), err)
	}
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return fmt.Errorf("write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, f.path); err != nil {
		return fmt.Errorf("rename %s -> %s: %w", tmp, f.path, err)
	}
	return nil
}

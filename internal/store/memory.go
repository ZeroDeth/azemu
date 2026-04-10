package store

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// Resource represents any ARM resource in the store.
type Resource struct {
	ID         string                 `json:"id"`
	Name       string                 `json:"name"`
	Type       string                 `json:"type"`
	Location   string                 `json:"location"`
	Tags       map[string]string      `json:"tags,omitempty"`
	Properties map[string]interface{} `json:"properties,omitempty"`
	CreatedAt  time.Time              `json:"-"`
	UpdatedAt  time.Time              `json:"-"`
}

// Store defines the interface for resource persistence.
type Store interface {
	Put(id string, r *Resource) error
	Get(id string) (*Resource, bool)
	Delete(id string) bool
	List(prefix string) []*Resource
	Export() ([]byte, error)
	Import(data []byte) error
}

// MemoryStore is the default in-memory implementation.
type MemoryStore struct {
	mu        sync.RWMutex
	resources map[string]*Resource
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{resources: make(map[string]*Resource)}
}

func (s *MemoryStore) Put(id string, r *Resource) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	if existing, ok := s.resources[id]; ok {
		r.CreatedAt = existing.CreatedAt
	} else {
		r.CreatedAt = now
	}
	r.UpdatedAt = now
	r.ID = id
	s.resources[id] = r
	return nil
}

func (s *MemoryStore) Get(id string) (*Resource, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.resources[id]
	return r, ok
}

func (s *MemoryStore) Delete(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Cascade: delete anything with this prefix
	deleted := false
	for k := range s.resources {
		if k == id || len(k) > len(id) && k[:len(id)+1] == id+"/" {
			delete(s.resources, k)
			deleted = true
		}
	}
	return deleted
}

func (s *MemoryStore) List(prefix string) []*Resource {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []*Resource
	for k, v := range s.resources {
		if len(k) >= len(prefix) && k[:len(prefix)] == prefix {
			result = append(result, v)
		}
	}
	return result
}

func (s *MemoryStore) Export() ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return json.MarshalIndent(s.resources, "", "  ")
}

func (s *MemoryStore) Import(data []byte) error {
	var imported map[string]*Resource
	if err := json.Unmarshal(data, &imported); err != nil {
		return fmt.Errorf("import: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resources = imported
	return nil
}

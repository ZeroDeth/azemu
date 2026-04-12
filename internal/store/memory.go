package store

import (
	"encoding/json"
	"fmt"
	"strings"
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
	CreatedAt  time.Time              `json:"createdAt"`
	UpdatedAt  time.Time              `json:"updatedAt"`
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
	stored := *r // shallow copy; caller keeps their own pointer
	stored.ID = id
	stored.UpdatedAt = now
	if existing, ok := s.resources[id]; ok {
		stored.CreatedAt = existing.CreatedAt
	} else {
		stored.CreatedAt = now
	}
	s.resources[id] = &stored
	return nil
}

func (s *MemoryStore) Get(id string) (*Resource, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.resources[id]
	if !ok {
		return nil, false
	}
	copy := *r // shallow copy; caller cannot mutate stored state
	return &copy, true
}

func (s *MemoryStore) Delete(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Cascade: delete anything with this prefix
	deleted := false
	prefix := id + "/"
	for k := range s.resources {
		if k == id || strings.HasPrefix(k, prefix) {
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
		if strings.HasPrefix(k, prefix) {
			copy := *v
			result = append(result, &copy)
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
	for k, v := range imported {
		if v == nil {
			return fmt.Errorf("import: nil resource at key %q", k)
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resources = imported
	return nil
}

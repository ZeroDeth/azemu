package middleware

import (
	"encoding/json"
	"net/http"
	"sync"

	"github.com/rs/zerolog/log"
)

// UnhandledTracker keeps a thread-safe record of unhandled routes
// encountered by the server.
type UnhandledTracker struct {
	mu     sync.RWMutex
	routes map[string]int
}

// NewUnhandledTracker creates a new tracker instance.
func NewUnhandledTracker() *UnhandledTracker {
	return &UnhandledTracker{routes: make(map[string]int)}
}

// Record tracks a method+path combination and logs a warning.
func (t *UnhandledTracker) Record(method, path string) {
	key := method + " " + path
	log.Warn().Str("method", method).Str("path", path).Msg("unhandled route requested")
	t.mu.Lock()
	defer t.mu.Unlock()
	t.routes[key]++
}

// List returns a copy of all unhandled routes seen so far.
func (t *UnhandledTracker) List() map[string]int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	out := make(map[string]int, len(t.routes))
	for k, v := range t.routes {
		out[k] = v
	}
	return out
}

// LogUnhandledRequests returns an http.Handler that records the request
// as unhandled, logs it at WARN level, and returns 501 Not Implemented
// with an Azure-style error payload.
func LogUnhandledRequests(tracker *UnhandledTracker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tracker.Record(r.Method, r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotImplemented)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]interface{}{
				"code":    "NotImplemented",
				"message": "This endpoint is not implemented by azemu. See GET /api/unhandled for details.",
			},
		})
	}
}

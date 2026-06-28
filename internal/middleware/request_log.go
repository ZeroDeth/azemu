package middleware

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// RequestEntry represents a single recorded ARM request.
type RequestEntry struct {
	Timestamp  string `json:"ts"`
	Method     string `json:"method"`
	Path       string `json:"path"`
	Status     int    `json:"status"`
	DurationMs int64  `json:"durationMs"`
}

// RequestRecorder is a ring-buffer that captures ARM request metadata and
// fans out entries to connected SSE clients.
type RequestRecorder struct {
	mu      sync.Mutex
	buf     []RequestEntry
	head    int
	count   int
	clients map[chan RequestEntry]struct{}
}

// NewRequestRecorder creates a recorder with the given ring-buffer capacity.
func NewRequestRecorder(capacity int) *RequestRecorder {
	return &RequestRecorder{
		buf:     make([]RequestEntry, capacity),
		clients: make(map[chan RequestEntry]struct{}),
	}
}

func (rr *RequestRecorder) record(entry RequestEntry) {
	rr.mu.Lock()
	rr.buf[rr.head] = entry
	rr.head = (rr.head + 1) % len(rr.buf)
	if rr.count < len(rr.buf) {
		rr.count++
	}
	clients := make([]chan RequestEntry, 0, len(rr.clients))
	for ch := range rr.clients {
		clients = append(clients, ch)
	}
	rr.mu.Unlock()

	for _, ch := range clients {
		select {
		case ch <- entry:
		default:
		}
	}
}

// Recent returns the last n entries in chronological order.
func (rr *RequestRecorder) Recent(n int) []RequestEntry {
	rr.mu.Lock()
	defer rr.mu.Unlock()
	if n > rr.count {
		n = rr.count
	}
	result := make([]RequestEntry, n)
	start := (rr.head - n + len(rr.buf)) % len(rr.buf)
	for i := 0; i < n; i++ {
		result[i] = rr.buf[(start+i)%len(rr.buf)]
	}
	return result
}

func (rr *RequestRecorder) subscribe() chan RequestEntry {
	ch := make(chan RequestEntry, 64)
	rr.mu.Lock()
	rr.clients[ch] = struct{}{}
	rr.mu.Unlock()
	return ch
}

// subscribeWithBackfill atomically registers the client and captures the
// backfill snapshot under a single lock. This guarantees every entry is
// either in the snapshot or in the channel – never both, never neither.
func (rr *RequestRecorder) subscribeWithBackfill(n int) (chan RequestEntry, []RequestEntry) {
	ch := make(chan RequestEntry, 64)
	rr.mu.Lock()
	rr.clients[ch] = struct{}{}
	if n > rr.count {
		n = rr.count
	}
	recent := make([]RequestEntry, n)
	start := (rr.head - n + len(rr.buf)) % len(rr.buf)
	for i := 0; i < n; i++ {
		recent[i] = rr.buf[(start+i)%len(rr.buf)]
	}
	rr.mu.Unlock()
	return ch, recent
}

func (rr *RequestRecorder) unsubscribe(ch chan RequestEntry) {
	rr.mu.Lock()
	delete(rr.clients, ch)
	rr.mu.Unlock()
	close(ch)
}

// Middleware returns an http middleware that records request metadata.
func (rr *RequestRecorder) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(sw, r)
		rr.record(RequestEntry{
			Timestamp:  start.Format("15:04:05.000"),
			Method:     r.Method,
			Path:       r.URL.Path,
			Status:     sw.status,
			DurationMs: time.Since(start).Milliseconds(),
		})
	})
}

// SSEHandler serves an SSE stream of request log entries. It sends a backfill
// of recent entries on connect, then streams new entries as they arrive.
func (rr *RequestRecorder) SSEHandler(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch, recent := rr.subscribeWithBackfill(500)
	defer rr.unsubscribe(ch)

	for _, entry := range recent {
		data, err := json.Marshal(entry)
		if err != nil {
			continue
		}
		fmt.Fprintf(w, "data: %s\n\n", data)
	}
	flusher.Flush()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case entry, ok := <-ch:
			if !ok {
				return
			}
			data, err := json.Marshal(entry)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}

type statusWriter struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (sw *statusWriter) WriteHeader(code int) {
	if !sw.wrote {
		sw.status = code
		sw.wrote = true
	}
	sw.ResponseWriter.WriteHeader(code)
}

func (sw *statusWriter) Write(b []byte) (int, error) {
	if !sw.wrote {
		sw.wrote = true
	}
	return sw.ResponseWriter.Write(b)
}

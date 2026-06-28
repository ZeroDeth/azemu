package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequestRecorder_Recent(t *testing.T) {
	rr := NewRequestRecorder(5)

	rr.record(RequestEntry{Timestamp: "1", Method: "GET", Path: "/a", Status: 200, DurationMs: 1})
	rr.record(RequestEntry{Timestamp: "2", Method: "PUT", Path: "/b", Status: 201, DurationMs: 2})
	rr.record(RequestEntry{Timestamp: "3", Method: "DELETE", Path: "/c", Status: 202, DurationMs: 3})

	entries := rr.Recent(10)
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
	if entries[0].Timestamp != "1" || entries[2].Timestamp != "3" {
		t.Fatalf("entries not in chronological order: %v", entries)
	}
}

func TestRequestRecorder_RingBuffer(t *testing.T) {
	rr := NewRequestRecorder(3)

	for i := 0; i < 5; i++ {
		rr.record(RequestEntry{Timestamp: string(rune('A' + i)), Method: "GET", Path: "/", Status: 200})
	}

	entries := rr.Recent(3)
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
	if entries[0].Timestamp != "C" || entries[1].Timestamp != "D" || entries[2].Timestamp != "E" {
		t.Fatalf("ring buffer not wrapping correctly: got %v", entries)
	}
}

func TestRequestRecorder_Middleware(t *testing.T) {
	rr := NewRequestRecorder(10)

	handler := rr.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))

	req := httptest.NewRequest("PUT", "/subscriptions/00000000/resourceGroups/rg1", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w.Code)
	}

	entries := rr.Recent(1)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Method != "PUT" || entries[0].Status != 201 {
		t.Fatalf("unexpected entry: %+v", entries[0])
	}
}

func TestRequestRecorder_Subscribe(t *testing.T) {
	rr := NewRequestRecorder(10)
	ch := rr.subscribe()

	go func() {
		rr.record(RequestEntry{Timestamp: "1", Method: "GET", Path: "/test", Status: 200})
	}()

	entry := <-ch
	if entry.Path != "/test" {
		t.Fatalf("expected /test, got %s", entry.Path)
	}

	rr.unsubscribe(ch)
}

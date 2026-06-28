package middleware

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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

// A handler that writes a body without an explicit WriteHeader implies 200.
// This exercises statusWriter.Write's lazy-200 path.
func TestRequestRecorder_Middleware_implicit200(t *testing.T) {
	rr := NewRequestRecorder(10)

	handler := rr.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))

	req := httptest.NewRequest("GET", "/subscriptions/00000000/resourceGroups", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Body.String() != "ok" {
		t.Fatalf("expected body passthrough, got %q", w.Body.String())
	}
	entries := rr.Recent(1)
	if len(entries) != 1 || entries[0].Status != 200 {
		t.Fatalf("expected recorded status 200, got %+v", entries)
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

func TestRequestRecorder_SubscribeWithBackfill_pastEntriesInBackfill(t *testing.T) {
	rr := NewRequestRecorder(10)
	rr.record(RequestEntry{Timestamp: "1", Method: "GET", Path: "/a", Status: 200})
	rr.record(RequestEntry{Timestamp: "2", Method: "PUT", Path: "/b", Status: 201})

	ch, backfill := rr.subscribeWithBackfill(10)
	defer rr.unsubscribe(ch)

	if len(backfill) != 2 {
		t.Fatalf("expected 2 backfilled entries, got %d", len(backfill))
	}
	if backfill[0].Path != "/a" || backfill[1].Path != "/b" {
		t.Fatalf("backfill out of order: %v", backfill)
	}
}

func TestRequestRecorder_SubscribeWithBackfill_futureEntriesOnChannel(t *testing.T) {
	rr := NewRequestRecorder(10)
	rr.record(RequestEntry{Timestamp: "1", Method: "GET", Path: "/a", Status: 200})

	ch, backfill := rr.subscribeWithBackfill(10)
	defer rr.unsubscribe(ch)

	// Entry recorded after subscribe must arrive on the channel, not the
	// backfill snapshot.
	rr.record(RequestEntry{Timestamp: "2", Method: "PUT", Path: "/b", Status: 201})

	if len(backfill) != 1 || backfill[0].Path != "/a" {
		t.Fatalf("backfill should hold only the pre-subscribe entry: %v", backfill)
	}

	select {
	case entry := <-ch:
		if entry.Path != "/b" {
			t.Fatalf("expected /b on channel, got %s", entry.Path)
		}
	case <-time.After(time.Second):
		t.Fatal("post-subscribe entry never arrived on channel")
	}
}

// The subscribe-before-backfill guarantee: an entry recorded concurrently with
// a connecting client lands in exactly one of {backfill, channel}, never both
// and never neither.
func TestRequestRecorder_SubscribeWithBackfill_noLossNoDuplication(t *testing.T) {
	// n stays under the per-client channel buffer (64) so no entry is dropped
	// by record's non-blocking fan-out. This test targets the subscribe/backfill
	// transition (every entry lands in exactly one of backfill/channel), not the
	// separate buffer-overflow backpressure behaviour.
	const n = 50
	rr := NewRequestRecorder(128)

	// The writer races the subscribe/backfill transition: it starts before
	// subscribeWithBackfill and records concurrently, so each entry may land in
	// either the backfill snapshot or the live channel. Every unique id must
	// appear in exactly one of them, never both (duplication) and never neither
	// (loss). Unique ids per entry are what make both failure modes detectable;
	// reusing one id would let a duplicate mask a drop.
	go func() {
		for i := 0; i < n; i++ {
			rr.record(RequestEntry{Timestamp: fmt.Sprintf("id-%d", i), Method: "GET", Path: "/p", Status: 200})
		}
	}()

	// Subscribe while records are still streaming in.
	ch, backfill := rr.subscribeWithBackfill(128)
	defer rr.unsubscribe(ch)

	seen := make(map[string]int, n)
	for _, e := range backfill {
		seen[e.Timestamp]++
	}

	// Drain the channel as entries arrive until every id is accounted for. The
	// split between backfill and channel is nondeterministic, so read until the
	// union is complete rather than assuming a fixed channel count.
	timeout := time.After(5 * time.Second)
	for len(seen) < n {
		select {
		case e := <-ch:
			seen[e.Timestamp]++
		case <-timeout:
			t.Fatalf("only accounted for %d of %d ids before timeout", len(seen), n)
		}
	}

	for i := 0; i < n; i++ {
		id := fmt.Sprintf("id-%d", i)
		switch seen[id] {
		case 1:
			// exactly once: correct
		case 0:
			t.Fatalf("%s was lost (in neither backfill nor channel)", id)
		default:
			t.Fatalf("%s was duplicated (seen %d times)", id, seen[id])
		}
	}
}

func TestRequestRecorder_SubscribeWithBackfill_capsBackfill(t *testing.T) {
	rr := NewRequestRecorder(10)
	for i := 0; i < 5; i++ {
		rr.record(RequestEntry{Timestamp: string(rune('A' + i)), Method: "GET", Path: "/", Status: 200})
	}

	ch, backfill := rr.subscribeWithBackfill(3)
	defer rr.unsubscribe(ch)

	if len(backfill) != 3 {
		t.Fatalf("expected backfill capped at 3, got %d", len(backfill))
	}
	if backfill[0].Timestamp != "C" || backfill[2].Timestamp != "E" {
		t.Fatalf("expected last 3 entries C,D,E, got %v", backfill)
	}
}

// SSEHandler must emit the backfill on connect and then stream entries
// recorded after the client connects, over a real HTTP connection.
func TestRequestRecorder_SSEHandler_backfillThenStream(t *testing.T) {
	rr := NewRequestRecorder(10)
	rr.record(RequestEntry{Timestamp: "seed", Method: "GET", Path: "/seed", Status: 200})

	srv := httptest.NewServer(http.HandlerFunc(rr.SSEHandler))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer resp.Body.Close()

	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("expected SSE content type, got %q", ct)
	}

	type readResult struct {
		line string
		err  error
	}
	lines := make(chan readResult, 8)
	go func() {
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			if line := scanner.Text(); strings.HasPrefix(line, "data:") {
				lines <- readResult{line: line}
			}
		}
		lines <- readResult{err: scanner.Err()}
	}()

	readDataLine := func(want string) {
		t.Helper()
		select {
		case r := <-lines:
			if r.err != nil {
				t.Fatalf("stream read error: %v", r.err)
			}
			if !strings.Contains(r.line, want) {
				t.Fatalf("expected data line containing %q, got %q", want, r.line)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for data line containing %q", want)
		}
	}

	// Backfill arrives first.
	readDataLine(`"path":"/seed"`)

	// An entry recorded after the client connected must stream through.
	rr.record(RequestEntry{Timestamp: "live", Method: "PUT", Path: "/live", Status: 201})
	readDataLine(`"path":"/live"`)
}

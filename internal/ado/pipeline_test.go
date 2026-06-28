package ado

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

func newPipelineTestServer(t *testing.T) (*httptest.Server, *PipelineRunService) {
	t.Helper()
	svc := NewPipelineRunService()
	r := chi.NewRouter()
	svc.PipelineRunRoutes(r)
	return httptest.NewServer(r), svc
}

func TestQueueRun_ReturnsRunWithInProgressState(t *testing.T) {
	srv, _ := newPipelineTestServer(t)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/myorg/myproj/_apis/pipelines/42/runs", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("POST failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	if body["state"] != "inProgress" {
		t.Errorf("expected state inProgress, got %v", body["state"])
	}
	if _, ok := body["result"]; ok {
		t.Errorf("result should not be present for new run, got %v", body["result"])
	}

	pipeline, ok := body["pipeline"].(map[string]interface{})
	if !ok {
		t.Fatal("pipeline field missing or wrong type")
	}
	if pipeline["id"] != float64(42) {
		t.Errorf("expected pipeline id 42, got %v", pipeline["id"])
	}
	if body["id"] != float64(1) {
		t.Errorf("expected run id 1, got %v", body["id"])
	}
}

func TestQueueRun_InvalidPipelineID(t *testing.T) {
	srv, _ := newPipelineTestServer(t)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/o/p/_apis/pipelines/abc/runs", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("POST failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestGetBuild_NotFound(t *testing.T) {
	srv, _ := newPipelineTestServer(t)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/o/p/_apis/build/builds/999")
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestStatusFor_Progression(t *testing.T) {
	tests := []struct {
		elapsed    time.Duration
		wantStatus string
		wantResult string
	}{
		{0, "notStarted", ""},
		{1 * time.Second, "notStarted", ""},
		{2 * time.Second, "inProgress", ""}, // exact lower boundary
		{3 * time.Second, "inProgress", ""},
		{5 * time.Second, "completed", "succeeded"}, // exact upper boundary
		{10 * time.Second, "completed", "succeeded"},
	}

	for _, tt := range tests {
		t.Run(tt.elapsed.String(), func(t *testing.T) {
			status, result := statusFor(tt.elapsed)
			if status != tt.wantStatus {
				t.Errorf("statusFor(%v): status = %q, want %q", tt.elapsed, status, tt.wantStatus)
			}
			if result != tt.wantResult {
				t.Errorf("statusFor(%v): result = %q, want %q", tt.elapsed, result, tt.wantResult)
			}
		})
	}
}

func TestStateFor_Progression(t *testing.T) {
	tests := []struct {
		elapsed    time.Duration
		wantState  string
		wantResult string
	}{
		{0, "inProgress", ""},
		{3 * time.Second, "inProgress", ""},
		{5 * time.Second, "completed", "succeeded"}, // exact transition boundary
		{10 * time.Second, "completed", "succeeded"},
	}

	for _, tt := range tests {
		t.Run(tt.elapsed.String(), func(t *testing.T) {
			state, result := stateFor(tt.elapsed)
			if state != tt.wantState {
				t.Errorf("stateFor(%v): state = %q, want %q", tt.elapsed, state, tt.wantState)
			}
			if result != tt.wantResult {
				t.Errorf("stateFor(%v): result = %q, want %q", tt.elapsed, result, tt.wantResult)
			}
		})
	}
}

func TestGetBuild_StatusDerivedFromElapsed(t *testing.T) {
	srv, svc := newPipelineTestServer(t)
	defer srv.Close()

	// Freeze time at creation
	frozen := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	svc.nowFn = func() time.Time { return frozen }

	resp, err := http.Post(srv.URL+"/o/p/_apis/pipelines/1/runs", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("POST failed: %v", err)
	}
	resp.Body.Close()

	// Advance clock past 5s -> completed
	svc.nowFn = func() time.Time { return frozen.Add(10 * time.Second) }

	resp, err = http.Get(srv.URL + "/o/p/_apis/build/builds/1")
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	defer resp.Body.Close()

	var body map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	if body["status"] != "completed" {
		t.Errorf("expected status completed, got %v", body["status"])
	}
	if body["result"] != "succeeded" {
		t.Errorf("expected result succeeded, got %v", body["result"])
	}
	if _, ok := body["finishTime"]; !ok {
		t.Error("finishTime should be present for completed build")
	}
}

func TestGetBuildLogs_ReturnsMockLogs(t *testing.T) {
	srv, _ := newPipelineTestServer(t)
	defer srv.Close()

	// Create a run first
	resp, err := http.Post(srv.URL+"/o/p/_apis/pipelines/1/runs", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("POST failed: %v", err)
	}
	resp.Body.Close()

	resp, err = http.Get(srv.URL + "/o/p/_apis/build/builds/1/logs")
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	if body["count"] != float64(3) {
		t.Errorf("expected 3 log entries, got %v", body["count"])
	}

	logs, ok := body["value"].([]interface{})
	if !ok || len(logs) != 3 {
		t.Fatalf("expected 3 log entries in value array, got %v", body["value"])
	}
}

func TestGetBuildLogs_NotFound(t *testing.T) {
	srv, _ := newPipelineTestServer(t)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/o/p/_apis/build/builds/999/logs")
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

// TestGetBuild_CrossProjectReturns404 verifies a run created under one
// org/project is invisible under another even when the numeric id is known.
func TestGetBuild_CrossProjectReturns404(t *testing.T) {
	srv, _ := newPipelineTestServer(t)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/orgA/projA/_apis/pipelines/1/runs", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("POST failed: %v", err)
	}
	resp.Body.Close()

	cases := []struct {
		name string
		url  string
	}{
		{"build wrong org", "/orgB/projA/_apis/build/builds/1"},
		{"build wrong project", "/orgA/projB/_apis/build/builds/1"},
		{"logs wrong org", "/orgB/projA/_apis/build/builds/1/logs"},
		{"logs wrong project", "/orgA/projB/_apis/build/builds/1/logs"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := http.Get(srv.URL + tc.url)
			if err != nil {
				t.Fatalf("GET failed: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusNotFound {
				t.Fatalf("expected 404, got %d", resp.StatusCode)
			}
		})
	}

	// Sanity: the owning project still resolves.
	resp, err = http.Get(srv.URL + "/orgA/projA/_apis/build/builds/1")
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for owning project, got %d", resp.StatusCode)
	}
}

// TestGetBuild_InvalidBuildID verifies the 400 contract for non-numeric ids on
// both build endpoints.
func TestGetBuild_InvalidBuildID(t *testing.T) {
	srv, _ := newPipelineTestServer(t)
	defer srv.Close()

	for _, url := range []string{
		"/o/p/_apis/build/builds/abc",
		"/o/p/_apis/build/builds/abc/logs",
	} {
		t.Run(url, func(t *testing.T) {
			resp, err := http.Get(srv.URL + url)
			if err != nil {
				t.Fatalf("GET failed: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d", resp.StatusCode)
			}
		})
	}
}

func TestQueueRun_SequentialIDs(t *testing.T) {
	srv, _ := newPipelineTestServer(t)
	defer srv.Close()

	for i := 1; i <= 3; i++ {
		resp, err := http.Post(srv.URL+"/o/p/_apis/pipelines/1/runs", "application/json", strings.NewReader("{}"))
		if err != nil {
			t.Fatalf("POST %d failed: %v", i, err)
		}
		var body map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("decode %d failed: %v", i, err)
		}
		resp.Body.Close()

		if body["id"] != float64(i) {
			t.Errorf("run %d: expected id %d, got %v", i, i, body["id"])
		}
	}
}

// TestQueueRun_ConcurrentUniqueIDs fires parallel queue requests and asserts
// every returned run ID is unique. Guards the nextID increment under the lock;
// run with -race to catch unsynchronised access.
func TestQueueRun_ConcurrentUniqueIDs(t *testing.T) {
	srv, _ := newPipelineTestServer(t)
	defer srv.Close()

	const n = 50
	ids := make(chan int, n)
	var wg sync.WaitGroup
	wg.Add(n)
	for range n {
		go func() {
			defer wg.Done()
			resp, err := http.Post(srv.URL+"/o/p/_apis/pipelines/1/runs", "application/json", strings.NewReader("{}"))
			if err != nil {
				t.Errorf("POST failed: %v", err)
				return
			}
			defer resp.Body.Close()
			var body map[string]interface{}
			if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
				t.Errorf("decode failed: %v", err)
				return
			}
			ids <- int(body["id"].(float64))
		}()
	}
	wg.Wait()
	close(ids)

	seen := make(map[int]bool, n)
	for id := range ids {
		if seen[id] {
			t.Errorf("duplicate run id %d", id)
		}
		seen[id] = true
	}
	if len(seen) != n {
		t.Errorf("expected %d unique ids, got %d", n, len(seen))
	}
}

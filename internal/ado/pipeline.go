package ado

import (
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"
)

type pipelineRun struct {
	ID         int
	PipelineID int
	Org        string
	Project    string
	CreatedAt  time.Time
}

// statusFor derives build status from elapsed time since creation.
// Pure function for testability: no time.Now() call.
func statusFor(elapsed time.Duration) (status string, result string) {
	switch {
	case elapsed < 2*time.Second:
		return "notStarted", ""
	case elapsed < 5*time.Second:
		return "inProgress", ""
	default:
		return "completed", "succeeded"
	}
}

// stateFor derives pipeline run state from elapsed time.
// Pipelines API uses state/result; Build API uses status/result. The
// Pipelines run state machine has no "notStarted" stage, so anything before
// completion reports "inProgress".
func stateFor(elapsed time.Duration) (state string, result string) {
	if elapsed < 5*time.Second {
		return "inProgress", ""
	}
	return "completed", "succeeded"
}

// PipelineRunService stores and serves ADO pipeline runs.
type PipelineRunService struct {
	mu     sync.RWMutex
	runs   map[int]*pipelineRun
	nextID int
	nowFn  func() time.Time
}

// NewPipelineRunService returns an empty, ready-to-mount service.
func NewPipelineRunService() *PipelineRunService {
	return &PipelineRunService{
		runs:   make(map[int]*pipelineRun),
		nextID: 1,
		nowFn:  time.Now,
	}
}

// PipelineRunRoutes mounts the pipeline run surface on the provided chi.Router.
func (s *PipelineRunService) PipelineRunRoutes(r chi.Router) {
	r.Post("/{organization}/{project}/_apis/pipelines/{pipelineId}/runs", s.queueRun)
	r.Get("/{organization}/{project}/_apis/build/builds/{buildId}", s.getBuild)
	r.Get("/{organization}/{project}/_apis/build/builds/{buildId}/logs", s.getBuildLogs)
}

func (s *PipelineRunService) queueRun(w http.ResponseWriter, r *http.Request) {
	org := chi.URLParam(r, "organization")
	project := chi.URLParam(r, "project")
	pipelineIDStr := chi.URLParam(r, "pipelineId")

	pipelineID, err := strconv.Atoi(pipelineIDStr)
	if err != nil {
		writeADOError(w, http.StatusBadRequest,
			fmt.Sprintf("invalid pipelineId %q: %s", pipelineIDStr, err))
		return
	}

	s.mu.Lock()
	id := s.nextID
	s.nextID++
	run := &pipelineRun{
		ID:         id,
		PipelineID: pipelineID,
		Org:        org,
		Project:    project,
		CreatedAt:  s.nowFn(),
	}
	s.runs[id] = run
	s.mu.Unlock()

	log.Info().
		Str("org", org).
		Str("project", project).
		Int("pipeline_id", pipelineID).
		Int("run_id", id).
		Msg("ADO pipeline run queued")

	state, result := stateFor(0)
	writeADOJSON(w, http.StatusOK, s.pipelineRunResponse(run, state, result))
}

// lookupBuild parses the buildId path param, resolves the run, and snapshots
// the current clock. On any failure it writes the ADO error response and
// returns ok=false. The clock is read under the same lock as the map so a
// test mutating nowFn never races a concurrent handler.
//
// Builds are scoped to their org/project: a run created under one project is
// invisible under another even if the numeric id is guessed, matching how ADO
// isolates builds per project. A mismatch is reported as the same 404 as a
// missing build so the id space is not enumerable across projects.
func (s *PipelineRunService) lookupBuild(w http.ResponseWriter, r *http.Request) (run *pipelineRun, now time.Time, ok bool) {
	org := chi.URLParam(r, "organization")
	project := chi.URLParam(r, "project")
	buildIDStr := chi.URLParam(r, "buildId")

	buildID, err := strconv.Atoi(buildIDStr)
	if err != nil {
		writeADOError(w, http.StatusBadRequest,
			fmt.Sprintf("invalid buildId %q: %s", buildIDStr, err))
		return nil, time.Time{}, false
	}

	s.mu.RLock()
	run, found := s.runs[buildID]
	now = s.nowFn()
	s.mu.RUnlock()

	if !found || run.Org != org || run.Project != project {
		writeADOError(w, http.StatusNotFound,
			fmt.Sprintf("build %d not found in %s/%s", buildID, org, project))
		return nil, time.Time{}, false
	}
	return run, now, true
}

func (s *PipelineRunService) getBuild(w http.ResponseWriter, r *http.Request) {
	run, now, ok := s.lookupBuild(w, r)
	if !ok {
		return
	}

	status, result := statusFor(now.Sub(run.CreatedAt))
	writeADOJSON(w, http.StatusOK, s.buildResponse(run, status, result))
}

func (s *PipelineRunService) getBuildLogs(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := s.lookupBuild(w, r); !ok {
		return
	}

	writeADOJSON(w, http.StatusOK, map[string]interface{}{
		"count": 3,
		"value": []map[string]interface{}{
			{"id": 1, "type": "Container", "lineCount": 5, "createdOn": "2025-01-01T00:00:00Z"},
			{"id": 2, "type": "Container", "lineCount": 12, "createdOn": "2025-01-01T00:00:01Z"},
			{"id": 3, "type": "Container", "lineCount": 3, "createdOn": "2025-01-01T00:00:02Z"},
		},
	})
}

func (s *PipelineRunService) pipelineRunResponse(run *pipelineRun, state, result string) map[string]interface{} {
	resp := map[string]interface{}{
		"id": run.ID,
		"pipeline": map[string]interface{}{
			"id":   run.PipelineID,
			"name": fmt.Sprintf("pipeline-%d", run.PipelineID),
			"url":  fmt.Sprintf("https://dev.azure.com/%s/%s/_apis/pipelines/%d", run.Org, run.Project, run.PipelineID),
		},
		"state":       state,
		"createdDate": run.CreatedAt.UTC().Format(time.RFC3339),
		"url":         fmt.Sprintf("https://dev.azure.com/%s/%s/_apis/pipelines/%d/runs/%d", run.Org, run.Project, run.PipelineID, run.ID),
	}
	if result != "" {
		resp["result"] = result
	}
	return resp
}

func (s *PipelineRunService) buildResponse(run *pipelineRun, status, result string) map[string]interface{} {
	resp := map[string]interface{}{
		"id":          run.ID,
		"buildNumber": fmt.Sprintf("%d.1", run.ID),
		"status":      status,
		"definition": map[string]interface{}{
			"id":   run.PipelineID,
			"name": fmt.Sprintf("pipeline-%d", run.PipelineID),
		},
		"project": map[string]interface{}{
			"name": run.Project,
		},
		"startTime": run.CreatedAt.UTC().Format(time.RFC3339),
		"url":       fmt.Sprintf("https://dev.azure.com/%s/%s/_apis/build/builds/%d", run.Org, run.Project, run.ID),
	}
	if result != "" {
		resp["result"] = result
		resp["finishTime"] = run.CreatedAt.Add(5 * time.Second).UTC().Format(time.RFC3339)
	}
	return resp
}

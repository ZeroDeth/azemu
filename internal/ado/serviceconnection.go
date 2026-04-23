package ado

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

// serviceEndpoint is the in-memory representation of an ADO service endpoint
// (service connection). Only the fields the azuredevops Terraform provider
// reads or writes are stored; everything else is preserved verbatim so
// round-trip PUT → GET fidelity holds.
type serviceEndpoint struct {
	ID            string                 `json:"id"`
	Name          string                 `json:"name"`
	Type          string                 `json:"type"`
	URL           string                 `json:"url"`
	Description   string                 `json:"description"`
	IsShared      bool                   `json:"isShared"`
	IsReady       bool                   `json:"isReady"`
	Owner         string                 `json:"owner"`
	ProjectRefs   []projectRef           `json:"serviceEndpointProjectReferences"`
	Data          map[string]interface{} `json:"data"`
	Authorization map[string]interface{} `json:"authorization"`
}

type projectRef struct {
	ProjectReference projectID              `json:"projectReference"`
	Name             string                 `json:"name"`
	Description      string                 `json:"description"`
	Data             map[string]interface{} `json:"data"`
}

type projectID struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ServiceConnectionService stores and serves ADO service endpoints.
type ServiceConnectionService struct {
	mu        sync.RWMutex
	endpoints map[string]*serviceEndpoint
}

// NewServiceConnectionService returns an empty, ready-to-mount service.
func NewServiceConnectionService() *ServiceConnectionService {
	return &ServiceConnectionService{
		endpoints: make(map[string]*serviceEndpoint),
	}
}

// ServiceConnectionRoutes mounts the service-endpoint surface on the provided
// chi.Router.
func (s *ServiceConnectionService) ServiceConnectionRoutes(r chi.Router) {
	r.Post("/{organization}/{project}/_apis/serviceendpoint/endpoints", s.createEndpoint)
	r.Get("/{organization}/{project}/_apis/serviceendpoint/endpoints/{endpointID}", s.getEndpoint)
	r.Put("/{organization}/{project}/_apis/serviceendpoint/endpoints/{endpointID}", s.updateEndpoint)
	r.Delete("/{organization}/{project}/_apis/serviceendpoint/endpoints/{endpointID}", s.deleteEndpoint)
	r.Get("/{organization}/{project}/_apis/serviceendpoint/endpoints", s.listEndpoints)
}

func (s *ServiceConnectionService) createEndpoint(w http.ResponseWriter, r *http.Request) {
	org := chi.URLParam(r, "organization")
	project := chi.URLParam(r, "project")

	var ep serviceEndpoint
	if err := json.NewDecoder(r.Body).Decode(&ep); err != nil {
		writeADOError(w, http.StatusBadRequest, fmt.Sprintf("invalid request body: %s", err))
		return
	}
	if strings.TrimSpace(ep.Name) == "" {
		writeADOError(w, http.StatusBadRequest, "name is required")
		return
	}

	if strings.TrimSpace(ep.ID) == "" {
		ep.ID = uuid.New().String()
	}
	ep.IsReady = true
	ep.Owner = "Library"

	s.mu.Lock()
	s.endpoints[ep.ID] = &ep
	s.mu.Unlock()

	log.Info().Str("org", org).Str("project", project).Str("endpoint_id", ep.ID).Str("name", ep.Name).Msg("ADO service endpoint created")

	writeADOJSON(w, http.StatusOK, endpointResponse(&ep))
}

func (s *ServiceConnectionService) getEndpoint(w http.ResponseWriter, r *http.Request) {
	org := chi.URLParam(r, "organization")
	project := chi.URLParam(r, "project")
	id := chi.URLParam(r, "endpointID")

	s.mu.RLock()
	ep, ok := s.endpoints[id]
	s.mu.RUnlock()

	if !ok {
		writeADOError(w, http.StatusNotFound,
			fmt.Sprintf("service endpoint %q not found in %s/%s", id, org, project))
		return
	}
	writeADOJSON(w, http.StatusOK, endpointResponse(ep))
}

func (s *ServiceConnectionService) updateEndpoint(w http.ResponseWriter, r *http.Request) {
	org := chi.URLParam(r, "organization")
	project := chi.URLParam(r, "project")
	id := chi.URLParam(r, "endpointID")

	s.mu.RLock()
	_, ok := s.endpoints[id]
	s.mu.RUnlock()

	if !ok {
		writeADOError(w, http.StatusNotFound,
			fmt.Sprintf("service endpoint %q not found in %s/%s", id, org, project))
		return
	}

	var ep serviceEndpoint
	if err := json.NewDecoder(r.Body).Decode(&ep); err != nil {
		writeADOError(w, http.StatusBadRequest, fmt.Sprintf("invalid request body: %s", err))
		return
	}
	ep.ID = id
	ep.IsReady = true
	ep.Owner = "Library"

	s.mu.Lock()
	s.endpoints[id] = &ep
	s.mu.Unlock()

	log.Info().Str("org", org).Str("project", project).Str("endpoint_id", id).Msg("ADO service endpoint updated")

	writeADOJSON(w, http.StatusOK, endpointResponse(&ep))
}

func (s *ServiceConnectionService) deleteEndpoint(w http.ResponseWriter, r *http.Request) {
	org := chi.URLParam(r, "organization")
	project := chi.URLParam(r, "project")
	id := chi.URLParam(r, "endpointID")

	s.mu.Lock()
	_, ok := s.endpoints[id]
	if ok {
		delete(s.endpoints, id)
	}
	s.mu.Unlock()

	if !ok {
		writeADOError(w, http.StatusNotFound,
			fmt.Sprintf("service endpoint %q not found in %s/%s", id, org, project))
		return
	}

	log.Info().Str("org", org).Str("project", project).Str("endpoint_id", id).Msg("ADO service endpoint deleted")
	w.WriteHeader(http.StatusNoContent)
}

func (s *ServiceConnectionService) listEndpoints(w http.ResponseWriter, r *http.Request) {
	org := chi.URLParam(r, "organization")
	project := chi.URLParam(r, "project")

	nameFilter := r.URL.Query().Get("endpointNames")
	var filterNames map[string]bool
	if nameFilter != "" {
		filterNames = make(map[string]bool)
		for _, n := range strings.Split(nameFilter, ",") {
			filterNames[strings.TrimSpace(n)] = true
		}
	}

	s.mu.RLock()
	items := make([]map[string]interface{}, 0, len(s.endpoints))
	for _, ep := range s.endpoints {
		if filterNames != nil && !filterNames[ep.Name] {
			continue
		}
		if !endpointBelongsToProject(ep, project) {
			continue
		}
		items = append(items, endpointResponse(ep))
	}
	s.mu.RUnlock()

	log.Info().Str("org", org).Str("project", project).Int("count", len(items)).Msg("ADO service endpoints listed")

	writeADOJSON(w, http.StatusOK, map[string]interface{}{
		"count": len(items),
		"value": items,
	})
}

// endpointBelongsToProject returns true when the endpoint has no project refs
// (global endpoint) or at least one ref matches the given project name/id.
func endpointBelongsToProject(ep *serviceEndpoint, project string) bool {
	if len(ep.ProjectRefs) == 0 {
		return true
	}
	for _, pr := range ep.ProjectRefs {
		if pr.ProjectReference.ID == project || pr.ProjectReference.Name == project {
			return true
		}
	}
	return false
}

func endpointResponse(ep *serviceEndpoint) map[string]interface{} {
	auth := ep.Authorization
	if auth == nil {
		auth = map[string]interface{}{}
	}
	data := ep.Data
	if data == nil {
		data = map[string]interface{}{}
	}
	refs := ep.ProjectRefs
	if refs == nil {
		refs = []projectRef{}
	}
	return map[string]interface{}{
		"id":                               ep.ID,
		"name":                             ep.Name,
		"type":                             ep.Type,
		"url":                              ep.URL,
		"description":                      ep.Description,
		"isShared":                         ep.IsShared,
		"isReady":                          ep.IsReady,
		"owner":                            ep.Owner,
		"data":                             data,
		"authorization":                    auth,
		"serviceEndpointProjectReferences": refs,
	}
}

// writeADOJSON writes a JSON response with the ADO-style content type.
func writeADOJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Error().Err(err).Msg("writeADOJSON: encode failed")
	}
}

// writeADOError writes a minimal ADO error envelope.
func writeADOError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"$id":            "1",
		"innerException": nil,
		"message":        message,
		"typeName":       "Microsoft.VisualStudio.Services.Common.VssServiceException, Microsoft.VisualStudio.Services.Common",
	}); err != nil {
		log.Error().Err(err).Msg("writeADOError: encode failed")
	}
}

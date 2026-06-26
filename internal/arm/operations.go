package arm

import (
	"fmt"
	"net/http"

	"github.com/google/uuid"
)

// operationResultLocation builds the absolute URL that azurerm's async pollers
// follow after a 202 Accepted DELETE.
//
// It MUST be absolute. The older go-autorest poller (still used by some
// resources, notably CDN) cannot resolve a relative Location and fails
// immediately with `StatusCode=0`. The request's api-version is carried
// through so the subsequent poll satisfies the RequireAPIVersion middleware
// instead of being rejected with 400.
func operationResultLocation(r *http.Request, subID string) string {
	scheme := "https"
	if r.TLS == nil {
		scheme = "http"
	}
	loc := fmt.Sprintf("%s://%s/subscriptions/%s/operationresults/%s",
		scheme, r.Host, subID, uuid.New().String())
	if v := r.URL.Query().Get("api-version"); v != "" {
		loc += "?api-version=" + v
	}
	return loc
}

// getOperationResult serves the async-operation polling endpoint that every
// 202 Accepted DELETE points its Location header at.
//
// azemu performs deletes synchronously: by the time the provider polls, the
// resource is already gone from the store, so the operation is always already
// complete. Report a terminal Succeeded status. Without this endpoint the
// provider polls a dead URL until its delete timeout fires (the
// `polling after Delete: context deadline exceeded` failure that hung every
// scenario teardown).
func (a *Router) getOperationResult(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status": "Succeeded",
	})
}

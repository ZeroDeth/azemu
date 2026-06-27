package arm

import (
	"net/http"
	"strings"
	"testing"
)

// TestOperationResult_GET_ReturnsSucceeded pins the async-operation polling
// endpoint: it always reports a terminal Succeeded status because azemu
// deletes synchronously.
func TestOperationResult_GET_ReturnsSucceeded(t *testing.T) {
	srv := newTestServer(t)

	resp := httpGet(t, srv.URL+"/subscriptions/sub1/operationresults/11111111-1111-1111-1111-111111111111")
	assertStatus(t, resp, http.StatusOK)

	body := decodeJSON(t, resp)
	if body["status"] != "Succeeded" {
		t.Errorf("status = %v, want Succeeded", body["status"])
	}
}

// TestDelete_SetsAbsolutePollableLocation pins the full async-delete contract
// that hung every scenario teardown before the operationresults endpoint
// existed: a 202 DELETE must return an ABSOLUTE Location (carrying
// api-version) that the provider can actually GET to a terminal status.
func TestDelete_SetsAbsolutePollableLocation(t *testing.T) {
	srv := newTestServer(t)
	rgURL := srv.URL + "/subscriptions/sub1/resourcegroups/rg1"

	assertStatus(t, httpPut(t, rgURL, `{"location":"uksouth"}`), http.StatusCreated)

	del := httpDelete(t, rgURL)
	assertStatus(t, del, http.StatusAccepted)

	loc := del.Header.Get("Location")
	if loc == "" {
		t.Fatal("DELETE response missing Location header")
	}
	// Azure-AsyncOperation must also be set (and match): the go-azure-sdk poller
	// prefers it and expects the {"status":"Succeeded"} body the endpoint returns.
	if ao := del.Header.Get("Azure-AsyncOperation"); ao != loc {
		t.Errorf("Azure-AsyncOperation = %q, want it to match Location %q", ao, loc)
	}
	// Must be absolute: the older go-autorest poller cannot resolve a relative
	// Location and fails with StatusCode=0.
	if !strings.HasPrefix(loc, "http://") && !strings.HasPrefix(loc, "https://") {
		t.Errorf("Location is not absolute: %q", loc)
	}
	if !strings.Contains(loc, "/operationresults/") {
		t.Errorf("Location does not point at operationresults: %q", loc)
	}
	// Must carry api-version or the RequireAPIVersion middleware rejects the poll.
	if !strings.Contains(loc, "api-version=") {
		t.Errorf("Location missing api-version: %q", loc)
	}

	// The provider polls the Location verbatim; it must resolve to Succeeded.
	poll := httpGetRaw(t, loc)
	assertStatus(t, poll, http.StatusOK)
	if got := decodeJSON(t, poll)["status"]; got != "Succeeded" {
		t.Errorf("poll status = %v, want Succeeded", got)
	}
}

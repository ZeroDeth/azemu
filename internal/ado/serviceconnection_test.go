package ado

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

func newSCTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	svc := NewServiceConnectionService()
	r := chi.NewRouter()
	svc.ServiceConnectionRoutes(r)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv
}

func scURL(srv *httptest.Server, org, project, id string) string {
	base := fmt.Sprintf("%s/%s/%s/_apis/serviceendpoint/endpoints", srv.URL, org, project)
	if id != "" {
		return base + "/" + id
	}
	return base
}

func scPost(t *testing.T, srv *httptest.Server, org, project, body string) *http.Response {
	t.Helper()
	resp, err := http.Post(scURL(srv, org, project, ""), "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	return resp
}

func scGet(t *testing.T, srv *httptest.Server, org, project, id string) *http.Response {
	t.Helper()
	resp, err := http.Get(scURL(srv, org, project, id))
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	return resp
}

func scPut(t *testing.T, srv *httptest.Server, org, project, id, body string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPut, scURL(srv, org, project, id), bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("PUT request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT: %v", err)
	}
	return resp
}

func scDelete(t *testing.T, srv *httptest.Server, org, project, id string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodDelete, scURL(srv, org, project, id), nil)
	if err != nil {
		t.Fatalf("DELETE request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE: %v", err)
	}
	return resp
}

func decodeADO(t *testing.T, resp *http.Response) map[string]interface{} {
	t.Helper()
	defer resp.Body.Close()
	var m map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		t.Fatalf("decode json: %v", err)
	}
	return m
}

func assertSCStatus(t *testing.T, resp *http.Response, want int) {
	t.Helper()
	if resp.StatusCode != want {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("status = %d, want %d. body=%s", resp.StatusCode, want, string(body))
	}
}

func TestSC_Create_Returns200(t *testing.T) {
	srv := newSCTestServer(t)
	resp := scPost(t, srv, "myorg", "myproject", `{"name":"my-connection","type":"AzureRM"}`)
	assertSCStatus(t, resp, http.StatusOK)
}

func TestSC_Create_MissingName_Returns400(t *testing.T) {
	srv := newSCTestServer(t)
	resp := scPost(t, srv, "myorg", "myproject", `{"type":"AzureRM"}`)
	assertSCStatus(t, resp, http.StatusBadRequest)
}

func TestSC_Create_AutoAssignsID(t *testing.T) {
	srv := newSCTestServer(t)
	resp := scPost(t, srv, "myorg", "myproject", `{"name":"my-connection"}`)
	body := decodeADO(t, resp)
	id, ok := body["id"].(string)
	if !ok || id == "" {
		t.Error("id missing or empty in response")
	}
}

func TestSC_Create_ResponseShape(t *testing.T) {
	srv := newSCTestServer(t)
	resp := scPost(t, srv, "myorg", "myproject", `{
		"name": "my-conn",
		"type": "AzureRM",
		"url": "https://management.azure.com/"
	}`)
	assertSCStatus(t, resp, http.StatusOK)
	body := decodeADO(t, resp)

	if body["name"] != "my-conn" {
		t.Errorf("name = %v, want my-conn", body["name"])
	}
	if body["type"] != "AzureRM" {
		t.Errorf("type = %v, want AzureRM", body["type"])
	}
	if body["isReady"] != true {
		t.Errorf("isReady = %v, want true", body["isReady"])
	}
	if body["owner"] != "Library" {
		t.Errorf("owner = %v, want Library", body["owner"])
	}
}

func TestSC_Get_Returns200(t *testing.T) {
	srv := newSCTestServer(t)
	createResp := scPost(t, srv, "myorg", "myproject", `{"name":"my-conn"}`)
	body := decodeADO(t, createResp)
	id := body["id"].(string)

	resp := scGet(t, srv, "myorg", "myproject", id)
	assertSCStatus(t, resp, http.StatusOK)
}

func TestSC_Get_NotFound_Returns404(t *testing.T) {
	srv := newSCTestServer(t)
	resp := scGet(t, srv, "myorg", "myproject", "nonexistent-id")
	assertSCStatus(t, resp, http.StatusNotFound)
}

func TestSC_Get_NotFound_ADOErrorShape(t *testing.T) {
	srv := newSCTestServer(t)
	resp := scGet(t, srv, "myorg", "myproject", "nonexistent-id")
	body := decodeADO(t, resp)
	if body["message"] == nil {
		t.Error("ADO error envelope missing 'message' field")
	}
}

func TestSC_Update_Returns200(t *testing.T) {
	srv := newSCTestServer(t)
	createResp := scPost(t, srv, "myorg", "myproject", `{"name":"my-conn"}`)
	body := decodeADO(t, createResp)
	id := body["id"].(string)

	resp := scPut(t, srv, "myorg", "myproject", id, `{"name":"my-conn-updated"}`)
	assertSCStatus(t, resp, http.StatusOK)
}

func TestSC_Update_NotFound_Returns404(t *testing.T) {
	srv := newSCTestServer(t)
	resp := scPut(t, srv, "myorg", "myproject", "nonexistent-id", `{"name":"x"}`)
	assertSCStatus(t, resp, http.StatusNotFound)
}

func TestSC_Update_PreservesID(t *testing.T) {
	srv := newSCTestServer(t)
	createResp := scPost(t, srv, "myorg", "myproject", `{"name":"my-conn"}`)
	createBody := decodeADO(t, createResp)
	id := createBody["id"].(string)

	updateResp := scPut(t, srv, "myorg", "myproject", id, `{"name":"updated-name"}`)
	updateBody := decodeADO(t, updateResp)
	if updateBody["id"] != id {
		t.Errorf("id changed after update: got %v, want %v", updateBody["id"], id)
	}
}

func TestSC_Delete_Returns204(t *testing.T) {
	srv := newSCTestServer(t)
	createResp := scPost(t, srv, "myorg", "myproject", `{"name":"my-conn"}`)
	body := decodeADO(t, createResp)
	id := body["id"].(string)

	resp := scDelete(t, srv, "myorg", "myproject", id)
	assertSCStatus(t, resp, http.StatusNoContent)
}

func TestSC_Delete_NotFound_Returns404(t *testing.T) {
	srv := newSCTestServer(t)
	resp := scDelete(t, srv, "myorg", "myproject", "nonexistent-id")
	assertSCStatus(t, resp, http.StatusNotFound)
}

func TestSC_Delete_ThenGet_Returns404(t *testing.T) {
	srv := newSCTestServer(t)
	createResp := scPost(t, srv, "myorg", "myproject", `{"name":"my-conn"}`)
	body := decodeADO(t, createResp)
	id := body["id"].(string)

	scDelete(t, srv, "myorg", "myproject", id)

	resp := scGet(t, srv, "myorg", "myproject", id)
	assertSCStatus(t, resp, http.StatusNotFound)
}

func TestSC_List_ReturnsCreatedEndpoints(t *testing.T) {
	srv := newSCTestServer(t)
	scPost(t, srv, "myorg", "myproject", `{"name":"conn-a"}`)
	scPost(t, srv, "myorg", "myproject", `{"name":"conn-b"}`)

	resp := scGet(t, srv, "myorg", "myproject", "")
	assertSCStatus(t, resp, http.StatusOK)
	body := decodeADO(t, resp)

	count, ok := body["count"].(float64)
	if !ok {
		t.Fatal("count field missing or not a number")
	}
	if int(count) < 2 {
		t.Errorf("count = %d, want >= 2", int(count))
	}
}

func TestSC_List_FilterByName(t *testing.T) {
	srv := newSCTestServer(t)
	scPost(t, srv, "myorg", "myproject", `{"name":"target-conn"}`)
	scPost(t, srv, "myorg", "myproject", `{"name":"other-conn"}`)

	listURL := scURL(srv, "myorg", "myproject", "") + "?endpointNames=target-conn"
	resp, err := http.Get(listURL)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	body := decodeADO(t, resp)
	items, ok := body["value"].([]interface{})
	if !ok {
		t.Fatal("value missing")
	}
	if len(items) != 1 {
		t.Errorf("expected 1 item after name filter, got %d", len(items))
	}
	item := items[0].(map[string]interface{})
	if item["name"] != "target-conn" {
		t.Errorf("filtered item name = %v, want target-conn", item["name"])
	}
}

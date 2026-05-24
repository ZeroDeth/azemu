package arm

import (
	"fmt"
	"net/http"
	"testing"
)

func storageContainerURL(srvURL, sub, rg, account, container string) string {
	return fmt.Sprintf(
		"%s/subscriptions/%s/resourcegroups/%s/providers/microsoft.storage/storageaccounts/%s/blobservices/default/containers/%s",
		srvURL, sub, rg, account, container,
	)
}

func storageContainerListURL(srvURL, sub, rg, account string) string {
	return fmt.Sprintf(
		"%s/subscriptions/%s/resourcegroups/%s/providers/microsoft.storage/storageaccounts/%s/blobservices/default/containers",
		srvURL, sub, rg, account,
	)
}

const containerBody = `{"properties": {"publicAccess": "None"}}`

// createStorageAccount is a helper that creates a storage account and fails the
// test if the PUT does not return 200 (storage accounts return 200 on both
// create and update — azurerm's storageaccounts.Create accepts 200 or 202 only).
func createStorageAccount(t *testing.T, srvURL, sub, rg, name string) {
	t.Helper()
	resp := httpPut(t, storageAccountURL(srvURL, sub, rg, name), storageAccountBodyLRS)
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()
}

func TestStorageContainer_PUT_Creates_Returns201(t *testing.T) {
	srv := newTestServer(t)
	createStorageAccount(t, srv.URL, "sub1", "rg1", "containeracct")

	resp := httpPut(t, storageContainerURL(srv.URL, "sub1", "rg1", "containeracct", "mycontainer"), containerBody)
	assertStatus(t, resp, http.StatusCreated)

	body := decodeJSON(t, resp)
	if body["name"] != "mycontainer" {
		t.Errorf("name = %v, want mycontainer", body["name"])
	}
	if body["type"] != storageContainerTypeString {
		t.Errorf("type = %v, want %s", body["type"], storageContainerTypeString)
	}
	wantID := "/subscriptions/sub1/resourceGroups/rg1/providers/Microsoft.Storage/storageAccounts/containeracct/blobServices/default/containers/mycontainer"
	if body["id"] != wantID {
		t.Errorf("id = %v, want %s", body["id"], wantID)
	}

	props, ok := body["properties"].(map[string]interface{})
	if !ok {
		t.Fatalf("properties missing or wrong type: %T", body["properties"])
	}
	if props["provisioningState"] != "Succeeded" {
		t.Errorf("provisioningState = %v, want Succeeded", props["provisioningState"])
	}
	if props["publicAccess"] != "None" {
		t.Errorf("publicAccess = %v, want None", props["publicAccess"])
	}
	if props["leaseStatus"] != "Unlocked" {
		t.Errorf("leaseStatus = %v, want Unlocked", props["leaseStatus"])
	}
	if props["leaseState"] != "Available" {
		t.Errorf("leaseState = %v, want Available", props["leaseState"])
	}
}

func TestStorageContainer_PUT_Update_Returns200(t *testing.T) {
	srv := newTestServer(t)
	createStorageAccount(t, srv.URL, "sub1", "rg1", "updateacct")

	url := storageContainerURL(srv.URL, "sub1", "rg1", "updateacct", "updatecont")
	resp := httpPut(t, url, containerBody)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = httpPut(t, url, containerBody)
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()
}

func TestStorageContainer_PUT_ParentNotFound_Returns404(t *testing.T) {
	srv := newTestServer(t)
	// No storage account created — parent existence check must fail.
	resp := httpPut(t, storageContainerURL(srv.URL, "sub1", "rg1", "missingacct", "orphan"), containerBody)
	assertStatus(t, resp, http.StatusNotFound)

	body := decodeJSON(t, resp)
	errObj, ok := body["error"].(map[string]interface{})
	if !ok {
		t.Fatalf("error field missing: %v", body)
	}
	if errObj["code"] != "ParentResourceNotFound" {
		t.Errorf("error.code = %v, want ParentResourceNotFound", errObj["code"])
	}
}

func TestStorageContainer_GET_Returns200(t *testing.T) {
	srv := newTestServer(t)
	createStorageAccount(t, srv.URL, "sub1", "rg1", "getacct")

	url := storageContainerURL(srv.URL, "sub1", "rg1", "getacct", "getcont")
	resp := httpPut(t, url, containerBody)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = httpGet(t, url)
	assertStatus(t, resp, http.StatusOK)
	body := decodeJSON(t, resp)
	if body["name"] != "getcont" {
		t.Errorf("name = %v, want getcont", body["name"])
	}
}

func TestStorageContainer_GET_NotFound_Returns404(t *testing.T) {
	srv := newTestServer(t)
	createStorageAccount(t, srv.URL, "sub1", "rg1", "getnotfoundacct")

	resp := httpGet(t, storageContainerURL(srv.URL, "sub1", "rg1", "getnotfoundacct", "notexist"))
	assertStatus(t, resp, http.StatusNotFound)

	body := decodeJSON(t, resp)
	errObj, ok := body["error"].(map[string]interface{})
	if !ok {
		t.Fatalf("error field missing: %v", body)
	}
	if errObj["code"] != "ResourceNotFound" {
		t.Errorf("error.code = %v, want ResourceNotFound", errObj["code"])
	}
}

func TestStorageContainer_HEAD_Exists_Returns204(t *testing.T) {
	srv := newTestServer(t)
	createStorageAccount(t, srv.URL, "sub1", "rg1", "headacct")

	url := storageContainerURL(srv.URL, "sub1", "rg1", "headacct", "headcont")
	resp := httpPut(t, url, containerBody)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = httpHead(t, url)
	assertStatus(t, resp, http.StatusNoContent)
	if body := readBody(t, resp); body != "" {
		t.Errorf("HEAD response body = %q, want empty", body)
	}
}

func TestStorageContainer_HEAD_NotFound_Returns404(t *testing.T) {
	srv := newTestServer(t)
	createStorageAccount(t, srv.URL, "sub1", "rg1", "headnotfoundacct")

	resp := httpHead(t, storageContainerURL(srv.URL, "sub1", "rg1", "headnotfoundacct", "notexist"))
	assertStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
}

func TestStorageContainer_DELETE_Returns200(t *testing.T) {
	srv := newTestServer(t)
	createStorageAccount(t, srv.URL, "sub1", "rg1", "delacct")

	url := storageContainerURL(srv.URL, "sub1", "rg1", "delacct", "delcont")
	resp := httpPut(t, url, containerBody)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = httpDelete(t, url)
	// azurerm's containers.Client#Delete expects 200 OK (synchronous delete).
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()
}

func TestStorageContainer_DELETE_NotFound_Returns404(t *testing.T) {
	srv := newTestServer(t)
	createStorageAccount(t, srv.URL, "sub1", "rg1", "delnotfoundacct")

	resp := httpDelete(t, storageContainerURL(srv.URL, "sub1", "rg1", "delnotfoundacct", "notexist"))
	assertStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
}

func TestStorageContainer_DELETE_SubsequentGET_Returns404(t *testing.T) {
	srv := newTestServer(t)
	createStorageAccount(t, srv.URL, "sub1", "rg1", "goneacct")

	url := storageContainerURL(srv.URL, "sub1", "rg1", "goneacct", "gonecont")
	resp := httpPut(t, url, containerBody)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = httpDelete(t, url)
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	resp = httpGet(t, url)
	assertStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
}

func TestStorageContainer_LIST_Returns200(t *testing.T) {
	srv := newTestServer(t)
	createStorageAccount(t, srv.URL, "sub1", "rg1", "listcontacct")

	for _, name := range []string{"cont-a", "cont-b", "cont-c"} {
		resp := httpPut(t, storageContainerURL(srv.URL, "sub1", "rg1", "listcontacct", name), containerBody)
		assertStatus(t, resp, http.StatusCreated)
		resp.Body.Close()
	}

	resp := httpGet(t, storageContainerListURL(srv.URL, "sub1", "rg1", "listcontacct"))
	assertStatus(t, resp, http.StatusOK)
	body := decodeJSON(t, resp)
	items, ok := body["value"].([]interface{})
	if !ok {
		t.Fatalf("value field missing: %T", body["value"])
	}
	if len(items) != 3 {
		t.Errorf("len(value) = %d, want 3", len(items))
	}
}

func TestStorageAccount_DELETE_CascadesContainers(t *testing.T) {
	srv := newTestServer(t)
	createStorageAccount(t, srv.URL, "sub1", "rg1", "cascadeacct")

	// Create two containers under the account.
	for _, name := range []string{"alpha", "beta"} {
		resp := httpPut(t, storageContainerURL(srv.URL, "sub1", "rg1", "cascadeacct", name), containerBody)
		assertStatus(t, resp, http.StatusCreated)
		resp.Body.Close()
	}

	// Delete the storage account.
	resp := httpDelete(t, storageAccountURL(srv.URL, "sub1", "rg1", "cascadeacct"))
	assertStatus(t, resp, http.StatusAccepted)
	resp.Body.Close()

	// Both containers must be gone.
	for _, name := range []string{"alpha", "beta"} {
		resp = httpGet(t, storageContainerURL(srv.URL, "sub1", "rg1", "cascadeacct", name))
		assertStatus(t, resp, http.StatusNotFound)
		resp.Body.Close()
	}
}

func TestStorageContainer_DefaultPublicAccess_WhenOmitted(t *testing.T) {
	srv := newTestServer(t)
	createStorageAccount(t, srv.URL, "sub1", "rg1", "defpublicacct")

	resp := httpPut(t, storageContainerURL(srv.URL, "sub1", "rg1", "defpublicacct", "defcont"),
		`{}`)
	assertStatus(t, resp, http.StatusCreated)

	body := decodeJSON(t, resp)
	props := body["properties"].(map[string]interface{})
	if props["publicAccess"] != "None" {
		t.Errorf("publicAccess = %v, want None (default when omitted)", props["publicAccess"])
	}
	resp.Body.Close()
}

func TestStorageContainer_AzureHeaders_Present(t *testing.T) {
	srv := newTestServer(t)
	createStorageAccount(t, srv.URL, "sub1", "rg1", "headercontacct")

	resp := httpPut(t, storageContainerURL(srv.URL, "sub1", "rg1", "headercontacct", "headercont"), containerBody)
	if resp.Header.Get("x-ms-request-id") == "" {
		t.Error("x-ms-request-id header missing")
	}
	resp.Body.Close()
}

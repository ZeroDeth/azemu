package arm

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func aksClusterURL(srv *httptest.Server, name string) string {
	return fmt.Sprintf("%s/subscriptions/%s/resourcegroups/%s/providers/microsoft.containerservice/managedclusters/%s",
		srv.URL, testSubID, testRGName, name)
}

func aksNodePoolURL(srv *httptest.Server, clusterName, poolName string) string {
	return fmt.Sprintf("%s/subscriptions/%s/resourcegroups/%s/providers/microsoft.containerservice/managedclusters/%s/agentpools/%s",
		srv.URL, testSubID, testRGName, clusterName, poolName)
}

// --- AKS Cluster tests ---

func TestAKSCluster_PUT_Creates_Returns201(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, aksClusterURL(srv, "my-cluster"), `{"location":"uksouth"}`)
	assertStatus(t, resp, http.StatusCreated)
}

func TestAKSCluster_PUT_Update_Returns200(t *testing.T) {
	srv := newTestServer(t)
	httpPut(t, aksClusterURL(srv, "my-cluster"), `{"location":"uksouth"}`)
	resp := httpPut(t, aksClusterURL(srv, "my-cluster"), `{"location":"uksouth"}`)
	assertStatus(t, resp, http.StatusOK)
}

func TestAKSCluster_PUT_MissingLocation_Returns400(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, aksClusterURL(srv, "my-cluster"), `{}`)
	assertStatus(t, resp, http.StatusBadRequest)
}

func TestAKSCluster_PUT_ResponseShape(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, aksClusterURL(srv, "my-cluster"), `{
		"location": "uksouth",
		"properties": {"kubernetesVersion": "1.30.0"},
		"sku": {"name": "Base", "tier": "Standard"}
	}`)
	assertStatus(t, resp, http.StatusCreated)
	body := decodeJSON(t, resp)

	if body["id"] == nil {
		t.Error("id missing")
	}
	if body["name"] != "my-cluster" {
		t.Errorf("name = %v, want my-cluster", body["name"])
	}
	if body["type"] != "Microsoft.ContainerService/managedClusters" {
		t.Errorf("type = %v", body["type"])
	}
	if body["location"] != "uksouth" {
		t.Errorf("location = %v, want uksouth", body["location"])
	}

	props, ok := body["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("properties missing")
	}
	if props["provisioningState"] != "Succeeded" {
		t.Errorf("provisioningState = %v", props["provisioningState"])
	}
	if props["kubernetesVersion"] != "1.30.0" {
		t.Errorf("kubernetesVersion = %v, want 1.30.0", props["kubernetesVersion"])
	}
	if props["fqdn"] == nil || props["fqdn"] == "" {
		t.Error("fqdn missing")
	}

	sku, ok := body["sku"].(map[string]interface{})
	if !ok {
		t.Fatal("sku missing")
	}
	if sku["tier"] != "Standard" {
		t.Errorf("sku.tier = %v, want Standard", sku["tier"])
	}
}

func TestAKSCluster_PUT_DefaultKubernetesVersion(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, aksClusterURL(srv, "my-cluster"), `{"location":"uksouth"}`)
	body := decodeJSON(t, resp)
	props := body["properties"].(map[string]interface{})
	if props["kubernetesVersion"] != "1.29.0" {
		t.Errorf("default kubernetesVersion = %v, want 1.29.0", props["kubernetesVersion"])
	}
}

func TestAKSCluster_PUT_WithIdentity(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, aksClusterURL(srv, "my-cluster"), `{
		"location": "uksouth",
		"identity": {"type": "SystemAssigned"}
	}`)
	assertStatus(t, resp, http.StatusCreated)
	body := decodeJSON(t, resp)
	if body["identity"] == nil {
		t.Error("identity missing from response")
	}
}

func TestAKSCluster_GET_Returns200(t *testing.T) {
	srv := newTestServer(t)
	httpPut(t, aksClusterURL(srv, "my-cluster"), `{"location":"uksouth"}`)
	resp := httpGet(t, aksClusterURL(srv, "my-cluster"))
	assertStatus(t, resp, http.StatusOK)
}

func TestAKSCluster_GET_NotFound_Returns404(t *testing.T) {
	srv := newTestServer(t)
	resp := httpGet(t, aksClusterURL(srv, "nonexistent"))
	assertStatus(t, resp, http.StatusNotFound)
}

func TestAKSCluster_HEAD_Exists_Returns204(t *testing.T) {
	srv := newTestServer(t)
	httpPut(t, aksClusterURL(srv, "my-cluster"), `{"location":"uksouth"}`)
	resp := httpHead(t, aksClusterURL(srv, "my-cluster"))
	assertStatus(t, resp, http.StatusNoContent)
	body := readBody(t, resp)
	if body != "" {
		t.Errorf("HEAD body should be empty, got %q", body)
	}
}

func TestAKSCluster_HEAD_NotFound_Returns404(t *testing.T) {
	srv := newTestServer(t)
	resp := httpHead(t, aksClusterURL(srv, "nonexistent"))
	assertStatus(t, resp, http.StatusNotFound)
}

func TestAKSCluster_DELETE_Returns202(t *testing.T) {
	srv := newTestServer(t)
	httpPut(t, aksClusterURL(srv, "my-cluster"), `{"location":"uksouth"}`)
	resp := httpDelete(t, aksClusterURL(srv, "my-cluster"))
	assertStatus(t, resp, http.StatusAccepted)
	if resp.Header.Get("Location") == "" {
		t.Error("DELETE missing Location header")
	}
}

func TestAKSCluster_DELETE_NotFound_Returns404(t *testing.T) {
	srv := newTestServer(t)
	resp := httpDelete(t, aksClusterURL(srv, "nonexistent"))
	assertStatus(t, resp, http.StatusNotFound)
}

func TestAKSCluster_DELETE_CascadesNodePools(t *testing.T) {
	srv := newTestServer(t)
	httpPut(t, aksClusterURL(srv, "my-cluster"), `{"location":"uksouth"}`)
	httpPut(t, aksNodePoolURL(srv, "my-cluster", "pool1"), `{"properties":{}}`)
	httpPut(t, aksNodePoolURL(srv, "my-cluster", "pool2"), `{"properties":{}}`)

	httpDelete(t, aksClusterURL(srv, "my-cluster"))

	// Both node pools should now be gone
	resp1 := httpGet(t, aksNodePoolURL(srv, "my-cluster", "pool1"))
	assertStatus(t, resp1, http.StatusNotFound)

	resp2 := httpGet(t, aksNodePoolURL(srv, "my-cluster", "pool2"))
	assertStatus(t, resp2, http.StatusNotFound)
}

func TestAKSCluster_ListByRG(t *testing.T) {
	srv := newTestServer(t)
	httpPut(t, aksClusterURL(srv, "cluster-a"), `{"location":"uksouth"}`)
	httpPut(t, aksClusterURL(srv, "cluster-b"), `{"location":"uksouth"}`)

	listURL := fmt.Sprintf("%s/subscriptions/%s/resourcegroups/%s/providers/microsoft.containerservice/managedclusters",
		srv.URL, testSubID, testRGName)
	resp := httpGet(t, listURL)
	assertStatus(t, resp, http.StatusOK)
	body := decodeJSON(t, resp)
	items, ok := body["value"].([]interface{})
	if !ok {
		t.Fatal("value missing or not array")
	}
	if len(items) < 2 {
		t.Errorf("expected at least 2 clusters, got %d", len(items))
	}
}

func TestAKSCluster_ListBySub(t *testing.T) {
	srv := newTestServer(t)
	httpPut(t, aksClusterURL(srv, "cluster-a"), `{"location":"uksouth"}`)

	listURL := fmt.Sprintf("%s/subscriptions/%s/providers/microsoft.containerservice/managedclusters",
		srv.URL, testSubID)
	resp := httpGet(t, listURL)
	assertStatus(t, resp, http.StatusOK)
	body := decodeJSON(t, resp)
	if body["value"] == nil {
		t.Error("value missing")
	}
}

// --- AKS Node Pool tests ---

func TestAKSNodePool_PUT_Creates_Returns201(t *testing.T) {
	srv := newTestServer(t)
	httpPut(t, aksClusterURL(srv, "my-cluster"), `{"location":"uksouth"}`)
	resp := httpPut(t, aksNodePoolURL(srv, "my-cluster", "nodepool1"), `{"properties":{}}`)
	assertStatus(t, resp, http.StatusCreated)
}

func TestAKSNodePool_PUT_Update_Returns200(t *testing.T) {
	srv := newTestServer(t)
	httpPut(t, aksClusterURL(srv, "my-cluster"), `{"location":"uksouth"}`)
	httpPut(t, aksNodePoolURL(srv, "my-cluster", "nodepool1"), `{"properties":{}}`)
	resp := httpPut(t, aksNodePoolURL(srv, "my-cluster", "nodepool1"), `{"properties":{}}`)
	assertStatus(t, resp, http.StatusOK)
}

func TestAKSNodePool_PUT_ParentNotFound_Returns404(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, aksNodePoolURL(srv, "nonexistent-cluster", "nodepool1"), `{"properties":{}}`)
	assertStatus(t, resp, http.StatusNotFound)
	body := decodeJSON(t, resp)
	errObj, ok := body["error"].(map[string]interface{})
	if !ok {
		t.Fatal("error field missing")
	}
	if errObj["code"] != "ParentResourceNotFound" {
		t.Errorf("error.code = %v, want ParentResourceNotFound", errObj["code"])
	}
}

func TestAKSNodePool_PUT_ResponseShape(t *testing.T) {
	srv := newTestServer(t)
	httpPut(t, aksClusterURL(srv, "my-cluster"), `{"location":"uksouth"}`)
	resp := httpPut(t, aksNodePoolURL(srv, "my-cluster", "nodepool1"), `{
		"properties": {"vmSize": "Standard_D4s_v3", "count": 3, "osType": "Linux"}
	}`)
	assertStatus(t, resp, http.StatusCreated)
	body := decodeJSON(t, resp)

	if body["name"] != "nodepool1" {
		t.Errorf("name = %v, want nodepool1", body["name"])
	}
	if body["type"] != "Microsoft.ContainerService/managedClusters/agentPools" {
		t.Errorf("type = %v", body["type"])
	}

	props := body["properties"].(map[string]interface{})
	if props["provisioningState"] != "Succeeded" {
		t.Errorf("provisioningState = %v", props["provisioningState"])
	}
	if props["vmSize"] != "Standard_D4s_v3" {
		t.Errorf("vmSize = %v", props["vmSize"])
	}
}

func TestAKSNodePool_PUT_InheritsClusterLocation(t *testing.T) {
	srv := newTestServer(t)
	httpPut(t, aksClusterURL(srv, "my-cluster"), `{"location":"eastus"}`)
	resp := httpPut(t, aksNodePoolURL(srv, "my-cluster", "nodepool1"), `{"properties":{}}`)
	body := decodeJSON(t, resp)
	if body["location"] != "eastus" {
		t.Errorf("location = %v, want eastus (inherited from cluster)", body["location"])
	}
}

func TestAKSNodePool_GET_Returns200(t *testing.T) {
	srv := newTestServer(t)
	httpPut(t, aksClusterURL(srv, "my-cluster"), `{"location":"uksouth"}`)
	httpPut(t, aksNodePoolURL(srv, "my-cluster", "nodepool1"), `{"properties":{}}`)
	resp := httpGet(t, aksNodePoolURL(srv, "my-cluster", "nodepool1"))
	assertStatus(t, resp, http.StatusOK)
}

func TestAKSNodePool_GET_NotFound_Returns404(t *testing.T) {
	srv := newTestServer(t)
	httpPut(t, aksClusterURL(srv, "my-cluster"), `{"location":"uksouth"}`)
	resp := httpGet(t, aksNodePoolURL(srv, "my-cluster", "nonexistent"))
	assertStatus(t, resp, http.StatusNotFound)
}

func TestAKSNodePool_HEAD_Exists_Returns204(t *testing.T) {
	srv := newTestServer(t)
	httpPut(t, aksClusterURL(srv, "my-cluster"), `{"location":"uksouth"}`)
	httpPut(t, aksNodePoolURL(srv, "my-cluster", "nodepool1"), `{"properties":{}}`)
	resp := httpHead(t, aksNodePoolURL(srv, "my-cluster", "nodepool1"))
	assertStatus(t, resp, http.StatusNoContent)
}

func TestAKSNodePool_HEAD_NotFound_Returns404(t *testing.T) {
	srv := newTestServer(t)
	httpPut(t, aksClusterURL(srv, "my-cluster"), `{"location":"uksouth"}`)
	resp := httpHead(t, aksNodePoolURL(srv, "my-cluster", "nonexistent"))
	assertStatus(t, resp, http.StatusNotFound)
}

func TestAKSNodePool_DELETE_Returns202(t *testing.T) {
	srv := newTestServer(t)
	httpPut(t, aksClusterURL(srv, "my-cluster"), `{"location":"uksouth"}`)
	httpPut(t, aksNodePoolURL(srv, "my-cluster", "nodepool1"), `{"properties":{}}`)
	resp := httpDelete(t, aksNodePoolURL(srv, "my-cluster", "nodepool1"))
	assertStatus(t, resp, http.StatusAccepted)
	if resp.Header.Get("Location") == "" {
		t.Error("DELETE missing Location header")
	}
}

func TestAKSNodePool_DELETE_NotFound_Returns404(t *testing.T) {
	srv := newTestServer(t)
	httpPut(t, aksClusterURL(srv, "my-cluster"), `{"location":"uksouth"}`)
	resp := httpDelete(t, aksNodePoolURL(srv, "my-cluster", "nonexistent"))
	assertStatus(t, resp, http.StatusNotFound)
}

func TestAKSNodePool_List(t *testing.T) {
	srv := newTestServer(t)
	httpPut(t, aksClusterURL(srv, "my-cluster"), `{"location":"uksouth"}`)
	httpPut(t, aksNodePoolURL(srv, "my-cluster", "pool1"), `{"properties":{}}`)
	httpPut(t, aksNodePoolURL(srv, "my-cluster", "pool2"), `{"properties":{}}`)

	listURL := fmt.Sprintf("%s/subscriptions/%s/resourcegroups/%s/providers/microsoft.containerservice/managedclusters/my-cluster/agentpools",
		srv.URL, testSubID, testRGName)
	resp := httpGet(t, listURL)
	assertStatus(t, resp, http.StatusOK)
	body := decodeJSON(t, resp)
	items, ok := body["value"].([]interface{})
	if !ok {
		t.Fatal("value missing or not array")
	}
	if len(items) != 2 {
		t.Errorf("expected 2 node pools, got %d", len(items))
	}
}

func TestAKSCluster_MissingAPIVersion_Returns400(t *testing.T) {
	srv := newTestServer(t)
	resp, err := http.Get(aksClusterURL(srv, "my-cluster"))
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	assertStatus(t, resp, http.StatusBadRequest)
}

//go:build integration

// Package integration exercises the ARM router through the same middleware
// stack used by cmd/azemu/main.go, but in-process via httptest. This is the
// closest we get to end-to-end coverage without requiring a real TCP listener,
// TLS trust, or a running Terraform CLI.
//
// Tests in this package are guarded by the `integration` build tag so they
// are not picked up by a bare `go test ./...`. Run them with:
//
//	go test ./test/integration/... -tags=integration -race -count=1
package integration

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/zerodeth/azemu/internal/arm"
	mw "github.com/zerodeth/azemu/internal/middleware"
	"github.com/zerodeth/azemu/internal/store"
)

// buildFullServer assembles a chi router with the same middleware stack as
// cmd/azemu/main.go (minus TLS and OAuth, which are orthogonal to ARM
// routing) and wraps it in an httptest.Server. Each call produces a fresh
// MemoryStore so tests are isolated.
func buildFullServer(t *testing.T) *httptest.Server {
	t.Helper()
	s := store.NewMemoryStore()
	ar := arm.NewRouter(s)
	r := chi.NewRouter()
	r.Use(mw.NormalizePath)
	r.Use(mw.AzureHeaders)
	r.Use(mw.RequireAPIVersion)
	r.Route("/subscriptions", ar.Routes)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv
}

const apiVersionQ = "?api-version=2023-09-01"

func doJSON(t *testing.T, method, url, body string) *http.Response {
	t.Helper()
	var rdr io.Reader
	if body != "" {
		rdr = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, url, rdr)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do %s %s: %v", method, url, err)
	}
	return resp
}

func mustStatus(t *testing.T, resp *http.Response, want int) {
	t.Helper()
	if resp.StatusCode != want {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("status = %d, want %d. body=%s", resp.StatusCode, want, string(b))
	}
}

func decode(t *testing.T, resp *http.Response) map[string]interface{} {
	t.Helper()
	defer resp.Body.Close()
	var m map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		buf := &bytes.Buffer{}
		buf.ReadFrom(resp.Body)
		t.Fatalf("decode json: %v", err)
	}
	return m
}

// TestARM_VNetSubnetFullFlow exercises the canonical Terraform lifecycle:
// create an RG, create a vnet, create subnets, read everything back, delete
// the vnet, and confirm the subnets cascaded away while the RG remains.
func TestARM_VNetSubnetFullFlow(t *testing.T) {
	srv := buildFullServer(t)
	base := srv.URL

	rgURL := base + "/subscriptions/sub1/resourcegroups/rg1" + apiVersionQ
	vnetURL := base + "/subscriptions/sub1/resourcegroups/rg1/providers/microsoft.network/virtualnetworks/vnet1" + apiVersionQ
	sub1URL := base + "/subscriptions/sub1/resourcegroups/rg1/providers/microsoft.network/virtualnetworks/vnet1/subnets/sub-a" + apiVersionQ
	sub2URL := base + "/subscriptions/sub1/resourcegroups/rg1/providers/microsoft.network/virtualnetworks/vnet1/subnets/sub-b" + apiVersionQ

	// 1. Create resource group.
	resp := doJSON(t, http.MethodPut, rgURL, `{"location":"uksouth"}`)
	mustStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	// 2. Create vnet.
	resp = doJSON(t, http.MethodPut, vnetURL,
		`{"location":"uksouth","properties":{"addressSpace":{"addressPrefixes":["10.0.0.0/16"]}}}`)
	mustStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	// 3. Create two subnets.
	resp = doJSON(t, http.MethodPut, sub1URL, `{"properties":{"addressPrefix":"10.0.1.0/24"}}`)
	mustStatus(t, resp, http.StatusCreated)
	resp.Body.Close()
	resp = doJSON(t, http.MethodPut, sub2URL, `{"properties":{"addressPrefix":"10.0.2.0/24"}}`)
	mustStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	// 4. GET vnet and confirm both subnets are embedded in the response.
	resp = doJSON(t, http.MethodGet, vnetURL, "")
	mustStatus(t, resp, http.StatusOK)
	body := decode(t, resp)
	props, ok := body["properties"].(map[string]interface{})
	if !ok {
		t.Fatalf("properties missing: %v", body)
	}
	subnets, ok := props["subnets"].([]interface{})
	if !ok {
		t.Fatalf("subnets missing: %v", props)
	}
	if len(subnets) != 2 {
		t.Fatalf("len(subnets) = %d, want 2", len(subnets))
	}

	// 5. Delete vnet (cascade semantics).
	resp = doJSON(t, http.MethodDelete, vnetURL, "")
	mustStatus(t, resp, http.StatusAccepted)
	resp.Body.Close()

	// 6. Child subnet must be 404 after cascade.
	resp = doJSON(t, http.MethodGet, sub1URL, "")
	mustStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()

	// 7. Parent RG must still be intact.
	resp = doJSON(t, http.MethodGet, rgURL, "")
	mustStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	// 8. Azure request headers must be stamped on every response (middleware).
	if got := resp.Header.Get("x-ms-request-id"); got == "" {
		// Header assertion is intentionally on the last captured response;
		// any earlier one would do just as well.
		t.Errorf("x-ms-request-id header missing")
	}
}

// TestARM_PublicIPFullFlow exercises the Public IP lifecycle: create, read,
// verify SKU and fake ipAddress are present, update (idempotent PUT), delete,
// confirm 404.
func TestARM_PublicIPFullFlow(t *testing.T) {
	srv := buildFullServer(t)
	base := srv.URL

	pipURL := base + "/subscriptions/sub1/resourcegroups/rg1/providers/microsoft.network/publicipaddresses/pip1" + apiVersionQ
	listURL := base + "/subscriptions/sub1/resourcegroups/rg1/providers/microsoft.network/publicipaddresses" + apiVersionQ

	pipBody := `{
		"location": "uksouth",
		"sku": {"name": "Standard"},
		"properties": {
			"publicIPAllocationMethod": "Static",
			"publicIPAddressVersion": "IPv4"
		}
	}`

	// 1. Create.
	resp := doJSON(t, http.MethodPut, pipURL, pipBody)
	mustStatus(t, resp, http.StatusCreated)
	body := decode(t, resp)

	props, ok := body["properties"].(map[string]interface{})
	if !ok {
		t.Fatalf("properties missing: %v", body)
	}
	if props["provisioningState"] != "Succeeded" {
		t.Errorf("provisioningState = %v, want Succeeded", props["provisioningState"])
	}
	ip, ok := props["ipAddress"].(string)
	if !ok || ip == "" {
		t.Errorf("ipAddress missing or empty: %v", props["ipAddress"])
	}
	sku, ok := body["sku"].(map[string]interface{})
	if !ok {
		t.Fatalf("sku missing: %v", body)
	}
	if sku["name"] != "Standard" {
		t.Errorf("sku.name = %v, want Standard", sku["name"])
	}

	// 2. GET reads back the same IP.
	resp = doJSON(t, http.MethodGet, pipURL, "")
	mustStatus(t, resp, http.StatusOK)
	got := decode(t, resp)
	gotProps := got["properties"].(map[string]interface{})
	if gotProps["ipAddress"] != ip {
		t.Errorf("GET ipAddress = %v, want %v", gotProps["ipAddress"], ip)
	}

	// 3. Second PUT (update) preserves the assigned IP and returns 200.
	resp = doJSON(t, http.MethodPut, pipURL, pipBody)
	mustStatus(t, resp, http.StatusOK)
	updated := decode(t, resp)
	updatedProps := updated["properties"].(map[string]interface{})
	if updatedProps["ipAddress"] != ip {
		t.Errorf("update changed ipAddress: got %v, want %v", updatedProps["ipAddress"], ip)
	}

	// 4. LIST shows the resource.
	resp = doJSON(t, http.MethodGet, listURL, "")
	mustStatus(t, resp, http.StatusOK)
	list := decode(t, resp)
	items, ok := list["value"].([]interface{})
	if !ok || len(items) != 1 {
		t.Fatalf("list value = %v, want 1 item", list["value"])
	}

	// 5. DELETE is async (202 Accepted).
	resp = doJSON(t, http.MethodDelete, pipURL, "")
	mustStatus(t, resp, http.StatusAccepted)
	if resp.Header.Get("Location") == "" {
		t.Errorf("Location header missing on DELETE")
	}
	resp.Body.Close()

	// 6. Subsequent GET must return 404.
	resp = doJSON(t, http.MethodGet, pipURL, "")
	mustStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
}

// TestARM_LBFullFlow exercises the Load Balancer lifecycle: create an LB with
// Standard SKU, add a backend pool, a rule, and a probe; verify all three are
// embedded in the LB GET response; delete the LB and confirm children cascade.
func TestARM_LBFullFlow(t *testing.T) {
	srv := buildFullServer(t)
	base := srv.URL

	lbURL := base + "/subscriptions/sub1/resourcegroups/rg1/providers/microsoft.network/loadbalancers/lb1" + apiVersionQ
	poolURL := lbURL[:len(lbURL)-len(apiVersionQ)] + "/backendaddresspools/pool1" + apiVersionQ
	ruleURL := lbURL[:len(lbURL)-len(apiVersionQ)] + "/loadbalancingrules/rule1" + apiVersionQ
	probeURL := lbURL[:len(lbURL)-len(apiVersionQ)] + "/probes/probe1" + apiVersionQ
	listURL := base + "/subscriptions/sub1/resourcegroups/rg1/providers/microsoft.network/loadbalancers" + apiVersionQ

	lbBody := `{
		"location": "uksouth",
		"sku": {"name": "Standard"},
		"properties": {
			"frontendIPConfigurations": [
				{"name":"fe-config","properties":{"privateIPAllocationMethod":"Dynamic"}}
			]
		}
	}`

	// 1. Create LB.
	resp := doJSON(t, http.MethodPut, lbURL, lbBody)
	mustStatus(t, resp, http.StatusCreated)
	body := decode(t, resp)
	sku, ok := body["sku"].(map[string]interface{})
	if !ok {
		t.Fatalf("sku missing: %v", body)
	}
	if sku["name"] != "Standard" {
		t.Errorf("sku.name = %v, want Standard", sku["name"])
	}
	props := body["properties"].(map[string]interface{})
	if props["provisioningState"] != "Succeeded" {
		t.Errorf("provisioningState = %v, want Succeeded", props["provisioningState"])
	}

	// 2. Add backend pool, rule, probe.
	resp = doJSON(t, http.MethodPut, poolURL, `{"properties":{}}`)
	mustStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = doJSON(t, http.MethodPut, ruleURL, `{"properties":{"protocol":"Tcp","frontendPort":80,"backendPort":80}}`)
	mustStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = doJSON(t, http.MethodPut, probeURL, `{"properties":{"protocol":"Http","port":80,"requestPath":"/health"}}`)
	mustStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	// 3. GET LB: all three child types must be embedded.
	resp = doJSON(t, http.MethodGet, lbURL, "")
	mustStatus(t, resp, http.StatusOK)
	got := decode(t, resp)
	gotProps := got["properties"].(map[string]interface{})

	pools, ok := gotProps["backendAddressPools"].([]interface{})
	if !ok || len(pools) != 1 {
		t.Fatalf("backendAddressPools = %v, want 1 item", gotProps["backendAddressPools"])
	}
	rules, ok := gotProps["loadBalancingRules"].([]interface{})
	if !ok || len(rules) != 1 {
		t.Fatalf("loadBalancingRules = %v, want 1 item", gotProps["loadBalancingRules"])
	}
	probes, ok := gotProps["probes"].([]interface{})
	if !ok || len(probes) != 1 {
		t.Fatalf("probes = %v, want 1 item", gotProps["probes"])
	}

	// 4. Idempotent PUT returns 200.
	resp = doJSON(t, http.MethodPut, lbURL, lbBody)
	mustStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	// 5. LIST shows the LB.
	resp = doJSON(t, http.MethodGet, listURL, "")
	mustStatus(t, resp, http.StatusOK)
	list := decode(t, resp)
	items, ok := list["value"].([]interface{})
	if !ok || len(items) != 1 {
		t.Fatalf("list value = %v, want 1 item", list["value"])
	}

	// 6. DELETE LB is async (202 Accepted).
	resp = doJSON(t, http.MethodDelete, lbURL, "")
	mustStatus(t, resp, http.StatusAccepted)
	if resp.Header.Get("Location") == "" {
		t.Errorf("Location header missing on DELETE")
	}
	resp.Body.Close()

	// 7. Children must cascade away.
	resp = doJSON(t, http.MethodGet, poolURL, "")
	mustStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()

	resp = doJSON(t, http.MethodGet, ruleURL, "")
	mustStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()

	resp = doJSON(t, http.MethodGet, probeURL, "")
	mustStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()

	// 8. Subsequent GET on LB returns 404.
	resp = doJSON(t, http.MethodGet, lbURL, "")
	mustStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
}

// TestARM_NSGRuleFullFlow exercises the NSG + security rule lifecycle:
// create NSG, add a rule, verify it is embedded in the NSG response, delete
// the NSG, and confirm the rule cascaded away.
func TestARM_NSGRuleFullFlow(t *testing.T) {
	srv := buildFullServer(t)
	base := srv.URL

	nsgURL := base + "/subscriptions/sub1/resourcegroups/rg1/providers/microsoft.network/networksecuritygroups/nsg1" + apiVersionQ
	ruleURL := nsgURL + "/securityrules/allow-http"
	listURL := base + "/subscriptions/sub1/resourcegroups/rg1/providers/microsoft.network/networksecuritygroups" + apiVersionQ

	nsgBody := `{"location":"uksouth"}`
	ruleBody := `{"properties":{"priority":100,"protocol":"Tcp","access":"Allow","direction":"Inbound","sourceAddressPrefix":"*","sourcePortRange":"*","destinationAddressPrefix":"*","destinationPortRange":"80"}}`

	// 1. Create NSG.
	resp := doJSON(t, http.MethodPut, nsgURL, nsgBody)
	mustStatus(t, resp, http.StatusCreated)
	body := decode(t, resp)
	props := body["properties"].(map[string]interface{})
	rules := props["securityRules"].([]interface{})
	if len(rules) != 0 {
		t.Fatalf("fresh NSG should have 0 rules, got %d", len(rules))
	}

	// 2. Add a security rule.
	resp = doJSON(t, http.MethodPut, ruleURL, ruleBody)
	mustStatus(t, resp, http.StatusCreated)
	ruleBody2 := decode(t, resp)
	if ruleBody2["name"] != "allow-http" {
		t.Errorf("rule name = %v, want allow-http", ruleBody2["name"])
	}

	// 3. GET NSG: rule must be embedded in securityRules array.
	resp = doJSON(t, http.MethodGet, nsgURL, "")
	mustStatus(t, resp, http.StatusOK)
	got := decode(t, resp)
	gotProps := got["properties"].(map[string]interface{})
	gotRules, ok := gotProps["securityRules"].([]interface{})
	if !ok || len(gotRules) != 1 {
		t.Fatalf("securityRules = %v, want 1 rule", gotProps["securityRules"])
	}
	gotRule := gotRules[0].(map[string]interface{})
	if gotRule["name"] != "allow-http" {
		t.Errorf("embedded rule name = %v, want allow-http", gotRule["name"])
	}

	// 4. LIST NSGs shows 1 entry; rules are not included as separate items.
	resp = doJSON(t, http.MethodGet, listURL, "")
	mustStatus(t, resp, http.StatusOK)
	list := decode(t, resp)
	items := list["value"].([]interface{})
	if len(items) != 1 {
		t.Fatalf("list count = %d, want 1", len(items))
	}

	// 5. DELETE NSG — rule cascades.
	resp = doJSON(t, http.MethodDelete, nsgURL, "")
	mustStatus(t, resp, http.StatusAccepted)
	resp.Body.Close()

	// 6. Rule GET must be 404 after cascade.
	resp = doJSON(t, http.MethodGet, ruleURL, "")
	mustStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
}

// TestARM_AppGWFullFlow exercises the Application Gateway lifecycle: create
// with a full Standard_v2 config, read back verifying inline properties are
// preserved, idempotent PUT returns 200, list shows 1 entry, delete returns
// 202 Accepted, subsequent GET returns 404.
func TestARM_AppGWFullFlow(t *testing.T) {
	srv := buildFullServer(t)
	base := srv.URL

	agwURL := base + "/subscriptions/sub1/resourcegroups/rg1/providers/microsoft.network/applicationgateways/agw1" + apiVersionQ
	listURL := base + "/subscriptions/sub1/resourcegroups/rg1/providers/microsoft.network/applicationgateways" + apiVersionQ

	agwBody := `{
		"location": "uksouth",
		"sku": {"name": "Standard_v2", "tier": "Standard_v2", "capacity": 2},
		"properties": {
			"gatewayIPConfigurations": [{"name":"gw-ip-cfg","properties":{"subnet":{"id":"/fake/subnet"}}}],
			"frontendIPConfigurations": [{"name":"fe-ip-cfg","properties":{"publicIPAddress":{"id":"/fake/pip"}}}],
			"frontendPorts": [{"name":"port-80","properties":{"port":80}}],
			"backendAddressPools": [{"name":"backend-pool","properties":{"backendAddresses":[]}}],
			"backendHttpSettingsCollection": [{"name":"http-settings","properties":{"port":80,"protocol":"Http","cookieBasedAffinity":"Disabled","requestTimeout":30}}],
			"httpListeners": [{"name":"http-listener","properties":{"frontendIPConfiguration":{"id":"fe-ip-cfg"},"frontendPort":{"id":"port-80"},"protocol":"Http"}}],
			"requestRoutingRules": [{"name":"routing-rule","properties":{"ruleType":"Basic","httpListener":{"id":"http-listener"},"backendAddressPool":{"id":"backend-pool"},"backendHttpSettings":{"id":"http-settings"},"priority":1}}]
		}
	}`

	// 1. Create.
	resp := doJSON(t, http.MethodPut, agwURL, agwBody)
	mustStatus(t, resp, http.StatusCreated)
	body := decode(t, resp)

	sku, ok := body["sku"].(map[string]interface{})
	if !ok {
		t.Fatalf("sku missing: %v", body)
	}
	if sku["name"] != "Standard_v2" {
		t.Errorf("sku.name = %v, want Standard_v2", sku["name"])
	}
	props := body["properties"].(map[string]interface{})
	if props["provisioningState"] != "Succeeded" {
		t.Errorf("provisioningState = %v, want Succeeded", props["provisioningState"])
	}
	if props["operationalState"] != "Running" {
		t.Errorf("operationalState = %v, want Running", props["operationalState"])
	}

	// 2. GET reads back inline properties intact.
	resp = doJSON(t, http.MethodGet, agwURL, "")
	mustStatus(t, resp, http.StatusOK)
	got := decode(t, resp)
	gotProps := got["properties"].(map[string]interface{})
	pools, ok := gotProps["backendAddressPools"].([]interface{})
	if !ok || len(pools) != 1 {
		t.Errorf("backendAddressPools = %v, want 1 item", gotProps["backendAddressPools"])
	}
	rules, ok := gotProps["requestRoutingRules"].([]interface{})
	if !ok || len(rules) != 1 {
		t.Errorf("requestRoutingRules = %v, want 1 item", gotProps["requestRoutingRules"])
	}

	// 3. Idempotent PUT returns 200.
	resp = doJSON(t, http.MethodPut, agwURL, agwBody)
	mustStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	// 4. LIST shows 1 entry.
	resp = doJSON(t, http.MethodGet, listURL, "")
	mustStatus(t, resp, http.StatusOK)
	list := decode(t, resp)
	items, ok := list["value"].([]interface{})
	if !ok || len(items) != 1 {
		t.Fatalf("list value = %v, want 1 item", list["value"])
	}

	// 5. DELETE is async (202 Accepted).
	resp = doJSON(t, http.MethodDelete, agwURL, "")
	mustStatus(t, resp, http.StatusAccepted)
	if resp.Header.Get("Location") == "" {
		t.Errorf("Location header missing on DELETE")
	}
	resp.Body.Close()

	// 6. Subsequent GET returns 404.
	resp = doJSON(t, http.MethodGet, agwURL, "")
	mustStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
}

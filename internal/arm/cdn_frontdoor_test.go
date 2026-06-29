package arm

import (
	"fmt"
	"net/http"
	"testing"
)

// --- URL + body helpers ---

func afdEndpointURL(srvURL, sub, rg, profile, endpoint string) string {
	return fmt.Sprintf(
		"%s/subscriptions/%s/resourcegroups/%s/providers/microsoft.cdn/profiles/%s/afdendpoints/%s",
		srvURL, sub, rg, profile, endpoint,
	)
}

func afdOriginGroupURL(srvURL, sub, rg, profile, group string) string {
	return fmt.Sprintf(
		"%s/subscriptions/%s/resourcegroups/%s/providers/microsoft.cdn/profiles/%s/origingroups/%s",
		srvURL, sub, rg, profile, group,
	)
}

func afdOriginURL(srvURL, sub, rg, profile, group, origin string) string {
	return fmt.Sprintf(
		"%s/subscriptions/%s/resourcegroups/%s/providers/microsoft.cdn/profiles/%s/origingroups/%s/origins/%s",
		srvURL, sub, rg, profile, group, origin,
	)
}

func afdRouteURL(srvURL, sub, rg, profile, endpoint, route string) string {
	return fmt.Sprintf(
		"%s/subscriptions/%s/resourcegroups/%s/providers/microsoft.cdn/profiles/%s/afdendpoints/%s/routes/%s",
		srvURL, sub, rg, profile, endpoint, route,
	)
}

const afdProfileBody = `{
  "location": "global",
  "sku": {"name": "Standard_AzureFrontDoor"},
  "properties": {"originResponseTimeoutSeconds": 120}
}`

const afdEndpointBody = `{
  "location": "global",
  "properties": {"enabledState": "Enabled"}
}`

const afdOriginGroupBody = `{
  "properties": {
    "loadBalancingSettings": {"sampleSize": 4, "successfulSamplesRequired": 3, "additionalLatencyInMilliseconds": 50},
    "healthProbeSettings": {"probePath": "/", "probeRequestType": "HEAD", "probeProtocol": "Http", "probeIntervalInSeconds": 100}
  }
}`

const afdOriginBody = `{
  "properties": {
    "hostName": "otasa.blob.core.windows.net",
    "httpPort": 80,
    "httpsPort": 443,
    "originHostHeader": "otasa.blob.core.windows.net",
    "priority": 1,
    "weight": 1000,
    "enabledState": "Enabled",
    "enforceCertificateNameCheck": true
  }
}`

// seedAFDProfile creates the parent Microsoft.Cdn/profiles resource every AFD
// child hangs off, returning the server URL for convenience.
func seedAFDProfile(t *testing.T, srv string, sub, rg, profile string) {
	t.Helper()
	resp := httpPut(t, cdnProfileURL(srv, sub, rg, profile), afdProfileBody)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()
}

func afdRouteBody(originGroupID string) string {
	return fmt.Sprintf(`{
  "properties": {
    "originGroup": {"id": %q},
    "patternsToMatch": ["/*"],
    "forwardingProtocol": "MatchRequest",
    "linkToDefaultDomain": "Enabled",
    "httpsRedirect": "Disabled",
    "supportedProtocols": ["Http", "Https"]
  }
}`, originGroupID)
}

// --- afdEndpoint ---

func TestAFDEndpoint_PUT_Creates_Returns201(t *testing.T) {
	srv := newTestServer(t).URL
	seedAFDProfile(t, srv, "sub1", "rg1", "fd1")
	resp := httpPut(t, afdEndpointURL(srv, "sub1", "rg1", "fd1", "ep1"), afdEndpointBody)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()
}

func TestAFDEndpoint_PUT_HostNameGenerated(t *testing.T) {
	srv := newTestServer(t).URL
	seedAFDProfile(t, srv, "sub1", "rg1", "fd1")
	resp := httpPut(t, afdEndpointURL(srv, "sub1", "rg1", "fd1", "myedge"), afdEndpointBody)
	body := decodeJSON(t, resp)
	props := body["properties"].(map[string]interface{})
	if got := props["hostName"]; got != "myedge.azurefd.net" {
		t.Errorf("hostName = %v, want myedge.azurefd.net", got)
	}
}

func TestAFDEndpoint_PUT_ParentMissing_Returns404(t *testing.T) {
	srv := newTestServer(t).URL
	resp := httpPut(t, afdEndpointURL(srv, "sub1", "rg1", "ghost", "ep1"), afdEndpointBody)
	assertStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
}

func TestAFDEndpoint_PUT_Idempotent_Returns200(t *testing.T) {
	srv := newTestServer(t).URL
	seedAFDProfile(t, srv, "sub1", "rg1", "fd1")
	httpPut(t, afdEndpointURL(srv, "sub1", "rg1", "fd1", "ep1"), afdEndpointBody).Body.Close()
	resp := httpPut(t, afdEndpointURL(srv, "sub1", "rg1", "fd1", "ep1"), afdEndpointBody)
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()
}

func TestAFDEndpoint_GET_ReturnsStored(t *testing.T) {
	srv := newTestServer(t).URL
	seedAFDProfile(t, srv, "sub1", "rg1", "fd1")
	httpPut(t, afdEndpointURL(srv, "sub1", "rg1", "fd1", "ep1"), afdEndpointBody).Body.Close()
	resp := httpGet(t, afdEndpointURL(srv, "sub1", "rg1", "fd1", "ep1"))
	body := decodeJSON(t, resp)
	if body["type"] != afdEndpointTypeString {
		t.Errorf("type = %v, want %s", body["type"], afdEndpointTypeString)
	}
	if body["location"] != "global" {
		t.Errorf("location = %v, want global", body["location"])
	}
}

func TestAFDEndpoint_HEAD_And_404(t *testing.T) {
	srv := newTestServer(t).URL
	seedAFDProfile(t, srv, "sub1", "rg1", "fd1")
	httpPut(t, afdEndpointURL(srv, "sub1", "rg1", "fd1", "ep1"), afdEndpointBody).Body.Close()
	assertStatus(t, httpHead(t, afdEndpointURL(srv, "sub1", "rg1", "fd1", "ep1")), http.StatusNoContent)
	assertStatus(t, httpHead(t, afdEndpointURL(srv, "sub1", "rg1", "fd1", "ghost")), http.StatusNotFound)
}

func TestAFDEndpoint_DELETE_Then_GET_404(t *testing.T) {
	srv := newTestServer(t).URL
	seedAFDProfile(t, srv, "sub1", "rg1", "fd1")
	httpPut(t, afdEndpointURL(srv, "sub1", "rg1", "fd1", "ep1"), afdEndpointBody).Body.Close()
	assertStatus(t, httpDelete(t, afdEndpointURL(srv, "sub1", "rg1", "fd1", "ep1")), http.StatusAccepted)
	assertStatus(t, httpGet(t, afdEndpointURL(srv, "sub1", "rg1", "fd1", "ep1")), http.StatusNotFound)
}

func TestAFDEndpoint_LIST_ValueWrapper(t *testing.T) {
	srv := newTestServer(t).URL
	seedAFDProfile(t, srv, "sub1", "rg1", "fd1")
	httpPut(t, afdEndpointURL(srv, "sub1", "rg1", "fd1", "ep1"), afdEndpointBody).Body.Close()
	resp := httpGet(t, fmt.Sprintf(
		"%s/subscriptions/sub1/resourcegroups/rg1/providers/microsoft.cdn/profiles/fd1/afdendpoints", srv))
	body := decodeJSON(t, resp)
	items, ok := body["value"].([]interface{})
	if !ok || len(items) != 1 {
		t.Fatalf("value = %v, want 1 endpoint", body["value"])
	}
}

// --- originGroup ---

func TestAFDOriginGroup_PUT_Creates_NoLocation(t *testing.T) {
	srv := newTestServer(t).URL
	seedAFDProfile(t, srv, "sub1", "rg1", "fd1")
	resp := httpPut(t, afdOriginGroupURL(srv, "sub1", "rg1", "fd1", "og1"), afdOriginGroupBody)
	assertStatus(t, resp, http.StatusCreated)
	body := decodeJSON(t, resp)
	if _, hasLoc := body["location"]; hasLoc {
		t.Errorf("originGroup response should omit location, got %v", body["location"])
	}
	props := body["properties"].(map[string]interface{})
	hp := props["healthProbeSettings"].(map[string]interface{})
	if hp["probeProtocol"] != "Http" {
		t.Errorf("probeProtocol = %v, want Http (echoed verbatim)", hp["probeProtocol"])
	}
}

func TestAFDOriginGroup_PUT_ParentMissing_Returns404(t *testing.T) {
	srv := newTestServer(t).URL
	resp := httpPut(t, afdOriginGroupURL(srv, "sub1", "rg1", "ghost", "og1"), afdOriginGroupBody)
	assertStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
}

// --- origin ---

func TestAFDOrigin_PUT_Creates_PropsEchoed(t *testing.T) {
	srv := newTestServer(t).URL
	seedAFDProfile(t, srv, "sub1", "rg1", "fd1")
	httpPut(t, afdOriginGroupURL(srv, "sub1", "rg1", "fd1", "og1"), afdOriginGroupBody).Body.Close()
	resp := httpPut(t, afdOriginURL(srv, "sub1", "rg1", "fd1", "og1", "o1"), afdOriginBody)
	assertStatus(t, resp, http.StatusCreated)
	props := decodeJSON(t, resp)["properties"].(map[string]interface{})
	if props["hostName"] != "otasa.blob.core.windows.net" {
		t.Errorf("hostName = %v, want otasa.blob.core.windows.net", props["hostName"])
	}
}

func TestAFDOrigin_PUT_ParentGroupMissing_Returns404(t *testing.T) {
	srv := newTestServer(t).URL
	seedAFDProfile(t, srv, "sub1", "rg1", "fd1")
	resp := httpPut(t, afdOriginURL(srv, "sub1", "rg1", "fd1", "ghost", "o1"), afdOriginBody)
	assertStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
}

// --- route ---

func TestAFDRoute_PUT_Creates_OriginGroupEchoed(t *testing.T) {
	srv := newTestServer(t).URL
	seedAFDProfile(t, srv, "sub1", "rg1", "fd1")
	httpPut(t, afdEndpointURL(srv, "sub1", "rg1", "fd1", "ep1"), afdEndpointBody).Body.Close()
	httpPut(t, afdOriginGroupURL(srv, "sub1", "rg1", "fd1", "og1"), afdOriginGroupBody).Body.Close()
	ogID := afdOriginGroupID("sub1", "rg1", "fd1", "og1")
	resp := httpPut(t, afdRouteURL(srv, "sub1", "rg1", "fd1", "ep1", "r1"), afdRouteBody(ogID))
	assertStatus(t, resp, http.StatusCreated)
	props := decodeJSON(t, resp)["properties"].(map[string]interface{})
	og := props["originGroup"].(map[string]interface{})
	if og["id"] != ogID {
		t.Errorf("originGroup.id = %v, want %s", og["id"], ogID)
	}
	if props["linkToDefaultDomain"] != "Enabled" {
		t.Errorf("linkToDefaultDomain = %v, want Enabled", props["linkToDefaultDomain"])
	}
}

func TestAFDRoute_PUT_ParentEndpointMissing_Returns404(t *testing.T) {
	srv := newTestServer(t).URL
	seedAFDProfile(t, srv, "sub1", "rg1", "fd1")
	resp := httpPut(t, afdRouteURL(srv, "sub1", "rg1", "fd1", "ghost", "r1"),
		afdRouteBody(afdOriginGroupID("sub1", "rg1", "fd1", "og1")))
	assertStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
}

// --- cascade ---

func TestAFDEndpoint_DELETE_CascadesRoutes(t *testing.T) {
	srv := newTestServer(t).URL
	seedAFDProfile(t, srv, "sub1", "rg1", "fd1")
	httpPut(t, afdEndpointURL(srv, "sub1", "rg1", "fd1", "ep1"), afdEndpointBody).Body.Close()
	httpPut(t, afdOriginGroupURL(srv, "sub1", "rg1", "fd1", "og1"), afdOriginGroupBody).Body.Close()
	httpPut(t, afdRouteURL(srv, "sub1", "rg1", "fd1", "ep1", "r1"),
		afdRouteBody(afdOriginGroupID("sub1", "rg1", "fd1", "og1"))).Body.Close()

	// Deleting the endpoint must cascade its routes (store.Delete prefix match).
	assertStatus(t, httpDelete(t, afdEndpointURL(srv, "sub1", "rg1", "fd1", "ep1")), http.StatusAccepted)
	assertStatus(t, httpGet(t, afdRouteURL(srv, "sub1", "rg1", "fd1", "ep1", "r1")), http.StatusNotFound)
}

func TestAFDOriginGroup_DELETE_CascadesOrigins(t *testing.T) {
	srv := newTestServer(t).URL
	seedAFDProfile(t, srv, "sub1", "rg1", "fd1")
	httpPut(t, afdOriginGroupURL(srv, "sub1", "rg1", "fd1", "og1"), afdOriginGroupBody).Body.Close()
	httpPut(t, afdOriginURL(srv, "sub1", "rg1", "fd1", "og1", "o1"), afdOriginBody).Body.Close()

	// Deleting the origin group must cascade its origins.
	assertStatus(t, httpDelete(t, afdOriginGroupURL(srv, "sub1", "rg1", "fd1", "og1")), http.StatusAccepted)
	assertStatus(t, httpGet(t, afdOriginURL(srv, "sub1", "rg1", "fd1", "og1", "o1")), http.StatusNotFound)
}

func TestAFDProfile_DELETE_CascadesChildren(t *testing.T) {
	srv := newTestServer(t).URL
	seedAFDProfile(t, srv, "sub1", "rg1", "fd1")
	httpPut(t, afdEndpointURL(srv, "sub1", "rg1", "fd1", "ep1"), afdEndpointBody).Body.Close()
	httpPut(t, afdOriginGroupURL(srv, "sub1", "rg1", "fd1", "og1"), afdOriginGroupBody).Body.Close()
	httpPut(t, afdOriginURL(srv, "sub1", "rg1", "fd1", "og1", "o1"), afdOriginBody).Body.Close()
	httpPut(t, afdRouteURL(srv, "sub1", "rg1", "fd1", "ep1", "r1"),
		afdRouteBody(afdOriginGroupID("sub1", "rg1", "fd1", "og1"))).Body.Close()

	assertStatus(t, httpDelete(t, cdnProfileURL(srv, "sub1", "rg1", "fd1")), http.StatusAccepted)

	// Every child must be gone after the parent profile cascade-deletes.
	assertStatus(t, httpGet(t, afdEndpointURL(srv, "sub1", "rg1", "fd1", "ep1")), http.StatusNotFound)
	assertStatus(t, httpGet(t, afdOriginGroupURL(srv, "sub1", "rg1", "fd1", "og1")), http.StatusNotFound)
	assertStatus(t, httpGet(t, afdOriginURL(srv, "sub1", "rg1", "fd1", "og1", "o1")), http.StatusNotFound)
	assertStatus(t, httpGet(t, afdRouteURL(srv, "sub1", "rg1", "fd1", "ep1", "r1")), http.StatusNotFound)
}

package arm

import (
	"fmt"
	"strings"
	"testing"
)

// --- URL helpers ---

const (
	dnsTestSub  = "00000000-0000-0000-0000-000000000000"
	dnsTestRG   = "test-rg"
	dnsTestZone = "example.com"
)

func dnsZoneURL(srv, zone string) string {
	return fmt.Sprintf("%s/subscriptions/%s/resourcegroups/%s/providers/microsoft.network/dnszones/%s",
		srv, dnsTestSub, dnsTestRG, zone)
}

func dnsZoneListByRGURL(srv string) string {
	return fmt.Sprintf("%s/subscriptions/%s/resourcegroups/%s/providers/microsoft.network/dnszones",
		srv, dnsTestSub, dnsTestRG)
}

func dnsZoneListBySubURL(srv string) string {
	return fmt.Sprintf("%s/subscriptions/%s/providers/microsoft.network/dnszones",
		srv, dnsTestSub)
}

func dnsRecordURL(srv, zone, recordType, recordName string) string {
	return fmt.Sprintf("%s/subscriptions/%s/resourcegroups/%s/providers/microsoft.network/dnszones/%s/%s/%s",
		srv, dnsTestSub, dnsTestRG, zone, recordType, recordName)
}

func dnsRecordListByTypeURL(srv, zone, recordType string) string {
	return fmt.Sprintf("%s/subscriptions/%s/resourcegroups/%s/providers/microsoft.network/dnszones/%s/%s",
		srv, dnsTestSub, dnsTestRG, zone, recordType)
}

func dnsRecordListAllURL(srv, zone string) string {
	return fmt.Sprintf("%s/subscriptions/%s/resourcegroups/%s/providers/microsoft.network/dnszones/%s/recordsets",
		srv, dnsTestSub, dnsTestRG, zone)
}

// --- DNS Zone tests ---

func TestDNSZone_PUT_ReturnsCreated(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, dnsZoneURL(srv.URL, dnsTestZone), `{"location":"global","tags":{}}`)
	assertStatus(t, resp, 201)
	body := decodeJSON(t, resp)
	if body["name"] != dnsTestZone {
		t.Errorf("name = %q, want %q", body["name"], dnsTestZone)
	}
	if body["type"] != "Microsoft.Network/dnsZones" {
		t.Errorf("type = %q, want Microsoft.Network/dnsZones", body["type"])
	}
}

func TestDNSZone_PUT_LocationIsAlwaysGlobal(t *testing.T) {
	srv := newTestServer(t)
	// Even if the caller supplies a region, azemu overwrites with "global".
	resp := httpPut(t, dnsZoneURL(srv.URL, dnsTestZone), `{"location":"eastus","tags":{}}`)
	assertStatus(t, resp, 201)
	body := decodeJSON(t, resp)
	if body["location"] != "global" {
		t.Errorf("location = %q, want global", body["location"])
	}
}

func TestDNSZone_PUT_Returns200OnUpdate(t *testing.T) {
	srv := newTestServer(t)
	httpPut(t, dnsZoneURL(srv.URL, dnsTestZone), `{"location":"global","tags":{}}`)
	resp := httpPut(t, dnsZoneURL(srv.URL, dnsTestZone), `{"location":"global","tags":{"env":"prod"}}`)
	assertStatus(t, resp, 200)
}

func TestDNSZone_PUT_AutoCreatesSOAAndNS(t *testing.T) {
	srv := newTestServer(t)
	httpPut(t, dnsZoneURL(srv.URL, dnsTestZone), `{"location":"global","tags":{}}`)

	soaResp := httpGet(t, dnsRecordURL(srv.URL, dnsTestZone, "SOA", "@"))
	assertStatus(t, soaResp, 200)
	soaBody := decodeJSON(t, soaResp)
	if soaBody["type"] != "Microsoft.Network/dnsZones/SOA" {
		t.Errorf("SOA type = %q, want Microsoft.Network/dnsZones/SOA", soaBody["type"])
	}

	nsResp := httpGet(t, dnsRecordURL(srv.URL, dnsTestZone, "NS", "@"))
	assertStatus(t, nsResp, 200)
	nsBody := decodeJSON(t, nsResp)
	if nsBody["type"] != "Microsoft.Network/dnsZones/NS" {
		t.Errorf("NS type = %q, want Microsoft.Network/dnsZones/NS", nsBody["type"])
	}
}

func TestDNSZone_PUT_AutoSOANotDuplicatedOnUpdate(t *testing.T) {
	srv := newTestServer(t)
	// Create — auto-seeds SOA + NS.
	httpPut(t, dnsZoneURL(srv.URL, dnsTestZone), `{"location":"global"}`)
	// Update — should not re-create SOA (idempotent, SOA was not deleted).
	httpPut(t, dnsZoneURL(srv.URL, dnsTestZone), `{"location":"global","tags":{"k":"v"}}`)

	// Zone GET should still show 2 record sets (SOA + NS), not 4.
	resp := httpGet(t, dnsZoneURL(srv.URL, dnsTestZone))
	assertStatus(t, resp, 200)
	body := decodeJSON(t, resp)
	props := body["properties"].(map[string]interface{})
	if got := props["numberOfRecordSets"].(float64); got != 2 {
		t.Errorf("numberOfRecordSets = %.0f, want 2 after idempotent update", got)
	}
}

func TestDNSZone_PUT_ResponseIncludesNameServers(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, dnsZoneURL(srv.URL, dnsTestZone), `{"location":"global"}`)
	assertStatus(t, resp, 201)
	body := decodeJSON(t, resp)
	props := body["properties"].(map[string]interface{})
	ns, ok := props["nameServers"].([]interface{})
	if !ok || len(ns) == 0 {
		t.Errorf("nameServers missing or empty; props = %v", props)
	}
}

func TestDNSZone_PUT_InvalidJSON_Returns400(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, dnsZoneURL(srv.URL, dnsTestZone), `not-json`)
	assertStatus(t, resp, 400)
}

func TestDNSZone_GET_Returns200(t *testing.T) {
	srv := newTestServer(t)
	httpPut(t, dnsZoneURL(srv.URL, dnsTestZone), `{"location":"global"}`)
	resp := httpGet(t, dnsZoneURL(srv.URL, dnsTestZone))
	assertStatus(t, resp, 200)
	body := decodeJSON(t, resp)
	if body["name"] != dnsTestZone {
		t.Errorf("name = %q, want %q", body["name"], dnsTestZone)
	}
}

func TestDNSZone_GET_Returns404WhenMissing(t *testing.T) {
	srv := newTestServer(t)
	resp := httpGet(t, dnsZoneURL(srv.URL, "missing.com"))
	assertStatus(t, resp, 404)
}

func TestDNSZone_GET_NumberOfRecordSetsIncludesAutoRecords(t *testing.T) {
	srv := newTestServer(t)
	httpPut(t, dnsZoneURL(srv.URL, dnsTestZone), `{"location":"global"}`)
	// Add an A record.
	httpPut(t, dnsRecordURL(srv.URL, dnsTestZone, "A", "www"),
		`{"properties":{"TTL":300,"ARecords":[{"ipv4Address":"1.2.3.4"}]}}`)

	resp := httpGet(t, dnsZoneURL(srv.URL, dnsTestZone))
	assertStatus(t, resp, 200)
	body := decodeJSON(t, resp)
	props := body["properties"].(map[string]interface{})
	// SOA + NS (auto) + 1 A record = 3
	if got := props["numberOfRecordSets"].(float64); got != 3 {
		t.Errorf("numberOfRecordSets = %.0f, want 3", got)
	}
}

func TestDNSZone_HEAD_Returns204WhenExists(t *testing.T) {
	srv := newTestServer(t)
	httpPut(t, dnsZoneURL(srv.URL, dnsTestZone), `{"location":"global"}`)
	resp := httpHead(t, dnsZoneURL(srv.URL, dnsTestZone))
	assertStatus(t, resp, 204)
	if body := readBody(t, resp); body != "" {
		t.Errorf("HEAD body = %q, want empty", body)
	}
}

func TestDNSZone_HEAD_Returns404WhenMissing(t *testing.T) {
	srv := newTestServer(t)
	resp := httpHead(t, dnsZoneURL(srv.URL, "missing.com"))
	assertStatus(t, resp, 404)
}

func TestDNSZone_DELETE_Returns202(t *testing.T) {
	srv := newTestServer(t)
	httpPut(t, dnsZoneURL(srv.URL, dnsTestZone), `{"location":"global"}`)
	resp := httpDelete(t, dnsZoneURL(srv.URL, dnsTestZone))
	assertStatus(t, resp, 202)
	resp.Body.Close()
	if resp.Header.Get("Location") == "" {
		t.Error("Location header missing on DELETE 202")
	}
}

func TestDNSZone_DELETE_Returns404WhenMissing(t *testing.T) {
	srv := newTestServer(t)
	resp := httpDelete(t, dnsZoneURL(srv.URL, "missing.com"))
	assertStatus(t, resp, 404)
	resp.Body.Close()
}

func TestDNSZone_DELETE_CascadesRecordSets(t *testing.T) {
	srv := newTestServer(t)
	httpPut(t, dnsZoneURL(srv.URL, dnsTestZone), `{"location":"global"}`)
	httpPut(t, dnsRecordURL(srv.URL, dnsTestZone, "A", "www"),
		`{"properties":{"TTL":300,"ARecords":[{"ipv4Address":"1.2.3.4"}]}}`)
	httpPut(t, dnsRecordURL(srv.URL, dnsTestZone, "TXT", "verify"),
		`{"properties":{"TTL":300,"TXTRecords":[{"value":["hello"]}]}}`)

	del := httpDelete(t, dnsZoneURL(srv.URL, dnsTestZone))
	assertStatus(t, del, 202)
	del.Body.Close()

	// Zone is gone.
	assertStatus(t, httpGet(t, dnsZoneURL(srv.URL, dnsTestZone)), 404)
	// Auto-seeded records are gone.
	assertStatus(t, httpGet(t, dnsRecordURL(srv.URL, dnsTestZone, "SOA", "@")), 404)
	assertStatus(t, httpGet(t, dnsRecordURL(srv.URL, dnsTestZone, "NS", "@")), 404)
	// User-created records are gone.
	assertStatus(t, httpGet(t, dnsRecordURL(srv.URL, dnsTestZone, "A", "www")), 404)
	assertStatus(t, httpGet(t, dnsRecordURL(srv.URL, dnsTestZone, "TXT", "verify")), 404)
}

func TestDNSZone_LIST_ByRG_ReturnsValueWrapper(t *testing.T) {
	srv := newTestServer(t)
	httpPut(t, dnsZoneURL(srv.URL, dnsTestZone), `{"location":"global"}`)
	httpPut(t, dnsZoneURL(srv.URL, "other.com"), `{"location":"global"}`)

	resp := httpGet(t, dnsZoneListByRGURL(srv.URL))
	assertStatus(t, resp, 200)
	body := decodeJSON(t, resp)
	items := body["value"].([]interface{})
	if len(items) != 2 {
		t.Errorf("list by RG returned %d items, want 2", len(items))
	}
}

func TestDNSZone_LIST_ByRG_Empty(t *testing.T) {
	srv := newTestServer(t)
	resp := httpGet(t, dnsZoneListByRGURL(srv.URL))
	assertStatus(t, resp, 200)
	body := decodeJSON(t, resp)
	items := body["value"].([]interface{})
	if len(items) != 0 {
		t.Errorf("list empty returned %d items, want 0", len(items))
	}
}

func TestDNSZone_LIST_ByRG_FiltersOutRecordSets(t *testing.T) {
	srv := newTestServer(t)
	httpPut(t, dnsZoneURL(srv.URL, dnsTestZone), `{"location":"global"}`)
	httpPut(t, dnsRecordURL(srv.URL, dnsTestZone, "A", "www"),
		`{"properties":{"TTL":300,"ARecords":[{"ipv4Address":"1.2.3.4"}]}}`)

	resp := httpGet(t, dnsZoneListByRGURL(srv.URL))
	assertStatus(t, resp, 200)
	body := decodeJSON(t, resp)
	items := body["value"].([]interface{})
	// Only the zone, not the A record or auto-seeded records.
	if len(items) != 1 {
		t.Errorf("list by RG returned %d items, want 1 (zone only)", len(items))
	}
}

func TestDNSZone_LIST_BySub(t *testing.T) {
	srv := newTestServer(t)
	httpPut(t, dnsZoneURL(srv.URL, dnsTestZone), `{"location":"global"}`)
	resp := httpGet(t, dnsZoneListBySubURL(srv.URL))
	assertStatus(t, resp, 200)
	body := decodeJSON(t, resp)
	items := body["value"].([]interface{})
	if len(items) != 1 {
		t.Errorf("list by sub returned %d items, want 1", len(items))
	}
}

func TestDNSZone_MissingAPIVersion_Returns400(t *testing.T) {
	srv := newTestServer(t)
	resp := httpGetRaw(t, dnsZoneURL(srv.URL, dnsTestZone))
	assertStatus(t, resp, 400)
	resp.Body.Close()
}

func TestDNSZone_AzureHeaders(t *testing.T) {
	srv := newTestServer(t)
	httpPut(t, dnsZoneURL(srv.URL, dnsTestZone), `{"location":"global"}`)
	resp := httpGet(t, dnsZoneURL(srv.URL, dnsTestZone))
	assertStatus(t, resp, 200)
	resp.Body.Close()
	if resp.Header.Get("x-ms-request-id") == "" {
		t.Error("x-ms-request-id header missing")
	}
	if resp.Header.Get("x-ms-correlation-request-id") == "" {
		t.Error("x-ms-correlation-request-id header missing")
	}
}

func TestDNSZone_PUT_NilTags(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, dnsZoneURL(srv.URL, dnsTestZone), `{"location":"global"}`)
	assertStatus(t, resp, 201)
	body := decodeJSON(t, resp)
	if body["tags"] == nil {
		t.Error("tags should not be nil in response")
	}
}

// --- DNS Record Set tests ---

func TestDNSRecordSet_PUT_ARecord_ReturnsCreated(t *testing.T) {
	srv := newTestServer(t)
	httpPut(t, dnsZoneURL(srv.URL, dnsTestZone), `{"location":"global"}`)
	resp := httpPut(t, dnsRecordURL(srv.URL, dnsTestZone, "A", "www"),
		`{"properties":{"TTL":300,"ARecords":[{"ipv4Address":"1.2.3.4"}]}}`)
	assertStatus(t, resp, 201)
	body := decodeJSON(t, resp)
	if body["type"] != "Microsoft.Network/dnsZones/A" {
		t.Errorf("type = %q, want Microsoft.Network/dnsZones/A", body["type"])
	}
	if body["name"] != "www" {
		t.Errorf("name = %q, want www", body["name"])
	}
}

func TestDNSRecordSet_PUT_Returns200OnUpdate(t *testing.T) {
	srv := newTestServer(t)
	httpPut(t, dnsZoneURL(srv.URL, dnsTestZone), `{"location":"global"}`)
	httpPut(t, dnsRecordURL(srv.URL, dnsTestZone, "A", "www"),
		`{"properties":{"TTL":300,"ARecords":[{"ipv4Address":"1.2.3.4"}]}}`)
	resp := httpPut(t, dnsRecordURL(srv.URL, dnsTestZone, "A", "www"),
		`{"properties":{"TTL":300,"ARecords":[{"ipv4Address":"5.6.7.8"}]}}`)
	assertStatus(t, resp, 200)
}

func TestDNSRecordSet_PUT_NoParentZone_Returns404(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, dnsRecordURL(srv.URL, "missing.com", "A", "www"),
		`{"properties":{"TTL":300,"ARecords":[{"ipv4Address":"1.2.3.4"}]}}`)
	assertStatus(t, resp, 404)
	body := decodeJSON(t, resp)
	errObj := body["error"].(map[string]interface{})
	if errObj["code"] != "ParentResourceNotFound" {
		t.Errorf("error code = %q, want ParentResourceNotFound", errObj["code"])
	}
}

func TestDNSRecordSet_PUT_InvalidJSON_Returns400(t *testing.T) {
	srv := newTestServer(t)
	httpPut(t, dnsZoneURL(srv.URL, dnsTestZone), `{"location":"global"}`)
	resp := httpPut(t, dnsRecordURL(srv.URL, dnsTestZone, "A", "www"), `not-json`)
	assertStatus(t, resp, 400)
	resp.Body.Close()
}

func TestDNSRecordSet_PUT_RecordTypeUppercasedInType(t *testing.T) {
	srv := newTestServer(t)
	httpPut(t, dnsZoneURL(srv.URL, dnsTestZone), `{"location":"global"}`)
	// Send with lowercase "a" — chi param is lowercase after NormalizePath.
	resp := httpPut(t, dnsRecordURL(srv.URL, dnsTestZone, "a", "www"),
		`{"properties":{"TTL":300,"ARecords":[{"ipv4Address":"1.2.3.4"}]}}`)
	assertStatus(t, resp, 201)
	body := decodeJSON(t, resp)
	// The type in the response must use uppercase record type.
	if body["type"] != "Microsoft.Network/dnsZones/A" {
		t.Errorf("type = %q, want Microsoft.Network/dnsZones/A", body["type"])
	}
}

func TestDNSRecordSet_PUT_FQDNComputed(t *testing.T) {
	srv := newTestServer(t)
	httpPut(t, dnsZoneURL(srv.URL, dnsTestZone), `{"location":"global"}`)
	resp := httpPut(t, dnsRecordURL(srv.URL, dnsTestZone, "A", "www"),
		`{"properties":{"TTL":300,"ARecords":[{"ipv4Address":"1.2.3.4"}]}}`)
	assertStatus(t, resp, 201)
	body := decodeJSON(t, resp)
	props := body["properties"].(map[string]interface{})
	want := "www." + dnsTestZone + "."
	if props["fqdn"] != want {
		t.Errorf("fqdn = %q, want %q", props["fqdn"], want)
	}
}

func TestDNSRecordSet_PUT_ApexFQDN(t *testing.T) {
	srv := newTestServer(t)
	httpPut(t, dnsZoneURL(srv.URL, dnsTestZone), `{"location":"global"}`)
	// The auto-seeded SOA has name "@" — its fqdn should be just the zone name.
	resp := httpGet(t, dnsRecordURL(srv.URL, dnsTestZone, "SOA", "@"))
	assertStatus(t, resp, 200)
	body := decodeJSON(t, resp)
	props := body["properties"].(map[string]interface{})
	want := dnsTestZone + "."
	if props["fqdn"] != want {
		t.Errorf("apex fqdn = %q, want %q", props["fqdn"], want)
	}
}

func TestDNSRecordSet_PUT_PropertiesRoundTrip(t *testing.T) {
	srv := newTestServer(t)
	httpPut(t, dnsZoneURL(srv.URL, dnsTestZone), `{"location":"global"}`)
	resp := httpPut(t, dnsRecordURL(srv.URL, dnsTestZone, "TXT", "spf"),
		`{"properties":{"TTL":600,"TXTRecords":[{"value":["v=spf1 include:_spf.google.com ~all"]}]}}`)
	assertStatus(t, resp, 201)
	body := decodeJSON(t, resp)
	props := body["properties"].(map[string]interface{})
	records, ok := props["TXTRecords"].([]interface{})
	if !ok || len(records) == 0 {
		t.Errorf("TXTRecords missing in response; props = %v", props)
	}
}

func TestDNSRecordSet_PUT_MultipleTypes(t *testing.T) {
	srv := newTestServer(t)
	httpPut(t, dnsZoneURL(srv.URL, dnsTestZone), `{"location":"global"}`)

	types := []struct {
		rt   string
		name string
		body string
	}{
		{"A", "www", `{"properties":{"TTL":300,"ARecords":[{"ipv4Address":"1.2.3.4"}]}}`},
		{"AAAA", "www", `{"properties":{"TTL":300,"AaaaRecords":[{"ipv6Address":"::1"}]}}`},
		{"CNAME", "mail", `{"properties":{"TTL":300,"cnameRecord":{"cname":"smtp.example.com"}}}`},
		{"MX", "@", `{"properties":{"TTL":300,"MXRecords":[{"preference":10,"exchange":"mail.example.com"}]}}`},
		{"SRV", "_sip._tcp", `{"properties":{"TTL":300,"SRVRecords":[{"priority":1,"weight":100,"port":5060,"target":"sip.example.com"}]}}`},
	}
	for _, tc := range types {
		resp := httpPut(t, dnsRecordURL(srv.URL, dnsTestZone, tc.rt, tc.name), tc.body)
		assertStatus(t, resp, 201)
		wantType := "Microsoft.Network/dnsZones/" + tc.rt
		body := decodeJSON(t, resp)
		if body["type"] != wantType {
			t.Errorf("[%s] type = %q, want %q", tc.rt, body["type"], wantType)
		}
	}
}

func TestDNSRecordSet_GET_Returns200(t *testing.T) {
	srv := newTestServer(t)
	httpPut(t, dnsZoneURL(srv.URL, dnsTestZone), `{"location":"global"}`)
	httpPut(t, dnsRecordURL(srv.URL, dnsTestZone, "A", "www"),
		`{"properties":{"TTL":300,"ARecords":[{"ipv4Address":"1.2.3.4"}]}}`)
	resp := httpGet(t, dnsRecordURL(srv.URL, dnsTestZone, "A", "www"))
	assertStatus(t, resp, 200)
}

func TestDNSRecordSet_GET_Returns404WhenMissing(t *testing.T) {
	srv := newTestServer(t)
	httpPut(t, dnsZoneURL(srv.URL, dnsTestZone), `{"location":"global"}`)
	resp := httpGet(t, dnsRecordURL(srv.URL, dnsTestZone, "A", "missing"))
	assertStatus(t, resp, 404)
	resp.Body.Close()
}

func TestDNSRecordSet_HEAD_Returns204WhenExists(t *testing.T) {
	srv := newTestServer(t)
	httpPut(t, dnsZoneURL(srv.URL, dnsTestZone), `{"location":"global"}`)
	httpPut(t, dnsRecordURL(srv.URL, dnsTestZone, "A", "www"),
		`{"properties":{"TTL":300}}`)
	resp := httpHead(t, dnsRecordURL(srv.URL, dnsTestZone, "A", "www"))
	assertStatus(t, resp, 204)
	if body := readBody(t, resp); body != "" {
		t.Errorf("HEAD body = %q, want empty", body)
	}
}

func TestDNSRecordSet_HEAD_Returns404WhenMissing(t *testing.T) {
	srv := newTestServer(t)
	httpPut(t, dnsZoneURL(srv.URL, dnsTestZone), `{"location":"global"}`)
	resp := httpHead(t, dnsRecordURL(srv.URL, dnsTestZone, "A", "nope"))
	assertStatus(t, resp, 404)
}

func TestDNSRecordSet_DELETE_Returns204(t *testing.T) {
	srv := newTestServer(t)
	httpPut(t, dnsZoneURL(srv.URL, dnsTestZone), `{"location":"global"}`)
	httpPut(t, dnsRecordURL(srv.URL, dnsTestZone, "A", "www"),
		`{"properties":{"TTL":300,"ARecords":[{"ipv4Address":"1.2.3.4"}]}}`)
	resp := httpDelete(t, dnsRecordURL(srv.URL, dnsTestZone, "A", "www"))
	assertStatus(t, resp, 204)
	resp.Body.Close()
}

func TestDNSRecordSet_DELETE_SubsequentGETReturns404(t *testing.T) {
	srv := newTestServer(t)
	httpPut(t, dnsZoneURL(srv.URL, dnsTestZone), `{"location":"global"}`)
	httpPut(t, dnsRecordURL(srv.URL, dnsTestZone, "A", "www"),
		`{"properties":{"TTL":300}}`)
	del := httpDelete(t, dnsRecordURL(srv.URL, dnsTestZone, "A", "www"))
	assertStatus(t, del, 204)
	del.Body.Close()
	assertStatus(t, httpGet(t, dnsRecordURL(srv.URL, dnsTestZone, "A", "www")), 404)
}

func TestDNSRecordSet_DELETE_Returns404WhenMissing(t *testing.T) {
	srv := newTestServer(t)
	httpPut(t, dnsZoneURL(srv.URL, dnsTestZone), `{"location":"global"}`)
	resp := httpDelete(t, dnsRecordURL(srv.URL, dnsTestZone, "A", "nope"))
	assertStatus(t, resp, 404)
	resp.Body.Close()
}

func TestDNSRecordSet_LIST_ByType(t *testing.T) {
	srv := newTestServer(t)
	httpPut(t, dnsZoneURL(srv.URL, dnsTestZone), `{"location":"global"}`)
	httpPut(t, dnsRecordURL(srv.URL, dnsTestZone, "A", "www"),
		`{"properties":{"TTL":300,"ARecords":[{"ipv4Address":"1.2.3.4"}]}}`)
	httpPut(t, dnsRecordURL(srv.URL, dnsTestZone, "A", "mail"),
		`{"properties":{"TTL":300,"ARecords":[{"ipv4Address":"5.6.7.8"}]}}`)
	httpPut(t, dnsRecordURL(srv.URL, dnsTestZone, "TXT", "spf"),
		`{"properties":{"TTL":300}}`)

	resp := httpGet(t, dnsRecordListByTypeURL(srv.URL, dnsTestZone, "A"))
	assertStatus(t, resp, 200)
	body := decodeJSON(t, resp)
	items := body["value"].([]interface{})
	if len(items) != 2 {
		t.Errorf("list A records returned %d items, want 2", len(items))
	}
	// Verify types are correct.
	for _, item := range items {
		m := item.(map[string]interface{})
		if m["type"] != "Microsoft.Network/dnsZones/A" {
			t.Errorf("unexpected type %q in A list", m["type"])
		}
	}
}

func TestDNSRecordSet_LIST_All(t *testing.T) {
	srv := newTestServer(t)
	httpPut(t, dnsZoneURL(srv.URL, dnsTestZone), `{"location":"global"}`)
	// After zone create: SOA + NS auto-seeded.
	httpPut(t, dnsRecordURL(srv.URL, dnsTestZone, "A", "www"),
		`{"properties":{"TTL":300}}`)

	resp := httpGet(t, dnsRecordListAllURL(srv.URL, dnsTestZone))
	assertStatus(t, resp, 200)
	body := decodeJSON(t, resp)
	items := body["value"].([]interface{})
	// SOA + NS + www A = 3
	if len(items) != 3 {
		t.Errorf("list all returned %d items, want 3", len(items))
	}
}

func TestDNSRecordSet_LIST_AllDoesNotIncludeZone(t *testing.T) {
	srv := newTestServer(t)
	httpPut(t, dnsZoneURL(srv.URL, dnsTestZone), `{"location":"global"}`)

	resp := httpGet(t, dnsRecordListAllURL(srv.URL, dnsTestZone))
	assertStatus(t, resp, 200)
	body := decodeJSON(t, resp)
	items := body["value"].([]interface{})
	for _, item := range items {
		m := item.(map[string]interface{})
		if m["type"] == "Microsoft.Network/dnsZones" {
			t.Errorf("listAll returned the zone itself; should only return record sets")
		}
	}
}

func TestDNSRecordSet_LIST_ByTypeEmpty(t *testing.T) {
	srv := newTestServer(t)
	httpPut(t, dnsZoneURL(srv.URL, dnsTestZone), `{"location":"global"}`)
	resp := httpGet(t, dnsRecordListByTypeURL(srv.URL, dnsTestZone, "CNAME"))
	assertStatus(t, resp, 200)
	body := decodeJSON(t, resp)
	items := body["value"].([]interface{})
	if len(items) != 0 {
		t.Errorf("empty CNAME list returned %d items, want 0", len(items))
	}
}

func TestDNSZone_ProvisioningStateSucceeded(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, dnsZoneURL(srv.URL, dnsTestZone), `{"location":"global"}`)
	assertStatus(t, resp, 201)
	body := decodeJSON(t, resp)
	props := body["properties"].(map[string]interface{})
	if props["provisioningState"] != "Succeeded" {
		t.Errorf("provisioningState = %q, want Succeeded", props["provisioningState"])
	}
}

func TestDNSRecordSet_ProvisioningStateSucceeded(t *testing.T) {
	srv := newTestServer(t)
	httpPut(t, dnsZoneURL(srv.URL, dnsTestZone), `{"location":"global"}`)
	resp := httpPut(t, dnsRecordURL(srv.URL, dnsTestZone, "A", "www"),
		`{"properties":{"TTL":300}}`)
	assertStatus(t, resp, 201)
	body := decodeJSON(t, resp)
	props := body["properties"].(map[string]interface{})
	if props["provisioningState"] != "Succeeded" {
		t.Errorf("provisioningState = %q, want Succeeded", props["provisioningState"])
	}
}

func TestDNSZone_IDShape(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, dnsZoneURL(srv.URL, dnsTestZone), `{"location":"global"}`)
	assertStatus(t, resp, 201)
	body := decodeJSON(t, resp)
	id := body["id"].(string)
	wantSuffix := fmt.Sprintf("/providers/Microsoft.Network/dnsZones/%s", dnsTestZone)
	if !strings.HasSuffix(id, wantSuffix) {
		t.Errorf("id = %q, want suffix %q", id, wantSuffix)
	}
}

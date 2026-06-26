package arm

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"

	"github.com/zerodeth/azemu/internal/store"
)

const dnsZoneTypeString = "Microsoft.Network/dnsZones"

// dnsRecordType returns the ARM resource type for a record set, e.g.
// "Microsoft.Network/dnsZones/A" for chi param "a".
func dnsRecordType(recordType string) string {
	return dnsZoneTypeString + "/" + strings.ToUpper(recordType)
}

func dnsZoneID(subID, rgName, zoneName string) string {
	return fmt.Sprintf(
		"/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/dnsZones/%s",
		subID, rgName, zoneName,
	)
}

func dnsRecordSetID(subID, rgName, zoneName, recordType, recordName string) string {
	return fmt.Sprintf(
		"/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/dnsZones/%s/%s/%s",
		subID, rgName, zoneName, strings.ToUpper(recordType), recordName,
	)
}

// dnsZoneBody is the PUT payload azemu reads for azurerm_dns_zone.
type dnsZoneBody struct {
	Location   string                 `json:"location"`
	Tags       map[string]string      `json:"tags"`
	Properties map[string]interface{} `json:"properties"`
}

// azureNameServers are the default name servers returned for every emulated DNS zone.
var azureNameServers = []string{
	"ns1-01.azure-dns.com.",
	"ns2-01.azure-dns.net.",
	"ns3-01.azure-dns.org.",
	"ns4-01.azure-dns.info.",
}

func dnsZoneResponse(zone *store.Resource, recordSets []*store.Resource) map[string]interface{} {
	props := map[string]interface{}{
		"provisioningState":     "Succeeded",
		"zoneType":              "Public",
		"nameServers":           azureNameServers,
		"numberOfRecordSets":    len(recordSets),
		"maxNumberOfRecordSets": 10000,
	}
	// Carry through any user-supplied properties (e.g. a zoneType override).
	for k, v := range zone.Properties {
		switch k {
		case "provisioningState", "nameServers", "numberOfRecordSets", "maxNumberOfRecordSets":
			// always overwritten above
		default:
			props[k] = v
		}
	}
	return map[string]interface{}{
		"id":         zone.ID,
		"name":       zone.Name,
		"type":       zone.Type,
		"location":   zone.Location,
		"tags":       zone.Tags,
		"properties": props,
	}
}

func dnsRecordSetResponse(rs *store.Resource, zoneName string) map[string]interface{} {
	// Build the fully-qualified DNS name. "@" means the zone apex.
	fqdn := zoneName + "."
	if rs.Name != "@" {
		fqdn = rs.Name + "." + zoneName + "."
	}

	props := map[string]interface{}{
		"provisioningState": "Succeeded",
		"fqdn":              fqdn,
		"TTL":               float64(3600),
	}
	for k, v := range rs.Properties {
		switch k {
		case "provisioningState", "fqdn":
			// always overwritten above
		default:
			props[k] = v
		}
	}
	return map[string]interface{}{
		"id":         rs.ID,
		"name":       rs.Name,
		"type":       rs.Type,
		"properties": props,
	}
}

// autoCreateSOA seeds the mandatory SOA record that every Azure DNS zone has at the apex.
func (a *Router) autoCreateSOA(subID, rgName, zoneName string) error {
	id := dnsRecordSetID(subID, rgName, zoneName, "SOA", "@")
	res := &store.Resource{
		ID:       id,
		Name:     "@",
		Type:     dnsRecordType("SOA"),
		Location: "global",
		Tags:     map[string]string{},
		Properties: map[string]interface{}{
			"TTL": float64(3600),
			"SOARecord": map[string]interface{}{
				"host":         "ns1-01.azure-dns.com.",
				"email":        "azuredns-hostmaster.microsoft.com.",
				"serialNumber": float64(1),
				"refreshTime":  float64(3600),
				"retryTime":    float64(300),
				"expireTime":   float64(2419200),
				"minimumTTL":   float64(300),
			},
		},
	}
	return a.store.Put(id, res)
}

// autoCreateNS seeds the mandatory NS record that every Azure DNS zone has at the apex.
func (a *Router) autoCreateNS(subID, rgName, zoneName string) error {
	id := dnsRecordSetID(subID, rgName, zoneName, "NS", "@")
	nsRecords := make([]map[string]interface{}, len(azureNameServers))
	for i, ns := range azureNameServers {
		nsRecords[i] = map[string]interface{}{"nsdname": ns}
	}
	res := &store.Resource{
		ID:       id,
		Name:     "@",
		Type:     dnsRecordType("NS"),
		Location: "global",
		Tags:     map[string]string{},
		Properties: map[string]interface{}{
			"TTL":       float64(172800),
			"NSRecords": nsRecords,
		},
	}
	return a.store.Put(id, res)
}

// --- DNS Zone handlers ---

func (a *Router) putDNSZone(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	name := chi.URLParam(r, "zoneName")

	var body dnsZoneBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeAzureError(w, http.StatusBadRequest, "InvalidRequestContent", err.Error())
		return
	}
	if body.Properties == nil {
		body.Properties = map[string]interface{}{}
	}

	id := dnsZoneID(subID, rgName, name)
	_, exists := a.store.Get(id)

	res := &store.Resource{
		ID:         id,
		Name:       name,
		Type:       dnsZoneTypeString,
		Location:   "global", // DNS is always global
		Tags:       normaliseTags(body.Tags),
		Properties: body.Properties,
	}
	if err := a.store.Put(id, res); err != nil {
		writeAzureError(w, http.StatusInternalServerError, "InternalServerError",
			fmt.Sprintf("put dns zone %q: %s", name, err))
		return
	}

	// Auto-seed SOA and NS at the apex on first create only.
	if !exists {
		if err := a.autoCreateSOA(subID, rgName, name); err != nil {
			writeAzureError(w, http.StatusInternalServerError, "InternalServerError",
				fmt.Sprintf("auto-create SOA for zone %q: %s", name, err))
			return
		}
		if err := a.autoCreateNS(subID, rgName, name); err != nil {
			writeAzureError(w, http.StatusInternalServerError, "InternalServerError",
				fmt.Sprintf("auto-create NS for zone %q: %s", name, err))
			return
		}
	}

	recordSets := a.store.List(id + "/")
	status := http.StatusCreated
	if exists {
		status = http.StatusOK
	}
	log.Info().Str("resource_id", id).Bool("existed", exists).Msg("dns zone upsert")
	writeJSON(w, status, dnsZoneResponse(res, recordSets))
}

func (a *Router) getDNSZone(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	name := chi.URLParam(r, "zoneName")
	id := dnsZoneID(subID, rgName, name)

	res, ok := a.store.Get(id)
	if !ok {
		writeAzureError(w, http.StatusNotFound, "ResourceNotFound",
			fmt.Sprintf("DNS zone '%s' could not be found.", name))
		return
	}
	recordSets := a.store.List(id + "/")
	writeJSON(w, http.StatusOK, dnsZoneResponse(res, recordSets))
}

func (a *Router) headDNSZone(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	name := chi.URLParam(r, "zoneName")
	id := dnsZoneID(subID, rgName, name)

	if _, ok := a.store.Get(id); !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *Router) deleteDNSZone(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	name := chi.URLParam(r, "zoneName")
	id := dnsZoneID(subID, rgName, name)

	// store.Delete cascades by prefix, removing all record-set children.
	if !a.store.Delete(id) {
		writeAzureError(w, http.StatusNotFound, "ResourceNotFound",
			fmt.Sprintf("DNS zone '%s' could not be found.", name))
		return
	}

	log.Info().Str("resource_id", id).Msg("dns zone deleted")
	w.Header().Set("Location",
		operationResultLocation(r, subID))
	w.WriteHeader(http.StatusAccepted)
}

func (a *Router) listDNSZonesByRG(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	prefix := fmt.Sprintf(
		"/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/dnsZones/",
		subID, rgName,
	)
	a.writeDNSZoneList(w, prefix)
}

func (a *Router) listDNSZonesBySub(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	prefix := fmt.Sprintf("/subscriptions/%s/resourceGroups/", subID)
	a.writeDNSZoneList(w, prefix)
}

func (a *Router) writeDNSZoneList(w http.ResponseWriter, prefix string) {
	resources := a.store.List(prefix)
	items := []map[string]interface{}{}
	for _, res := range resources {
		if res.Type != dnsZoneTypeString {
			continue
		}
		recordSets := a.store.List(res.ID + "/")
		items = append(items, dnsZoneResponse(res, recordSets))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"value": items})
}

// --- DNS Record Set handlers ---

// recordSetBody is the PUT payload azemu reads for azurerm_dns_*_record resources.
type recordSetBody struct {
	Properties map[string]interface{} `json:"properties"`
}

func (a *Router) putDNSRecordSet(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	zoneName := chi.URLParam(r, "zoneName")
	recordType := chi.URLParam(r, "recordType")
	recordName := chi.URLParam(r, "recordName")

	// Parent-exists check mirrors the subnet pattern.
	zoneID := dnsZoneID(subID, rgName, zoneName)
	if _, ok := a.store.Get(zoneID); !ok {
		writeAzureError(w, http.StatusNotFound, "ParentResourceNotFound",
			fmt.Sprintf("DNS zone '%s' could not be found.", zoneName))
		return
	}

	var body recordSetBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeAzureError(w, http.StatusBadRequest, "InvalidRequestContent", err.Error())
		return
	}
	if body.Properties == nil {
		body.Properties = map[string]interface{}{}
	}

	id := dnsRecordSetID(subID, rgName, zoneName, recordType, recordName)
	resType := dnsRecordType(recordType)
	_, exists := a.store.Get(id)

	res := &store.Resource{
		ID:         id,
		Name:       recordName,
		Type:       resType,
		Location:   "global",
		Tags:       map[string]string{},
		Properties: body.Properties,
	}
	if err := a.store.Put(id, res); err != nil {
		writeAzureError(w, http.StatusInternalServerError, "InternalServerError",
			fmt.Sprintf("put dns record set %q: %s", id, err))
		return
	}

	status := http.StatusCreated
	if exists {
		status = http.StatusOK
	}
	log.Info().Str("resource_id", id).Bool("existed", exists).Msg("dns record set upsert")
	writeJSON(w, status, dnsRecordSetResponse(res, zoneName))
}

func (a *Router) getDNSRecordSet(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	zoneName := chi.URLParam(r, "zoneName")
	recordType := chi.URLParam(r, "recordType")
	recordName := chi.URLParam(r, "recordName")

	id := dnsRecordSetID(subID, rgName, zoneName, recordType, recordName)
	res, ok := a.store.Get(id)
	if !ok {
		writeAzureError(w, http.StatusNotFound, "ResourceNotFound",
			fmt.Sprintf("DNS record set '%s' of type '%s' could not be found.",
				recordName, strings.ToUpper(recordType)))
		return
	}
	writeJSON(w, http.StatusOK, dnsRecordSetResponse(res, zoneName))
}

func (a *Router) headDNSRecordSet(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	zoneName := chi.URLParam(r, "zoneName")
	recordType := chi.URLParam(r, "recordType")
	recordName := chi.URLParam(r, "recordName")

	id := dnsRecordSetID(subID, rgName, zoneName, recordType, recordName)
	if _, ok := a.store.Get(id); !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *Router) deleteDNSRecordSet(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	zoneName := chi.URLParam(r, "zoneName")
	recordType := chi.URLParam(r, "recordType")
	recordName := chi.URLParam(r, "recordName")

	id := dnsRecordSetID(subID, rgName, zoneName, recordType, recordName)
	if !a.store.Delete(id) {
		writeAzureError(w, http.StatusNotFound, "ResourceNotFound",
			fmt.Sprintf("DNS record set '%s' of type '%s' could not be found.",
				recordName, strings.ToUpper(recordType)))
		return
	}

	log.Info().Str("resource_id", id).Msg("dns record set deleted")
	// DNS record set deletes are synchronous; return 204 No Content.
	// Zone deletes (deleteDNSZone) remain async (202) to match Azure behaviour.
	w.WriteHeader(http.StatusNoContent)
}

// listDNSRecordSetsByType handles GET .../dnszones/{zone}/{recordType}
func (a *Router) listDNSRecordSetsByType(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	zoneName := chi.URLParam(r, "zoneName")
	recordType := chi.URLParam(r, "recordType")

	zoneID := dnsZoneID(subID, rgName, zoneName)
	prefix := zoneID + "/" + strings.ToUpper(recordType) + "/"
	wantType := dnsRecordType(recordType)

	resources := a.store.List(prefix)
	items := []map[string]interface{}{}
	for _, res := range resources {
		if res.Type != wantType {
			continue
		}
		items = append(items, dnsRecordSetResponse(res, zoneName))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"value": items})
}

// listAllDNSRecordSets handles GET .../dnszones/{zone}/recordsets (list all record sets regardless of type).
func (a *Router) listAllDNSRecordSets(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	zoneName := chi.URLParam(r, "zoneName")

	zoneID := dnsZoneID(subID, rgName, zoneName)
	resources := a.store.List(zoneID + "/")
	items := []map[string]interface{}{}
	for _, res := range resources {
		if res.Type == dnsZoneTypeString {
			continue
		}
		items = append(items, dnsRecordSetResponse(res, zoneName))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"value": items})
}

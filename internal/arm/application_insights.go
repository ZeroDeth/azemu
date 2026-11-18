package arm

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/zerodeth/azemu/internal/store"
)

const applicationInsightsType = "Microsoft.Insights/components"

func (a *Router) applicationInsightsRoutes(r chi.Router) {
	base := "/{subscriptionID}/resourcegroups/{resourceGroupName}/providers/microsoft.insights/components"
	r.Get(base, a.listApplicationInsights)
	r.Get("/{subscriptionID}/providers/microsoft.insights/components", a.listApplicationInsights)
	r.Put(base+"/{componentName}", a.putApplicationInsights)
	r.Get(base+"/{componentName}", a.getApplicationInsights)
	r.Head(base+"/{componentName}", a.headApplicationInsights)
	r.Delete(base+"/{componentName}", a.deleteApplicationInsights)
	r.Get(base+"/{componentName}/currentbillingfeatures", a.applicationInsightsBilling)
	r.Put(base+"/{componentName}/currentbillingfeatures", a.applicationInsightsBilling)
}

func applicationInsightsID(r *http.Request) string {
	return fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Insights/components/%s",
		chi.URLParam(r, "subscriptionID"), chi.URLParam(r, "resourceGroupName"), chi.URLParam(r, "componentName"))
}

func applicationInsightsResponse(res *store.Resource) map[string]any {
	props := map[string]any{}
	for k, v := range res.Properties {
		if !strings.HasPrefix(k, "_") {
			props[k] = v
		}
	}
	return map[string]any{"id": res.ID, "name": res.Name, "type": res.Type, "location": res.Location,
		"tags": normaliseTags(res.Tags), "kind": res.Properties["_kind"], "properties": props}
}

func (a *Router) putApplicationInsights(w http.ResponseWriter, r *http.Request) {
	id := applicationInsightsID(r)
	parent := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s", chi.URLParam(r, "subscriptionID"), chi.URLParam(r, "resourceGroupName"))
	if _, ok := a.store.Get(parent); !ok {
		writeAzureError(w, 404, "ParentResourceNotFound", "Resource group was not found.")
		return
	}
	var body struct {
		ID         string            `json:"id"`
		Name       string            `json:"name"`
		Type       string            `json:"type"`
		Kind       string            `json:"kind"`
		Location   string            `json:"location"`
		Tags       map[string]string `json:"tags"`
		Properties map[string]any    `json:"properties"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil || strings.TrimSpace(body.Location) == "" || body.Properties == nil {
		writeAzureError(w, 400, "InvalidRequestContent", "location and properties are required.")
		return
	}
	props := body.Properties
	if props["Application_Type"] != "web" && props["Application_Type"] != "other" {
		writeAzureError(w, 400, "ValidationError", "Application_Type must be web or other.")
		return
	}
	for key, value := range props {
		var valid bool
		switch key {
		case "Application_Type", "ApplicationId", "AppId", "InstrumentationKey", "ConnectionString", "provisioningState":
			_, valid = value.(string)
		case "DisableIpMasking", "DisableLocalAuth", "ForceCustomerStorageForProfiler":
			_, valid = value.(bool)
		case "publicNetworkAccessForIngestion", "publicNetworkAccessForQuery":
			valid = value == "Enabled" || value == "Disabled"
		case "SamplingPercentage":
			n, ok := value.(float64)
			valid = ok && n >= 0 && n <= 100
		case "RetentionInDays":
			n, ok := value.(float64)
			valid = ok && (n == 30 || n == 60 || n == 90 || n == 120 || n == 180 || n == 270 || n == 365 || n == 550 || n == 730)
		default:
			writeAzureError(w, 501, "NotImplemented", "Application Insights property "+key+" is not implemented.")
			return
		}
		if !valid {
			writeAzureError(w, 400, "ValidationError", "Invalid "+key+".")
			return
		}
	}
	existing, exists := a.store.Get(id)
	key := uuid.NewSHA1(uuid.NameSpaceURL, []byte(strings.ToLower(id))).String()
	props["InstrumentationKey"] = key
	props["AppId"] = uuid.NewSHA1(uuid.NameSpaceOID, []byte(strings.ToLower(id))).String()
	// Management-plane credentials only. No telemetry ingestion is served.
	props["ConnectionString"] = "InstrumentationKey=" + key + ";IngestionEndpoint=https://dc.services.visualstudio.com/"
	props["provisioningState"] = "Succeeded"
	props["_kind"] = body.Kind
	props["_billing"] = map[string]any{"CurrentBillingFeatures": []any{"Basic"}, "DataVolumeCap": map[string]any{"Cap": float64(100), "StopSendNotificationWhenHitCap": false}}
	if exists {
		props["_billing"] = existing.Properties["_billing"]
	}
	for k, v := range map[string]any{"SamplingPercentage": float64(100), "RetentionInDays": float64(90),
		"DisableIpMasking": false, "DisableLocalAuth": false, "ForceCustomerStorageForProfiler": false,
		"publicNetworkAccessForIngestion": "Enabled", "publicNetworkAccessForQuery": "Enabled"} {
		if _, ok := props[k]; !ok {
			props[k] = v
		}
	}
	res := &store.Resource{ID: id, Name: chi.URLParam(r, "componentName"), Type: applicationInsightsType,
		Location: strings.ToLower(strings.ReplaceAll(body.Location, " ", "")), Tags: body.Tags, Properties: props}
	if err := a.store.Put(id, res); err != nil {
		log.Error().Err(err).Str("resource_id", id).Msg("failed to store Application Insights")
		writeAzureError(w, 500, "InternalServerError", "Failed to store Application Insights.")
		return
	}
	log.Info().Str("resource_id", id).Bool("existed", exists).Msg("Application Insights upsert")
	// ComponentsCreateOrUpdate 2020-02-02 accepts 200 only, including create.
	writeJSON(w, http.StatusOK, applicationInsightsResponse(res))
}

func (a *Router) getApplicationInsights(w http.ResponseWriter, r *http.Request) {
	res, ok := a.store.Get(applicationInsightsID(r))
	if !ok {
		writeAzureError(w, 404, "ResourceNotFound", "Application Insights was not found.")
		return
	}
	writeJSON(w, 200, applicationInsightsResponse(res))
}

func (a *Router) headApplicationInsights(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.store.Get(applicationInsightsID(r)); !ok {
		w.WriteHeader(404)
		return
	}
	w.WriteHeader(204)
}

func (a *Router) deleteApplicationInsights(w http.ResponseWriter, r *http.Request) {
	a.store.Delete(applicationInsightsID(r))
	log.Info().Str("resource_id", applicationInsightsID(r)).Msg("Application Insights deleted")
	// ComponentsDelete (2020-02-02) is synchronous, accepting 200/204.
	w.WriteHeader(http.StatusNoContent)
}

func (a *Router) listApplicationInsights(w http.ResponseWriter, r *http.Request) {
	prefix := "/subscriptions/" + chi.URLParam(r, "subscriptionID") + "/resourceGroups/"
	if rg := chi.URLParam(r, "resourceGroupName"); rg != "" {
		prefix += rg + "/providers/Microsoft.Insights/components/"
	}
	items := []map[string]any{}
	for _, res := range a.store.List(prefix) {
		if res.Type == applicationInsightsType {
			items = append(items, applicationInsightsResponse(res))
		}
	}
	writeJSON(w, 200, map[string]any{"value": items})
}

func (a *Router) applicationInsightsBilling(w http.ResponseWriter, r *http.Request) {
	res, ok := a.store.Get(applicationInsightsID(r))
	if !ok {
		writeAzureError(w, 404, "ParentResourceNotFound", "Application Insights was not found.")
		return
	}
	if r.Method == http.MethodPut {
		var body struct {
			CurrentBillingFeatures []string `json:"CurrentBillingFeatures"`
			DataVolumeCap          *struct {
				Cap                            float64 `json:"Cap"`
				StopSendNotificationWhenHitCap bool    `json:"StopSendNotificationWhenHitCap"`
			} `json:"DataVolumeCap"`
		}
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&body); err != nil || body.DataVolumeCap == nil || body.DataVolumeCap.Cap < 0 {
			writeAzureError(w, 400, "InvalidRequestContent", "Invalid billing features.")
			return
		}
		for _, feature := range body.CurrentBillingFeatures {
			if feature != "Basic" {
				writeAzureError(w, 501, "NotImplemented", "Only Basic billing features are implemented.")
				return
			}
		}
		props := make(map[string]any, len(res.Properties))
		for k, v := range res.Properties {
			props[k] = v
		}
		props["_billing"] = body
		res.Properties = props
		if err := a.store.Put(res.ID, res); err != nil {
			log.Error().Err(err).Str("resource_id", res.ID).Msg("failed to store billing features")
			writeAzureError(w, 500, "InternalServerError", "Failed to store billing features.")
			return
		}
		log.Info().Str("resource_id", res.ID).Msg("Application Insights billing updated")
	}
	writeJSON(w, 200, res.Properties["_billing"])
}

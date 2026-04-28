package arm

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/zerodeth/azemu/internal/store"
)

const redisCacheTypeString = "Microsoft.Cache/Redis"

// redisDevPrimaryKey and redisDevSecondaryKey are deterministic development
// keys returned by listKeys. The primary value MUST match the Redis sidecar's
// --requirepass directive so SDK clients authenticated by these keys succeed
// against the real Redis data plane. See ADR 0003.
const (
	redisDevPrimaryKey   = "azemu-dev-primary-key"
	redisDevSecondaryKey = "azemu-dev-secondary-key"
)

// fallbackRedisHost is used when redisEndpoint is empty or unparseable. It
// matches the docker-compose service name so the URL stays meaningful inside
// the default sidecar topology. The real-Azure suffix
// (redis.cache.windows.net) is intentionally avoided so callers cannot
// silently end up pointed at the public cloud.
const fallbackRedisHost = "azemu-redis"

func redisCacheID(subID, rgName, name string) string {
	return fmt.Sprintf(
		"/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Cache/Redis/%s",
		subID, rgName, name,
	)
}

// redisCacheBody is the subset of the azurerm_redis_cache PUT payload that
// azemu understands. Unlike storage_account, the SKU lives INSIDE properties
// (matching the real Microsoft.Cache/Redis ARM contract), so there is no
// top-level Sku field to peel off.
type redisCacheBody struct {
	Location   string                 `json:"location"`
	Tags       map[string]string      `json:"tags"`
	Properties map[string]interface{} `json:"properties"`
}

// validRedisSKUNames is the closed set of accepted SKU tier names.
var validRedisSKUNames = map[string]bool{
	"Basic": true, "Standard": true, "Premium": true,
}

// premiumOnlyTopLevelProps reject the request when present and the SKU is not
// Premium. These are configuration knobs the real Microsoft.Cache/Redis API
// only honours on Premium clusters.
var premiumOnlyTopLevelProps = []string{
	"shardCount", "replicasPerMaster", "replicasPerPrimary", "subnetId", "staticIP",
}

// premiumOnlyRedisConfigKeys reject the request when present inside
// properties.redisConfiguration on a non-Premium SKU. RDB and AOF persistence
// are Premium-only features.
var premiumOnlyRedisConfigKeys = []string{
	"rdb-backup-enabled",
	"rdb-backup-frequency",
	"rdb-backup-max-snapshot-count",
	"rdb-storage-connection-string",
	"aof-backup-enabled",
	"aof-storage-connection-string-0",
	"aof-storage-connection-string-1",
}

// validateAndDefaultRedisSKU validates the supplied SKU and returns a
// canonical form (with defaults filled in). It returns ok=false plus an
// Azure-formatted message when the SKU is invalid.
func validateAndDefaultRedisSKU(raw interface{}) (map[string]interface{}, bool, string) {
	if raw == nil {
		return map[string]interface{}{
			"name":     "Standard",
			"family":   "C",
			"capacity": float64(1),
		}, true, ""
	}
	m, ok := raw.(map[string]interface{})
	if !ok {
		return nil, false, "properties.sku must be an object"
	}

	name, _ := m["name"].(string)
	if name == "" {
		name = "Standard"
	}
	if !validRedisSKUNames[name] {
		return nil, false, fmt.Sprintf("properties.sku.name %q must be one of Basic, Standard, Premium", name)
	}

	family, _ := m["family"].(string)
	if family == "" {
		if name == "Premium" {
			family = "P"
		} else {
			family = "C"
		}
	}
	switch family {
	case "C":
		if name == "Premium" {
			return nil, false, "properties.sku.family C is invalid for Premium tier; use P"
		}
	case "P":
		if name != "Premium" {
			return nil, false, fmt.Sprintf("properties.sku.family P is only valid for Premium tier (got %s)", name)
		}
	default:
		return nil, false, fmt.Sprintf("properties.sku.family %q must be C or P", family)
	}

	var capacity float64
	switch v := m["capacity"].(type) {
	case float64:
		capacity = v
	case nil:
		capacity = 1
	default:
		return nil, false, "properties.sku.capacity must be a number"
	}
	switch family {
	case "C":
		if capacity < 0 || capacity > 6 {
			return nil, false, fmt.Sprintf("properties.sku.capacity %v is out of range for family C (0-6)", capacity)
		}
	case "P":
		if capacity < 1 || capacity > 5 {
			return nil, false, fmt.Sprintf("properties.sku.capacity %v is out of range for family P (1-5)", capacity)
		}
	}

	return map[string]interface{}{
		"name":     name,
		"family":   family,
		"capacity": capacity,
	}, true, ""
}

// redisHostFromEndpoint extracts the hostname from an endpoint URL like
// "redis://azemu-redis:6379". An empty/unparseable input falls back to
// fallbackRedisHost so we never accidentally advertise a real-Azure host.
func redisHostFromEndpoint(endpoint string) string {
	if endpoint == "" {
		return fallbackRedisHost
	}
	u, err := url.Parse(endpoint)
	if err != nil || u.Hostname() == "" {
		return fallbackRedisHost
	}
	return u.Hostname()
}

func (a *Router) putRedisCache(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	name := chi.URLParam(r, "cacheName")

	var body redisCacheBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeAzureError(w, http.StatusBadRequest, "InvalidRequestContent", err.Error())
		return
	}
	if strings.TrimSpace(body.Location) == "" {
		writeAzureError(w, http.StatusBadRequest, "InvalidRequestContent",
			"location is required")
		return
	}
	if body.Properties == nil {
		body.Properties = map[string]interface{}{}
	}

	sku, ok, msg := validateAndDefaultRedisSKU(body.Properties["sku"])
	if !ok {
		writeAzureError(w, http.StatusBadRequest, "InvalidRequestContent", msg)
		return
	}
	skuName, _ := sku["name"].(string)
	isPremium := skuName == "Premium"

	// Reject Premium-only top-level properties on non-Premium SKUs.
	if !isPremium {
		for _, key := range premiumOnlyTopLevelProps {
			if _, present := body.Properties[key]; present {
				writeAzureError(w, http.StatusBadRequest, "InvalidRequestContent",
					fmt.Sprintf("properties.%s is only supported on Premium tier", key))
				return
			}
		}
	}

	// Default and validate redisConfiguration.
	rcRaw, hasRC := body.Properties["redisConfiguration"]
	var redisConfig map[string]interface{}
	if hasRC {
		rc, isMap := rcRaw.(map[string]interface{})
		if !isMap {
			writeAzureError(w, http.StatusBadRequest, "InvalidRequestContent",
				"properties.redisConfiguration must be an object")
			return
		}
		if !isPremium {
			for _, key := range premiumOnlyRedisConfigKeys {
				if _, present := rc[key]; present {
					writeAzureError(w, http.StatusBadRequest, "InvalidRequestContent",
						fmt.Sprintf("properties.redisConfiguration.%s is only supported on Premium tier", key))
					return
				}
			}
		}
		redisConfig = rc
	} else {
		redisConfig = map[string]interface{}{}
	}

	// Stash the canonical SKU under a private key so it survives the round
	// trip through store.Resource without colliding with the public field
	// the response builder promotes.
	body.Properties["_sku"] = sku
	delete(body.Properties, "sku")

	// Computed fields.
	body.Properties["provisioningState"] = "Succeeded"
	body.Properties["redisVersion"] = "7.0"
	body.Properties["port"] = float64(6379)
	body.Properties["sslPort"] = float64(6380)
	body.Properties["hostName"] = redisHostFromEndpoint(a.redisEndpoint)
	body.Properties["redisConfiguration"] = redisConfig

	if _, present := body.Properties["enableNonSslPort"]; !present {
		body.Properties["enableNonSslPort"] = true
	}
	if _, present := body.Properties["minimumTlsVersion"]; !present {
		body.Properties["minimumTlsVersion"] = "1.2"
	}
	if _, present := body.Properties["publicNetworkAccess"]; !present {
		body.Properties["publicNetworkAccess"] = "Enabled"
	}

	id := redisCacheID(subID, rgName, name)
	res := &store.Resource{
		ID:         id,
		Name:       name,
		Type:       redisCacheTypeString,
		Location:   strings.ToLower(body.Location),
		Tags:       normaliseTags(body.Tags),
		Properties: body.Properties,
	}

	_, exists := a.store.Get(id)
	if err := a.store.Put(id, res); err != nil {
		writeAzureError(w, http.StatusInternalServerError, "InternalServerError",
			fmt.Sprintf("put redis cache %q: %s", name, err))
		return
	}

	status := http.StatusCreated
	if exists {
		status = http.StatusOK
	}
	log.Info().Str("resource_id", id).Bool("existed", exists).Msg("redis cache upsert")
	writeJSON(w, status, redisCacheResponse(res))
}

func (a *Router) getRedisCache(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	name := chi.URLParam(r, "cacheName")
	id := redisCacheID(subID, rgName, name)

	res, ok := a.store.Get(id)
	if !ok {
		writeAzureError(w, http.StatusNotFound, "ResourceNotFound",
			fmt.Sprintf("The Resource 'Microsoft.Cache/Redis/%s' under resource group '%s' was not found.", name, rgName))
		return
	}
	writeJSON(w, http.StatusOK, redisCacheResponse(res))
}

func (a *Router) headRedisCache(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	name := chi.URLParam(r, "cacheName")
	id := redisCacheID(subID, rgName, name)

	if _, ok := a.store.Get(id); !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *Router) deleteRedisCache(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	name := chi.URLParam(r, "cacheName")
	id := redisCacheID(subID, rgName, name)

	if !a.store.Delete(id) {
		writeAzureError(w, http.StatusNotFound, "ResourceNotFound",
			fmt.Sprintf("The Resource 'Microsoft.Cache/Redis/%s' under resource group '%s' was not found.", name, rgName))
		return
	}

	log.Info().Str("resource_id", id).Msg("redis cache deleted")
	w.Header().Set("Location",
		fmt.Sprintf("/subscriptions/%s/operationresults/%s", subID, uuid.New().String()))
	w.WriteHeader(http.StatusAccepted)
}

func (a *Router) listRedisCachesByRG(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	prefix := fmt.Sprintf(
		"/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Cache/Redis/",
		subID, rgName,
	)
	a.writeRedisCacheList(w, prefix)
}

func (a *Router) listRedisCachesBySub(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	prefix := fmt.Sprintf("/subscriptions/%s/resourceGroups/", subID)
	a.writeRedisCacheList(w, prefix)
}

func (a *Router) writeRedisCacheList(w http.ResponseWriter, prefix string) {
	items := []map[string]interface{}{}
	for _, res := range a.store.List(prefix) {
		if res.Type != redisCacheTypeString {
			continue
		}
		items = append(items, redisCacheResponse(res))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"value": items})
}

// listRedisCacheKeys handles POST .../listKeys. Returns the deterministic dev
// keys defined above. The Redis ARM contract is {primaryKey, secondaryKey},
// not the Storage-style {keys: [...]}.
func (a *Router) listRedisCacheKeys(w http.ResponseWriter, r *http.Request) {
	subID := chi.URLParam(r, "subscriptionID")
	rgName := chi.URLParam(r, "resourceGroupName")
	name := chi.URLParam(r, "cacheName")
	id := redisCacheID(subID, rgName, name)

	if _, ok := a.store.Get(id); !ok {
		writeAzureError(w, http.StatusNotFound, "ResourceNotFound",
			fmt.Sprintf("The Resource 'Microsoft.Cache/Redis/%s' under resource group '%s' was not found.", name, rgName))
		return
	}

	log.Info().Str("resource_id", id).Msg("redis cache listKeys")
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"primaryKey":   redisDevPrimaryKey,
		"secondaryKey": redisDevSecondaryKey,
	})
}

// redisCacheResponse builds the canonical ARM response for a Redis cache.
// The SKU stored under properties._sku is promoted back to properties.sku.
func redisCacheResponse(s *store.Resource) map[string]interface{} {
	props := map[string]interface{}{}
	for k, v := range s.Properties {
		if k == "_sku" {
			continue
		}
		props[k] = v
	}
	if sku, ok := s.Properties["_sku"].(map[string]interface{}); ok {
		props["sku"] = sku
	}
	props["provisioningState"] = "Succeeded"

	return map[string]interface{}{
		"id":         s.ID,
		"name":       s.Name,
		"type":       s.Type,
		"location":   s.Location,
		"tags":       s.Tags,
		"properties": props,
	}
}

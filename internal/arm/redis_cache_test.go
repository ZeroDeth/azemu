package arm

import (
	"fmt"
	"net/http"
	"testing"
)

func redisCacheURL(srvURL, sub, rg, name string) string {
	return fmt.Sprintf(
		"%s/subscriptions/%s/resourcegroups/%s/providers/microsoft.cache/redis/%s",
		srvURL, sub, rg, name,
	)
}

func redisCacheListByRGURL(srvURL, sub, rg string) string {
	return fmt.Sprintf(
		"%s/subscriptions/%s/resourcegroups/%s/providers/microsoft.cache/redis",
		srvURL, sub, rg,
	)
}

func redisCacheListBySubURL(srvURL, sub string) string {
	return fmt.Sprintf(
		"%s/subscriptions/%s/providers/microsoft.cache/redis",
		srvURL, sub,
	)
}

func redisCacheListKeysURL(srvURL, sub, rg, name string) string {
	return fmt.Sprintf(
		"%s/subscriptions/%s/resourcegroups/%s/providers/microsoft.cache/redis/%s/listkeys",
		srvURL, sub, rg, name,
	)
}

const redisCacheBodyStandardC1 = `{
  "location": "uksouth",
  "properties": {
    "sku": {"name": "Standard", "family": "C", "capacity": 1}
  }
}`

func TestRedisCache_PUT_Create_Returns201(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, redisCacheURL(srv.URL, "sub1", "rg1", "redis1"), redisCacheBodyStandardC1)
	assertStatus(t, resp, http.StatusCreated)

	body := decodeJSON(t, resp)
	if body["name"] != "redis1" {
		t.Errorf("name = %v, want redis1", body["name"])
	}
	if body["type"] != redisCacheTypeString {
		t.Errorf("type = %v, want %s", body["type"], redisCacheTypeString)
	}
	if body["location"] != "uksouth" {
		t.Errorf("location = %v, want uksouth", body["location"])
	}
	wantID := "/subscriptions/sub1/resourceGroups/rg1/providers/Microsoft.Cache/Redis/redis1"
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
	if props["redisVersion"] != "7.0" {
		t.Errorf("redisVersion = %v, want 7.0", props["redisVersion"])
	}
	if props["port"].(float64) != 6379 {
		t.Errorf("port = %v, want 6379", props["port"])
	}
	if props["sslPort"].(float64) != 6380 {
		t.Errorf("sslPort = %v, want 6380", props["sslPort"])
	}
	if props["enableNonSslPort"] != true {
		t.Errorf("enableNonSslPort = %v, want true", props["enableNonSslPort"])
	}

	sku, ok := props["sku"].(map[string]interface{})
	if !ok {
		t.Fatalf("properties.sku missing or wrong type: %T", props["sku"])
	}
	if sku["name"] != "Standard" {
		t.Errorf("sku.name = %v, want Standard", sku["name"])
	}
	if sku["family"] != "C" {
		t.Errorf("sku.family = %v, want C", sku["family"])
	}
	if sku["capacity"].(float64) != 1 {
		t.Errorf("sku.capacity = %v, want 1", sku["capacity"])
	}
}

func TestRedisCache_PUT_HostNameDerivedFromEndpoint(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, redisCacheURL(srv.URL, "sub1", "rg1", "redis-host"), redisCacheBodyStandardC1)
	assertStatus(t, resp, http.StatusCreated)

	body := decodeJSON(t, resp)
	props := body["properties"].(map[string]interface{})
	// newTestServer wires AZEMU_REDIS_ENDPOINT="redis://redis-test:6379".
	// The hostname must come from there, NOT the real-Azure
	// redis.cache.windows.net suffix.
	if props["hostName"] != "redis-test" {
		t.Errorf("hostName = %v, want redis-test (configured endpoint, NOT redis.cache.windows.net)", props["hostName"])
	}
}

func TestRedisCache_PUT_Update_Returns200(t *testing.T) {
	srv := newTestServer(t)
	url := redisCacheURL(srv.URL, "sub1", "rg1", "redis-update")

	resp := httpPut(t, url, redisCacheBodyStandardC1)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = httpPut(t, url, redisCacheBodyStandardC1)
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()
}

func TestRedisCache_PUT_MissingLocation_Returns400(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, redisCacheURL(srv.URL, "sub1", "rg1", "noloc"),
		`{"properties":{"sku":{"name":"Standard","family":"C","capacity":1}}}`)
	assertStatus(t, resp, http.StatusBadRequest)
	resp.Body.Close()
}

func TestRedisCache_PUT_InvalidSku_Returns400(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{
			name: "unknown_name",
			body: `{"location":"uksouth","properties":{"sku":{"name":"Diamond","family":"C","capacity":1}}}`,
		},
		{
			name: "wrong_family_for_standard",
			body: `{"location":"uksouth","properties":{"sku":{"name":"Standard","family":"P","capacity":1}}}`,
		},
		{
			name: "wrong_family_for_premium",
			body: `{"location":"uksouth","properties":{"sku":{"name":"Premium","family":"C","capacity":1}}}`,
		},
		{
			name: "capacity_out_of_range_C",
			body: `{"location":"uksouth","properties":{"sku":{"name":"Standard","family":"C","capacity":99}}}`,
		},
		{
			name: "capacity_out_of_range_P",
			body: `{"location":"uksouth","properties":{"sku":{"name":"Premium","family":"P","capacity":99}}}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := newTestServer(t)
			resp := httpPut(t, redisCacheURL(srv.URL, "sub1", "rg1", "badsku"), tc.body)
			assertStatus(t, resp, http.StatusBadRequest)
			body := decodeJSON(t, resp)
			errObj, ok := body["error"].(map[string]interface{})
			if !ok {
				t.Fatalf("error field missing: %v", body)
			}
			if errObj["code"] != "InvalidRequestContent" {
				t.Errorf("error.code = %v, want InvalidRequestContent", errObj["code"])
			}
		})
	}
}

func TestRedisCache_PUT_PremiumOnlyFieldOnStandard_Returns400(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{
			name: "shardCount",
			body: `{"location":"uksouth","properties":{"sku":{"name":"Standard","family":"C","capacity":1},"shardCount":2}}`,
		},
		{
			name: "subnetId",
			body: `{"location":"uksouth","properties":{"sku":{"name":"Standard","family":"C","capacity":1},"subnetId":"/foo"}}`,
		},
		{
			name: "rdb_backup_enabled",
			body: `{"location":"uksouth","properties":{"sku":{"name":"Standard","family":"C","capacity":1},"redisConfiguration":{"rdb-backup-enabled":"true"}}}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := newTestServer(t)
			resp := httpPut(t, redisCacheURL(srv.URL, "sub1", "rg1", "nopremium"), tc.body)
			assertStatus(t, resp, http.StatusBadRequest)
			resp.Body.Close()
		})
	}
}

func TestRedisCache_PUT_PremiumAcceptsShardCount(t *testing.T) {
	srv := newTestServer(t)
	body := `{"location":"uksouth","properties":{"sku":{"name":"Premium","family":"P","capacity":1},"shardCount":2}}`
	resp := httpPut(t, redisCacheURL(srv.URL, "sub1", "rg1", "premium-shard"), body)
	assertStatus(t, resp, http.StatusCreated)

	respBody := decodeJSON(t, resp)
	props := respBody["properties"].(map[string]interface{})
	if props["shardCount"].(float64) != 2 {
		t.Errorf("shardCount = %v, want 2", props["shardCount"])
	}
}

func TestRedisCache_PUT_DefaultsSkuWhenAbsent(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, redisCacheURL(srv.URL, "sub1", "rg1", "default-sku"),
		`{"location":"uksouth","properties":{}}`)
	assertStatus(t, resp, http.StatusCreated)

	body := decodeJSON(t, resp)
	props := body["properties"].(map[string]interface{})
	sku, ok := props["sku"].(map[string]interface{})
	if !ok {
		t.Fatalf("sku missing: %T", props["sku"])
	}
	if sku["name"] != "Standard" {
		t.Errorf("sku.name = %v, want Standard", sku["name"])
	}
	if sku["family"] != "C" {
		t.Errorf("sku.family = %v, want C", sku["family"])
	}
	if sku["capacity"].(float64) != 1 {
		t.Errorf("sku.capacity = %v, want 1", sku["capacity"])
	}
}

func TestRedisCache_GET_Returns200(t *testing.T) {
	srv := newTestServer(t)
	url := redisCacheURL(srv.URL, "sub1", "rg1", "redis-get")

	resp := httpPut(t, url, redisCacheBodyStandardC1)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = httpGet(t, url)
	assertStatus(t, resp, http.StatusOK)
	body := decodeJSON(t, resp)
	if body["name"] != "redis-get" {
		t.Errorf("name = %v, want redis-get", body["name"])
	}
}

func TestRedisCache_GET_NotFound_Returns404(t *testing.T) {
	srv := newTestServer(t)
	resp := httpGet(t, redisCacheURL(srv.URL, "sub1", "rg1", "ghost"))
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

func TestRedisCache_HEAD_Exists_Returns204(t *testing.T) {
	srv := newTestServer(t)
	url := redisCacheURL(srv.URL, "sub1", "rg1", "redis-head")

	resp := httpPut(t, url, redisCacheBodyStandardC1)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = httpHead(t, url)
	assertStatus(t, resp, http.StatusNoContent)
	if body := readBody(t, resp); body != "" {
		t.Errorf("HEAD body = %q, want empty", body)
	}
}

func TestRedisCache_HEAD_NotFound_Returns404(t *testing.T) {
	srv := newTestServer(t)
	resp := httpHead(t, redisCacheURL(srv.URL, "sub1", "rg1", "ghost"))
	assertStatus(t, resp, http.StatusNotFound)
	if body := readBody(t, resp); body != "" {
		t.Errorf("HEAD body = %q, want empty", body)
	}
}

func TestRedisCache_DELETE_Returns202(t *testing.T) {
	srv := newTestServer(t)
	url := redisCacheURL(srv.URL, "sub1", "rg1", "redis-del")

	resp := httpPut(t, url, redisCacheBodyStandardC1)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = httpDelete(t, url)
	assertStatus(t, resp, http.StatusAccepted)
	if resp.Header.Get("Location") == "" {
		t.Error("DELETE Location header missing")
	}
	resp.Body.Close()

	resp = httpGet(t, url)
	assertStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
}

func TestRedisCache_DELETE_NotFound_Returns404(t *testing.T) {
	srv := newTestServer(t)
	resp := httpDelete(t, redisCacheURL(srv.URL, "sub1", "rg1", "ghost"))
	assertStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
}

func TestRedisCache_LIST_ByRG_ReturnsValueWrapper(t *testing.T) {
	srv := newTestServer(t)

	resp := httpPut(t, redisCacheURL(srv.URL, "sub1", "rg1", "list-a"), redisCacheBodyStandardC1)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()
	resp = httpPut(t, redisCacheURL(srv.URL, "sub1", "rg1", "list-b"), redisCacheBodyStandardC1)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = httpGet(t, redisCacheListByRGURL(srv.URL, "sub1", "rg1"))
	assertStatus(t, resp, http.StatusOK)
	body := decodeJSON(t, resp)
	items, ok := body["value"].([]interface{})
	if !ok {
		t.Fatalf("value field missing or wrong type: %T", body["value"])
	}
	if len(items) != 2 {
		t.Errorf("len(value) = %d, want 2", len(items))
	}
}

func TestRedisCache_LIST_BySub_FiltersOnType(t *testing.T) {
	srv := newTestServer(t)

	// Create a sibling storage account in the same subscription. The Redis
	// list-by-sub MUST filter to Microsoft.Cache/Redis only and not include
	// the storage account. Storage account PUT returns 200 (not 201).
	resp := httpPut(t, storageAccountURL(srv.URL, "sub1", "rg1", "sibstoreact"), storageAccountBodyLRS)
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	resp = httpPut(t, redisCacheURL(srv.URL, "sub1", "rg1", "subscope-cache"), redisCacheBodyStandardC1)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = httpGet(t, redisCacheListBySubURL(srv.URL, "sub1"))
	assertStatus(t, resp, http.StatusOK)
	body := decodeJSON(t, resp)
	items, ok := body["value"].([]interface{})
	if !ok {
		t.Fatalf("value field missing: %T", body["value"])
	}
	if len(items) != 1 {
		t.Errorf("len(value) = %d, want 1 (storage account must NOT leak in)", len(items))
	}
	if len(items) > 0 {
		first := items[0].(map[string]interface{})
		if first["type"] != redisCacheTypeString {
			t.Errorf("listed item type = %v, want %s", first["type"], redisCacheTypeString)
		}
	}
}

func TestRedisCache_ListKeys_ReturnsDeterministicKeys(t *testing.T) {
	srv := newTestServer(t)
	url := redisCacheURL(srv.URL, "sub1", "rg1", "redis-keys")
	keysURL := redisCacheListKeysURL(srv.URL, "sub1", "rg1", "redis-keys")

	resp := httpPut(t, url, redisCacheBodyStandardC1)
	assertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = httpPost(t, keysURL, "")
	assertStatus(t, resp, http.StatusOK)
	body := decodeJSON(t, resp)
	if body["primaryKey"] != redisDevPrimaryKey {
		t.Errorf("primaryKey = %v, want %s", body["primaryKey"], redisDevPrimaryKey)
	}
	if body["secondaryKey"] != redisDevSecondaryKey {
		t.Errorf("secondaryKey = %v, want %s", body["secondaryKey"], redisDevSecondaryKey)
	}
}

func TestRedisCache_ListKeys_NotFound_Returns404(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPost(t, redisCacheListKeysURL(srv.URL, "sub1", "rg1", "ghost"), "")
	assertStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
}

func TestRedisCache_MissingApiVersion_Returns400(t *testing.T) {
	srv := newTestServer(t)
	resp := httpGetRaw(t, redisCacheURL(srv.URL, "sub1", "rg1", "noapi"))
	assertStatus(t, resp, http.StatusBadRequest)
	resp.Body.Close()
}

func TestRedisCache_TagsNormalisedToEmptyObject(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, redisCacheURL(srv.URL, "sub1", "rg1", "notags"), redisCacheBodyStandardC1)
	assertStatus(t, resp, http.StatusCreated)
	body := decodeJSON(t, resp)
	tags, ok := body["tags"].(map[string]interface{})
	if !ok {
		t.Fatalf("tags missing or wrong type: %T (must serialise as {}, not null)", body["tags"])
	}
	if len(tags) != 0 {
		t.Errorf("tags = %v, want empty", tags)
	}
}

func TestRedisCache_AzureHeaders_PresentOnResponse(t *testing.T) {
	srv := newTestServer(t)
	resp := httpPut(t, redisCacheURL(srv.URL, "sub1", "rg1", "headers"), redisCacheBodyStandardC1)
	if resp.Header.Get("x-ms-request-id") == "" {
		t.Error("x-ms-request-id header missing")
	}
	if resp.Header.Get("x-ms-correlation-request-id") == "" {
		t.Error("x-ms-correlation-request-id header missing")
	}
	resp.Body.Close()
}

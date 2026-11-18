package arm

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

const apimUserBody = `{"properties":{"firstName":"Example","lastName":"User","email":"tom+tfdev@example.test","state":"active"}}`

func TestAPIMUser_Lifecycle(t *testing.T) {
	service, _ := newAPIMAPIParent(t)
	user := service + "/users/5931a75ae4bbd512288c680b"
	resp := httpGet(t, user+apimAPIVersion)
	assertStatus(t, resp, 404)
	resp.Body.Close()
	resp = httpHead(t, user+apimAPIVersion)
	assertStatus(t, resp, 404)
	if readBody(t, resp) != "" {
		t.Fatal("missing HEAD has body")
	}
	registered := ""
	for _, status := range []int{201, 200} {
		resp = httpPut(t, user+apimAPIVersion+"&notify=false", apimUserBody)
		assertStatus(t, resp, status)
		for _, header := range []string{"ETag", "x-ms-request-id", "x-ms-correlation-request-id"} {
			if resp.Header.Get(header) == "" {
				t.Fatalf("missing %s", header)
			}
		}
		body := decodeJSON(t, resp)
		props := body["properties"].(map[string]any)
		if body["type"] != apimUserType || body["name"] != "5931a75ae4bbd512288c680b" || len(body) != 4 ||
			props["firstName"] != "Example" || props["lastName"] != "User" || props["email"] != "tom+tfdev@example.test" ||
			props["state"] != "active" || len(props["groups"].([]any)) != 0 ||
			props["identities"].([]any)[0].(map[string]any)["provider"] != "Basic" ||
			props["identities"].([]any)[0].(map[string]any)["id"] != props["email"] {
			t.Fatalf("invalid UserContract: %v", body)
		}
		date := props["registrationDate"].(string)
		if _, err := time.Parse(time.RFC3339Nano, date); err != nil {
			t.Fatal("invalid registrationDate")
		}
		if registered != "" && registered != date {
			t.Fatal("update changed registrationDate")
		}
		registered = date
	}
	resp = httpHead(t, user+apimAPIVersion)
	assertStatus(t, resp, 200)
	if resp.Header.Get("ETag") == "" || readBody(t, resp) != "" {
		t.Fatal("HEAD needs ETag and no body")
	}
	updated := `{"properties":{"firstName":"Updated","lastName":"Name","email":"new@example.test","note":"administrator note","state":"blocked"}}`
	resp = httpPut(t, user+apimAPIVersion, updated)
	assertStatus(t, resp, 200)
	etag := resp.Header.Get("ETag")
	resp.Body.Close()
	resp = httpGet(t, user+apimAPIVersion)
	assertStatus(t, resp, 200)
	props := decodeJSON(t, resp)["properties"].(map[string]any)
	if props["firstName"] != "Updated" || props["state"] != "blocked" || props["note"] != "administrator note" ||
		props["registrationDate"] != registered || props["identities"].([]any)[0].(map[string]any)["id"] != "new@example.test" {
		t.Fatalf("update did not round-trip: %v", props)
	}
	for _, method := range []string{http.MethodPut, http.MethodPatch, http.MethodDelete} {
		req, err := http.NewRequest(method, user+apimAPIVersion, strings.NewReader(updated))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("If-Match", `"stale"`)
		resp, err = http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		assertStatus(t, resp, 412)
		resp.Body.Close()
	}
	req, err := http.NewRequest(http.MethodPatch, user+apimAPIVersion, strings.NewReader(`{"properties":{"state":"pending"}}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("If-Match", etag)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	assertStatus(t, resp, 200)
	props = decodeJSON(t, resp)["properties"].(map[string]any)
	if props["state"] != "pending" || props["firstName"] != "Updated" || props["note"] != "administrator note" {
		t.Fatal("PATCH did not merge")
	}
	resp = httpGet(t, service+"/users"+apimAPIVersion)
	if len(decodeJSON(t, resp)["value"].([]any)) != 1 {
		t.Fatal("missing user from list")
	}
	for i := 0; i < 2; i++ {
		resp = httpDelete(t, user+apimAPIVersion+"&deleteSubscriptions=true&notify=false&appType=developerPortal")
		assertStatus(t, resp, 204)
		resp.Body.Close()
	}
	resp = httpGet(t, user+apimAPIVersion)
	assertStatus(t, resp, 404)
	resp.Body.Close()
	resp = httpGet(t, service+"/users"+apimAPIVersion)
	if len(decodeJSON(t, resp)["value"].([]any)) != 0 {
		t.Fatal("user survived delete")
	}
}

func TestAPIMUser_ValidationAndSecrets(t *testing.T) {
	service, _ := newAPIMAPIParent(t)
	user := service + "/users/example"
	for _, test := range []struct {
		body   string
		status int
	}{
		{`{`, 400}, {`{}`, 400}, {`{"properties":null}`, 400},
		{`{"properties":{"firstName":"Example"}}`, 400},
		{strings.Replace(apimUserBody, `"Example"`, `false`, 1), 400},
		{strings.Replace(apimUserBody, `"Example"`, `""`, 1), 400},
		{strings.Replace(apimUserBody, `"Example"`, `"`+strings.Repeat("x", 101)+`"`, 1), 400},
		{strings.Replace(apimUserBody, `tom+tfdev@example.test`, `invalid`, 1), 400},
		{strings.Replace(apimUserBody, `active`, `invalid`, 1), 400},
		{strings.Replace(apimUserBody, `active`, `deleted`, 1), 501},
		{apimUserBody + `{}`, 400},
		{`{"properties":{"password":42}}`, 400},
		{`{"properties":{"confirmation":"invalid"}}`, 400},
		{`{"properties":{"confirmation":"invite"}}`, 501},
		{`{"properties":{"identities":[{"provider":"Google","id":"example"}]}}`, 501},
	} {
		resp := httpPut(t, user+apimAPIVersion, test.body)
		assertStatus(t, resp, test.status)
		if decodeJSON(t, resp)["error"] == nil {
			t.Fatal("expected Azure error")
		}
	}
	resp := httpGetRaw(t, user)
	assertStatus(t, resp, 400)
	if decodeJSON(t, resp)["error"].(map[string]any)["code"] != "MissingApiVersionParameter" {
		t.Fatal("missing version error")
	}
	for _, query := range []string{"&notify=true", "&notify=invalid", "&notify=false&notify=true", "&appType=invalid", "&deleteSubscriptions=no"} {
		resp = httpPut(t, user+apimAPIVersion+query, apimUserBody)
		if resp.StatusCode != 400 && resp.StatusCode != 501 {
			t.Fatal("invalid notification query accepted")
		}
		resp.Body.Close()
	}
	// Generate only at runtime: never place new passwords in repository fixtures.
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		t.Fatal(err)
	}
	secret := hex.EncodeToString(random)
	for _, body := range []string{
		`{"properties":{"password":"` + secret + `"}}`,
		`{"` + secret + `":true,"properties":{}}`,
		`{"properties":{"` + secret + `":true}}`,
		`{"properties":{"password":"` + secret + `"`,
	} {
		resp = httpPut(t, user+apimAPIVersion, body)
		if resp.StatusCode != 400 && resp.StatusCode != 501 {
			t.Fatal("password input unexpectedly succeeded")
		}
		if strings.Contains(readBody(t, resp), secret) {
			t.Fatal("secret leaked through error")
		}
	}
	resp = httpGet(t, service+"/users"+apimAPIVersion)
	if len(decodeJSON(t, resp)["value"].([]any)) != 0 {
		t.Fatal("rejected secret requests persisted users")
	}
	// Default active state, optional note replacement and service email uniqueness.
	defaultBody := strings.Replace(apimUserBody, `,"state":"active"`, "", 1)
	resp = httpPut(t, user+apimAPIVersion, defaultBody)
	assertStatus(t, resp, 201)
	if decodeJSON(t, resp)["properties"].(map[string]any)["state"] != "active" {
		t.Fatal("state did not default active")
	}
	resp = httpPut(t, user+"-duplicate"+apimAPIVersion, strings.ReplaceAll(defaultBody, "tom+tfdev", "TOM+TFDEV"))
	assertStatus(t, resp, 400)
	resp.Body.Close()
	resp = httpGet(t, service+"/users"+apimAPIVersion+"&$filter=state%20eq%20%27active%27")
	assertStatus(t, resp, 501)
	resp.Body.Close()
}

func TestAPIMUser_ParentAndCascade(t *testing.T) {
	for _, scope := range []string{"service", "rg"} {
		t.Run(scope, func(t *testing.T) {
			service, rg := newAPIMAPIParent(t)
			user := service + "/users/example"
			resp := httpPut(t, user+apimAPIVersion, apimUserBody)
			assertStatus(t, resp, 201)
			resp.Body.Close()
			parent := service
			if scope == "rg" {
				parent = rg
			}
			resp = httpDelete(t, parent+apimAPIVersion)
			resp.Body.Close()
			resp = httpGet(t, user+apimAPIVersion)
			assertStatus(t, resp, 404)
			resp.Body.Close()
			resp = httpPut(t, user+apimAPIVersion, apimUserBody)
			assertStatus(t, resp, 404)
			if decodeJSON(t, resp)["error"].(map[string]any)["code"] != "ParentResourceNotFound" {
				t.Fatal("missing parent error")
			}
		})
	}
}

func TestAPIMUser_ContractJSON(t *testing.T) {
	// Guard against accidentally adding secret fields to the response shape.
	service, _ := newAPIMAPIParent(t)
	resp := httpPut(t, service+"/users/example"+apimAPIVersion, apimUserBody)
	body := decodeJSON(t, resp)
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"password", "confirmation", "provisioningState", "location", "tags"} {
		if strings.Contains(string(data), `"`+field+`"`) {
			t.Fatalf("unexpected UserContract field %s", field)
		}
	}
}

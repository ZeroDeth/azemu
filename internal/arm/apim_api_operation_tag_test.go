package arm

import (
	"net/http"
	"strings"
	"testing"
)

func TestAPIMAPIOperationTag_Lifecycle(t *testing.T) {
	api, service, _ := newAPIMOperationParent(t)
	operation := service + "/apis/Example/operations/sample"
	resp := httpPut(t, operation+apimAPIVersion, apimOperationBody)
	assertStatus(t, resp, 201)
	resp.Body.Close()
	tag := service + "/tags/example-Tag"
	assignment := operation + "/tags/example-Tag"
	for _, status := range []int{201, 200} {
		resp = httpPut(t, tag+apimAPIVersion, `{"properties":{"displayName":"display-name"}}`)
		assertStatus(t, resp, status)
		resp.Body.Close()
		resp = httpPut(t, assignment+apimAPIVersion, "")
		assertStatus(t, resp, status)
		for _, header := range []string{"ETag", "x-ms-request-id", "x-ms-correlation-request-id"} {
			if resp.Header.Get(header) == "" {
				t.Fatalf("missing %s", header)
			}
		}
		body := decodeJSON(t, resp)
		if body["type"] != apimTagType || body["name"] != "example-Tag" ||
			!strings.HasSuffix(body["id"].(string), "/service/example-apim/tags/example-Tag") || len(body) != 4 ||
			body["properties"].(map[string]any)["displayName"] != "display-name" {
			t.Fatalf("unexpected TagContract: %v", body)
		}
	}
	for _, base := range []string{operation, api + "/operations/sample"} {
		resp = httpGet(t, base+"/tags/example-Tag"+apimAPIVersion)
		assertStatus(t, resp, 200)
		resp.Body.Close()
		resp = httpHead(t, base+"/tags/example-Tag"+apimAPIVersion)
		assertStatus(t, resp, 200)
		if resp.Header.Get("ETag") == "" || readBody(t, resp) != "" {
			t.Fatal("HEAD needs ETag and no body")
		}
		resp = httpGet(t, base+"/tags"+apimAPIVersion)
		if len(decodeJSON(t, resp)["value"].([]any)) != 1 {
			t.Fatal("alias list lost/duplicated assignment")
		}
	}
	resp = httpPut(t, tag+apimAPIVersion, `{"properties":{"displayName":"renamed"}}`)
	assertStatus(t, resp, 200)
	resp.Body.Close()
	resp = httpGet(t, assignment+apimAPIVersion)
	if decodeJSON(t, resp)["properties"].(map[string]any)["displayName"] != "renamed" {
		t.Fatal("association retained stale displayName")
	}
	// A non-current revision must not borrow the current revision's links.
	revision2 := service + "/apis/Example;rev=2"
	resp = httpPut(t, revision2+apimAPIVersion, apimAPIBody)
	assertStatus(t, resp, 201)
	resp.Body.Close()
	resp = httpPut(t, revision2+"/operations/sample"+apimAPIVersion, apimOperationBody)
	assertStatus(t, resp, 201)
	resp.Body.Close()
	resp = httpGet(t, revision2+"/operations/sample/tags/example-Tag"+apimAPIVersion)
	assertStatus(t, resp, 404)
	resp.Body.Close()
	resp = httpHead(t, tag+apimAPIVersion)
	assertStatus(t, resp, 200)
	if readBody(t, resp) != "" {
		t.Fatal("tag HEAD has body")
	}
	resp = httpGet(t, service+"/tags"+apimAPIVersion)
	if len(decodeJSON(t, resp)["value"].([]any)) != 1 {
		t.Fatal("missing service tag")
	}
	for i := 0; i < 2; i++ {
		resp = httpDelete(t, assignment+apimAPIVersion)
		assertStatus(t, resp, 204)
		resp.Body.Close()
	}
	resp = httpGet(t, assignment+apimAPIVersion)
	assertStatus(t, resp, 404)
	resp.Body.Close()
	resp = httpHead(t, assignment+apimAPIVersion)
	assertStatus(t, resp, 404)
	if readBody(t, resp) != "" {
		t.Fatal("missing HEAD has body")
	}
	resp = httpGet(t, tag+apimAPIVersion)
	assertStatus(t, resp, 200)
	resp.Body.Close()
	resp = httpGet(t, operation+"/tags"+apimAPIVersion)
	if len(decodeJSON(t, resp)["value"].([]any)) != 0 {
		t.Fatal("assignment survived detach")
	}
}

func TestAPIMAPIOperationTag_ValidationAndCascade(t *testing.T) {
	api, service, _ := newAPIMOperationParent(t)
	operation := api + "/operations/sample"
	tag := service + "/tags/example"
	assignment := operation + "/tags/example"
	resp := httpPut(t, assignment+apimAPIVersion, "")
	assertStatus(t, resp, 404)
	if decodeJSON(t, resp)["error"].(map[string]any)["code"] != "ParentResourceNotFound" {
		t.Fatal("missing operation error")
	}
	resp = httpPut(t, operation+apimAPIVersion, apimOperationBody)
	assertStatus(t, resp, 201)
	resp.Body.Close()
	resp = httpPut(t, assignment+apimAPIVersion, "")
	assertStatus(t, resp, 404)
	resp.Body.Close()
	for _, body := range []string{`{`, `{}`, `null`, `{"properties":null}`, `{"properties":{"displayName":3}}`,
		`{"properties":{"displayName":""}}`, `{"properties":{"displayName":"x","unknown":true}}`,
		`{"properties":{"displayName":"x"}} {}`, `{"properties":{"displayName":"` + strings.Repeat("x", 161) + `"}}`} {
		resp = httpPut(t, tag+apimAPIVersion, body)
		assertStatus(t, resp, 400)
		if decodeJSON(t, resp)["error"] == nil {
			t.Fatal("expected Azure error")
		}
	}
	resp = httpGetRaw(t, tag)
	assertStatus(t, resp, 400)
	if decodeJSON(t, resp)["error"].(map[string]any)["code"] != "MissingApiVersionParameter" {
		t.Fatal("missing API version error")
	}
	resp = httpGetRaw(t, assignment)
	assertStatus(t, resp, 400)
	resp.Body.Close()
	resp = httpPut(t, tag+apimAPIVersion, `{"properties":{"displayName":"example"}}`)
	assertStatus(t, resp, 201)
	etag := resp.Header.Get("ETag")
	resp.Body.Close()
	for _, method := range []string{http.MethodPut, http.MethodDelete} {
		req, err := http.NewRequest(method, tag+apimAPIVersion, strings.NewReader(`{"properties":{"displayName":"new"}}`))
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
	if etag == "" {
		t.Fatal("missing ETag")
	}
	resp = httpPut(t, assignment+apimAPIVersion, `{}`)
	assertStatus(t, resp, 400)
	resp.Body.Close()
	resp = httpGet(t, operation+"/tags"+apimAPIVersion+"&$top=1")
	assertStatus(t, resp, 501)
	resp.Body.Close()
	for _, parent := range []string{tag, operation, api, service} {
		t.Run(parent[strings.LastIndex(parent, "/")+1:], func(t *testing.T) {
			// Recreate each deleted ancestor and assignment for this cascade.
			for _, item := range []struct{ url, body string }{
				{service, apimBasicBody}, {api, apimAPIBody}, {operation, apimOperationBody},
				{tag, `{"properties":{"displayName":"example"}}`}, {assignment, ""},
			} {
				resp = httpPut(t, item.url+apimAPIVersion, item.body)
				if resp.StatusCode != 200 && resp.StatusCode != 201 {
					t.Fatalf("setup: %d %s", resp.StatusCode, readBody(t, resp))
				}
				resp.Body.Close()
			}
			resp = httpDelete(t, parent+apimAPIVersion)
			resp.Body.Close()
			resp = httpGet(t, assignment+apimAPIVersion)
			assertStatus(t, resp, 404)
			resp.Body.Close()
		})
	}
	resp = httpPut(t, tag+apimAPIVersion, `{"properties":{"displayName":"example"}}`)
	assertStatus(t, resp, 404)
	if decodeJSON(t, resp)["error"].(map[string]any)["code"] != "ParentResourceNotFound" {
		t.Fatal("missing service error")
	}
}

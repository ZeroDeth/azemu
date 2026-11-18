package arm

import (
	"net/http"
	"strings"
	"testing"
)

func TestAPIMTag_Contract(t *testing.T) {
	service, _ := newAPIMAPIParent(t)
	tag := service + "/tags/space%20allowed"
	resp := httpPut(t, tag+apimAPIVersion, `{"properties":{"displayName":"display"}}`)
	assertStatus(t, resp, 201)
	etag := resp.Header.Get("ETag")
	resp.Body.Close()
	req, err := http.NewRequest(http.MethodPut, tag+apimAPIVersion, strings.NewReader(`{"properties":{"displayName":"updated"}}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("If-Match", etag)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	assertStatus(t, resp, 200)
	if resp.Header.Get("ETag") == etag {
		t.Fatal("ETag did not change")
	}
	resp.Body.Close()
	resp = httpPut(t, service+"/tags/other"+apimAPIVersion, `{"properties":{"displayName":"UPDATED"}}`)
	assertStatus(t, resp, 400)
	resp.Body.Close()
	for _, name := range []string{"bad+name", strings.Repeat("x", 81)} {
		resp = httpPut(t, service+"/tags/"+name+apimAPIVersion, `{"properties":{"displayName":"new"}}`)
		assertStatus(t, resp, 400)
		resp.Body.Close()
	}
	resp = httpGet(t, service+"/tags"+apimAPIVersion+"&$top=1")
	assertStatus(t, resp, 501)
	resp.Body.Close()
	resp = httpDelete(t, tag+apimAPIVersion)
	assertStatus(t, resp, 204)
	resp.Body.Close()
	resp = httpHead(t, tag+apimAPIVersion)
	assertStatus(t, resp, 404)
	if readBody(t, resp) != "" {
		t.Fatal("missing HEAD has body")
	}
	resp = httpGet(t, tag+apimAPIVersion)
	assertStatus(t, resp, 404)
	resp.Body.Close()
}

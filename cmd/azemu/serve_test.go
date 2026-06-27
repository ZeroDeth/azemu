package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zerodeth/azemu/internal/arm"
	"github.com/zerodeth/azemu/internal/store"
)

// TestCDNHostMux_routing locks down the real entrypoint: a
// {endpoint}.azureedge.net host must be dispatched to the CDN data plane and
// bypass the ARM router, while every other host falls through to ARM.
func TestCDNHostMux_routing(t *testing.T) {
	ar := arm.NewRouter(store.NewMemoryStore(), "http://azurite:10000", "https://kv", "redis://r:6379")

	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusTeapot)
	})
	mux := cdnHostMux(ar, next)

	// CDN host: handled by ServeCDNContent, so the ARM next handler is bypassed.
	// No endpoint is seeded, so it returns 404, but the point is that the ARM
	// path was not taken.
	req := httptest.NewRequest(http.MethodGet, "http://otacdn.azureedge.net/c/blob", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if nextCalled {
		t.Fatal("CDN host was routed to the ARM handler instead of ServeCDNContent")
	}

	// Non-CDN host: falls through to the ARM handler.
	nextCalled = false
	req = httptest.NewRequest(http.MethodGet, "http://localhost:4566/subscriptions", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if !nextCalled {
		t.Fatal("non-CDN host did not fall through to the ARM handler")
	}
}

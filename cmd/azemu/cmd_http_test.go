package main

import (
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// probeHealth
// ---------------------------------------------------------------------------

func TestProbeHealth_returns200_true(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	if !probeHealth(srv.URL) {
		t.Fatal("want true for 200 response, got false")
	}
}

func TestProbeHealth_returns500_false(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	if probeHealth(srv.URL) {
		t.Fatal("want false for 500 response, got true")
	}
}

func TestProbeHealth_returns503_false(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	if probeHealth(srv.URL) {
		t.Fatal("want false for 503 response, got true")
	}
}

func TestProbeHealth_connectionRefused_false(t *testing.T) {
	// Bind an ephemeral port, record the address, close immediately.
	// The OS will not immediately reassign it, so probeHealth receives
	// connection refused rather than a timeout.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()
	if probeHealth("http://" + addr) {
		t.Fatal("want false for connection refused, got true")
	}
}

// ---------------------------------------------------------------------------
// waitForHealth
// ---------------------------------------------------------------------------

func TestWaitForHealth_alreadyHealthy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	if err := waitForHealth(srv.URL, 2*time.Second); err != nil {
		t.Fatalf("want nil, got %v", err)
	}
}

func TestWaitForHealth_becomesHealthy(t *testing.T) {
	var callCount atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := callCount.Add(1)
		if n <= 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	if err := waitForHealth(srv.URL, 5*time.Second); err != nil {
		t.Fatalf("want nil after server becomes healthy, got %v", err)
	}
	if got := callCount.Load(); got < 3 {
		t.Errorf("want at least 3 poll calls, got %d", got)
	}
}

func TestWaitForHealth_timeoutExpired(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	// Very short timeout so the test finishes quickly.
	err := waitForHealth(srv.URL, 10*time.Millisecond)
	if err == nil {
		t.Fatal("want error on timeout, got nil")
	}
}

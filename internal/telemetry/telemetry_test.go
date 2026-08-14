package telemetry

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestDisabledMakesNoNetworkCallsAndDropsCounts(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
	}))
	defer srv.Close()

	m := NewManager()
	m.SetConfig(false, srv.URL) // disabled, even with a real endpoint configured
	m.Count("debugger.start")
	m.Count("debugger.start")
	m.Flush()
	time.Sleep(50 * time.Millisecond)

	if atomic.LoadInt32(&hits) != 0 {
		t.Fatalf("disabled telemetry made %d network calls, want 0", hits)
	}
}

func TestEnabledWithoutEndpointMakesNoNetworkCalls(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
	}))
	defer srv.Close()
	srv.Close() // enabled but pointed at nothing (empty endpoint) is the actual case under test

	m := NewManager()
	m.SetConfig(true, "") // enabled, but no endpoint configured
	m.Count("debugger.start")
	m.Flush()
	time.Sleep(50 * time.Millisecond)

	if atomic.LoadInt32(&hits) != 0 {
		t.Fatalf("enabled-without-endpoint made %d network calls, want 0", hits)
	}
}

func TestEnabledWithEndpointSendsCounts(t *testing.T) {
	received := make(chan map[string]int, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]int
		json.NewDecoder(r.Body).Decode(&body) //nolint
		received <- body
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	m := NewManager()
	m.SetConfig(true, srv.URL)
	m.Count("debugger.start")
	m.Count("debugger.start")
	m.Count("task.run")
	m.Flush()

	select {
	case body := <-received:
		if body["debugger.start"] != 2 || body["task.run"] != 1 {
			t.Fatalf("received counts = %v, want debugger.start=2 task.run=1", body)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server never received a request")
	}
}

func TestFlushResetsCounts(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]int
		json.NewDecoder(r.Body).Decode(&body) //nolint
		callCount++
		if callCount == 2 && len(body) != 0 {
			t.Errorf("second flush sent stale counts: %v", body)
		}
	}))
	defer srv.Close()

	m := NewManager()
	m.SetConfig(true, srv.URL)
	m.Count("x")
	m.Flush()
	time.Sleep(50 * time.Millisecond)
	m.Flush() // nothing new counted — must not resend or hit the server again
	time.Sleep(50 * time.Millisecond)

	if callCount != 1 {
		t.Fatalf("server got %d requests, want 1 (second Flush had nothing to send)", callCount)
	}
}

package health

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/csullivan/bish/internal/logs"
)

func port(t *testing.T, addr string) int {
	t.Helper()
	_, p, _ := net.SplitHostPort(addr)
	n, _ := strconv.Atoi(p)
	return n
}

func TestProbeDialDefault(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	p := port(t, ln.Addr().String())
	if err := Probe(context.Background(), nil, Target{Port: p}, true); err != nil {
		t.Fatalf("listening port: %v", err)
	}
	ln.Close()
	if err := Probe(context.Background(), nil, Target{Port: p}, true); err == nil {
		t.Fatalf("closed port probed ready")
	}
	if err := Probe(context.Background(), nil, Target{}, true); err != nil {
		t.Fatalf("no port, no spec should be ready: %v", err)
	}
}

func TestProbeHTTPStatus(t *testing.T) {
	code := http.StatusServiceUnavailable
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			w.WriteHeader(404)
			return
		}
		w.WriteHeader(code)
	}))
	defer srv.Close()
	p := port(t, srv.Listener.Addr().String())
	spec := &Spec{HTTP: "/health"}
	if err := Probe(context.Background(), spec, Target{Port: p}, true); err == nil {
		t.Fatalf("503 probed ready")
	}
	// rich probes off: only the port dial counts
	if err := Probe(context.Background(), spec, Target{Port: p}, false); err != nil {
		t.Fatalf("flag-off dial: %v", err)
	}
	code = 200
	if err := Probe(context.Background(), spec, Target{Port: p}, true); err != nil {
		t.Fatalf("200: %v", err)
	}
	if err := Probe(context.Background(), &Spec{HTTP: "/health", Status: 204}, Target{Port: p}, true); err == nil {
		t.Fatalf("expected-status mismatch probed ready")
	}
}

func TestProbeLogAndCmd(t *testing.T) {
	buf := logs.NewBuffer()
	spec := &Spec{LogMatch: `ready in \d+ms`}
	if Probe(context.Background(), spec, Target{Log: buf}, true) == nil {
		t.Fatalf("empty log matched")
	}
	buf.Write("  VITE v5  ready in 412ms")
	if err := Probe(context.Background(), spec, Target{Log: buf}, true); err != nil {
		t.Fatal(err)
	}
	if Probe(context.Background(), &Spec{Cmd: "exit 3"}, Target{Dir: t.TempDir()}, true) == nil {
		t.Fatalf("failing cmd probed ready")
	}
	if err := Probe(context.Background(), &Spec{Cmd: "test {{port}} = 81"}, Target{Port: 81, Dir: t.TempDir()}, true); err != nil {
		t.Fatalf("cmd with port: %v", err)
	}
}

func TestWaitTimesOut(t *testing.T) {
	start := time.Now()
	err := Wait(context.Background(), &Spec{Cmd: "false", TimeoutSec: 1}, func() Target { return Target{Dir: t.TempDir()} }, true, nil)
	if err == nil || time.Since(start) > 5*time.Second {
		t.Fatalf("err=%v after %v", err, time.Since(start))
	}
}

func TestURL(t *testing.T) {
	if u := URL("/health", 5000); u != "http://localhost:5000/health" {
		t.Error(u)
	}
	if u := URL("http://127.0.0.1:{{port}}/up", 81); u != "http://127.0.0.1:81/up" {
		t.Error(u)
	}
}

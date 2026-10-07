// Package health decides when a launched service is "ready": a TCP dial on
// its port by default, or any composition of an HTTP check, a log-line
// match, and a shell command when a service declares one.
package health

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/csullivan/bish/internal/logs"
	"github.com/csullivan/bish/internal/shellenv"
)

// DefaultTimeout matches the old shell probe's 600-iteration wait — a cold
// dependency boot (install + migrate + build) can be slow.
const DefaultTimeout = 600 * time.Second

// Interval is how often Wait re-probes.
const Interval = 500 * time.Millisecond

// Spec declares what "ready" means for a service. The zero value is a TCP
// dial on the service's effective port.
type Spec struct {
	HTTP       string `json:"http,omitempty"`       // "/health", or a full URL; {{port}} substituted
	Status     int    `json:"status,omitempty"`     // expected code; 0 = any 2xx/3xx
	LogMatch   string `json:"logMatch,omitempty"`   // regexp matched against the service's log buffer
	Cmd        string `json:"cmd,omitempty"`        // exit 0 = ready
	TimeoutSec int    `json:"timeoutSec,omitempty"` // 0 = DefaultTimeout
}

// Timeout is how long Wait keeps probing before giving up.
func (s *Spec) Timeout() time.Duration {
	if s == nil || s.TimeoutSec <= 0 {
		return DefaultTimeout
	}
	return time.Duration(s.TimeoutSec) * time.Second
}

// Rich reports whether s declares anything beyond the default port dial.
func (s *Spec) Rich() bool {
	return s != nil && (s.HTTP != "" || s.LogMatch != "" || s.Cmd != "")
}

// Target is what a probe runs against.
type Target struct {
	Port int             // effective port; 0 = no port
	Dir  string          // working dir for Cmd probes
	Log  *logs.LogBuffer // nil = LogMatch can't pass yet
}

// Probe makes one readiness attempt. rich=false ignores HTTP/LogMatch/Cmd
// and only dials the port (the feature-flag-off path). Declared probe kinds
// compose: every one set must pass. With nothing declared and no port,
// there's nothing to wait on and the service counts as ready.
func Probe(ctx context.Context, s *Spec, t Target, rich bool) error {
	if !rich || !s.Rich() {
		if t.Port <= 0 {
			return nil
		}
		return Dial(t.Port)
	}
	if s.HTTP != "" {
		if err := probeHTTP(ctx, s, t.Port); err != nil {
			return err
		}
	}
	if s.LogMatch != "" {
		if err := probeLog(s.LogMatch, t.Log); err != nil {
			return err
		}
	}
	if s.Cmd != "" {
		if err := probeCmd(ctx, s.Cmd, t); err != nil {
			return err
		}
	}
	return nil
}

// Dial reports whether something accepts connections on port, over either
// stack — some servers bind *:port, others [::1] only.
func Dial(port int) error {
	var last error
	for _, host := range []string{"127.0.0.1", "::1"} {
		c, err := net.DialTimeout("tcp", net.JoinHostPort(host, strconv.Itoa(port)), 300*time.Millisecond)
		if err == nil {
			c.Close()
			return nil
		}
		last = err
	}
	return fmt.Errorf("nothing listening on :%d (%v)", port, last)
}

// URL resolves an HTTP spec ("/health" or a full URL) against port.
func URL(raw string, port int) string {
	raw = strings.ReplaceAll(raw, "{{port}}", strconv.Itoa(port))
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		return raw
	}
	if !strings.HasPrefix(raw, "/") {
		raw = "/" + raw
	}
	return "http://localhost:" + strconv.Itoa(port) + raw
}

var client = &http.Client{
	Timeout: 2 * time.Second,
	// a redirect to a login page is still "the server is up"; don't follow
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}

func probeHTTP(ctx context.Context, s *Spec, port int) error {
	u := URL(s.HTTP, port)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("http %s: unreachable", s.HTTP)
	}
	resp.Body.Close()
	ok := resp.StatusCode >= 200 && resp.StatusCode < 400
	if s.Status != 0 {
		ok = resp.StatusCode == s.Status
	}
	if !ok {
		return fmt.Errorf("http %s → %d", s.HTTP, resp.StatusCode)
	}
	return nil
}

func probeLog(pattern string, buf *logs.LogBuffer) error {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return fmt.Errorf("bad logMatch: %v", err)
	}
	if buf != nil {
		for _, line := range buf.Lines(1000) {
			if re.MatchString(line) {
				return nil
			}
		}
	}
	return fmt.Errorf("log hasn't matched /%s/ yet", pattern)
}

func probeCmd(ctx context.Context, cmd string, t Target) error {
	cmd = strings.ReplaceAll(cmd, "{{port}}", strconv.Itoa(t.Port))
	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	c := exec.CommandContext(cctx, shellenv.DefaultShell(), "-l", "-c", cmd)
	c.Dir = t.Dir
	if err := c.Run(); err != nil {
		return fmt.Errorf("probe cmd: %v", err)
	}
	return nil
}

// Wait probes every Interval until ready, the spec's timeout elapses, or ctx
// is cancelled. onFail sees each failed attempt's error (for live status
// detail). Returns nil once ready, ctx.Err() on cancel, or the last probe
// error on timeout.
func Wait(ctx context.Context, s *Spec, t func() Target, rich bool, onFail func(error)) error {
	deadline := time.Now().Add(s.Timeout())
	for {
		err := Probe(ctx, s, t(), rich)
		if err == nil {
			return nil
		}
		if onFail != nil {
			onFail(err)
		}
		if time.Now().After(deadline) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(Interval):
		}
	}
}

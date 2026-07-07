package app

import "testing"

func TestAppBundlePath(t *testing.T) {
	got, ok := appBundlePath("/Applications/bish.app/Contents/MacOS/bish")
	if !ok || got != "/Applications/bish.app" {
		t.Fatalf("got %q, %v", got, ok)
	}
	if _, ok := appBundlePath("/usr/local/bin/bish"); ok {
		t.Fatalf("expected no bundle match for raw binary path")
	}
}

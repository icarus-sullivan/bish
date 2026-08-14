package extensions

import (
	"os"
	"path/filepath"
	"testing"
)

func writeExt(t *testing.T, root, name, manifest, script string) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if manifest != "" {
		if err := os.WriteFile(filepath.Join(dir, "bish-extension.json"), []byte(manifest), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if script != "" {
		if err := os.WriteFile(filepath.Join(dir, "extension.js"), []byte(script), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDiscoverFindsValidExtensionAndSkipsBroken(t *testing.T) {
	root := t.TempDir()

	writeExt(t, root, "hello", `{"name":"hello","main":"extension.js","commands":[{"id":"sayHi","title":"Say Hi"}]}`,
		"onmessage = () => {}")
	writeExt(t, root, "no-manifest", "", "onmessage = () => {}")                // missing manifest
	writeExt(t, root, "bad-json", "{not json", "onmessage = () => {}")          // malformed manifest
	writeExt(t, root, "missing-script", `{"name":"x","main":"missing.js"}`, "") // main doesn't exist

	got := Discover(root)
	if len(got) != 1 {
		t.Fatalf("got %d extensions, want 1: %+v", len(got), got)
	}
	if got[0].Name != "hello" {
		t.Errorf("Name = %q, want %q", got[0].Name, "hello")
	}
	if got[0].Script != "onmessage = () => {}" {
		t.Errorf("Script not loaded correctly: %q", got[0].Script)
	}
	if len(got[0].Commands) != 1 || got[0].Commands[0].ID != "sayHi" {
		t.Errorf("Commands = %+v, want one contribution with id sayHi", got[0].Commands)
	}
}

func TestDiscoverMissingDirReturnsNil(t *testing.T) {
	if got := Discover(filepath.Join(t.TempDir(), "does-not-exist")); got != nil {
		t.Fatalf("got %+v, want nil", got)
	}
}

// The shipped examples/extensions/hello-world sample is the acceptance-test
// fixture for this whole feature — this guards it from silently bit-rotting
// (a typo'd manifest key, a renamed field) since nothing else exercises it.
func TestDiscoverFindsShippedHelloWorldSample(t *testing.T) {
	root := "../../examples/extensions"
	got := Discover(root)
	var hello *Extension
	for i := range got {
		if got[i].Name == "hello-world" {
			hello = &got[i]
		}
	}
	if hello == nil {
		t.Fatalf("hello-world sample not found in %s (got %+v)", root, got)
	}
	if len(hello.Commands) != 1 || hello.Commands[0].ID != "sayHello" {
		t.Errorf("Commands = %+v, want one contribution with id sayHello", hello.Commands)
	}
	if len(hello.Panels) != 1 || hello.Panels[0].ID != "greeting" {
		t.Errorf("Panels = %+v, want one contribution with id greeting", hello.Panels)
	}
	if hello.Script == "" {
		t.Error("Script is empty — extension.js failed to load")
	}
}

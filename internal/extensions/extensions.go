// Package extensions discovers local, unsigned bish extensions —
// ~/.bish/extensions/<name>/bish-extension.json declaring contributed
// commands/panels plus an entry script. No marketplace, no download step:
// same "you already trust code you run" posture as the rest of a shell IDE.
// The manifest declares commands/panels statically (not registered at
// runtime by the script) so the Command Palette can list them at startup
// without waiting on — or trusting the timing of — the extension's own code.
package extensions

import (
	"embed"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
)

type Contribution struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	// Key is an optional default keybind ("mod+shift+i") the host registers
	// the moment the contributing extension's worker starts — no separate
	// Settings step needed, unlike user-defined keybinds (keymap.ts).
	Key string `json:"key,omitempty"`
}

type Manifest struct {
	Name     string         `json:"name"`
	Main     string         `json:"main"` // entry script, relative to the extension's own dir
	Commands []Contribution `json:"commands,omitempty"`
	Panels   []Contribution `json:"panels,omitempty"`
}

// Extension is a discovered, loadable extension — Script is the entry
// file's full source, read now so the frontend can run it in a Web Worker
// via a blob URL without needing filesystem access of its own.
type Extension struct {
	Manifest
	Dir    string `json:"dir"`
	Script string `json:"script"`
}

func Dir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".bish", "extensions")
}

// Discover reads every <root>/*/bish-extension.json under root. A malformed
// manifest or unreadable entry script just drops that one extension —
// never fails the whole scan.
func Discover(root string) []Extension {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var out []Extension
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(root, e.Name())
		data, err := os.ReadFile(filepath.Join(dir, "bish-extension.json"))
		if err != nil {
			continue
		}
		var m Manifest
		if json.Unmarshal(data, &m) != nil || m.Name == "" || m.Main == "" {
			continue
		}
		script, err := os.ReadFile(filepath.Join(dir, m.Main))
		if err != nil {
			continue
		}
		out = append(out, Extension{Manifest: m, Dir: dir, Script: string(script)})
	}
	return out
}

// builtinFS holds bish's own bundled extensions — shipped in the binary so
// they show up under root without the user hand-copying files, same shape
// as any other discovered extension once written out.
//
//go:embed builtin
var builtinFS embed.FS

// SeedBuiltins writes every builtin/<name>/ extension into root, skipping
// any that's already there. Callers should only invoke this once ever (see
// config.Config.BuiltinExtensionsSeeded) — re-running it on every launch
// would resurrect an extension the user deliberately uninstalled.
func SeedBuiltins(root string) error {
	entries, err := builtinFS.ReadDir("builtin")
	if err != nil {
		return err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		srcDir := "builtin/" + e.Name()
		files, err := builtinFS.ReadDir(srcDir)
		if err != nil {
			continue
		}
		dstDir := filepath.Join(root, e.Name())
		if err := os.MkdirAll(dstDir, 0o755); err != nil {
			continue
		}
		for _, f := range files {
			data, err := fs.ReadFile(builtinFS, srcDir+"/"+f.Name())
			if err != nil {
				continue
			}
			_ = os.WriteFile(filepath.Join(dstDir, f.Name()), data, 0o644)
		}
	}
	return nil
}

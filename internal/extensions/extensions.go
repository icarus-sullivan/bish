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
	"encoding/base64"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type Contribution struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	// Key is an optional default keybind ("mod+shift+i") the host registers
	// the moment the contributing extension's worker starts — no separate
	// Settings step needed, unlike user-defined keybinds (keymap.ts).
	Key string `json:"key,omitempty"`
	// Icon is this panel's own icon (tab bar / sidebar entry): a
	// @tabler/icons-svelte name ("IconTimeline"), an http(s) URL, base64
	// image data, or a local image file. Only meaningful on panel
	// contributions — unknown/empty falls back to a generic icon.
	Icon string `json:"icon,omitempty"`
	// Icon may also be an http(s) URL or base64 (data: URI or raw) — those
	// the frontend handles itself. When it's a local image file path
	// (relative to the extension's dir, absolute, or ~/…), Discover reads it
	// here since the webview has no filesystem access: SVGs become raw
	// markup in IconSVG (inlined, so they pick up the theme color; the
	// frontend sanitizes it), rasters become a data: URI in IconSrc.
	IconSVG string `json:"iconSvg,omitempty"`
	IconSrc string `json:"iconSrc,omitempty"`
}

// maxIconFile caps a manifest-referenced icon file — icons are tiny; anything
// bigger is a mistake (or not an icon) and is just ignored.
const maxIconFile = 256 << 10

var iconMIME = map[string]string{
	".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg",
	".gif": "image/gif", ".webp": "image/webp", ".ico": "image/x-icon",
}

// readIconFile loads a panel's icon when it names a local image file. Any
// failure (not a file path, missing, too big) returns empty strings and the
// frontend falls back to its other icon forms / a generic icon.
func readIconFile(dir, icon string) (svg, src string) {
	lower := strings.ToLower(icon)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") || strings.HasPrefix(lower, "data:") {
		return "", ""
	}
	ext := filepath.Ext(lower)
	mime, raster := iconMIME[ext]
	if ext != ".svg" && !raster {
		return "", ""
	}
	p := icon
	if strings.HasPrefix(p, "~/") {
		home, _ := os.UserHomeDir()
		p = filepath.Join(home, p[2:])
	} else if !filepath.IsAbs(p) {
		p = filepath.Join(dir, p)
	}
	info, err := os.Stat(p)
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxIconFile {
		return "", ""
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return "", ""
	}
	if !raster {
		return string(data), ""
	}
	return "", "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data)
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
		for i := range m.Panels {
			m.Panels[i].IconSVG, m.Panels[i].IconSrc = readIconFile(dir, m.Panels[i].Icon)
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

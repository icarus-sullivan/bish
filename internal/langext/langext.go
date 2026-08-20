// Package langext is bish's per-language extension registry: one JSON
// manifest per language declaring its intelligence-server command, its
// (separate) formatter command, and the extensions it claims. Go owns this
// as the source of truth because Go is what actually spawns these
// processes — frontend-only concerns (the CodeMirror grammar to mount, a
// custom settings component) live in a same-id frontend module under
// frontend/src/lib/languages/, paired up by ID at runtime, not here.
//
// This replaces the old hardcoded serverCmds/installCmds maps in
// internal/lsp — adding a language is now a JSON file, not a code change.
package langext

import (
	"embed"
	"encoding/json"
	"sort"
)

// ProcessDef is the shape shared by a language's intelligence server and its
// formatter: candidate argv's (first found on PATH wins) plus the argv that
// installs the first candidate.
type ProcessDef struct {
	Candidates [][]string `json:"candidates"`
	Install    []string   `json:"install,omitempty"`
	// InstallHint is shown instead of an Install button when there's no
	// scriptable installer (e.g. "install via rustup: rustup component add
	// rust-analyzer").
	InstallHint string `json:"installHint,omitempty"`
}

// Definition is one language's complete self-contained, spawn-relevant
// config. Frontend-only concerns (CodeMirror grammar, custom settings UI)
// are deliberately not here.
type Definition struct {
	ID   string `json:"id"`
	Name string `json:"name"`

	// Extensions this language claims (leading dot, lowercase) — the source
	// of truth replacing codeintel.ts's intelKindFor and lsp.ts's languageId
	// switch.
	Extensions []string `json:"extensions"`

	// LanguageIDs maps an extension to its LSP textDocument languageId, for
	// the (common) case where one Definition covers several LSP languageIds
	// (e.g. js/ts/tsx/jsx all share server+formatter but need distinct
	// languageIds). An extension absent here falls back to ID.
	LanguageIDs map[string]string `json:"languageIds,omitempty"`

	Server    *ProcessDef `json:"server,omitempty"`    // nil = no intelligence server (formatter/data-format-only entry)
	Formatter *ProcessDef `json:"formatter,omitempty"` // nil = no dedicated formatter; caller falls back to the server's documentFormattingProvider

	// BuiltinFormatter means formatting is a zero-install JS/WASM function
	// the frontend module for this ID supplies directly — no Go process
	// involved at all (json/yaml/sql/xml/html/markdown/csv/toml).
	BuiltinFormatter bool `json:"builtinFormatter,omitempty"`
}

// LanguageIDFor returns the LSP languageId for ext (leading dot), falling
// back to the Definition's ID when the extension isn't listed explicitly.
func (d Definition) LanguageIDFor(ext string) string {
	if id, ok := d.LanguageIDs[ext]; ok {
		return id
	}
	return d.ID
}

//go:embed definitions/*.json
var defsFS embed.FS

var all []Definition

func init() {
	entries, err := defsFS.ReadDir("definitions")
	if err != nil {
		panic("langext: " + err.Error())
	}
	for _, e := range entries {
		data, err := defsFS.ReadFile("definitions/" + e.Name())
		if err != nil {
			continue
		}
		var d Definition
		if json.Unmarshal(data, &d) != nil || d.ID == "" {
			continue
		}
		all = append(all, d)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].Name < all[j].Name })
}

// All returns every registered language, sorted by display name.
func All() []Definition { return all }

// Get looks up a language by ID.
func Get(id string) (Definition, bool) {
	for _, d := range all {
		if d.ID == id {
			return d, true
		}
	}
	return Definition{}, false
}

// ForExtension finds the language claiming ext (leading dot, lowercase).
func ForExtension(ext string) (Definition, bool) {
	for _, d := range all {
		for _, e := range d.Extensions {
			if e == ext {
				return d, true
			}
		}
	}
	return Definition{}, false
}

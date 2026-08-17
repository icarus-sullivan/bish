package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Config struct {
	Theme        string `json:"theme"`
	Shell        string `json:"shell"`
	FormatOnSave bool   `json:"format_on_save"`
	// PanelSide docks the sidebar left or right; empty/"right" = right (default)
	PanelSide string `json:"panel_side,omitempty"`
	// nil = persist everything (frontend treats missing as true)
	Persist *PersistConfig `json:"persist,omitempty"`
	// per-feature toggles; missing key = frontend registry default (features.ts)
	Features   map[string]bool  `json:"features,omitempty"`
	Assistant  AssistantConfig  `json:"assistant,omitempty"`
	Completion CompletionConfig `json:"completion,omitempty"`
	// per-extension enable state, keyed by extension name; missing key = enabled
	Extensions map[string]bool `json:"extensions,omitempty"`
	Telemetry  TelemetryConfig `json:"telemetry,omitempty"`
	// nil = notify (frontend/backend both treat missing as true, same convention as Persist)
	Notifications  *bool                  `json:"notifications,omitempty"`
	Snippets       []Snippet              `json:"snippets,omitempty"`
	CustomThemes   map[string]CustomTheme `json:"custom_themes,omitempty"`
	OnboardingSeen bool                   `json:"onboarding_seen,omitempty"`
	// BuiltinExtensionsSeeded guards extensions.SeedBuiltins to a single
	// write-once — once true, a user who uninstalls a bundled extension
	// won't have it silently reappear on the next launch.
	BuiltinExtensionsSeeded bool `json:"builtin_extensions_seeded,omitempty"`
}

// NotificationsEnabled applies the nil-means-on convention.
func (c Config) NotificationsEnabled() bool {
	return c.Notifications == nil || *c.Notifications
}

// Snippet is a user-defined autocomplete snippet, additive to the built-in
// set in frontend/src/lib/snippets.ts (keyed by the same IntelKind strings:
// "js", "go", "py", ...).
type Snippet struct {
	Lang     string `json:"lang"`
	Label    string `json:"label"`
	Detail   string `json:"detail"`
	Template string `json:"template"`
}

// CustomTheme mirrors app.ThemeDTO's field set — a user-tweaked copy of a
// built-in theme, keyed by its own name in Config.CustomThemes.
type CustomTheme struct {
	Background    string `json:"background"`
	Foreground    string `json:"foreground"`
	Border        string `json:"border"`
	BorderFocused string `json:"borderFocused"`
	Accent        string `json:"accent"`
	Muted         string `json:"muted"`
	Success       string `json:"success"`
	Error         string `json:"error"`
	Warning       string `json:"warning"`
}

// TelemetryConfig is off by default and does nothing at all unless both
// fields are set — Enabled alone isn't enough, so a user who flips the
// toggle without pointing it at a real (e.g. self-hosted) collector never
// has a stray request fire against a hardcoded default.
type TelemetryConfig struct {
	Enabled  bool   `json:"enabled"`
	Endpoint string `json:"endpoint,omitempty"`
}

// AssistantConfig selects and configures the Assistant panel's backend.
type AssistantConfig struct {
	Provider    string `json:"provider"`     // "claude" (default) | "ollama"
	OllamaURL   string `json:"ollama_url"`   // e.g. http://192.168.1.20:11434
	OllamaModel string `json:"ollama_model"` // e.g. "gemma3:4b"
}

// CompletionConfig configures the local inline-completion model. Unlike
// AssistantConfig, this always runs as a subprocess of the Go backend
// (internal/completion) — it never talks to a remote server.
type CompletionConfig struct {
	Enabled    bool   `json:"enabled"`
	ModelPath  string `json:"model_path"`  // path to a .gguf file
	ServerPath string `json:"server_path"` // optional override; empty = PATH lookup for "llama-server"
}

// PersistConfig gates which per-project UI state gets saved/restored.
type PersistConfig struct {
	PanelWidth   bool `json:"panel_width"`
	RightSidebar bool `json:"right_sidebar"`
	RightPanel   bool `json:"right_panel"`
	Tabs         bool `json:"tabs"`
}

func defaultConfig() Config {
	return Config{
		Theme: "default",
		Shell: "",
	}
}

func dir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "bish")
}

func path() string {
	return filepath.Join(dir(), "config.json")
}

func Load() (Config, error) {
	cfg := defaultConfig()
	data, err := os.ReadFile(path())
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, Save(cfg)
		}
		return cfg, err
	}
	err = json.Unmarshal(data, &cfg)
	return cfg, err
}

func Save(cfg Config) error {
	if err := os.MkdirAll(dir(), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path(), data, 0o644)
}

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
	// nil = persist everything (frontend treats missing as true)
	Persist *PersistConfig `json:"persist,omitempty"`
	// per-feature toggles; missing key = frontend registry default (features.ts)
	Features map[string]bool `json:"features,omitempty"`
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

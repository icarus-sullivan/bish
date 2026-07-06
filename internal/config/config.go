package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Config struct {
	Theme        string `json:"theme"`
	Shell        string `json:"shell"`
	LeftWidthPct int    `json:"left_width_pct"`
	RightWidthPct int   `json:"right_width_pct"`
}

func defaultConfig() Config {
	return Config{
		Theme:         "default",
		Shell:         "",
		LeftWidthPct:  20,
		RightWidthPct: 25,
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

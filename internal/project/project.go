package project

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type Cmd struct {
	ID        string `json:"id"`
	Command   string `json:"command"`
	Directory string `json:"directory"`
}

type Config struct {
	CWD           string   `json:"cwd"`
	Cmds          []*Cmd   `json:"cmds"`
	ExpandedPaths []string `json:"expanded_paths,omitempty"`
}

func configDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "bish", "projects")
}

func configPath(cwd string) string {
	hash := fmt.Sprintf("%x", md5.Sum([]byte(cwd)))
	return filepath.Join(configDir(), hash+".json")
}

func Load(cwd string) (*Config, error) {
	cfg := &Config{CWD: cwd}
	data, err := os.ReadFile(configPath(cwd))
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, err
	}
	err = json.Unmarshal(data, cfg)
	return cfg, err
}

func Save(cfg *Config) error {
	if err := os.MkdirAll(configDir(), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configPath(cfg.CWD), data, 0o644)
}

func (c *Config) Add(command, dir string) *Cmd {
	for _, cmd := range c.Cmds {
		if cmd.Command == command {
			return cmd
		}
	}
	cmd := &Cmd{
		ID:        fmt.Sprintf("%x", md5.Sum([]byte(command+time.Now().String()))),
		Command:   command,
		Directory: dir,
	}
	c.Cmds = append(c.Cmds, cmd)
	return cmd
}

func (c *Config) Delete(id string) {
	out := c.Cmds[:0]
	for _, cmd := range c.Cmds {
		if cmd.ID != id {
			out = append(out, cmd)
		}
	}
	c.Cmds = out
}

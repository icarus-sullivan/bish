package langext

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/csullivan/bish/internal/config"
)

const formatTimeout = 15 * time.Second

// FormatterManager runs a language's dedicated formatter (ruff, rustfmt,
// shfmt, ...) as a one-shot stdin→stdout process — no persistent lifecycle,
// no JSON-RPC framing, unlike lsp.Manager's servers. This is deliberate:
// most real-world formatters (black, prettier, shfmt) don't speak LSP at
// all, so this is the shape that generalizes, not "make every formatter act
// like a server."
type FormatterManager struct {
	mu        sync.Mutex
	overrides map[string]config.LanguageOverride
	emit      func(event string, data ...interface{})
}

func NewFormatterManager(emit func(string, ...interface{})) *FormatterManager {
	return &FormatterManager{emit: emit}
}

func (f *FormatterManager) SetOverrides(o map[string]config.LanguageOverride) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.overrides = o
}

func (f *FormatterManager) override(id string) config.LanguageOverride {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.overrides[id]
}

// candidates returns the argv's to try for id's formatter, honoring a
// user override (custom path wins outright, disable yields none).
func (f *FormatterManager) candidates(id string) [][]string {
	ov := f.override(id)
	if ov.DisableFormatter {
		return nil
	}
	if ov.FormatterPath != "" {
		return [][]string{append([]string{ov.FormatterPath}, ov.FormatterArgs...)}
	}
	def, ok := Get(id)
	if !ok || def.Formatter == nil {
		return nil
	}
	return def.Formatter.Candidates
}

// Installed reports whether a formatter binary for id is on PATH, with no
// side effects.
func (f *FormatterManager) Installed(id string) bool {
	FixPath()
	for _, c := range f.candidates(id) {
		if _, err := exec.LookPath(c[0]); err == nil {
			return true
		}
	}
	return false
}

// Install runs id's formatter installer, streaming output as
// "langext:formatter-install-output:<id>".
func (f *FormatterManager) Install(id string) error {
	def, ok := Get(id)
	if !ok || def.Formatter == nil || len(def.Formatter.Install) == 0 {
		return fmt.Errorf("langext: no formatter installer for %s", id)
	}
	FixPath()
	return RunInstaller(def.Formatter.Install, func(line string) {
		f.emit("langext:formatter-install-output:"+id, line)
	})
}

// Format spawns the first installed formatter candidate for id, pipes
// content to its stdin, and returns the formatted stdout. path is
// substituted for a literal "{path}" token in any candidate argument (some
// formatters, e.g. ruff, want the real file path to resolve per-directory
// config even when reading from stdin).
func (f *FormatterManager) Format(id, path, content string) (string, error) {
	FixPath()
	var argv []string
	for _, c := range f.candidates(id) {
		if _, err := exec.LookPath(c[0]); err == nil {
			argv = c
			break
		}
	}
	if argv == nil {
		return "", fmt.Errorf("langext: no formatter installed for %s", id)
	}
	resolved := make([]string, len(argv))
	for i, a := range argv {
		resolved[i] = strings.ReplaceAll(a, "{path}", path)
	}
	ctx, cancel := context.WithTimeout(context.Background(), formatTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, resolved[0], resolved[1:]...)
	cmd.Stdin = strings.NewReader(content)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("%s: %s", resolved[0], msg)
	}
	return stdout.String(), nil
}

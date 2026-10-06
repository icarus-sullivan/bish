// Read-only access to the `claude` CLI's own conversation transcripts, so
// the panel can list past conversations for this project and resume one.
// The CLI stores one JSONL file per session under
// <config dir>/projects/<sanitized project path>/<session uuid>.jsonl.
// Nothing here writes to that tree; every path is derived from a validated
// session UUID joined onto the project's own directory.
package assistant

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// SessionSummary is one past conversation in the History list.
type SessionSummary struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	FirstPrompt string `json:"firstPrompt"`
	GitBranch   string `json:"gitBranch"`
	Modified    int64  `json:"modified"` // unix ms
	Size        int64  `json:"size"`
}

const (
	maxSessionsListed = 200
	summaryScanBytes  = 128 << 10 // head and tail window read per file for the list
	maxTranscript     = 48 << 20  // refuse to load a transcript bigger than this
	maxProjectDirName = 200       // the CLI's own cap before it appends a hash
)

var nonAlnum = regexp.MustCompile(`[^a-zA-Z0-9]`)

func claudeConfigDir() (string, error) {
	if d := os.Getenv("CLAUDE_CONFIG_DIR"); d != "" {
		return d, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".claude"), nil
}

// projectDir mirrors the CLI's path → directory-name mapping: every
// non-alphanumeric rune becomes '-'. Names over 200 chars get a hash suffix
// the CLI computes itself; rather than reimplement its hash, match by the
// 200-char prefix when the plain name isn't there.
func projectDir(root string) (string, error) {
	if err := checkRoot(root); err != nil {
		return "", err
	}
	base, err := claudeConfigDir()
	if err != nil {
		return "", err
	}
	projects := filepath.Join(base, "projects")
	name := nonAlnum.ReplaceAllString(root, "-")
	if len(name) <= maxProjectDirName {
		return filepath.Join(projects, name), nil
	}
	prefix := name[:maxProjectDirName] + "-"
	entries, err := os.ReadDir(projects)
	if err != nil {
		return "", err
	}
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), prefix) {
			return filepath.Join(projects, e.Name()), nil
		}
	}
	return "", os.ErrNotExist
}

// ListSessions returns this project's past conversations, newest first.
func ListSessions(root string) ([]SessionSummary, error) {
	dir, err := projectDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return []SessionSummary{}, nil
		}
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []SessionSummary{}, nil
		}
		return nil, err
	}
	type cand struct {
		id   string
		mod  int64
		size int64
	}
	var cands []cand
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".jsonl") {
			continue
		}
		id := strings.TrimSuffix(name, ".jsonl")
		if !uuidRe.MatchString(id) {
			continue
		}
		info, err := e.Info()
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		cands = append(cands, cand{id, info.ModTime().UnixMilli(), info.Size()})
	}
	sort.Slice(cands, func(i, j int) bool { return cands[i].mod > cands[j].mod })
	if len(cands) > maxSessionsListed {
		cands = cands[:maxSessionsListed]
	}
	out := make([]SessionSummary, 0, len(cands))
	for _, c := range cands {
		s, ok := summarize(filepath.Join(dir, c.id+".jsonl"), c.size)
		if !ok {
			continue
		}
		s.ID, s.Modified, s.Size = c.id, c.mod, c.size
		out = append(out, s)
	}
	return out, nil
}

// summaryProbe is the union of fields the title fallback chain reads:
// customTitle (user rename) > aiTitle > summary > lastPrompt > first prompt.
type summaryProbe struct {
	Type        string          `json:"type"`
	CustomTitle string          `json:"customTitle"`
	AITitle     string          `json:"aiTitle"`
	Summary     string          `json:"summary"`
	LastPrompt  string          `json:"lastPrompt"`
	GitBranch   string          `json:"gitBranch"`
	IsMeta      bool            `json:"isMeta"`
	IsSidechain bool            `json:"isSidechain"`
	Message     json.RawMessage `json:"message"`
}

// summarize reads only a head and a tail window of the file — transcripts
// can be tens of MB, and every title-bearing line is either near the start
// (first prompt) or appended at the end (titles, last prompt).
func summarize(path string, size int64) (SessionSummary, bool) {
	var s SessionSummary
	f, err := os.Open(path)
	if err != nil {
		return s, false
	}
	defer f.Close()
	head := make([]byte, min64(size, summaryScanBytes))
	if _, err := io.ReadFull(f, head); err != nil {
		return s, false
	}
	var tail []byte
	if size > summaryScanBytes {
		off := max64(size-summaryScanBytes, summaryScanBytes)
		tail = make([]byte, size-off)
		if _, err := f.ReadAt(tail, off); err != nil && err != io.EOF {
			tail = nil
		}
		// drop the partial first line of the window
		if i := bytes.IndexByte(tail, '\n'); i >= 0 {
			tail = tail[i+1:]
		}
	}
	var custom, ai, summary, last string
	scan := func(buf []byte, wantFirst bool) {
		for _, line := range bytes.Split(buf, []byte{'\n'}) {
			if len(line) == 0 || line[0] != '{' {
				continue
			}
			var p summaryProbe
			if json.Unmarshal(line, &p) != nil {
				continue
			}
			switch {
			case p.CustomTitle != "":
				custom = p.CustomTitle
			case p.AITitle != "":
				ai = p.AITitle
			case p.Type == "summary" && p.Summary != "":
				summary = p.Summary
			case p.LastPrompt != "":
				last = p.LastPrompt
			}
			if p.GitBranch != "" {
				s.GitBranch = p.GitBranch
			}
			if wantFirst && s.FirstPrompt == "" && p.Type == "user" && !p.IsMeta && !p.IsSidechain {
				s.FirstPrompt = promptText(p.Message)
			}
		}
	}
	scan(head, true)
	if tail != nil {
		scan(tail, false)
	}
	for _, t := range []string{custom, ai, summary, last, s.FirstPrompt} {
		if t = strings.TrimSpace(t); t != "" {
			s.Title = clip(t, 140)
			break
		}
	}
	if s.Title == "" {
		return s, false // no user content at all (e.g. a crashed spawn)
	}
	s.FirstPrompt = clip(s.FirstPrompt, 300)
	return s, true
}

// promptText pulls the human-typed text out of a user message, skipping
// tool results and harness-injected command/caveat wrappers ("<command-…>").
func promptText(raw json.RawMessage) string {
	var m struct {
		Content json.RawMessage `json:"content"`
	}
	if json.Unmarshal(raw, &m) != nil {
		return ""
	}
	var str string
	if json.Unmarshal(m.Content, &str) == nil {
		return usable(str)
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(m.Content, &blocks) != nil {
		return ""
	}
	for _, b := range blocks {
		if b.Type == "text" {
			if t := usable(b.Text); t != "" {
				return t
			}
		}
	}
	return ""
}

func usable(t string) string {
	t = strings.TrimSpace(t)
	if t == "" || strings.HasPrefix(t, "<") {
		return ""
	}
	// the panel prefixes editor context before a "---" rule; show the ask
	if i := strings.LastIndex(t, "\n---\n"); i >= 0 {
		t = strings.TrimSpace(t[i+5:])
	}
	return t
}

func clip(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

// LoadTranscript returns the conversation lines (user/assistant/system) of
// one past session as a JSON array, for re-rendering before a resume.
func LoadTranscript(root, sessionID string) (string, error) {
	if !uuidRe.MatchString(sessionID) {
		return "", fmt.Errorf("assistant: invalid session id")
	}
	dir, err := projectDir(root)
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, sessionID+".jsonl")
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("assistant: transcript is not a regular file")
	}
	if info.Size() > maxTranscript {
		return "", fmt.Errorf("assistant: transcript too large to display (%d MB)", info.Size()>>20)
	}
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	var out bytes.Buffer
	out.WriteByte('[')
	n := 0
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64<<10), maxLine)
	for sc.Scan() {
		line := sc.Bytes()
		var p struct {
			Type        string `json:"type"`
			IsSidechain bool   `json:"isSidechain"`
		}
		if json.Unmarshal(line, &p) != nil || p.IsSidechain {
			continue
		}
		if p.Type != "user" && p.Type != "assistant" && p.Type != "system" {
			continue
		}
		if n > 0 {
			out.WriteByte(',')
		}
		out.Write(line)
		n++
	}
	out.WriteByte(']')
	return out.String(), sc.Err()
}

func min64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

package app

import (
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// ponytail: no blame cache — blame on one file is <100ms; frontend fetches
// once per load + after save. Add mtime-keyed map if large repos hurt.

type BlameLine struct {
	SHA     string `json:"sha"`
	Author  string `json:"author"`
	Time    int64  `json:"time"`
	Summary string `json:"summary"`
}

type GitFileStatus struct {
	Status string `json:"status"`
	Path   string `json:"path"`
}

type GitStatusDTO struct {
	Branch string          `json:"branch"`
	Files  []GitFileStatus `json:"files"`
}

// GitBlame returns per-line blame for path, or nil on any error
// (untracked file, not a repo, git missing).
func (a *App) GitBlame(path string) []BlameLine {
	out, err := exec.Command("git", "-C", filepath.Dir(path), "blame", "--line-porcelain", "--", path).Output()
	if err != nil {
		return nil
	}
	var lines []BlameLine
	var cur BlameLine
	for _, l := range strings.Split(string(out), "\n") {
		switch {
		case strings.HasPrefix(l, "\t"):
			lines = append(lines, cur)
		case len(l) >= 40 && isHex40(l[:40]):
			cur = BlameLine{SHA: l[:40]}
		case strings.HasPrefix(l, "author "):
			cur.Author = l[len("author "):]
		case strings.HasPrefix(l, "author-time "):
			cur.Time, _ = strconv.ParseInt(l[len("author-time "):], 10, 64)
		case strings.HasPrefix(l, "summary "):
			cur.Summary = l[len("summary "):]
		}
	}
	return lines
}

func isHex40(s string) bool {
	for i := 0; i < 40; i++ {
		c := s[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// GitStatus returns branch + changed files for the project root (or CWD when
// no project is pinned), or nil if not a git repo.
func (a *App) GitStatus() *GitStatusDTO {
	dir := a.projectRoot
	if dir == "" {
		dir = a.GetCWD()
	}
	if dir == "" {
		return nil
	}
	out, err := exec.Command("git", "-C", dir, "status", "--porcelain=v1", "--branch").Output()
	if err != nil {
		return nil
	}
	st := &GitStatusDTO{Files: []GitFileStatus{}}
	for _, l := range strings.Split(string(out), "\n") {
		if l == "" {
			continue
		}
		if strings.HasPrefix(l, "## ") {
			branch := l[3:]
			if i := strings.Index(branch, "..."); i > 0 {
				branch = branch[:i]
			}
			st.Branch = branch
			continue
		}
		if len(l) < 4 {
			continue
		}
		p := l[3:]
		// renames show "old -> new"; keep the new path
		if i := strings.Index(p, " -> "); i >= 0 {
			p = p[i+4:]
		}
		st.Files = append(st.Files, GitFileStatus{Status: strings.TrimSpace(l[:2]), Path: filepath.Join(dir, p)})
	}
	return st
}

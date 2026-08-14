package app

import (
	"errors"
	"os"
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

// DiffLine marks one changed line on the new (working-tree) side.
// Type: "added" | "modified" | "deleted" (deleted anchors at the line the
// removed block sat above).
type DiffLine struct {
	Line int    `json:"line"`
	Type string `json:"type"`
}

// GitDiff returns per-line change markers for path vs. the index/HEAD, or nil
// on any error (untracked, not a repo, git missing).
func (a *App) GitDiff(path string) []DiffLine {
	out, err := exec.Command("git", "-C", filepath.Dir(path), "diff", "--no-color", "-U0", "--", path).Output()
	if err != nil {
		return nil
	}
	return parseUnifiedDiff(string(out))
}

// parseUnifiedDiff turns `git diff -U0` output into per-line change markers.
func parseUnifiedDiff(out string) []DiffLine {
	var res []DiffLine
	for _, l := range strings.Split(out, "\n") {
		if !strings.HasPrefix(l, "@@") {
			continue
		}
		// @@ -oldStart,oldCount +newStart,newCount @@
		plus := strings.IndexByte(l, '+')
		if plus < 0 {
			continue
		}
		seg := l[plus+1:]
		if sp := strings.IndexByte(seg, ' '); sp >= 0 {
			seg = seg[:sp]
		}
		newStart, newCount := parseDiffRange(seg)

		oldCount := 1
		if minus := strings.IndexByte(l, '-'); minus >= 0 && minus < plus {
			_, oldCount = parseDiffRange(strings.TrimSpace(l[minus+1 : plus]))
		}

		switch {
		case oldCount == 0 && newCount > 0:
			for i := 0; i < newCount; i++ {
				res = append(res, DiffLine{Line: newStart + i, Type: "added"})
			}
		case newCount == 0 && oldCount > 0:
			res = append(res, DiffLine{Line: newStart, Type: "deleted"})
		default:
			for i := 0; i < newCount; i++ {
				res = append(res, DiffLine{Line: newStart + i, Type: "modified"})
			}
		}
	}
	return res
}

// GitDiffText returns the unified `git diff` for path (working tree vs index/
// HEAD). Untracked files diff against empty so new files show as all-added.
// Returns "" when there's nothing to show.
func (a *App) GitDiffText(path string) string {
	dir := filepath.Dir(path)
	out, _ := exec.Command("git", "-C", dir, "diff", "--no-color", "--", path).Output()
	if len(out) == 0 {
		// untracked / new file: diff against /dev/null (exits non-zero when
		// differing, but stdout is still captured by Output()).
		b, _ := exec.Command("git", "-C", dir, "diff", "--no-color", "--no-index", "--", os.DevNull, path).Output()
		out = b
	}
	return string(out)
}

// parseDiffRange parses "start,count" or "start" (count defaults to 1).
func parseDiffRange(s string) (start, count int) {
	count = 1
	if c := strings.IndexByte(s, ','); c >= 0 {
		start, _ = strconv.Atoi(s[:c])
		count, _ = strconv.Atoi(s[c+1:])
	} else {
		start, _ = strconv.Atoi(s)
	}
	return
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
	return gitStatusFor(dir)
}

// GitStatusForRoots returns per-root status for a multi-root workspace
// (root path → status, omitting roots that aren't git repos), so the Git
// panel can show one section per workspace folder instead of assuming a
// single project root.
func (a *App) GitStatusForRoots(roots []string) map[string]*GitStatusDTO {
	out := make(map[string]*GitStatusDTO, len(roots))
	for _, r := range roots {
		if st := gitStatusFor(r); st != nil {
			out[r] = st
		}
	}
	return out
}

func gitStatusFor(dir string) *GitStatusDTO {
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
		// keep the raw 2-char XY code (index, worktree) so the UI can split
		// staged vs unstaged; e.g. "M " staged, " M" unstaged, "MM" both.
		st.Files = append(st.Files, GitFileStatus{Status: l[:2], Path: filepath.Join(dir, p)})
	}
	return st
}

func (a *App) gitDir() string {
	if a.projectRoot != "" {
		return a.projectRoot
	}
	return a.GetCWD()
}

// GitStage stages path (git add).
func (a *App) GitStage(path string) error {
	return exec.Command("git", "-C", filepath.Dir(path), "add", "--", path).Run()
}

// GitUnstage unstages path (git reset HEAD).
func (a *App) GitUnstage(path string) error {
	return exec.Command("git", "-C", filepath.Dir(path), "reset", "-q", "HEAD", "--", path).Run()
}

// GitCommit commits the staged changes; returns git's stderr on failure
// (nothing staged, empty message, hooks, etc.).
func (a *App) GitCommit(message string) error {
	dir := a.gitDir()
	if dir == "" {
		return errors.New("no project")
	}
	return gitCommitIn(dir, message)
}

// GitCommitForRoot commits in a specific workspace root — the multi-root
// Git panel's per-root commit box calls this instead of GitCommit, which
// only ever knows about the primary project root.
func (a *App) GitCommitForRoot(root, message string) error {
	return gitCommitIn(root, message)
}

func gitCommitIn(dir, message string) error {
	out, err := exec.Command("git", "-C", dir, "commit", "-m", message).CombinedOutput()
	if err != nil {
		return errors.New(strings.TrimSpace(string(out)))
	}
	return nil
}

// GitBranches lists local branch names, current branch first.
func (a *App) GitBranches() []string {
	dir := a.gitDir()
	if dir == "" {
		return nil
	}
	return gitBranchesIn(dir)
}

func (a *App) GitBranchesForRoot(root string) []string {
	return gitBranchesIn(root)
}

func gitBranchesIn(dir string) []string {
	out, err := exec.Command("git", "-C", dir, "branch", "--format=%(HEAD)%(refname:short)").Output()
	if err != nil {
		return nil
	}
	var cur string
	var rest []string
	for _, l := range strings.Split(string(out), "\n") {
		if l == "" {
			continue
		}
		if strings.HasPrefix(l, "*") {
			cur = l[1:]
		} else {
			rest = append(rest, l)
		}
	}
	if cur != "" {
		return append([]string{cur}, rest...)
	}
	return rest
}

// GitCheckout switches branches; returns git's stderr on failure (dirty tree,
// unknown branch, etc.).
func (a *App) GitCheckout(branch string) error {
	dir := a.gitDir()
	if dir == "" {
		return errors.New("no project")
	}
	return gitCheckoutIn(dir, branch)
}

func (a *App) GitCheckoutForRoot(root, branch string) error {
	return gitCheckoutIn(root, branch)
}

func gitCheckoutIn(dir, branch string) error {
	out, err := exec.Command("git", "-C", dir, "checkout", branch).CombinedOutput()
	if err != nil {
		return errors.New(strings.TrimSpace(string(out)))
	}
	return nil
}

package commandcenter

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

func git(dir string, args ...string) (string, error) {
	c := exec.Command("git", args...)
	c.Dir = dir
	out, err := c.CombinedOutput()
	if err != nil {
		return strings.TrimSpace(string(out)), fmt.Errorf("git %s: %s", strings.Join(args, " "), strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

func worktrees(repoPath string) ([]WorktreeInfo, error) {
	out, err := git(repoPath, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	var list []WorktreeInfo
	var cur WorktreeInfo
	flush := func() {
		if cur.Path != "" {
			list = append(list, cur)
		}
		cur = WorktreeInfo{}
	}
	for _, line := range strings.Split(out, "\n") {
		switch {
		case strings.HasPrefix(line, "worktree "):
			flush()
			cur.Path = strings.TrimPrefix(line, "worktree ")
			cur.Main = len(list) == 0
		case strings.HasPrefix(line, "branch "):
			cur.Branch = strings.TrimPrefix(strings.TrimPrefix(line, "branch "), "refs/heads/")
		case line == "detached":
			cur.Branch = "(detached)"
		}
	}
	flush()
	return list, nil
}

func currentBranch(dir string) string {
	b, err := git(dir, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return ""
	}
	return b
}

// branchSuffix labels a launched process with the branch actually checked
// out in its directory, so it's unambiguous which checkout is running.
func branchSuffix(dir string) string {
	if b := currentBranch(dir); b != "" {
		return " @" + b
	}
	return ""
}

func refExists(repoPath, ref string) bool {
	_, err := git(repoPath, "rev-parse", "--verify", "--quiet", ref)
	return err == nil
}

func branchExists(repoPath, branch string) bool {
	return refExists(repoPath, "refs/heads/"+branch)
}

// branches lists the branches you could run — ones whose tip commit is
// yours (git config user.email), most-recently-committed first, local
// before remote-only. A shared repo can have thousands of other people's
// branches; anything filtered out is still reachable by typing its exact
// name. Branches with a worktree and the main branch are always kept,
// whoever wrote the tip.
func branches(repoPath, mainBranch string) ([]BranchInfo, error) {
	wts, err := worktrees(repoPath)
	if err != nil {
		return nil, err
	}
	held := map[string]WorktreeInfo{}
	for _, wt := range wts {
		if wt.Branch != "" {
			held[wt.Branch] = wt
		}
	}
	me, _ := git(repoPath, "config", "user.email")
	byRecency := []string{"for-each-ref", "--sort=-committerdate",
		"--format=%(refname:short)%09%(authoremail)%09%(committeremail)"}

	mine := func(name, line string) bool {
		if me == "" { // no identity configured: can't filter, show everything
			return true
		}
		if name == mainBranch || held[name].Path != "" {
			return true
		}
		return strings.Contains(line, "<"+me+">")
	}

	local, err := git(repoPath, append(byRecency, "refs/heads")...)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var list []BranchInfo
	for _, line := range strings.Split(local, "\n") {
		name, _, _ := strings.Cut(line, "\t")
		if name == "" {
			continue
		}
		seen[name] = true
		if !mine(name, line) {
			continue
		}
		wt := held[name]
		list = append(list, BranchInfo{Name: name, Path: wt.Path, Main: wt.Main})
	}
	remote, err := git(repoPath, append(byRecency, "refs/remotes/origin")...)
	if err != nil {
		return list, nil // no remote configured: local branches are the whole list
	}
	for _, line := range strings.Split(remote, "\n") {
		ref, _, _ := strings.Cut(line, "\t")
		name := strings.TrimPrefix(ref, "origin/")
		if name == "" || name == "HEAD" || seen[name] {
			continue
		}
		seen[name] = true
		if !mine(name, line) {
			continue
		}
		list = append(list, BranchInfo{Name: name, Remote: true})
	}
	return list, nil
}

var safeName = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

func worktreePath(r *Repo, branch string) string {
	base := r.WorktreeIn
	if base == "" {
		base = filepath.Dir(r.Path)
	}
	return filepath.Join(base, r.Prefix+safeName.ReplaceAllString(branch, "-"))
}

// addWorktree creates (or adopts) a worktree for branch. An existing path or
// branch is reused rather than treated as an error.
func addWorktree(r *Repo, branch, base string) (string, error) {
	path := worktreePath(r, branch)
	if dirExists(path) {
		return path, nil
	}
	local := branchExists(r.Path, branch)
	if base == "" && !local {
		if refExists(r.Path, "refs/remotes/origin/"+branch) {
			base = "origin/" + branch // someone else's branch: start from the remote tip
		} else if _, err := git(r.Path, "fetch", "origin", r.MainBranch); err != nil {
			base = r.MainBranch // offline: fall back to the local base ref
		} else {
			base = "origin/" + r.MainBranch
		}
	}
	var err error
	if local {
		_, err = git(r.Path, "worktree", "add", path, branch)
	} else {
		_, err = git(r.Path, "worktree", "add", "-b", branch, path, base)
	}
	if err != nil {
		return "", err
	}
	return path, nil
}

// linkShared symlinks the gitignored directories listed in Repo.Link from
// the main checkout into a worktree (e.g. node_modules), so a fresh
// worktree doesn't need a full reinstall. Idempotent, never clobbers.
func linkShared(r *Repo, dir string) []string {
	if dir == "" || dir == r.Path {
		return nil
	}
	var made []string
	for _, pat := range r.Link {
		matches, err := filepath.Glob(filepath.Join(r.Path, pat))
		if err != nil {
			continue
		}
		for _, src := range matches {
			rel, err := filepath.Rel(r.Path, src)
			if err != nil || strings.HasPrefix(rel, "..") {
				continue
			}
			dst := filepath.Join(dir, rel)
			if _, err := os.Lstat(dst); err == nil {
				continue // already a real dir or a link made earlier
			}
			if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
				continue
			}
			if err := os.Symlink(src, dst); err == nil {
				made = append(made, rel)
			}
		}
	}
	return made
}

// copyShared copies the small gitignored files (e.g. .env) a checkout needs
// but git will never give it. Copied, not symlinked, so each worktree can
// edit its own. Only fills gaps; an existing file is never overwritten.
func copyShared(r *Repo, dir string) []string {
	if dir == "" || dir == r.Path {
		return nil
	}
	var made []string
	for _, pat := range r.Copy {
		matches, err := filepath.Glob(filepath.Join(r.Path, pat))
		if err != nil {
			continue
		}
		for _, src := range matches {
			rel, err := filepath.Rel(r.Path, src)
			if err != nil || strings.HasPrefix(rel, "..") {
				continue
			}
			fi, err := os.Stat(src)
			if err != nil || fi.IsDir() {
				continue // directories belong in `link`
			}
			dst := filepath.Join(dir, rel)
			if _, err := os.Lstat(dst); err == nil {
				continue
			}
			b, err := os.ReadFile(src)
			if err != nil {
				continue
			}
			if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
				continue
			}
			if err := os.WriteFile(dst, b, fi.Mode().Perm()); err == nil {
				made = append(made, rel)
			}
		}
	}
	return made
}

func removeWorktree(r *Repo, path string) error {
	_, err := git(r.Path, "worktree", "remove", "--force", path)
	return err
}

func dirExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

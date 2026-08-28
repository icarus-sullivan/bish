package commandcenter

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// discoverRepos finds git checkouts among the project's roots (the primary
// project root plus any multi-root workspace folders) and merges them into
// def.Repos: existing entries (matched by Path) are left untouched, new ones
// are seeded from git — never from a hardcoded name/path.
func discoverRepos(def *Definition, roots []string) {
	known := map[string]bool{}
	for _, r := range def.Repos {
		known[r.Path] = true
	}
	usedIDs := map[string]bool{}
	for _, r := range def.Repos {
		usedIDs[r.ID] = true
	}
	for _, root := range roots {
		if root == "" || known[root] || !isGitRepo(root) {
			continue
		}
		id := uniqueID(filepath.Base(root), usedIDs)
		usedIDs[id] = true
		def.Repos = append(def.Repos, &Repo{
			ID:         id,
			Name:       id,
			Path:       root,
			MainBranch: detectMainBranch(root),
			WorktreeIn: filepath.Dir(root),
			Prefix:     id + "-",
			DependsOn:  []string{},
			Link:       []string{},
			Copy:       []string{},
			Env:        map[string]string{},
			Steps:      []*Step{},
			Services:   []*Service{},
		})
	}
}

func isGitRepo(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil
}

func uniqueID(base string, used map[string]bool) string {
	if base == "" {
		base = "repo"
	}
	if !used[base] {
		return base
	}
	for i := 2; ; i++ {
		candidate := base + "-" + strconv.Itoa(i)
		if !used[candidate] {
			return candidate
		}
	}
}

// detectMainBranch guesses a repo's default branch: origin's HEAD symref,
// then the currently checked-out branch, then "main".
func detectMainBranch(dir string) string {
	if out, err := git(dir, "symbolic-ref", "refs/remotes/origin/HEAD"); err == nil {
		return strings.TrimPrefix(out, "refs/remotes/origin/")
	}
	if b := currentBranch(dir); b != "" {
		return b
	}
	return "main"
}

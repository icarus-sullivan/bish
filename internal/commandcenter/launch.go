package commandcenter

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// depSettle is the grace period after a dependency that only has a plain
// port probe starts accepting connections — binding a port comes before
// serving on it for most dev servers.
const depSettle = 3 * time.Second

// startOrder lists repo IDs so a repo's dependsOn entries come first (Kahn;
// a dependency cycle just falls back to definition order for the repos in it).
func startOrder(def *Definition, targets map[string]*Target) []string {
	var pending []string
	for _, rp := range def.Repos {
		if _, ok := targets[rp.ID]; ok {
			pending = append(pending, rp.ID)
		}
	}
	done := map[string]bool{}
	var order []string
	for len(pending) > 0 {
		progress := false
		rest := pending[:0]
		for _, id := range pending {
			ready := true
			for _, dep := range def.repo(id).DependsOn {
				if _, ok := targets[dep]; ok && !done[dep] && dep != id {
					ready = false
				}
			}
			if ready {
				order, done[id], progress = append(order, id), true, true
			} else {
				rest = append(rest, id)
			}
		}
		pending = rest
		if !progress { // cycle — emit what's left as-is
			order = append(order, pending...)
			break
		}
	}
	return order
}

// hasLink reports whether dir's node_modules is a symlink into the main
// checkout, i.e. whether an install there would mutate the shared tree.
func hasLink(dir string) bool {
	fi, err := os.Lstat(filepath.Join(dir, "node_modules"))
	return err == nil && fi.Mode()&os.ModeSymlink != 0
}

// prestartCmd chains the enabled pre-start steps and reports which were
// skipped (superseded, a redundant install into a linked node_modules, or a
// step-cache hit) plus the cacheable steps that will actually run, so a
// successful prestart can record them. cache is nil when step caching is off.
func prestartCmd(r *Repo, t *Target, dir string, cache *stepCacher) (cmd string, skipped []string, ran []*Step) {
	enabled := func(st *Step) bool {
		on, set := t.Steps[st.Name]
		if !set {
			return st.Default
		}
		return on
	}
	// a step that supersedes others wins: db:reset already migrates
	covered := map[string]string{}
	for _, st := range r.Steps {
		if !enabled(st) {
			continue
		}
		for _, name := range st.Supersedes {
			covered[name] = st.Name
		}
	}
	shared := hasLink(dir)
	var cmds []string
	for _, st := range r.Steps {
		if !enabled(st) || strings.TrimSpace(st.Cmd) == "" {
			continue
		}
		if by, ok := covered[st.Name]; ok {
			skipped = append(skipped, st.Name+" ("+by+" covers it)")
			continue
		}
		if shared && strings.Contains(st.Cmd, "install") {
			skipped = append(skipped, st.Name+" (node_modules is symlinked from "+r.Path+")")
			continue
		}
		if cache != nil && cacheable(st) {
			if cache.hit(st) {
				// never a silent skip — the line lands in the prestart log
				cmds = append(cmds, "echo "+shellQuote("[command-center] "+st.Name+": skipped (cached)"))
				skipped = append(skipped, st.Name+" (cached)")
				continue
			}
			ran = append(ran, st)
		}
		cmds = append(cmds, st.Cmd)
	}
	return strings.Join(cmds, " && "), skipped, ran
}

// shellQuote single-quotes s for sh/zsh/bash/fish.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// mergeEnv layers per-target overrides on top of repo-wide ones.
func mergeEnv(maps ...map[string]string) map[string]string {
	out := map[string]string{}
	for _, m := range maps {
		for k, v := range m {
			out[k] = v
		}
	}
	return out
}

// writeOverrides merges env into the repo's override file, keeping keys
// already in the file that aren't managed here.
func writeOverrides(dir, file string, env map[string]string) error {
	if file == "" || len(env) == 0 {
		return nil
	}
	p := filepath.Join(dir, file)
	existing := map[string]string{}
	var order []string
	if b, err := os.ReadFile(p); err == nil {
		for _, line := range strings.Split(string(b), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			k, v, ok := strings.Cut(line, "=")
			if !ok {
				continue
			}
			if _, seen := existing[k]; !seen {
				order = append(order, k)
			}
			existing[k] = v
		}
	}
	for k, v := range env {
		if _, seen := existing[k]; !seen {
			order = append(order, k)
		}
		existing[k] = v
	}
	sort.Strings(order)
	var sb strings.Builder
	for _, k := range order {
		sb.WriteString(k + "=" + existing[k] + "\n")
	}
	return os.WriteFile(p, []byte(sb.String()), 0o644)
}

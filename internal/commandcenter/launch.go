package commandcenter

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	depWaitSeconds   = 600 // a cold dependency boot (install + migrate + build) can be slow
	depSettleSeconds = 3
)

// startOrder lists repo IDs so a repo's dependsOn entries come first (Kahn;
// a dependency cycle just falls back to definition order for the repos in it).
func startOrder(def *Definition, st *State) []string {
	var pending []string
	for _, rp := range def.Repos {
		if _, ok := st.Targets[rp.ID]; ok {
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
				if _, ok := st.Targets[dep]; ok && !done[dep] && dep != id {
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

// depWaitCmd blocks until every dependency's selected service ports accept
// connections, so a dependent's build/dev servers don't fire before its
// dependencies are up. Empty when the dependencies are off or aren't
// running any ports.
//
// This is a port-listening + settle probe, not a real health check — good
// enough for "the process bound its port," not "it's serving correctly."
func depWaitCmd(def *Definition, st *State, rp *Repo) string {
	var parts []string
	for _, dep := range rp.DependsOn {
		dt, drp := st.Targets[dep], def.repo(dep)
		if dt == nil || drp == nil || dt.Mode == "off" || dep == rp.ID {
			continue
		}
		for _, svcName := range dt.Services {
			svc := drp.service(svcName)
			if svc == nil || svc.Port <= 0 {
				continue
			}
			// both stacks: some servers bind *:port, others bind [::1] only —
			// a v4-only probe would wait forever on the latter
			probe := fmt.Sprintf("{ nc -z 127.0.0.1 %d || nc -z ::1 %d; } >/dev/null 2>&1", svc.Port, svc.Port)
			parts = append(parts, fmt.Sprintf(
				`echo "[command-center] waiting for %s %s :%d"; `+
					`for i in $(seq 1 %d); do %s && break; sleep 1; done; `+
					`%s || { echo "[command-center] timed out waiting for %s :%d"; exit 1; }`,
				dep, svcName, svc.Port, depWaitSeconds, probe, probe, dep, svc.Port))
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "; ") + fmt.Sprintf("; sleep %d", depSettleSeconds)
}

// hasLink reports whether dir's node_modules is a symlink into the main
// checkout, i.e. whether an install there would mutate the shared tree.
func hasLink(dir string) bool {
	fi, err := os.Lstat(filepath.Join(dir, "node_modules"))
	return err == nil && fi.Mode()&os.ModeSymlink != 0
}

// prestartCmd chains the enabled pre-start steps and reports which were
// skipped (superseded, or a redundant install into a linked node_modules).
func prestartCmd(r *Repo, t *Target, dir string) (cmd string, skipped []string) {
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
		cmds = append(cmds, st.Cmd)
	}
	return strings.Join(cmds, " && "), skipped
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

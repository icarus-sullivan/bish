// Package envdetect proposes Command Center services and pre-start steps
// for a repo from the files already in it — package.json scripts, Nx
// project targets, compose files, Makefile targets, Procfile lines, and the
// usual Go/Rust/Python/Rails markers. Pure functions, zero writes: the
// result is a reviewable proposal where every row carries the file it came
// from, never something applied silently.
package envdetect

import (
	"sort"
	"strings"
)

// Candidate is one proposed Service or Step, carrying the file it came from
// so the user can judge it instead of trusting it.
type Candidate struct {
	Name        string   `json:"name"`
	Cmd         string   `json:"cmd"`
	Port        int      `json:"port,omitempty"`
	PortEnv     string   `json:"portEnv,omitempty"`
	PortArgs    string   `json:"portArgs,omitempty"`
	Kind        string   `json:"kind"` // "service"|"install"|"migrate"|"codegen"|"reset"|"build"|"infra"
	Destructive bool     `json:"destructive,omitempty"`
	Supersedes  []string `json:"supersedes"`
	Source      string   `json:"source"`     // "package.json#scripts.dev", "docker/docker-compose.yml#postgres"
	Confidence  int      `json:"confidence"` // 0-100; drives whether the row is pre-ticked
	Accept      bool     `json:"accept"`
}

// Proposal is everything detected for one repo.
type Proposal struct {
	RepoID    string       `json:"repoId"`
	Toolchain string       `json:"toolchain"` // "pnpm@10.33.0 node@24.18.0" — displayed, never executed
	Services  []*Candidate `json:"services"`
	Steps     []*Candidate `json:"steps"`
	Compose   string       `json:"compose,omitempty"` // shared-infra compose file, rel to the repo
	Notes     []string     `json:"notes"`
}

// AcceptThreshold is the confidence at or above which a row is pre-ticked.
const AcceptThreshold = 70

type detector func(dir string, p *Proposal)

var detectors = []detector{
	detectNode,
	detectNx,
	detectCompose,
	detectMakefile,
	detectProcfile,
	detectPython,
	detectGo,
	detectCargo,
	detectRails,
}

// Detect runs every detector against the checkout at dir.
func Detect(repoID, dir string) *Proposal {
	p := &Proposal{RepoID: repoID, Toolchain: Toolchain(dir), Services: []*Candidate{}, Steps: []*Candidate{}, Notes: []string{}}
	for _, d := range detectors {
		d(dir, p)
	}
	applyPortEnvHints(dir, p)
	linkSupersedes(p)
	p.Services = dedupe(p.Services)
	p.Steps = dedupe(p.Steps)
	for _, c := range append(append([]*Candidate{}, p.Services...), p.Steps...) {
		if c.Supersedes == nil {
			c.Supersedes = []string{}
		}
		c.Accept = c.Confidence >= AcceptThreshold
	}
	for _, s := range p.Services {
		if s.Port == 0 {
			p.Notes = append(p.Notes, s.Name+": no port found in "+s.Source+" — set one if it serves HTTP")
		}
	}
	return p
}

// linkSupersedes points every reset step at the migrate steps it makes
// redundant (a db:reset already migrates).
func linkSupersedes(p *Proposal) {
	var migrates []string
	for _, s := range p.Steps {
		if s.Kind == "migrate" {
			migrates = append(migrates, s.Name)
		}
	}
	for _, s := range p.Steps {
		if s.Kind == "reset" {
			s.Destructive = true
			s.Supersedes = append([]string{}, migrates...)
		}
	}
}

// dedupe keeps the highest-confidence candidate per name, preserving first
// appearance order.
func dedupe(list []*Candidate) []*Candidate {
	best := map[string]*Candidate{}
	var order []string
	for _, c := range list {
		if b, ok := best[c.Name]; ok {
			if c.Confidence > b.Confidence {
				best[c.Name] = c
			}
			continue
		}
		best[c.Name] = c
		order = append(order, c.Name)
	}
	out := make([]*Candidate, 0, len(order))
	for _, n := range order {
		out = append(out, best[n])
	}
	return out
}

func (p *Proposal) service(c *Candidate) {
	c.Kind = "service"
	p.Services = append(p.Services, c)
}

func (p *Proposal) step(c *Candidate) {
	p.Steps = append(p.Steps, c)
}

func (p *Proposal) note(s string) {
	p.Notes = append(p.Notes, s)
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// classify maps a script/target name to a candidate kind ("" = ignore).
func classify(name string) string {
	n := strings.ToLower(name)
	base := n
	if i := strings.IndexAny(n, ":_-"); i > 0 {
		base = n[:i]
	}
	switch {
	case strings.HasSuffix(n, "reset") || n == "db:drop" || n == "db:nuke":
		return "reset"
	case strings.Contains(n, "migrat"):
		return "migrate"
	case strings.Contains(n, "codegen") || strings.Contains(n, "generate"):
		return "codegen"
	case n == "build":
		return "build"
	case strings.Contains(n, "build") || strings.Contains(n, "test") || strings.Contains(n, "lint"):
		return "" // storybook:build, dev:test, ... — not something to keep running
	case base == "dev" || base == "start" || base == "serve" || base == "storybook":
		return "service"
	case n == "install":
		return "install"
	}
	return ""
}

package commandcenter

import (
	"fmt"

	"github.com/csullivan/bish/internal/envdetect"
)

// DetectRepoEnv proposes services/steps for a repo from its files. Never
// runs on its own — it's a button, because a wrong proposal applied
// silently is worse than an empty card.
func (m *Manager) DetectRepoEnv(repoID string) (*envdetect.Proposal, error) {
	m.mu.Lock()
	on := m.featureOnLocked(FeatDetect)
	r := m.def.repo(repoID)
	m.mu.Unlock()
	if !on {
		return nil, fmt.Errorf("service detection is turned off (Settings → Harness: detect services)")
	}
	if r == nil {
		return nil, fmt.Errorf("repo %s not found", repoID)
	}
	return envdetect.Detect(repoID, r.Path), nil
}

// ApplyRepoProposal merges the accepted candidates into the definition by
// name — it never deletes or overwrites an entry the user already has — and
// ticks the newly added services in the active env if none were ticked.
func (m *Manager) ApplyRepoProposal(repoID string, p *envdetect.Proposal) error {
	if p == nil {
		return fmt.Errorf("no proposal")
	}
	m.mu.Lock()
	if !m.featureOnLocked(FeatDetect) {
		m.mu.Unlock()
		return fmt.Errorf("service detection is turned off")
	}
	r := m.def.repo(repoID)
	if r == nil {
		m.mu.Unlock()
		return fmt.Errorf("repo %s not found", repoID)
	}
	var added []string
	for _, c := range p.Services {
		if c == nil || !c.Accept || r.service(c.Name) != nil {
			continue
		}
		r.Services = append(r.Services, &Service{Name: c.Name, Cmd: c.Cmd, Port: c.Port, PortEnv: c.PortEnv, PortArgs: c.PortArgs})
		added = append(added, c.Name)
	}
	for _, c := range p.Steps {
		if c == nil || !c.Accept {
			continue
		}
		if c.Kind == "infra" && p.Compose != "" {
			// shared infra is brought up once per project, not chained into
			// every env's prestart
			if r.Compose == "" {
				r.Compose = p.Compose
			}
			continue
		}
		if r.step(c.Name) != nil {
			continue
		}
		r.Steps = append(r.Steps, &Step{
			Name:        c.Name,
			Cmd:         c.Cmd,
			Kind:        c.Kind,
			Default:     !c.Destructive && (c.Kind == "install" || c.Kind == "migrate" || c.Kind == "codegen"),
			Destructive: c.Destructive,
			Supersedes:  append([]string{}, c.Supersedes...),
			CacheInputs: []string{},
		})
	}
	if t := m.activeEnvLocked().Targets[repoID]; t != nil && len(t.Services) == 0 {
		t.Services = append([]string{}, added...)
	}
	def, st := m.def, m.state
	m.mu.Unlock()
	if err := m.SaveDefinition(def); err != nil {
		return err
	}
	return saveState(m.projectRoot, st)
}

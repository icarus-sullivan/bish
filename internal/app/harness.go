package app

import (
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/csullivan/bish/internal/commandcenter"
	"github.com/csullivan/bish/internal/envdetect"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// Harness extensions to Command Center (HARNESS_PLAN.md): service
// detection, ephemeral environments, per-env databases, and the step cache.
// Each is gated inside commandcenter.Manager by its feature flag, so these
// methods return an error rather than doing anything when it's off.

func (a *App) emitCC() {
	runtime.EventsEmit(a.ctx, "cc:update", a.cc.Snapshot())
}

func (a *App) DetectRepoEnv(repoID string) (*envdetect.Proposal, error) {
	return a.cc.DetectRepoEnv(repoID)
}

func (a *App) ApplyRepoProposal(repoID string, p *envdetect.Proposal) error {
	err := a.cc.ApplyRepoProposal(repoID, p)
	a.emitCC()
	return err
}

func (a *App) ListEnvs() []*commandcenter.Env {
	return a.cc.ListEnvs()
}

func (a *App) CreateEnv(name, branch string) (*commandcenter.Env, error) {
	a.telemetry.Count("cc_create_env")
	e, err := a.cc.CreateEnv(name, branch)
	a.emitCC()
	return e, err
}

func (a *App) SetActiveEnv(name string) error {
	err := a.cc.SetActiveEnv(name)
	a.emitCC()
	return err
}

func (a *App) StartEnv(name string) error {
	err := a.cc.StartEnv(name)
	a.emitCC()
	return err
}

func (a *App) StopEnv(name string) error {
	err := a.cc.StopEnv(name)
	a.emitCC()
	return err
}

func (a *App) DestroyEnv(name string, dropDB, removeWorktrees bool) error {
	err := a.cc.DestroyEnv(name, dropDB, removeWorktrees)
	a.emitCC()
	return err
}

func (a *App) ClearStepCache(repoID string) error {
	return a.cc.ClearStepCache(repoID)
}

// DefaultCommandCenterDBSpec is the editable starting point for a DB mode
// ("template" | "compose").
func (a *App) DefaultCommandCenterDBSpec(mode string) *commandcenter.DBSpec {
	return commandcenter.DefaultDBSpec(mode)
}

// PreviewFrameBlocked reports why url can't be shown in the Preview tab's
// iframe ("" = it can). A frame refused via X-Frame-Options or a CSP
// frame-ancestors policy fires no error event in the webview — it just
// stays blank — so the headers are checked here instead.
func (a *App) PreviewFrameBlocked(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return ""
	}
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(u.String())
	if err != nil {
		return ""
	}
	resp.Body.Close()
	if xfo := strings.ToUpper(strings.TrimSpace(resp.Header.Get("X-Frame-Options"))); xfo == "DENY" || xfo == "SAMEORIGIN" {
		return "X-Frame-Options: " + xfo
	}
	for _, csp := range resp.Header.Values("Content-Security-Policy") {
		for _, dir := range strings.Split(csp, ";") {
			f := strings.Fields(strings.TrimSpace(dir))
			if len(f) == 0 || strings.ToLower(f[0]) != "frame-ancestors" {
				continue
			}
			for _, src := range f[1:] {
				if src == "*" {
					return ""
				}
			}
			return "Content-Security-Policy: " + strings.TrimSpace(dir)
		}
	}
	return ""
}

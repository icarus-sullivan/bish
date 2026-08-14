package app

import (
	"fmt"

	"github.com/csullivan/bish/internal/process"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// notifyProcessChanges edge-detects Running -> Crashed/Stopped transitions
// against the previous refreshLoop tick and fires a native OS notification
// for each (crash or a task finishing), gated on the user's toggle and on
// the window being out of view — a foreground app doesn't need a system
// notification for something already visible in the Processes panel.
func (a *App) notifyProcessChanges(procs []*process.Process) {
	cur := make(map[string]process.Status, len(procs))
	for _, p := range procs {
		cur[p.ID] = p.Status
		prev, existed := a.prevProcStatus[p.ID]
		if existed && prev == process.StatusRunning && p.Status != process.StatusRunning {
			a.telemetry.Count("process_" + string(p.Status))
			a.notify(p)
		}
	}
	a.prevProcStatus = cur
}

func (a *App) notify(p *process.Process) {
	if !a.cfg.NotificationsEnabled() || !runtime.WindowIsMinimised(a.ctx) {
		return
	}
	title, body := p.Name+" finished", fmt.Sprintf("Exited cleanly (%s)", p.Cmd)
	if p.Status == process.StatusCrashed {
		title, body = p.Name+" crashed", fmt.Sprintf("Exit code %d — %s", p.ExitCode, p.Cmd)
	}
	_ = runtime.SendNotification(a.ctx, runtime.NotificationOptions{
		ID: "process:" + p.ID, Title: title, Body: body,
	})
}

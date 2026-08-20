package app

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/csullivan/bish/internal/assistant"
	"github.com/csullivan/bish/internal/commands"
	"github.com/csullivan/bish/internal/completion"
	"github.com/csullivan/bish/internal/config"
	"github.com/csullivan/bish/internal/dap"
	"github.com/csullivan/bish/internal/extensions"
	"github.com/csullivan/bish/internal/langext"
	"github.com/csullivan/bish/internal/liveshare"
	"github.com/csullivan/bish/internal/lsp"
	"github.com/csullivan/bish/internal/process"
	"github.com/csullivan/bish/internal/project"
	bishpty "github.com/csullivan/bish/internal/pty"
	"github.com/csullivan/bish/internal/remote"
	"github.com/csullivan/bish/internal/search"
	"github.com/csullivan/bish/internal/telemetry"
	"github.com/csullivan/bish/internal/theme"
	"github.com/csullivan/bish/internal/tree"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

var mediaExts = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true,
	".bmp": true, ".tiff": true, ".tif": true, ".webp": true,
	".mp4": true, ".mov": true, ".webm": true, ".mkv": true, ".avi": true,
}

var videoExts = map[string]bool{
	".mp4": true, ".mov": true, ".webm": true, ".mkv": true, ".avi": true,
}

const maxEditorFileSize = 5 * 1024 * 1024 // CodeMirror chokes on single docs much past this

var skipDirs = tree.SkipDirs

type App struct {
	mgr         *process.Manager
	cmdStore    *commands.Store
	cmdMu       sync.Mutex
	shell       *bishpty.PTY
	terminals   map[string]*bishpty.PTY
	terminalsMu sync.Mutex
	termCount   int
	fileTree    *tree.Tree
	treeMu      sync.Mutex
	// multi-root workspace: additional folders beyond the primary project
	// root, each with its own Tree. Guarded by treeMu (tree-display state,
	// not project identity) — order is add-order, matching the sidebar.
	extraRoots             []string
	extraTrees             map[string]*tree.Tree
	selectedPath           string
	fsw                    *fsnotify.Watcher
	cwd                    string
	cwdFile                string
	wFilePath              string
	wFilePos               int64
	galleryFile            string
	galleryCur             string
	projectRoot            string
	projectCfg             *project.Config
	projectMu              sync.Mutex
	remoteDest             string // "" = local project; else an SSH destination ("user@host")
	lsp                    *lsp.Manager
	langextFormatter       *langext.FormatterManager
	dap                    *dap.Manager
	liveShare              *liveshare.Manager
	assistant              *assistant.Manager
	completion             *completion.Manager
	telemetry              *telemetry.Manager
	prevProcStatus         map[string]process.Status // refreshLoop's crash/finish notification edge-detector
	cfg                    config.Config
	ctx                    context.Context
	DockMenuUpdater        func()
	QuitInterceptInstaller func()
	// PromoteInstance signals another live bish window (by pid) to take
	// over Dock-icon/menu-bar representation — see Shutdown and
	// promote_darwin.go.
	PromoteInstance func(pid int)
	StartupProject  string
	StartupFile     string
	MediaBase       string
	NoRestore       bool
	quitRequested   atomic.Bool
	// ChildWindow is true when this window launched without its own Dock
	// icon (collapsed under another window's — see launchNewInstance). Flips
	// to false in place if this window is later promoted to take over Dock
	// representation, so a *second* handoff still finds the right owner.
	ChildWindow atomic.Bool
}

// SetQuitRequested marks that the user chose Quit (vs. closing this window
// individually), so Shutdown knows to keep this project in the restore session.
func (a *App) SetQuitRequested() {
	a.quitRequested.Store(true)
}

func New(cfg config.Config, mgr *process.Manager, store *commands.Store,
	shell *bishpty.PTY, cwd, cwdFile, wFilePath, galleryFile string) *App {
	return &App{
		mgr:         mgr,
		cmdStore:    store,
		shell:       shell,
		terminals:   make(map[string]*bishpty.PTY),
		fileTree:    &tree.Tree{}, // loaded async in Startup — a sync walk of cwd (often $HOME) blocks the window
		extraTrees:  make(map[string]*tree.Tree),
		cwd:         cwd,
		cwdFile:     cwdFile,
		wFilePath:   wFilePath,
		galleryFile: galleryFile,
		cfg:         cfg,
	}
}

func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx
	a.lsp = lsp.NewManager(func(event string, data ...interface{}) {
		runtime.EventsEmit(a.ctx, event, data...)
	})
	a.lsp.SetOverrides(a.cfg.Languages)
	a.langextFormatter = langext.NewFormatterManager(func(event string, data ...interface{}) {
		runtime.EventsEmit(a.ctx, event, data...)
	})
	a.langextFormatter.SetOverrides(a.cfg.Languages)
	a.dap = dap.NewManager(func(event string, data ...interface{}) {
		runtime.EventsEmit(a.ctx, event, data...)
	})
	a.liveShare = liveshare.NewManager(func(event string, data ...interface{}) {
		runtime.EventsEmit(a.ctx, event, data...)
	})
	a.assistant = assistant.NewManager(func(event string, data ...interface{}) {
		runtime.EventsEmit(a.ctx, event, data...)
	}, a.cfg.Assistant)
	a.completion = completion.NewManager()
	a.completion.SetConfig(a.cfg.Completion)
	a.telemetry = telemetry.NewManager()
	a.telemetry.SetConfig(a.cfg.Telemetry.Enabled, a.cfg.Telemetry.Endpoint)
	applySearchConfig(a.cfg)
	a.telemetry.StartLoop(ctx.Done())
	a.prevProcStatus = map[string]process.Status{}
	if !a.cfg.BuiltinExtensionsSeeded {
		if extensions.SeedBuiltins(extensions.Dir()) == nil {
			a.cfg.BuiltinExtensionsSeeded = true
			_ = config.Save(a.cfg)
		}
	}
	_ = runtime.InitializeNotifications(ctx) // no-op error on unsupported platforms, notifications just won't fire
	project.RegisterInstance(os.Getpid())    //nolint
	go a.readPTYLoopFor("main", a.shell)
	go a.pollCWDLoop()
	go a.pollWLoop()
	go a.pollGalleryLoop()
	go a.refreshLoop()
	go pruneDrops()
	a.startWatcher()
	if a.QuitInterceptInstaller != nil {
		a.QuitInterceptInstaller()
	}
	if a.StartupProject != "" {
		go a.openProjectDir(a.StartupProject) //nolint
	} else {
		// no project directory to show — leave a.fileTree empty (its zero value)
		// rather than falling back to a.cwd, which used to leak whatever
		// directory the process happened to launch from (often the install
		// dir) into the tree/sidebar on a truly blank launch
		if a.StartupFile != "" {
			project.AddRecentFile(a.StartupFile) //nolint
		}
		// skip session restore when a bare file was passed on the command line —
		// restoring a prior project would clear the tab we're about to open for it
		if !a.NoRestore && a.StartupFile == "" {
			go a.restoreSession()
		}
	}
	if a.DockMenuUpdater != nil {
		a.DockMenuUpdater()
	}
}

// restoreSession reopens the project windows that were still open the last
// time bish quit (Cmd+Q), skipping any that were closed individually.
func (a *App) restoreSession() {
	paths, _ := project.LoadSession()
	if len(paths) == 0 {
		return
	}
	a.openProjectDir(paths[0]) //nolint
	for _, p := range paths[1:] {
		launchNewInstance("--project", p) //nolint
	}
}

func (a *App) Shutdown(ctx context.Context) {
	pid := os.Getpid()
	project.UnregisterInstance(pid) //nolint
	// This window owns the Dock icon/menu bar — closing it with other bish
	// windows still open would otherwise leave the app with no Dock
	// representation at all (accessory windows never had their own) and no
	// window able to reliably grab the menu bar. Hand off to another live
	// window before exiting instead.
	if !a.ChildWindow.Load() && a.PromoteInstance != nil {
		if target, ok := project.PickPromotionTarget(pid); ok {
			a.PromoteInstance(target)
		}
	}
	a.projectMu.Lock()
	root := a.projectRoot
	a.projectMu.Unlock()
	// unconditional: this pid is exiting either way, so it can no longer be
	// the window Open Recent should focus for root — unlike session.json
	// (which only drops root on an individual close, not a full quit, so it
	// restores next cold start), there's no "keep this to restore later" case
	if root != "" {
		project.UnregisterWindow(root) //nolint
	}
	if !a.quitRequested.Load() {
		if root != "" {
			project.RemoveFromSession(root) //nolint
		}
	}
	a.lsp.StopAll()
	a.dap.Stop()
	a.liveShare.StopAll()
	a.assistant.StopAll()
	a.completion.Stop()
	a.telemetry.Flush()
	a.mgr.KillAll()
	a.shell.Close()
	a.terminalsMu.Lock()
	for _, p := range a.terminals {
		p.Close()
	}
	a.terminalsMu.Unlock()
	os.Remove(a.cwdFile)
	os.Remove(a.wFilePath)
	os.Remove(a.galleryFile)
}

func (a *App) NewTerminal() (string, error) {
	a.projectMu.Lock()
	dir := a.projectRoot
	dest := a.remoteDest
	a.projectMu.Unlock()
	if dir == "" {
		dir = a.cwd
	}

	var p *bishpty.PTY
	var err error
	if dest != "" {
		p, err = bishpty.NewRemote(dest, dir)
	} else {
		p, err = bishpty.New(a.cfg.Shell, a.cwdFile, a.wFilePath, a.galleryFile)
	}
	if err != nil {
		return "", err
	}
	a.terminalsMu.Lock()
	a.termCount++
	id := fmt.Sprintf("t%d", a.termCount)
	a.terminals[id] = p
	a.terminalsMu.Unlock()
	go a.readPTYLoopFor(id, p)
	if dest == "" {
		fmt.Fprintf(p, "cd %q\n", dir) //nolint
	} // else: remote cwd is already baked into the ssh command itself (bishpty.NewRemote)
	return id, nil
}

func (a *App) CloseTerminal(id string) {
	a.terminalsMu.Lock()
	p, ok := a.terminals[id]
	if ok {
		delete(a.terminals, id)
	}
	a.terminalsMu.Unlock()
	if ok {
		p.Close()
	}
}

func (a *App) WritePTYTab(id, data string) error {
	a.terminalsMu.Lock()
	p, ok := a.terminals[id]
	a.terminalsMu.Unlock()
	if !ok {
		return fmt.Errorf("terminal %s not found", id)
	}
	_, err := fmt.Fprint(p, data)
	return err
}

func (a *App) ResizePTYTab(id string, rows, cols int) {
	a.terminalsMu.Lock()
	p, ok := a.terminals[id]
	a.terminalsMu.Unlock()
	if ok {
		p.Resize(rows, cols)
	}
}

// ptyFor resolves a terminal id ("main" or a NewTerminal-issued tab id) to
// its PTY — the same two places readPTYLoopFor's callers already look.
func (a *App) ptyFor(id string) *bishpty.PTY {
	if id == "main" {
		return a.shell
	}
	a.terminalsMu.Lock()
	defer a.terminalsMu.Unlock()
	return a.terminals[id]
}

// -- Live Share (terminal pairing) --

// StartLiveShare shares terminalId's live output with anyone who opens the
// returned link on the local network — idempotent, returns the existing
// link if already sharing. See internal/liveshare's doc comment for scope.
func (a *App) StartLiveShare(terminalId string) (string, error) {
	p := a.ptyFor(terminalId)
	if p == nil {
		return "", fmt.Errorf("terminal %s not found", terminalId)
	}
	return a.liveShare.Start(terminalId, func(b []byte) error {
		_, err := p.Write(b)
		return err
	})
}

func (a *App) StopLiveShare(terminalId string) {
	a.liveShare.Stop(terminalId)
}

func (a *App) IsLiveSharing(terminalId string) bool {
	return a.liveShare.IsSharing(terminalId)
}

func (a *App) GetLiveShareGuests(terminalId string) []liveshare.GuestInfo {
	return a.liveShare.Guests(terminalId)
}

// SetLiveShareGuestPermission is the host's read-only/can-type toggle for
// one connected guest — defaults to read-only when a guest first connects.
func (a *App) SetLiveShareGuestPermission(terminalId, guestId string, canType bool) error {
	return a.liveShare.SetGuestPermission(terminalId, guestId, canType)
}

// -- Live Share (editor co-editing, phase 2 — PM_ASKS.md #12.b) --

// StartEditShare shares path for co-editing. Go is a dumb relay here — guest
// bytes come back as an "editshare:data" event for the host frontend's
// Y.Doc (frontend/src/lib/coedit.ts) to apply; it never touches the file
// itself except via the normal WriteFile save path.
func (a *App) StartEditShare(path string) (string, error) {
	a.telemetry.Count("coedit_session")
	return a.liveShare.StartEdit(path, func(b []byte) error {
		runtime.EventsEmit(a.ctx, "editshare:data", liveshare.EditData{Path: path, Data: base64.StdEncoding.EncodeToString(b)})
		return nil
	})
}

func (a *App) StopEditShare(path string) {
	a.liveShare.StopEdit(path)
}

func (a *App) IsEditSharing(path string) bool {
	return a.liveShare.IsEditSharing(path)
}

func (a *App) GetEditShareGuests(path string) []liveshare.GuestInfo {
	return a.liveShare.EditGuests(path)
}

func (a *App) SetEditShareGuestPermission(path, guestId string, canType bool) error {
	return a.liveShare.SetEditGuestPermission(path, guestId, canType)
}

// EditShareBroadcast relays one blob of Yjs protocol bytes (base64) from the
// host frontend's Y.Doc to every guest connected to path's session.
func (a *App) EditShareBroadcast(path, dataB64 string) error {
	b, err := base64.StdEncoding.DecodeString(dataB64)
	if err != nil {
		return err
	}
	a.liveShare.BroadcastEdit(path, b)
	return nil
}

func (a *App) readPTYLoopFor(id string, p *bishpty.PTY) {
	dataEvent, exitEvent := "pty:data", "pty:exit"
	if id != "main" {
		dataEvent = "pty:data:" + id
		exitEvent = "pty:exit:" + id
	}

	ch := make(chan []byte, 512)

	// reader: push raw chunks into channel as fast as the PTY produces them
	go func() {
		buf := make([]byte, 32768)
		for {
			n, err := p.Read(buf)
			if n > 0 {
				tmp := make([]byte, n)
				copy(tmp, buf[:n])
				ch <- tmp
			}
			if err != nil {
				close(ch)
				return
			}
		}
	}()

	// emitter: coalesce chunks for up to 8ms so escape sequences
	// are never split across EventsEmit calls
	ticker := time.NewTicker(8 * time.Millisecond)
	defer ticker.Stop()
	var pending []byte

	flush := func() {
		if len(pending) > 0 {
			s := string(pending) // copies — pending's backing array gets reused below
			runtime.EventsEmit(a.ctx, dataEvent, s)
			a.liveShare.Broadcast(id, []byte(s)) // no-op unless this terminal is currently shared
			pending = pending[:0]
		}
	}

	for {
		select {
		case data, ok := <-ch:
			if !ok {
				flush()
				runtime.EventsEmit(a.ctx, exitEvent)
				a.liveShare.Stop(id)
				return
			}
			pending = append(pending, data...)
			if len(pending) > 65536 {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}

func (a *App) pollCWDLoop() {
	var lastMod time.Time
	for {
		select {
		case <-a.ctx.Done():
			return
		case <-time.After(500 * time.Millisecond):
			info, err := os.Stat(a.cwdFile)
			if err != nil || !info.ModTime().After(lastMod) {
				continue
			}
			data, err := os.ReadFile(a.cwdFile)
			if err != nil {
				continue
			}
			newCWD := strings.TrimSpace(string(data))
			lastMod = info.ModTime()
			if newCWD == "" || newCWD == a.cwd {
				continue
			}
			a.cwd = newCWD
			runtime.EventsEmit(a.ctx, "cwd:change", newCWD)
			// Only reload tree from CWD when no project is pinned
			a.projectMu.Lock()
			pinned := a.projectRoot != ""
			a.projectMu.Unlock()
			if !pinned {
				a.treeMu.Lock()
				a.fileTree.Load(newCWD)
				nodes := a.flatNodes()
				a.treeMu.Unlock()
				runtime.EventsEmit(a.ctx, "tree:update", nodes)
			}
		}
	}
}

func (a *App) pollWLoop() {
	for {
		select {
		case <-a.ctx.Done():
			return
		case <-time.After(500 * time.Millisecond):
			data, err := os.ReadFile(a.wFilePath)
			if err != nil || int64(len(data)) <= a.wFilePos {
				continue
			}
			newData := data[a.wFilePos:]
			a.wFilePos = int64(len(data))

			scanner := bufio.NewScanner(strings.NewReader(string(newData)))
			changed := false
			for scanner.Scan() {
				line := scanner.Text()
				if line == "" {
					continue
				}
				parts := strings.SplitN(line, "\t", 2)
				if len(parts) != 2 {
					continue
				}
				cwd, cmdStr := parts[0], parts[1]
				a.mgr.Add(cmdStr, cwd, "") //nolint
				a.projectMu.Lock()
				// only save into the project if the command ran inside it —
				// otherwise commands leak into whatever project happens to be open
				if a.projectCfg != nil && strings.HasPrefix(cwd+"/", a.projectRoot+"/") {
					a.projectCfg.Add(cmdStr, cwd)
					project.Save(a.projectCfg) //nolint
				}
				a.projectMu.Unlock()
				a.cmdMu.Lock()
				a.cmdStore.Add(cmdStr, cwd, cmdStr)
				a.cmdStore.Save() //nolint
				a.cmdMu.Unlock()
				changed = true
			}
			if changed {
				a.cmdMu.Lock()
				cmds := make([]*commands.SavedCommand, len(a.cmdStore.Commands))
				copy(cmds, a.cmdStore.Commands)
				a.cmdMu.Unlock()
				runtime.EventsEmit(a.ctx, "commands:update", cmds)
				runtime.EventsEmit(a.ctx, "processes:update", a.visibleProcesses())
				a.projectMu.Lock()
				if a.projectCfg != nil {
					runtime.EventsEmit(a.ctx, "project:commands", a.projectCfg.Cmds)
				}
				a.projectMu.Unlock()
			}
		}
	}
}

func (a *App) pollGalleryLoop() {
	var lastMod time.Time
	for {
		select {
		case <-a.ctx.Done():
			return
		case <-time.After(300 * time.Millisecond):
			info, err := os.Stat(a.galleryFile)
			if err != nil || !info.ModTime().After(lastMod) || info.Size() == 0 {
				continue
			}
			data, err := os.ReadFile(a.galleryFile)
			if err != nil {
				continue
			}
			target := strings.TrimSpace(string(data))
			if target == "" {
				continue
			}
			lastMod = info.ModTime()
			a.galleryCur = target
			runtime.EventsEmit(a.ctx, "gallery:open", target)
		}
	}
}

func (a *App) refreshLoop() {
	var last []byte
	for {
		select {
		case <-a.ctx.Done():
			return
		case <-time.After(2 * time.Second):
			a.mgr.Refresh()
			procs := a.visibleProcesses()
			a.notifyProcessChanges(procs)
			// emit only on change — idle app does zero store writes/re-renders
			cur, err := json.Marshal(procs)
			if err != nil || !bytes.Equal(cur, last) {
				last = cur
				runtime.EventsEmit(a.ctx, "processes:update", procs)
			}
		}
	}
}

// -- Process methods --

func (a *App) GetProcesses() []*process.Process {
	return a.visibleProcesses()
}

// visibleProcesses scopes the process list to the open project's directory
// tree (root cause of processes leaking between projects: Manager itself is
// global-in-memory and processes.json is one shared file, so every reader
// must filter here) — same "prefix of project root, else global" rule
// already used for saved commands at line ~349.
func (a *App) visibleProcesses() []*process.Process {
	a.projectMu.Lock()
	root := a.projectRoot
	a.projectMu.Unlock()
	all := a.mgr.List()
	if root == "" {
		return all
	}
	out := make([]*process.Process, 0, len(all))
	for _, p := range all {
		if strings.HasPrefix(p.CWD+"/", root+"/") {
			out = append(out, p)
		}
	}
	return out
}

func (a *App) KillProcess(id string) error {
	a.mgr.Remove(id)
	runtime.EventsEmit(a.ctx, "processes:update", a.visibleProcesses())
	a.mgr.SaveToDisk() //nolint
	return nil
}

func (a *App) RestartProcess(id string) error {
	if err := a.mgr.Restart(id); err != nil {
		return err
	}
	runtime.EventsEmit(a.ctx, "processes:update", a.visibleProcesses())
	a.mgr.SaveToDisk() //nolint
	return nil
}

// StopProcess kills the process but leaves its row in place, so Play
// (RestartProcess) can bring it back later — distinct from KillProcess,
// which removes the row entirely.
func (a *App) StopProcess(id string) error {
	if err := a.mgr.Stop(id); err != nil {
		return err
	}
	runtime.EventsEmit(a.ctx, "processes:update", a.visibleProcesses())
	a.mgr.SaveToDisk() //nolint
	return nil
}

func (a *App) GetProcessLogs(id string) []string {
	p := a.mgr.FindByID(id)
	if p == nil || p.Log == nil {
		return nil
	}
	return p.Log.Lines(200)
}

// -- Command methods --

func (a *App) GetCommands() []*commands.SavedCommand {
	a.cmdMu.Lock()
	defer a.cmdMu.Unlock()
	result := make([]*commands.SavedCommand, len(a.cmdStore.Commands))
	copy(result, a.cmdStore.Commands)
	return result
}

func (a *App) RunCommand(id string) error {
	a.cmdMu.Lock()
	var found *commands.SavedCommand
	for _, c := range a.cmdStore.Commands {
		if c.ID == id {
			found = c
			break
		}
	}
	a.cmdMu.Unlock()
	if found == nil {
		return fmt.Errorf("command %s not found", id)
	}
	// Both one-off and long-running commands go through the w-file flow
	// so they appear in the Processes panel.
	f, err := os.OpenFile(a.wFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprintf(f, "%s\t%s\n", found.CWD, found.Command)
	return err
}

func (a *App) AddCommand(name, cwd, command string) error {
	a.cmdMu.Lock()
	a.cmdStore.Add(name, cwd, command)
	err := a.cmdStore.Save()
	cmds := make([]*commands.SavedCommand, len(a.cmdStore.Commands))
	copy(cmds, a.cmdStore.Commands)
	a.cmdMu.Unlock()
	if err != nil {
		return err
	}
	runtime.EventsEmit(a.ctx, "commands:update", cmds)
	return nil
}

func (a *App) DeleteCommand(id string) error {
	a.cmdMu.Lock()
	a.cmdStore.Delete(id)
	err := a.cmdStore.Save()
	cmds := make([]*commands.SavedCommand, len(a.cmdStore.Commands))
	copy(cmds, a.cmdStore.Commands)
	a.cmdMu.Unlock()
	if err != nil {
		return err
	}
	runtime.EventsEmit(a.ctx, "commands:update", cmds)
	return nil
}

func (a *App) RenameCommand(id, name string) error {
	a.cmdMu.Lock()
	a.cmdStore.Edit(id, name, "")
	err := a.cmdStore.Save()
	cmds := make([]*commands.SavedCommand, len(a.cmdStore.Commands))
	copy(cmds, a.cmdStore.Commands)
	a.cmdMu.Unlock()
	if err != nil {
		return err
	}
	runtime.EventsEmit(a.ctx, "commands:update", cmds)
	return nil
}

// -- Tree methods --

func (a *App) GetTreeNodes() []TreeNodeDTO {
	a.treeMu.Lock()
	defer a.treeMu.Unlock()
	return a.flatNodes()
}

// treeForPathLocked finds which tree (primary or an extra workspace root)
// contains path — caller must hold treeMu. O(n) linear scan, matching the
// existing style of every other tree lookup here; trees are lazily loaded
// so this never scans more than what's already visible.
func (a *App) treeForPathLocked(path string) *tree.Tree {
	for _, n := range a.fileTree.Flat {
		if n.Path == path {
			return a.fileTree
		}
	}
	for _, t := range a.extraTrees {
		for _, n := range t.Flat {
			if n.Path == path {
				return t
			}
		}
	}
	return nil
}

func (a *App) ToggleTreeNode(path string) {
	a.treeMu.Lock()
	if t := a.treeForPathLocked(path); t != nil {
		for i, n := range t.Flat {
			if n.Path == path {
				t.Selected = i
				t.Toggle()
				break
			}
		}
	}
	a.selectedPath = path
	nodes := a.flatNodes()
	expanded := a.fileTree.ExpandedPaths()
	a.treeMu.Unlock()
	runtime.EventsEmit(a.ctx, "tree:update", nodes)
	a.rearmWatcher()
	go a.saveExpandedPaths(expanded)
}

// RevealInTree expands path's ancestor directories and selects it, emitting
// tree:update — used by Cmd+P selection to keep the sidebar in sync with the
// active tab even when the file's parent folders are collapsed. Mirrors
// ToggleTreeNode's lock/emit/rearm/save-expanded pattern. Only the primary
// root's tree supports reveal-by-ancestor-walk today — a path outside it
// (an extra workspace root) just no-ops rather than failing.
func (a *App) RevealInTree(path string) {
	a.treeMu.Lock()
	a.fileTree.ExpandToPath(path)
	for i, n := range a.fileTree.Flat {
		if n.Path == path {
			a.fileTree.Selected = i
			break
		}
	}
	a.selectedPath = path
	nodes := a.flatNodes()
	expanded := a.fileTree.ExpandedPaths()
	a.treeMu.Unlock()
	runtime.EventsEmit(a.ctx, "tree:update", nodes)
	a.rearmWatcher()
	go a.saveExpandedPaths(expanded)
}

func (a *App) saveExpandedPaths(paths []string) {
	a.projectMu.Lock()
	cfg := a.projectCfg
	a.projectMu.Unlock()
	if cfg == nil {
		return
	}
	cfg.ExpandedPaths = paths
	project.Save(cfg) //nolint
}

func (a *App) RefreshTree() {
	a.reloadTree()
}

func (a *App) CollapseAllTree() {
	a.treeMu.Lock()
	a.fileTree.CollapseAll()
	for _, t := range a.extraTrees {
		t.CollapseAll()
	}
	nodes := a.flatNodes()
	a.treeMu.Unlock()
	runtime.EventsEmit(a.ctx, "tree:update", nodes)
	go a.saveExpandedPaths(nil)
}

func (a *App) CdToPath(path string) error {
	if a.remoteDest != "" {
		return nil // no meaningful "cd" on the local main shell for a remote path
	}
	_, err := a.shell.Write([]byte(fmt.Sprintf("cd %q\n", path)))
	return err
}

// -- Filesystem operations --

func (a *App) FSNewFile(dirPath, name string) error {
	if name == "" {
		name = "newfile"
	}
	path := filepath.Join(dirPath, name)
	var err error
	if a.remoteDest != "" {
		err = remote.CreateFile(a.remoteDest, path)
	} else {
		var f *os.File
		if f, err = os.Create(path); err == nil {
			f.Close()
		}
	}
	if err != nil {
		return err
	}
	a.reloadTree()
	return nil
}

func (a *App) FSNewFolder(dirPath, name string) error {
	if name == "" {
		name = "newfolder"
	}
	path := filepath.Join(dirPath, name)
	var err error
	if a.remoteDest != "" {
		err = remote.Mkdir(a.remoteDest, path)
	} else {
		err = os.MkdirAll(path, 0o755)
	}
	if err != nil {
		return err
	}
	a.reloadTree()
	return nil
}

func (a *App) FSRename(oldPath, newPath string) error {
	var err error
	if a.remoteDest != "" {
		err = remote.Rename(a.remoteDest, oldPath, newPath)
	} else {
		err = os.Rename(oldPath, newPath)
	}
	if err != nil {
		return err
	}
	a.reloadTree()
	return nil
}

func dropsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "bish", "drops"), nil
}

// StashDropped copies drag-and-dropped temp files into ~/.config/bish/drops
// and returns the stable paths. macOS screenshot previews live under
// /var/folders and vanish when the thumbnail dismisses — a pasted path must
// outlive that. Non-temp paths pass through untouched.
func (a *App) StashDropped(paths []string) []string {
	tmp := strings.TrimSuffix(os.TempDir(), "/")
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		isTemp := strings.HasPrefix(p, "/var/folders/") || strings.HasPrefix(p, "/private/var/folders/") ||
			strings.HasPrefix(p, "/tmp/") || strings.HasPrefix(p, tmp+"/")
		if !isTemp {
			out = append(out, p)
			continue
		}
		dir, err := dropsDir()
		if err != nil || os.MkdirAll(dir, 0o755) != nil {
			out = append(out, p)
			continue
		}
		ext := filepath.Ext(p)
		base := strings.TrimSuffix(filepath.Base(p), ext)
		dst := filepath.Join(dir, base+ext)
		for i := 2; ; i++ {
			if _, err := os.Stat(dst); os.IsNotExist(err) {
				break
			}
			dst = filepath.Join(dir, fmt.Sprintf("%s-%d%s", base, i, ext))
		}
		// in-process copy, not exec("cp"): fork/exec costs ~10-30ms during
		// which macOS unlinks the screenshot promise temp. os.Open grabs the
		// FD immediately and the inode survives an unlink mid-read.
		if err := copyFile(p, dst); err != nil {
			out = append(out, p)
			continue
		}
		out = append(out, dst)
	}
	return out
}

// copyFile opens src first (FD survives an unlink-during-read) then writes dst.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(dst)
		return err
	}
	return out.Close()
}

// stashed drops are transient by nature; keep a week
func pruneDrops() {
	dir, err := dropsDir()
	if err != nil {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	cutoff := time.Now().AddDate(0, 0, -7)
	for _, e := range entries {
		if info, err := e.Info(); err == nil && info.ModTime().Before(cutoff) {
			os.RemoveAll(filepath.Join(dir, e.Name()))
		}
	}
}

// FSDuplicate copies path beside itself as name_copy.ext (then _copy2, …)
func (a *App) FSDuplicate(path string) error {
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(path, ext)
	dst := base + "_copy" + ext
	for i := 2; ; i++ {
		if _, err := os.Stat(dst); os.IsNotExist(err) {
			break
		}
		dst = fmt.Sprintf("%s_copy%d%s", base, i, ext)
	}
	if err := exec.Command("cp", "-a", path, dst).Run(); err != nil {
		return err
	}
	a.reloadTree()
	return nil
}

// FSMove moves paths into destDir (Finder / screenshot-preview drops on the
// file tree). Rename first; cross-volume falls back to cp -a + delete.
func (a *App) FSMove(paths []string, destDir string) error {
	var firstErr error
	for _, p := range paths {
		dst := filepath.Join(destDir, filepath.Base(p))
		// same spot, or a dir dropped into itself/its own subtree
		if dst == p || strings.HasPrefix(destDir+"/", p+"/") {
			continue
		}
		if _, err := os.Stat(dst); err == nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("%s already exists in %s", filepath.Base(p), destDir)
			}
			continue
		}
		err := os.Rename(p, dst)
		if err != nil {
			if err = exec.Command("cp", "-a", p, dst).Run(); err == nil {
				err = os.RemoveAll(p)
			}
		}
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	a.reloadTree()
	return firstErr
}

func (a *App) FSDelete(path string) error {
	var err error
	if a.remoteDest != "" {
		err = remote.Remove(a.remoteDest, path)
	} else {
		err = os.RemoveAll(path)
	}
	if err != nil {
		return err
	}
	a.reloadTree()
	return nil
}

func (a *App) FSDeletePaths(paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	choice, err := runtime.MessageDialog(a.ctx, runtime.MessageDialogOptions{
		Type:          runtime.QuestionDialog,
		Title:         "Delete",
		Message:       fmt.Sprintf("Delete %d items? This cannot be undone.", len(paths)),
		Buttons:       []string{"Delete", "Cancel"},
		DefaultButton: "Cancel",
		CancelButton:  "Cancel",
	})
	if err != nil || choice != "Delete" {
		return err
	}
	var firstErr error
	for _, p := range paths {
		var err error
		if a.remoteDest != "" {
			err = remote.Remove(a.remoteDest, p)
		} else {
			err = os.RemoveAll(p)
		}
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	a.reloadTree()
	return firstErr
}

func (a *App) FSCopyPath(path string) string {
	return path
}

// ConfirmDiscardChanges asks via a native OS dialog (same runtime.MessageDialog
// FSDeletePaths uses) whether to close a tab with unsaved edits — closeTab in
// stores.ts used window.confirm() for this, which is unreliable inside a
// WKWebView (no guaranteed WKUIDelegate wiring for JS dialogs): it can return
// immediately without ever prompting, so a modified tab's "×" looked like it
// silently did nothing. A real native dialog doesn't have that failure mode.
func (a *App) ConfirmDiscardChanges(label string) bool {
	choice, err := runtime.MessageDialog(a.ctx, runtime.MessageDialogOptions{
		Type:          runtime.QuestionDialog,
		Title:         "Unsaved Changes",
		Message:       fmt.Sprintf("Discard unsaved changes to %s?", label),
		Buttons:       []string{"Discard", "Cancel"},
		DefaultButton: "Cancel",
		CancelButton:  "Cancel",
	})
	return err == nil && choice == "Discard"
}

func (a *App) FSRevealInFinder(path string) error {
	return exec.Command("open", "-R", path).Run()
}

func (a *App) ReadFileBase64(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

func (a *App) ReadFile(path string) (string, error) {
	if a.remoteDest != "" {
		// ponytail: no cheap remote stat-then-size-guard round trip yet —
		// just cat and run the same binary heuristic below.
		content, err := remote.ReadFile(a.remoteDest, path)
		if err != nil {
			return "", err
		}
		data := []byte(content)
		if bytes.IndexByte(data[:min(len(data), 8000)], 0) != -1 {
			return "", fmt.Errorf("binary file — not displayable as text")
		}
		return content, nil
	}
	if fi, err := os.Stat(path); err == nil && fi.Size() > maxEditorFileSize {
		return "", fmt.Errorf("file too large to open in editor (%.1f MB, limit %d MB)",
			float64(fi.Size())/(1024*1024), maxEditorFileSize/(1024*1024))
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	// git heuristic: null byte in first 8000 bytes = binary
	if bytes.IndexByte(data[:min(len(data), 8000)], 0) != -1 {
		return "", fmt.Errorf("binary file — not displayable as text")
	}
	return string(data), nil
}

const maxChunkSize = 1 << 20
const maxRawFileSize = 1 << 30 // 1GB — hard ceiling even for the chunked raw view

type FileChunk struct {
	DataB64 string `json:"dataB64"`
	Size    int64  `json:"size"`
}

// ReadFileChunk reads length bytes at offset without loading the whole file —
// the raw-view fallback for files ReadFile refuses (too large / binary).
func (a *App) ReadFileChunk(path string, offset, length int64) (FileChunk, error) {
	if length <= 0 || length > maxChunkSize {
		length = maxChunkSize
	}
	f, err := os.Open(path)
	if err != nil {
		return FileChunk{}, err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return FileChunk{}, err
	}
	if fi.Size() > maxRawFileSize {
		return FileChunk{}, fmt.Errorf("file too large to view (%.1f GB, limit 1 GB)", float64(fi.Size())/(1<<30))
	}
	if offset < 0 || offset >= fi.Size() {
		return FileChunk{Size: fi.Size()}, nil
	}
	buf := make([]byte, min(length, fi.Size()-offset))
	n, err := f.ReadAt(buf, offset)
	if err != nil && err != io.EOF {
		return FileChunk{}, err
	}
	return FileChunk{DataB64: base64.StdEncoding.EncodeToString(buf[:n]), Size: fi.Size()}, nil
}

func (a *App) WriteFile(path, content string) error {
	if a.remoteDest != "" {
		return remote.WriteFile(a.remoteDest, path, content)
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

// -- PTY methods --

// LSPStart lazily spawns the language server for lang rooted at root.
// Returns false when no server is installed (frontend falls back to the
// heuristic autoimport) or the lang is in crash backoff.
func (a *App) LSPStart(lang, root string) bool {
	return a.lsp.Start(lang, root)
}

// LSPInstalled reports whether lang's server binary is already on PATH, with
// no side effects — the frontend uses this after a failed LSPStart to decide
// whether to offer an install prompt.
func (a *App) LSPInstalled(lang string) bool {
	return a.lsp.Installed(lang)
}

// LSPInstall runs lang's installer (go install / pnpm add -g), streaming
// progress as lsp:install-output:<lang> events, and blocks until it exits.
func (a *App) LSPInstall(lang string) error {
	return a.lsp.Install(lang)
}

// LSPSend forwards one JSON-RPC message (headerless) to the lang's server.
func (a *App) LSPSend(lang, msg string) error {
	return a.lsp.Send(lang, msg)
}

func (a *App) LSPStop(lang string) {
	a.lsp.Stop(lang)
}

// -- Language extensions (internal/langext) --

// LanguageExtensionDTO adds install status + the user's saved override to a
// langext.Definition, for the Languages panel.
type LanguageExtensionDTO struct {
	langext.Definition
	ServerInstalled    bool                    `json:"serverInstalled"`
	FormatterInstalled bool                    `json:"formatterInstalled"`
	Override           config.LanguageOverride `json:"override"`
}

// ListLanguageExtensions returns every registered language with its current
// install status and saved override — the single source the frontend fetches
// once at startup instead of hardcoding per-language maps of its own.
func (a *App) ListLanguageExtensions() []LanguageExtensionDTO {
	defs := langext.All()
	out := make([]LanguageExtensionDTO, len(defs))
	for i, d := range defs {
		out[i] = LanguageExtensionDTO{
			Definition:         d,
			ServerInstalled:    d.Server != nil && a.lsp.Installed(d.ID),
			FormatterInstalled: d.Formatter != nil && a.langextFormatter.Installed(d.ID),
			Override:           a.cfg.Languages[d.ID],
		}
	}
	return out
}

// FormatterInstalled reports whether id's dedicated formatter binary is on
// PATH, with no side effects.
func (a *App) FormatterInstalled(id string) bool {
	return a.langextFormatter.Installed(id)
}

// FormatterInstall runs id's formatter installer, streaming progress as
// langext:formatter-install-output:<id> events.
func (a *App) FormatterInstall(id string) error {
	return a.langextFormatter.Install(id)
}

// FormatWithExtension runs id's dedicated formatter (a one-shot
// stdin→stdout subprocess, not the LSP server) over content and returns the
// formatted result.
func (a *App) FormatWithExtension(id, path, content string) (string, error) {
	return a.langextFormatter.Format(id, path, content)
}

// GetLanguageOverride returns id's saved override (zero value = defaults).
func (a *App) GetLanguageOverride(id string) config.LanguageOverride {
	return a.cfg.Languages[id]
}

// SetLanguageOverride persists id's override and immediately propagates it
// to the server/formatter managers via the same path SaveConfig already
// uses for every other per-feature config.
func (a *App) SetLanguageOverride(id string, ov config.LanguageOverride) error {
	cfg := a.cfg
	if cfg.Languages == nil {
		cfg.Languages = map[string]config.LanguageOverride{}
	}
	cfg.Languages[id] = ov
	return a.SaveConfig(cfg)
}

// -- Debugger (DAP) methods --

// DebugStart spawns `dlv dap` rooted at root and runs the launch handshake,
// applying breakpoints (absolute path → 1-based line numbers) before the
// program starts running. Blocks until the program is confirmed running or
// the handshake fails (most commonly a build error).
func (a *App) DebugStart(root string, breakpoints map[string][]int) error {
	a.telemetry.Count("debug_session")
	return a.dap.Start(root, breakpoints)
}

// DebugSetBreakpoints replaces the breakpoint set for one file in the
// running session (no-op if no session is active).
func (a *App) DebugSetBreakpoints(path string, lines []int) error {
	return a.dap.SetBreakpoints(path, lines)
}

func (a *App) DebugContinue() error { return a.dap.Continue() }
func (a *App) DebugStepOver() error { return a.dap.StepOver() }
func (a *App) DebugStepIn() error   { return a.dap.StepIn() }
func (a *App) DebugStepOut() error  { return a.dap.StepOut() }
func (a *App) DebugStop()           { a.dap.Stop() }

// -- Assistant methods --

// AssistantPickFiles opens a native multi-file picker for attaching context
// to the Assistant panel (the "+" button next to the composer).
func (a *App) AssistantPickFiles() ([]string, error) {
	return runtime.OpenMultipleFilesDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Attach Files",
	})
}

// AssistantStart lazily spawns `claude` in headless plan-mode (or whatever
// permissionMode the panel asked for) rooted at root, and returns a session
// handle the frontend keeps using for the rest of the conversation.
func (a *App) AssistantStart(root, permissionMode string) (string, error) {
	a.telemetry.Count("assistant_session")
	return a.assistant.Start(root, permissionMode)
}

// AssistantSend writes one user turn to the session's stdin.
func (a *App) AssistantSend(sessionID, text string) error {
	return a.assistant.Send(sessionID, text)
}

// AssistantRespondPermission answers a pending permission ask — the plan
// card's Approve/Reject buttons, and any other tool-use prompt the CLI
// paused on, both route through here. requestID is the id captured off the
// "permission_request" event the panel received for that ask.
func (a *App) AssistantRespondPermission(sessionID, requestID string, allow bool, message string) error {
	return a.assistant.RespondPermission(sessionID, requestID, allow, message)
}

func (a *App) AssistantStop(sessionID string) {
	a.assistant.Stop(sessionID)
}

// AssistantInterrupt stops the in-flight turn but keeps the conversation
// alive (resumes in place) so the user can send a follow-up immediately.
func (a *App) AssistantInterrupt(sessionID string) error {
	return a.assistant.Interrupt(sessionID)
}

// AssistantSwitchMode changes the live session's permission mode in place,
// over the control protocol — no process restart involved.
func (a *App) AssistantSwitchMode(sessionID, mode string) error {
	return a.assistant.SwitchMode(sessionID, mode)
}

// OllamaListModels queries an Ollama server's /api/tags so Settings can
// offer a model picker instead of a freeform text field.
func (a *App) OllamaListModels(baseURL string) ([]assistant.ModelInfo, error) {
	return assistant.ListModels(baseURL)
}

// CompletionSuggest asks the local inline-completion model (independent of
// the Assistant panel's provider) to fill the gap between prefix and
// suffix. Returns "" with no error when the feature is disabled,
// unconfigured, or the model isn't ready yet.
func (a *App) CompletionSuggest(prefix, suffix string) (string, error) {
	return a.completion.Suggest(a.ctx, prefix, suffix)
}

func (a *App) WritePTY(data string) error {
	_, err := a.shell.Write([]byte(data))
	return err
}

func (a *App) ResizePTY(rows, cols int) {
	a.shell.Resize(rows, cols)
}

// -- Gallery methods --

func (a *App) GetGalleryImages(dirPath string) []string {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		// dirPath might be a file path, use its directory
		dirPath = filepath.Dir(dirPath)
		entries, err = os.ReadDir(dirPath)
		if err != nil {
			return nil
		}
	}
	var paths []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if mediaExts[ext] {
			paths = append(paths, filepath.Join(dirPath, e.Name()))
		}
	}
	sort.Strings(paths)
	return paths
}

func (a *App) GetCurrentGalleryPath() string {
	return a.galleryCur
}

func (a *App) IsVideo(path string) bool {
	return videoExts[strings.ToLower(filepath.Ext(path))]
}

// -- Theme/Config methods --

func (a *App) GetTheme() ThemeDTO {
	if ct, ok := a.cfg.CustomThemes[a.cfg.Theme]; ok {
		return ThemeDTO{
			Background: ct.Background, Foreground: ct.Foreground, Border: ct.Border,
			BorderFocused: ct.BorderFocused, Accent: ct.Accent, Muted: ct.Muted,
			Success: ct.Success, Error: ct.Error, Warning: ct.Warning,
		}
	}
	th := theme.Get(a.cfg.Theme)
	return ThemeDTO{
		Background:    th.Background,
		Foreground:    th.Foreground,
		Border:        th.Border,
		BorderFocused: th.BorderFocused,
		Accent:        th.Accent,
		Muted:         th.Muted,
		Success:       th.Success,
		Error:         th.Error,
		Warning:       th.Warning,
	}
}

func (a *App) GetConfig() config.Config {
	return a.cfg
}

func (a *App) SaveConfig(cfg config.Config) error {
	a.cfg = cfg
	th := a.GetTheme()
	runtime.EventsEmit(a.ctx, "theme:update", th)
	a.assistant.SetConfig(cfg.Assistant)
	a.completion.SetConfig(cfg.Completion)
	a.lsp.SetOverrides(cfg.Languages)
	a.langextFormatter.SetOverrides(cfg.Languages)
	a.telemetry.SetConfig(cfg.Telemetry.Enabled, cfg.Telemetry.Endpoint)
	applySearchConfig(cfg)
	return config.Save(cfg)
}

// applySearchConfig pushes cfg.SearchMaxDepth into the search package's
// process-wide MaxWalkDepth — shared by the Files panel search, the
// assistant's list_files/search_files tools, and Replace All, none of which
// otherwise have a route to per-request config.
func applySearchConfig(cfg config.Config) {
	if cfg.SearchMaxDepth > 0 {
		search.MaxWalkDepth = cfg.SearchMaxDepth
	} else {
		search.MaxWalkDepth = search.DefaultMaxWalkDepth
	}
}

// ExportSettingsFile writes content (a JSON bundle the frontend builds
// itself, since it covers theme/features from config.Config *and*
// frontend-local custom keybindings Go has no model of) to a user-chosen
// path via a native Save dialog.
func (a *App) ExportSettingsFile(content string) (string, error) {
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		DefaultFilename: "bish-settings.json",
		Title:           "Export Settings",
		Filters:         []runtime.FileFilter{{DisplayName: "JSON (*.json)", Pattern: "*.json"}},
	})
	if err != nil || path == "" {
		return "", err
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// ImportSettingsFile opens a native Open dialog and returns the chosen
// file's raw content ("" if cancelled) — the frontend parses and applies it.
func (a *App) ImportSettingsFile() (string, error) {
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title:   "Import Settings",
		Filters: []runtime.FileFilter{{DisplayName: "JSON (*.json)", Pattern: "*.json"}},
	})
	if err != nil || path == "" {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// -- Extensions --

// ExtensionDTO adds the persisted enable state to a discovered extension.
type ExtensionDTO struct {
	extensions.Extension
	Enabled bool `json:"enabled"`
}

// GetExtensions discovers extensions under ~/.bish/extensions and marks
// each enabled/disabled per the saved config (missing = enabled).
func (a *App) GetExtensions() []ExtensionDTO {
	found := extensions.Discover(extensions.Dir())
	out := make([]ExtensionDTO, len(found))
	for i, e := range found {
		enabled := true
		if v, ok := a.cfg.Extensions[e.Name]; ok {
			enabled = v
		}
		out[i] = ExtensionDTO{Extension: e, Enabled: enabled}
	}
	return out
}

// SetExtensionEnabled persists one extension's enable state — the frontend
// starts/stops its Worker immediately either way, this just makes it stick
// across restarts.
func (a *App) SetExtensionEnabled(name string, enabled bool) error {
	if a.cfg.Extensions == nil {
		a.cfg.Extensions = map[string]bool{}
	}
	a.cfg.Extensions[name] = enabled
	return config.Save(a.cfg)
}

// UninstallExtension deletes an extension's directory under ~/.bish/extensions
// and drops its enable-state entry. name must be a bare directory name (no
// separators) — it comes straight from the frontend, and extensions.Dir() is
// the only root this is ever allowed to touch.
func (a *App) UninstallExtension(name string) error {
	if name == "" || strings.ContainsAny(name, "/\\") || name == "." || name == ".." {
		return fmt.Errorf("invalid extension name %q", name)
	}
	if err := os.RemoveAll(filepath.Join(extensions.Dir(), name)); err != nil {
		return err
	}
	delete(a.cfg.Extensions, name)
	return config.Save(a.cfg)
}

// -- helpers --

// flatNodes concatenates the primary tree's flat rows with each extra
// workspace root's, in add-order — the frontend renders each tree's own
// depth-0 root row as that section's header, so no separate "group" DTO is
// needed (multi-root falls out of the existing single-tree row renderer).
func (a *App) flatNodes() []TreeNodeDTO {
	result := make([]TreeNodeDTO, 0, len(a.fileTree.Flat))
	add := func(t *tree.Tree) {
		for _, n := range t.Flat {
			result = append(result, TreeNodeDTO{
				Name:     n.Name,
				Path:     n.Path,
				IsDir:    n.IsDir,
				Depth:    n.Depth,
				Expanded: n.Expanded,
				Selected: n.Path == a.selectedPath,
			})
		}
	}
	add(a.fileTree)
	for _, r := range a.extraRoots {
		if t := a.extraTrees[r]; t != nil {
			add(t)
		}
	}
	return result
}

func (a *App) reloadTree() {
	a.projectMu.Lock()
	root := a.projectRoot
	dest := a.remoteDest
	a.projectMu.Unlock()
	a.treeMu.Lock()
	if root == "" {
		// no project open — empty state, not a fallback listing of a.cwd
		// (which is often wherever the binary happens to launch from)
		a.fileTree = &tree.Tree{}
	} else {
		if dest != "" {
			a.fileTree.FS = remote.TreeFS{Dest: dest}
		} else {
			a.fileTree.FS = nil
		}
		expanded := a.fileTree.ExpandedPaths()
		a.fileTree.Load(root)
		a.fileTree.RestoreExpanded(expanded)
	}

	for _, r := range a.extraRoots {
		t := a.extraTrees[r]
		if t == nil {
			t = &tree.Tree{}
			a.extraTrees[r] = t
		}
		exp := t.ExpandedPaths()
		t.Load(r)
		t.RestoreExpanded(exp)
	}

	nodes := a.flatNodes()
	a.treeMu.Unlock()
	runtime.EventsEmit(a.ctx, "tree:update", nodes)
	a.rearmWatcher()
}

// -- Window methods --

func (a *App) NewWindow() error {
	// --no-restore: this is a deliberate blank window, not a cold start —
	// it shouldn't re-trigger session restore in the spawned process.
	return launchNewInstance("--no-restore")
}

// OpenRecentFileInNewWindow opens a standalone file (no project) in a new
// window — used by the Dock menu's "Recent Files" section. launchNewInstance
// with just the path matches the CLI's own positional-arg-as-file handling
// (main.go), so no new flag is needed.
func (a *App) OpenRecentFileInNewWindow(path string) error {
	return launchNewInstance(path)
}

// launchNewInstance starts another bish window as a separate process.
// --child-window makes the new instance run with the accessory activation
// policy (no own Dock icon), so all windows collapse under the primary's icon.
func launchNewInstance(args ...string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	args = append(args, "--child-window")
	if bundle, ok := appBundlePath(exe); ok {
		openArgs := append([]string{"-n", bundle}, "--args")
		openArgs = append(openArgs, args...)
		return exec.Command("open", openArgs...).Start()
	}
	return exec.Command(exe, args...).Start()
}

// appBundlePath returns the .app bundle root for an executable path inside
// Contents/MacOS, e.g. "/A/bish.app/Contents/MacOS/bish" -> "/A/bish.app".
func appBundlePath(exe string) (string, bool) {
	const marker = ".app/Contents/MacOS/"
	i := strings.Index(exe, marker)
	if i == -1 {
		return "", false
	}
	return exe[:i+len(".app")], true
}

func (a *App) GetCWD() string {
	return a.cwd
}

func (a *App) TriggerNewFile() {
	runtime.EventsEmit(a.ctx, "file:new")
}

// GetStartupFile returns the file passed on the command line (`bish <file>`),
// if any. Read once by the frontend at startup, same as GetProjectRoot.
func (a *App) GetStartupFile() string {
	return a.StartupFile
}

func (a *App) TriggerPalette() {
	runtime.EventsEmit(a.ctx, "palette:open")
}

func (a *App) SaveNewFile(content, defaultDir string) (string, error) {
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		DefaultDirectory: defaultDir,
		DefaultFilename:  "untitled",
		Title:            "Save File",
	})
	if err != nil || path == "" {
		return "", err
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", err
	}
	a.reloadTree()
	return path, nil
}

// -- Project methods --

func (a *App) openProjectDir(dir string) error {
	a.lsp.StopAll() // stale servers point at the old root
	a.dap.Stop()    // stale debug session points at the old root
	cfg, err := project.Load(dir)
	if err != nil {
		cfg = &project.Config{CWD: dir}
	}
	a.projectMu.Lock()
	prevRoot := a.projectRoot
	a.projectRoot = dir
	a.projectCfg = cfg
	a.remoteDest = ""
	a.projectMu.Unlock()
	// this window is switching roots — it no longer "has open" prevRoot, so a
	// later Open Recent for prevRoot spawns a fresh window instead of trying
	// to focus this one (which would show dir, not prevRoot)
	if prevRoot != "" && prevRoot != dir {
		project.UnregisterWindow(prevRoot) //nolint
	}
	project.RegisterWindow(dir, os.Getpid()) //nolint
	a.treeMu.Lock()
	a.extraRoots = append([]string{}, cfg.ExtraRoots...)
	a.extraTrees = make(map[string]*tree.Tree, len(a.extraRoots))
	a.treeMu.Unlock()
	runtime.WindowSetTitle(a.ctx, filepath.Base(dir))
	a.reloadTree()
	// restore saved expansion from previous session
	if len(cfg.ExpandedPaths) > 0 {
		a.treeMu.Lock()
		a.fileTree.RestoreExpanded(cfg.ExpandedPaths)
		nodes := a.flatNodes()
		a.treeMu.Unlock()
		runtime.EventsEmit(a.ctx, "tree:update", nodes)
	}
	// cd main shell to project root
	fmt.Fprintf(a.shell, "cd %q\n", dir) //nolint
	runtime.EventsEmit(a.ctx, "project:change", dir)
	runtime.EventsEmit(a.ctx, "project:remote", false)
	runtime.EventsEmit(a.ctx, "project:commands", cfg.Cmds)
	runtime.EventsEmit(a.ctx, "project:tasks", project.LoadTasks(dir))
	project.AddRecent(dir)    //nolint
	project.AddToSession(dir) //nolint
	if a.DockMenuUpdater != nil {
		go a.DockMenuUpdater()
	}
	return nil
}

// OpenRemoteProject points the whole window (file tree, editor, new
// terminals) at path on an SSH destination — see internal/remote for why
// this shells out to `ssh` rather than speaking the protocol directly.
// Deliberately out of scope for this first cut: remote git panel, remote
// LSP/debugger, remote search, and the Recent Projects list (which assumes
// a locally-`os.Stat`-able path) — all degrade harmlessly rather than being
// specially handled.
func (a *App) OpenRemoteProject(dest, path string) error {
	if dest == "" || path == "" {
		return fmt.Errorf("host and path are required")
	}
	if err := remote.Reachable(dest); err != nil {
		return fmt.Errorf("can't reach %s: %w", dest, err)
	}
	if isDir, err := remote.Stat(dest, path); err != nil || !isDir {
		return fmt.Errorf("%s:%s is not a directory", dest, path)
	}
	a.lsp.StopAll()
	a.dap.Stop()
	a.projectMu.Lock()
	a.projectRoot = path
	a.remoteDest = dest
	a.projectCfg = &project.Config{CWD: path}
	a.projectMu.Unlock()
	// multi-root workspaces are local-only for now (extra roots are always
	// browsed via SSH to a.remoteDest, which is a single global destination)
	a.treeMu.Lock()
	a.extraRoots = nil
	a.extraTrees = make(map[string]*tree.Tree)
	a.treeMu.Unlock()
	runtime.WindowSetTitle(a.ctx, fmt.Sprintf("%s — %s", filepath.Base(path), remote.ShortDest(dest)))
	a.reloadTree()
	runtime.EventsEmit(a.ctx, "project:change", path)
	runtime.EventsEmit(a.ctx, "project:remote", true)
	runtime.EventsEmit(a.ctx, "project:commands", nil)
	runtime.EventsEmit(a.ctx, "project:tasks", nil)
	return nil
}

// IsRemoteProject reports whether the open project lives on an SSH
// destination (fetched once at startup, mirroring GetProjectRoot).
func (a *App) IsRemoteProject() bool {
	a.projectMu.Lock()
	defer a.projectMu.Unlock()
	return a.remoteDest != ""
}

// AddWorkspaceRoot opens a directory picker and attaches it as an extra
// folder in the current workspace (multi-root), alongside the primary
// project. Persisted on the primary project's config so reopening it
// restores the same set of folders.
func (a *App) AddWorkspaceRoot() (string, error) {
	dir, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Add Folder to Workspace",
	})
	if err != nil || dir == "" {
		return "", err
	}
	a.projectMu.Lock()
	if dir == a.projectRoot {
		a.projectMu.Unlock()
		return "", fmt.Errorf("already the workspace's primary folder")
	}
	a.projectMu.Unlock()

	a.treeMu.Lock()
	for _, r := range a.extraRoots {
		if r == dir {
			a.treeMu.Unlock()
			return "", fmt.Errorf("already in the workspace")
		}
	}
	a.extraRoots = append(a.extraRoots, dir)
	a.treeMu.Unlock()

	a.saveExtraRoots()
	a.reloadTree()
	return dir, nil
}

// RemoveWorkspaceRoot detaches an extra folder added via AddWorkspaceRoot.
// No-op if root is the primary project root (use CloseProject for that).
func (a *App) RemoveWorkspaceRoot(root string) {
	a.treeMu.Lock()
	out := a.extraRoots[:0]
	for _, r := range a.extraRoots {
		if r != root {
			out = append(out, r)
		}
	}
	a.extraRoots = out
	delete(a.extraTrees, root)
	a.treeMu.Unlock()

	a.saveExtraRoots()
	a.reloadTree()
}

func (a *App) saveExtraRoots() {
	a.treeMu.Lock()
	extra := append([]string{}, a.extraRoots...)
	a.treeMu.Unlock()
	a.projectMu.Lock()
	cfg := a.projectCfg
	a.projectMu.Unlock()
	if cfg == nil {
		return
	}
	cfg.ExtraRoots = extra
	project.Save(cfg) //nolint
}

// GetWorkspaceRoots returns the primary project root followed by every
// extra folder attached via AddWorkspaceRoot, in add-order.
func (a *App) GetWorkspaceRoots() []string {
	a.projectMu.Lock()
	root := a.projectRoot
	a.projectMu.Unlock()
	a.treeMu.Lock()
	defer a.treeMu.Unlock()
	out := make([]string, 0, len(a.extraRoots)+1)
	if root != "" {
		out = append(out, root)
	}
	return append(out, a.extraRoots...)
}

func (a *App) OpenProject() (string, error) {
	dir, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Open Project",
	})
	if err != nil || dir == "" {
		return "", err
	}
	return dir, a.openProjectDir(dir)
}

// OpenRecentProject opens a recent project. It never replaces the current
// window's project in place — if path is already open in some window (found
// via the live registry in internal/project/windows.go), that window is
// brought to front; otherwise a new window is spawned. This is the single
// entry point for both the File > Open Recent menu (main.go) and the Dock
// menu (dock_darwin.go) — they used to diverge (one replaced in place, the
// other always spawned new), which made the behavior look random depending
// on which menu was used.
func (a *App) OpenRecentProject(path string) error {
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("project path not found: %s", path)
	}
	if pid, ok := project.FindWindowPID(path); ok {
		if activateWindow(pid) {
			return nil
		}
	}
	return launchNewInstance("--project", path)
}

func (a *App) CloseProject() {
	a.lsp.StopAll()
	a.dap.Stop()
	a.projectMu.Lock()
	root := a.projectRoot
	wasRemote := a.remoteDest != ""
	a.projectRoot = ""
	a.projectCfg = nil
	a.remoteDest = ""
	a.projectMu.Unlock()
	if root != "" && !wasRemote {
		project.RemoveFromSession(root) //nolint
		project.UnregisterWindow(root)  //nolint
	}
	a.treeMu.Lock()
	a.extraRoots = nil
	a.extraTrees = make(map[string]*tree.Tree)
	a.treeMu.Unlock()
	runtime.WindowSetTitle(a.ctx, "bish")
	a.reloadTree()
	runtime.EventsEmit(a.ctx, "project:change", "")
	runtime.EventsEmit(a.ctx, "project:remote", false)
	runtime.EventsEmit(a.ctx, "project:commands", nil)
	runtime.EventsEmit(a.ctx, "project:tasks", nil)
}

// GetTasks returns the git-shareable .bish/tasks.json run buttons for the
// open project (nil if none open or no tasks file present).
func (a *App) GetTasks() []*project.Task {
	a.projectMu.Lock()
	root := a.projectRoot
	a.projectMu.Unlock()
	if root == "" {
		return nil
	}
	return project.LoadTasks(root)
}

// RunTask spawns a task directly via the process manager (not the `w`-file
// mechanism RunProjectCommand uses) so it shows up in the Process List with
// its task name and running/exit-code status, without also getting
// auto-saved into the user's local Project Commands (a side effect of the
// w-file path that only makes sense for manually-typed commands).
func (a *App) RunTask(id string) error {
	a.projectMu.Lock()
	root := a.projectRoot
	a.projectMu.Unlock()
	if root == "" {
		return fmt.Errorf("no project open")
	}
	var found *project.Task
	for _, t := range project.LoadTasks(root) {
		if t.ID == id {
			found = t
			break
		}
	}
	if found == nil {
		return fmt.Errorf("task not found (was .bish/tasks.json edited?)")
	}
	a.telemetry.Count("task_run")
	_, err := a.mgr.Add(found.Command, found.Cwd, found.Name)
	return err
}

func (a *App) GetProjectCommands() []*project.Cmd {
	a.projectMu.Lock()
	defer a.projectMu.Unlock()
	if a.projectCfg == nil {
		return nil
	}
	out := make([]*project.Cmd, len(a.projectCfg.Cmds))
	copy(out, a.projectCfg.Cmds)
	return out
}

func (a *App) RunProjectCommand(id string) error {
	a.projectMu.Lock()
	var found *project.Cmd
	if a.projectCfg != nil {
		for _, c := range a.projectCfg.Cmds {
			if c.ID == id {
				found = c
				break
			}
		}
	}
	a.projectMu.Unlock()
	if found == nil {
		return fmt.Errorf("command %s not found", id)
	}
	// write <cwd>\t<cmd>\n to the w-file — same path as the `w` shell function,
	// so the process manager picks it up and shows it in the process list
	f, err := os.OpenFile(a.wFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprintf(f, "%s\t%s\n", found.Directory, found.Command)
	return err
}

func (a *App) DeleteProjectCommand(id string) error {
	a.projectMu.Lock()
	if a.projectCfg == nil {
		a.projectMu.Unlock()
		return nil
	}
	a.projectCfg.Delete(id)
	cfg := a.projectCfg
	a.projectMu.Unlock()
	if err := project.Save(cfg); err != nil {
		return err
	}
	runtime.EventsEmit(a.ctx, "project:commands", cfg.Cmds)
	return nil
}

func (a *App) RenameProjectCommand(id, name string) error {
	a.projectMu.Lock()
	if a.projectCfg == nil {
		a.projectMu.Unlock()
		return nil
	}
	a.projectCfg.Rename(id, name)
	cfg := a.projectCfg
	a.projectMu.Unlock()
	if err := project.Save(cfg); err != nil {
		return err
	}
	runtime.EventsEmit(a.ctx, "project:commands", cfg.Cmds)
	return nil
}

func (a *App) AddProjectCommand(command, cwd, name string) error {
	a.projectMu.Lock()
	if a.projectCfg == nil {
		a.projectMu.Unlock()
		return fmt.Errorf("no project open")
	}
	cmd := a.projectCfg.Add(command, cwd)
	if name != "" {
		cmd.Name = name
	}
	cfg := a.projectCfg
	a.projectMu.Unlock()
	if err := project.Save(cfg); err != nil {
		return err
	}
	runtime.EventsEmit(a.ctx, "project:commands", cfg.Cmds)
	return nil
}

// GetMediaBase returns the localhost media-server URL prefix; append an
// URL-encoded absolute path. Empty when the server failed to start.
func (a *App) GetMediaBase() string {
	return a.MediaBase
}

func (a *App) GetProjectUI() *project.UIState {
	a.projectMu.Lock()
	defer a.projectMu.Unlock()
	if a.projectCfg == nil {
		return nil
	}
	return a.projectCfg.UI
}

func (a *App) SaveProjectUI(ui project.UIState) error {
	a.projectMu.Lock()
	if a.projectCfg == nil {
		a.projectMu.Unlock()
		return nil
	}
	a.projectCfg.UI = &ui
	cfg := a.projectCfg
	a.projectMu.Unlock()
	return project.Save(cfg)
}

func (a *App) GetRecentProjects() []*project.RecentEntry {
	entries, _ := project.LoadRecent()
	return entries
}

// GetRecentFiles returns standalone files (opened via `bish <file>`, not
// part of a project) recently opened, most-recent first.
func (a *App) GetRecentFiles() []*project.RecentFile {
	entries, _ := project.LoadRecentFiles()
	return entries
}

func (a *App) GetProjectRoot() string {
	a.projectMu.Lock()
	defer a.projectMu.Unlock()
	return a.projectRoot
}

func (a *App) SearchInFiles(dir, query string, caseSensitive, wholeWord, useRegex bool, include, exclude string) []SearchResultDTO {
	results := search.Search(dir, query, caseSensitive, wholeWord, useRegex, include, exclude)
	if results == nil {
		return nil
	}
	dtos := make([]SearchResultDTO, len(results))
	for i, r := range results {
		dtos[i] = SearchResultDTO{File: r.File, Line: r.Line, Col: r.Col, Text: r.Text}
	}
	return dtos
}

func (a *App) ReplaceInFiles(dir, query, replacement string, caseSensitive, wholeWord, useRegex bool, include, exclude string) (int, error) {
	return search.Replace(dir, query, replacement, caseSensitive, wholeWord, useRegex, include, exclude)
}

func (a *App) GetAllFiles(root string) []string {
	return search.AllFiles(root)
}

package app

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/csullivan/bish/internal/commands"
	"github.com/csullivan/bish/internal/config"
	"github.com/csullivan/bish/internal/lsp"
	"github.com/csullivan/bish/internal/process"
	"github.com/csullivan/bish/internal/project"
	bishpty "github.com/csullivan/bish/internal/pty"
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

const maxSearchFileSize = 2 * 1024 * 1024 // ponytail: skip huge files in search/replace; raise if it bites

var skipDirs = map[string]bool{
	".git": true, "node_modules": true, "vendor": true,
	"dist": true, "__pycache__": true, ".next": true,
	"target": true, ".cache": true, ".svelte-kit": true,
	".build": true, "build": false, // "build" kept — user might want it
}

type App struct {
	mgr                    *process.Manager
	cmdStore               *commands.Store
	cmdMu                  sync.Mutex
	shell                  *bishpty.PTY
	terminals              map[string]*bishpty.PTY
	terminalsMu            sync.Mutex
	termCount              int
	fileTree               *tree.Tree
	treeMu                 sync.Mutex
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
	lsp                    *lsp.Manager
	cfg                    config.Config
	ctx                    context.Context
	DockMenuUpdater        func()
	QuitInterceptInstaller func()
	StartupProject         string
	MediaBase              string
	NoRestore              bool
	quitRequested          atomic.Bool
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
		fileTree:    tree.New(cwd),
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
	go a.readPTYLoopFor("main", a.shell)
	go a.pollCWDLoop()
	go a.pollWLoop()
	go a.pollGalleryLoop()
	go a.refreshLoop()
	a.startWatcher()
	if a.QuitInterceptInstaller != nil {
		a.QuitInterceptInstaller()
	}
	if a.StartupProject != "" {
		go a.openProjectDir(a.StartupProject) //nolint
	} else if !a.NoRestore {
		go a.restoreSession()
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
	if !a.quitRequested.Load() {
		a.projectMu.Lock()
		root := a.projectRoot
		a.projectMu.Unlock()
		if root != "" {
			project.RemoveFromSession(root) //nolint
		}
	}
	a.lsp.StopAll()
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
	p, err := bishpty.New(a.cfg.Shell, a.cwdFile, a.wFilePath, a.galleryFile)
	if err != nil {
		return "", err
	}
	a.terminalsMu.Lock()
	a.termCount++
	id := fmt.Sprintf("t%d", a.termCount)
	a.terminals[id] = p
	a.terminalsMu.Unlock()
	go a.readPTYLoopFor(id, p)
	// cd to project root or current cwd
	a.projectMu.Lock()
	dir := a.projectRoot
	a.projectMu.Unlock()
	if dir == "" {
		dir = a.cwd
	}
	fmt.Fprintf(p, "cd %q\n", dir) //nolint
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
			runtime.EventsEmit(a.ctx, dataEvent, string(pending))
			pending = pending[:0]
		}
	}

	for {
		select {
		case data, ok := <-ch:
			if !ok {
				flush()
				runtime.EventsEmit(a.ctx, exitEvent)
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
				runtime.EventsEmit(a.ctx, "processes:update", a.mgr.List())
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
			// emit only on change — idle app does zero store writes/re-renders
			cur, err := json.Marshal(a.mgr.List())
			if err != nil || !bytes.Equal(cur, last) {
				last = cur
				runtime.EventsEmit(a.ctx, "processes:update", a.mgr.List())
			}
		}
	}
}

// -- Process methods --

func (a *App) GetProcesses() []*process.Process {
	return a.mgr.List()
}

func (a *App) KillProcess(id string) error {
	a.mgr.Remove(id)
	runtime.EventsEmit(a.ctx, "processes:update", a.mgr.List())
	a.mgr.SaveToDisk() //nolint
	return nil
}

func (a *App) RestartProcess(id string) error {
	return a.mgr.Restart(id)
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

func (a *App) ToggleTreeNode(path string) {
	a.treeMu.Lock()
	for i, n := range a.fileTree.Flat {
		if n.Path == path {
			a.fileTree.Selected = i
			a.fileTree.Toggle()
			break
		}
	}
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
	nodes := a.flatNodes()
	a.treeMu.Unlock()
	runtime.EventsEmit(a.ctx, "tree:update", nodes)
	go a.saveExpandedPaths(nil)
}

func (a *App) CdToPath(path string) error {
	_, err := a.shell.Write([]byte(fmt.Sprintf("cd %q\n", path)))
	return err
}

// -- Filesystem operations --

func (a *App) FSNewFile(dirPath, name string) error {
	if name == "" {
		name = "newfile"
	}
	f, err := os.Create(filepath.Join(dirPath, name))
	if err != nil {
		return err
	}
	f.Close()
	a.reloadTree()
	return nil
}

func (a *App) FSNewFolder(dirPath, name string) error {
	if name == "" {
		name = "newfolder"
	}
	if err := os.MkdirAll(filepath.Join(dirPath, name), 0o755); err != nil {
		return err
	}
	a.reloadTree()
	return nil
}

func (a *App) FSRename(oldPath, newPath string) error {
	if err := os.Rename(oldPath, newPath); err != nil {
		return err
	}
	a.reloadTree()
	return nil
}

func (a *App) FSDelete(path string) error {
	if err := os.RemoveAll(path); err != nil {
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
		if err := os.RemoveAll(p); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	a.reloadTree()
	return firstErr
}

func (a *App) FSCopyPath(path string) string {
	return path
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
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (a *App) WriteFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o644)
}

// -- PTY methods --

// LSPStart lazily spawns the language server for lang rooted at root.
// Returns false when no server is installed (frontend falls back to the
// heuristic autoimport) or the lang is in crash backoff.
func (a *App) LSPStart(lang, root string) bool {
	return a.lsp.Start(lang, root)
}

// LSPSend forwards one JSON-RPC message (headerless) to the lang's server.
func (a *App) LSPSend(lang, msg string) error {
	return a.lsp.Send(lang, msg)
}

func (a *App) LSPStop(lang string) {
	a.lsp.Stop(lang)
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
	return config.Save(cfg)
}

// -- helpers --

func (a *App) flatNodes() []TreeNodeDTO {
	result := make([]TreeNodeDTO, len(a.fileTree.Flat))
	for i, n := range a.fileTree.Flat {
		result[i] = TreeNodeDTO{
			Name:     n.Name,
			Path:     n.Path,
			IsDir:    n.IsDir,
			Depth:    n.Depth,
			Expanded: n.Expanded,
			Selected: i == a.fileTree.Selected,
		}
	}
	return result
}

func (a *App) reloadTree() {
	a.projectMu.Lock()
	root := a.projectRoot
	a.projectMu.Unlock()
	if root == "" {
		root = a.cwd
	}
	a.treeMu.Lock()
	expanded := a.fileTree.ExpandedPaths()
	a.fileTree.Load(root)
	a.fileTree.RestoreExpanded(expanded)
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

func (a *App) OpenRecentInNewWindow(path string) error {
	return launchNewInstance("--project", path)
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
	cfg, err := project.Load(dir)
	if err != nil {
		cfg = &project.Config{CWD: dir}
	}
	a.projectMu.Lock()
	a.projectRoot = dir
	a.projectCfg = cfg
	a.projectMu.Unlock()
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
	runtime.EventsEmit(a.ctx, "project:commands", cfg.Cmds)
	project.AddRecent(dir)    //nolint
	project.AddToSession(dir) //nolint
	if a.DockMenuUpdater != nil {
		go a.DockMenuUpdater()
	}
	return nil
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

func (a *App) OpenRecentProject(path string) error {
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("project path not found: %s", path)
	}
	return a.openProjectDir(path)
}

func (a *App) CloseProject() {
	a.lsp.StopAll()
	a.projectMu.Lock()
	root := a.projectRoot
	a.projectRoot = ""
	a.projectCfg = nil
	a.projectMu.Unlock()
	if root != "" {
		project.RemoveFromSession(root) //nolint
	}
	runtime.WindowSetTitle(a.ctx, "bish")
	a.reloadTree()
	runtime.EventsEmit(a.ctx, "project:change", "")
	runtime.EventsEmit(a.ctx, "project:commands", nil)
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

func (a *App) GetProjectRoot() string {
	a.projectMu.Lock()
	defer a.projectMu.Unlock()
	return a.projectRoot
}

func buildMatcher(query string, caseSensitive, wholeWord, useRegex bool) (*regexp.Regexp, string, error) {
	if useRegex || wholeWord {
		pattern := query
		if !useRegex {
			pattern = regexp.QuoteMeta(query)
		}
		if wholeWord {
			pattern = `\b` + pattern + `\b`
		}
		if !caseSensitive {
			pattern = "(?i)" + pattern
		}
		re, err := regexp.Compile(pattern)
		return re, "", err
	}
	plain := query
	if !caseSensitive {
		plain = strings.ToLower(query)
	}
	return nil, plain, nil
}

func (a *App) SearchInFiles(dir, query string, caseSensitive, wholeWord, useRegex bool) []SearchResultDTO {
	if query == "" {
		return nil
	}
	re, plain, err := buildMatcher(query, caseSensitive, wholeWord, useRegex)
	if err != nil {
		return nil
	}
	var results []SearchResultDTO
	var walk func(d string, depth int)
	walk = func(d string, depth int) {
		if depth > 10 || len(results) >= 500 {
			return
		}
		entries, err := os.ReadDir(d)
		if err != nil {
			return
		}
		for _, e := range entries {
			name := e.Name()
			if strings.HasPrefix(name, ".") {
				continue
			}
			fullPath := filepath.Join(d, name)
			if e.IsDir() {
				if skipDirs[name] {
					continue
				}
				walk(fullPath, depth+1)
			} else {
				if info, err := e.Info(); err != nil || info.Size() > maxSearchFileSize {
					continue
				}
				f, err := os.Open(fullPath)
				if err != nil {
					continue
				}
				scanner := bufio.NewScanner(f)
				// default 64KB line cap silently aborts files with long
				// (minified) lines — raise it so matches after them aren't lost
				scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
				lineNum := 0
				for scanner.Scan() {
					lineNum++
					raw := scanner.Text()
					if strings.ContainsRune(raw, 0) {
						break
					}
					var col int
					if re != nil {
						loc := re.FindStringIndex(raw)
						if loc == nil {
							continue
						}
						col = loc[0]
					} else {
						haystack := raw
						if !caseSensitive {
							haystack = strings.ToLower(raw)
						}
						col = strings.Index(haystack, plain)
						if col < 0 {
							continue
						}
					}
					results = append(results, SearchResultDTO{File: fullPath, Line: lineNum, Col: col, Text: raw})
					if len(results) >= 500 {
						f.Close()
						return
					}
				}
				f.Close()
			}
		}
	}
	walk(dir, 0)
	return results
}

func (a *App) ReplaceInFiles(dir, query, replacement string, caseSensitive, wholeWord, useRegex bool) (int, error) {
	if query == "" {
		return 0, nil
	}
	re, plain, err := buildMatcher(query, caseSensitive, wholeWord, useRegex)
	if err != nil {
		return 0, err
	}
	changed := 0
	var walk func(d string, depth int) error
	walk = func(d string, depth int) error {
		if depth > 10 {
			return nil
		}
		entries, err := os.ReadDir(d)
		if err != nil {
			return nil
		}
		for _, e := range entries {
			name := e.Name()
			if strings.HasPrefix(name, ".") {
				continue
			}
			fullPath := filepath.Join(d, name)
			if e.IsDir() {
				if skipDirs[name] {
					continue
				}
				walk(fullPath, depth+1) //nolint
			} else {
				if info, err := e.Info(); err != nil || info.Size() > maxSearchFileSize {
					continue
				}
				content, err := os.ReadFile(fullPath)
				if err != nil {
					continue
				}
				if strings.ContainsRune(string(content), 0) {
					continue
				}
				var newContent string
				if re != nil {
					newContent = re.ReplaceAllString(string(content), replacement)
				} else if caseSensitive {
					newContent = strings.ReplaceAll(string(content), query, replacement)
				} else {
					s := string(content)
					lower := strings.ToLower(s)
					lq := plain
					var b strings.Builder
					for {
						idx := strings.Index(lower, lq)
						if idx < 0 {
							b.WriteString(s)
							break
						}
						b.WriteString(s[:idx])
						b.WriteString(replacement)
						s = s[idx+len(query):]
						lower = lower[idx+len(query):]
					}
					newContent = b.String()
				}
				if newContent != string(content) {
					if err := os.WriteFile(fullPath, []byte(newContent), 0o644); err != nil {
						return fmt.Errorf("write %s: %w", fullPath, err)
					}
					changed++
				}
			}
		}
		return nil
	}
	err = walk(dir, 0)
	return changed, err
}

func (a *App) GetAllFiles(root string) []string {
	var files []string
	var walk func(dir string, depth int)
	walk = func(dir string, depth int) {
		if depth > 10 {
			return
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, e := range entries {
			name := e.Name()
			if strings.HasPrefix(name, ".") {
				continue
			}
			fullPath := filepath.Join(dir, name)
			if e.IsDir() {
				if skipDirs[name] {
					continue
				}
				walk(fullPath, depth+1)
			} else {
				files = append(files, fullPath)
			}
		}
	}
	walk(root, 0)
	return files
}

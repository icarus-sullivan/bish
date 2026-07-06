package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/menu"
	"github.com/wailsapp/wails/v2/pkg/menu/keys"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"

	"github.com/csullivan/bish/internal/app"
	"github.com/csullivan/bish/internal/project"
	"github.com/csullivan/bish/internal/commands"
	"github.com/csullivan/bish/internal/config"
	"github.com/csullivan/bish/internal/process"
	bishpty "github.com/csullivan/bish/internal/pty"
)

func main() {
	var themeName string
	var shellPath string
	var install bool

	root := &cobra.Command{
		Use:   "bish",
		Short: "Interactive shell dashboard",
		RunE: func(cmd *cobra.Command, args []string) error {
			if install {
				return runInstall()
			}
			return run(themeName, shellPath)
		},
	}

	root.Flags().StringVar(&themeName, "theme", "", "theme (default, light, catppuccin, gruvbox, nord, tokyo-night)")
	root.Flags().StringVar(&shellPath, "shell", "", "shell to use (default: $SHELL)")
	root.Flags().BoolVar(&install, "install", false, "print shell setup to stdout")

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func runInstall() error {
	fmt.Print(`# bish shell integration — add to ~/.zshrc or ~/.bashrc:
#   eval "$(bish --install)"

# Sync CWD with bish (BISH_CWD_FILE is set by bish at launch).
if [[ -n "$BISH_CWD_FILE" ]]; then
  PROMPT_COMMAND='echo "$PWD" > "$BISH_CWD_FILE"'
  precmd() { echo "$PWD" > "$BISH_CWD_FILE"; }
fi

# w: launch a managed process tracked by bish.
# BISH_W_FILE is injected by bish into the shell environment.
w() {
  if [[ $# -eq 0 ]]; then
    echo "usage: w <command> [args...]"
    return 1
  fi
  if [[ -z "$BISH_W_FILE" ]]; then
    echo "w: not inside a bish session"
    return 1
  fi
  printf '%s\t%s\n' "$PWD" "$*" >> "$BISH_W_FILE"
  echo "launched: $*"
}
`)
	return nil
}

func buildMenu(a *app.App) *menu.Menu {
	appMenu := menu.NewMenu()
	appMenu.Append(menu.AppMenu())

	fileMenu := appMenu.AddSubmenu("File")
	fileMenu.AddText("New File", keys.CmdOrCtrl("n"), func(_ *menu.CallbackData) {
		a.TriggerNewFile()
	})
	fileMenu.AddText("New Window", &keys.Accelerator{Key: "n", Modifiers: []keys.Modifier{keys.CmdOrCtrlKey, keys.ShiftKey}}, func(_ *menu.CallbackData) {
		a.NewWindow() //nolint
	})
	fileMenu.AddSeparator()
	fileMenu.AddText("Open Project…", keys.CmdOrCtrl("o"), func(_ *menu.CallbackData) {
		a.OpenProject() //nolint
	})
	recentMenu := fileMenu.AddSubmenu("Open Recent")
	recents, _ := project.LoadRecent()
	if len(recents) == 0 {
		recentMenu.AddText("No Recent Projects", nil, nil)
	} else {
		for _, entry := range recents {
			e := entry
			recentMenu.AddText(e.Name, nil, func(_ *menu.CallbackData) {
				a.OpenRecentProject(e.Path) //nolint
			})
		}
	}
	fileMenu.AddText("Close Project", nil, func(_ *menu.CallbackData) {
		a.CloseProject()
	})
	fileMenu.AddSeparator()
	fileMenu.AddText("Go to File…", keys.CmdOrCtrl("p"), func(_ *menu.CallbackData) {
		a.TriggerPalette()
	})

	appMenu.Append(menu.EditMenu())

	return appMenu
}

func run(themeName, shellPath string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	if themeName != "" {
		cfg.Theme = themeName
	}
	if shellPath != "" {
		cfg.Shell = shellPath
	}

	mgr := process.New()
	mgr.LoadFromDisk() //nolint

	store, err := commands.Load()
	if err != nil {
		return fmt.Errorf("commands: %w", err)
	}

	pid := os.Getpid()
	cwdFile := fmt.Sprintf("/tmp/bish_cwd_%d", pid)
	wFilePath := fmt.Sprintf("/tmp/bish_w_%d", pid)
	galleryFile := fmt.Sprintf("/tmp/bish_gallery_%d", pid)

	os.WriteFile(wFilePath, nil, 0o600)   //nolint
	os.WriteFile(galleryFile, nil, 0o600) //nolint

	shell, err := bishpty.New(cfg.Shell, cwdFile, wFilePath, galleryFile)
	if err != nil {
		return fmt.Errorf("pty: %w", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		cwd = os.Getenv("HOME")
	}

	a := app.New(cfg, mgr, store, shell, cwd, cwdFile, wFilePath, galleryFile)

	return wails.Run(&options.App{
		Menu: buildMenu(a),
		Title:  "bish",
		Width:  1400,
		Height: 900,
		AssetServer: &assetserver.Options{
			Assets:  assets,
			Handler: &localFileHandler{},
		},
		BackgroundColour: &options.RGBA{R: 27, G: 28, B: 38, A: 255},
		OnStartup:        a.Startup,
		OnShutdown:       a.Shutdown,
		Bind:             []any{a},
		DragAndDrop: &options.DragAndDrop{
			EnableFileDrop:  true,
			CSSDropProperty: "--wails-drop-target",
			CSSDropValue:    "drop",
		},
		Mac: &mac.Options{
			TitleBar:             mac.TitleBarHiddenInset(),
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
		},
	})
}

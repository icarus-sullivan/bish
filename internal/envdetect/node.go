package envdetect

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

type packageJSON struct {
	PackageManager string            `json:"packageManager"`
	Scripts        map[string]string `json:"scripts"`
	Engines        map[string]string `json:"engines"`
}

func readPackageJSON(dir string) *packageJSON {
	b, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		return nil
	}
	var pj packageJSON
	if json.Unmarshal(b, &pj) != nil {
		return nil
	}
	return &pj
}

func exists(dir string, rel ...string) bool {
	_, err := os.Stat(filepath.Join(append([]string{dir}, rel...)...))
	return err == nil
}

func readFile(dir string, rel ...string) string {
	b, err := os.ReadFile(filepath.Join(append([]string{dir}, rel...)...))
	if err != nil {
		return ""
	}
	return string(b)
}

// Toolchain summarizes the repo's declared toolchain versions, for display
// and as a step-cache key component — never executed.
func Toolchain(dir string) string {
	var parts []string
	pj := readPackageJSON(dir)
	if pj != nil && pj.PackageManager != "" {
		parts = append(parts, strings.SplitN(pj.PackageManager, "+", 2)[0])
	}
	node := strings.TrimSpace(readFile(dir, ".nvmrc"))
	if node == "" {
		node = strings.TrimSpace(readFile(dir, ".node-version"))
	}
	if node != "" {
		parts = append(parts, "node@"+strings.TrimPrefix(node, "v"))
	}
	for _, line := range strings.Split(readFile(dir, ".tool-versions"), "\n") {
		f := strings.Fields(line)
		if len(f) >= 2 && !strings.HasPrefix(f[0], "#") {
			if f[0] == "nodejs" && node != "" {
				continue
			}
			parts = append(parts, f[0]+"@"+f[1])
		}
	}
	if gv := goVersion(dir); gv != "" {
		parts = append(parts, "go@"+gv)
	}
	return strings.Join(parts, " ")
}

// packageManager picks the runner from packageManager or the lockfile.
func packageManager(dir string, pj *packageJSON) string {
	if pj != nil && pj.PackageManager != "" {
		return strings.SplitN(pj.PackageManager, "@", 2)[0]
	}
	switch {
	case exists(dir, "pnpm-lock.yaml"):
		return "pnpm"
	case exists(dir, "yarn.lock"):
		return "yarn"
	case exists(dir, "bun.lockb"), exists(dir, "bun.lock"):
		return "bun"
	}
	return "npm"
}

func runScript(pm, script string) string {
	switch pm {
	case "npm":
		if script == "start" {
			return "npm start"
		}
		return "npm run " + script
	case "bun":
		return "bun run " + script
	}
	return pm + " " + script
}

// extraArgs is how pm forwards arguments to a script.
func extraArgs(pm, args string) string {
	if pm == "npm" {
		return "-- " + args
	}
	return args
}

var (
	portFlag   = regexp.MustCompile(`--port[= ]([0-9]{2,5})\b`)
	portInline = regexp.MustCompile(`\bPORT=([0-9]{2,5})\b`)
	portConfig = regexp.MustCompile(`\bport\s*[:=]\s*([0-9]{2,5})\b`)
	portEnvVar = regexp.MustCompile(`(?m)^\s*PORT\s*=\s*"?([0-9]{2,5})"?\s*$`)
	// dev servers whose CLI takes --port and overrides the config file
	portCLI = regexp.MustCompile(`^\s*(npx\s+)?(vite|next\s+dev|next\s+start|astro\s+dev|nuxt\s+dev|nuxi\s+dev|webpack\s+serve|storybook\s+dev|start-storybook|remix\s+vite:dev)\b`)
)

func atoiPort(s string) int {
	n, _ := strconv.Atoi(s)
	if n <= 0 || n > 65535 {
		return 0
	}
	return n
}

// configPort reads a literal port from a framework config file in dir, in
// the documented order: vite, next, then .env files.
func configPort(dir string) (port int, src string) {
	for _, pat := range []string{"vite.config.*", "next.config.*", "astro.config.*", "nuxt.config.*"} {
		matches, _ := filepath.Glob(filepath.Join(dir, pat))
		for _, f := range matches {
			b, err := os.ReadFile(f)
			if err != nil {
				continue
			}
			if m := portConfig.FindStringSubmatch(string(b)); m != nil {
				if p := atoiPort(m[1]); p > 0 {
					return p, filepath.Base(f)
				}
			}
		}
	}
	for _, f := range []string{".env", ".env.local", ".env.development", ".env.example"} {
		if m := portEnvVar.FindStringSubmatch(readFile(dir, f)); m != nil {
			if p := atoiPort(m[1]); p > 0 {
				return p, f
			}
		}
	}
	return 0, ""
}

func scriptPort(script string) int {
	if m := portFlag.FindStringSubmatch(script); m != nil {
		return atoiPort(m[1])
	}
	if m := portInline.FindStringSubmatch(script); m != nil {
		return atoiPort(m[1])
	}
	return 0
}

func detectNode(dir string, p *Proposal) {
	pj := readPackageJSON(dir)
	if pj == nil {
		return
	}
	pm := packageManager(dir, pj)
	lock := map[string]string{"pnpm": "pnpm-lock.yaml", "yarn": "yarn.lock", "bun": "bun.lockb", "npm": "package-lock.json"}[pm]
	conf := 60
	if lock != "" && exists(dir, lock) {
		conf = 90
	}
	p.step(&Candidate{Name: "install", Cmd: pm + " install", Kind: "install", Source: "package.json (" + pm + ")", Confidence: conf})

	for _, name := range sortedKeys(pj.Scripts) {
		body := pj.Scripts[name]
		kind := classify(name)
		if kind == "" {
			continue
		}
		src := "package.json#scripts." + name
		c := &Candidate{Name: name, Cmd: runScript(pm, name), Kind: kind, Source: src}
		if kind != "service" {
			c.Confidence = map[string]int{"migrate": 75, "codegen": 70, "reset": 70, "install": 50, "build": 30}[kind]
			p.step(c)
			continue
		}
		switch {
		case name == "dev" || name == "start":
			c.Confidence = 80
		case strings.HasPrefix(name, "storybook"):
			c.Confidence = 75
		default:
			c.Confidence = 55
		}
		if c.Port = scriptPort(body); c.Port == 0 {
			if port, f := configPort(dir); port > 0 {
				c.Port, c.Source = port, src+" (port from "+f+")"
			}
		}
		if strings.HasPrefix(name, "storybook") && c.Port == 0 {
			if m := regexp.MustCompile(`-p\s+([0-9]{2,5})\b`).FindStringSubmatch(body); m != nil {
				c.Port = atoiPort(m[1])
			}
		}
		if portCLI.MatchString(body) && !portFlag.MatchString(body) {
			c.PortArgs = extraArgs(pm, "--port {{port}}")
		} else if strings.Contains(body, "PORT") || strings.Contains(readFile(dir, ".env.example"), "PORT=") {
			c.PortEnv = "PORT"
		}
		p.service(c)
	}
}

type nxProject struct {
	Name    string `json:"name"`
	Targets map[string]struct {
		Executor string                 `json:"executor"`
		Options  map[string]interface{} `json:"options"`
	} `json:"targets"`
}

// detectNx adds one service per project serve/dev target — this is what
// makes a monorepo's apps appear in one click.
func detectNx(dir string, p *Proposal) {
	if !exists(dir, "nx.json") {
		return
	}
	pm := packageManager(dir, readPackageJSON(dir))
	run := pm + " nx"
	if pm == "npm" {
		run = "npx nx"
	}
	inferred := nxInferred(readFile(dir, "nx.json"))
	var files []string
	for _, pat := range []string{"apps/*/project.json", "packages/*/project.json", "apps/*/*/project.json"} {
		m, _ := filepath.Glob(filepath.Join(dir, pat))
		files = append(files, m...)
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var proj nxProject
		if json.Unmarshal(b, &proj) != nil {
			continue
		}
		pdir := filepath.Dir(f)
		rel, _ := filepath.Rel(dir, f)
		if proj.Name == "" {
			proj.Name = filepath.Base(pdir)
		}
		targets := map[string]bool{}
		for t := range proj.Targets {
			targets[t] = true
		}
		// Nx plugins infer targets from config files without project.json
		// listing them (@nx/vite/plugin → serve from vite.config.*, ...)
		for _, inf := range inferred {
			if matches, _ := filepath.Glob(filepath.Join(pdir, inf.config)); len(matches) > 0 {
				targets[inf.target] = true
			}
		}
		served := false
		for _, target := range []string{"serve", "dev", "storybook"} {
			if !targets[target] || (target == "dev" && served) {
				continue
			}
			served = served || target != "storybook"
			name := proj.Name
			if target == "storybook" {
				name += "-storybook"
			}
			c := &Candidate{Name: name, Cmd: run + " " + target + " " + proj.Name, Source: rel + "#targets." + target, Confidence: 80}
			if v, ok := proj.Targets[target].Options["port"].(float64); ok {
				c.Port = int(v)
			} else if target == "storybook" {
				c.Port = 0
			} else if port, src := configPort(pdir); port > 0 {
				c.Port = port
				c.Source += " (port from " + src + ")"
			}
			c.PortArgs = "--port={{port}}"
			p.service(c)
		}
	}
}

type nxInfer struct{ target, config string }

// nxInferred lists the targets nx.json's plugins infer, with the config
// file that triggers each (target names honour the plugin's options).
func nxInferred(nxJSON string) []nxInfer {
	var cfg struct {
		Plugins []json.RawMessage `json:"plugins"`
	}
	json.Unmarshal([]byte(nxJSON), &cfg) //nolint
	var out []nxInfer
	for _, raw := range cfg.Plugins {
		var name string
		var obj struct {
			Plugin  string                 `json:"plugin"`
			Options map[string]interface{} `json:"options"`
		}
		if json.Unmarshal(raw, &name) != nil {
			if json.Unmarshal(raw, &obj) != nil {
				continue
			}
			name = obj.Plugin
		}
		opt := func(k, def string) string {
			if v, ok := obj.Options[k].(string); ok && v != "" {
				return v
			}
			return def
		}
		switch name {
		case "@nx/vite/plugin":
			out = append(out, nxInfer{opt("serveTargetName", "serve"), "vite.config.*"})
		case "@nx/next/plugin":
			out = append(out, nxInfer{opt("devTargetName", "dev"), "next.config.*"})
		case "@nx/storybook/plugin":
			out = append(out, nxInfer{opt("serveStorybookTargetName", "storybook"), ".storybook/main.*"})
		}
	}
	return out
}

package envdetect

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// detectCompose turns a compose file into one infra step (shared infra
// starts once per project, not once per env — deliberately not a service
// per container) plus a note per published port.
func detectCompose(dir string, p *Proposal) {
	var file string
	for _, d := range []string{"", "docker", "infra", "deploy"} {
		for _, n := range []string{"docker-compose.yml", "docker-compose.yaml", "compose.yml", "compose.yaml", "docker-compose.dev.yml", "docker-compose.local.yml"} {
			rel := filepath.Join(d, n)
			if exists(dir, rel) {
				file = rel
				break
			}
		}
		if file != "" {
			break
		}
	}
	if file == "" {
		return
	}
	p.Compose = file
	p.step(&Candidate{
		Name: "infra", Cmd: "docker compose -f " + file + " up -d", Kind: "infra",
		Source: file, Confidence: 70,
	})
	for _, s := range composeServices(readFile(dir, file)) {
		if len(s.ports) == 0 {
			p.note(file + "#" + s.name + ": container, no published ports")
			continue
		}
		var ps []string
		for _, port := range s.ports {
			ps = append(ps, ":"+strconv.Itoa(port))
		}
		p.note(file + "#" + s.name + ": container on " + strings.Join(ps, ", "))
	}
}

type composeSvc struct {
	name  string
	ports []int
}

var (
	composeShortPort = regexp.MustCompile(`^-\s*["']?(?:[0-9.]+:)?(?:\$\{[A-Z_]+:-)?([0-9]{2,5})\}?(?:-[0-9]+)?:[0-9]+`)
	composePublished = regexp.MustCompile(`^-?\s*published:\s*["']?([0-9]{2,5})`)
)

// composeServices is a deliberately small indentation scan of a compose
// file's services → published host ports (no YAML dependency for this).
func composeServices(src string) []composeSvc {
	var out []composeSvc
	inServices, inPorts := false, false
	svcIndent, portsIndent := -1, -1
	sc := bufio.NewScanner(strings.NewReader(src))
	for sc.Scan() {
		raw := sc.Text()
		trim := strings.TrimSpace(raw)
		if trim == "" || strings.HasPrefix(trim, "#") {
			continue
		}
		indent := len(raw) - len(strings.TrimLeft(raw, " "))
		if indent == 0 {
			inServices = trim == "services:"
			inPorts = false
			svcIndent = -1
			continue
		}
		if !inServices {
			continue
		}
		if svcIndent == -1 || indent == svcIndent {
			if strings.HasSuffix(trim, ":") && !strings.HasPrefix(trim, "-") {
				svcIndent = indent
				out = append(out, composeSvc{name: strings.TrimSuffix(trim, ":")})
				inPorts = false
				continue
			}
		}
		if len(out) == 0 {
			continue
		}
		if inPorts && indent <= portsIndent {
			inPorts = false
		}
		if trim == "ports:" {
			inPorts, portsIndent = true, indent
			continue
		}
		if !inPorts {
			continue
		}
		cur := &out[len(out)-1]
		if m := composeShortPort.FindStringSubmatch(trim); m != nil {
			cur.ports = append(cur.ports, atoiPort(m[1]))
		} else if m := composePublished.FindStringSubmatch(trim); m != nil {
			cur.ports = append(cur.ports, atoiPort(m[1]))
		}
	}
	return out
}

var makeTarget = regexp.MustCompile(`^([a-z][a-z0-9_-]*):([^=]|$)`)

func detectMakefile(dir string, p *Proposal) {
	src := readFile(dir, "Makefile")
	if src == "" {
		return
	}
	seen := map[string]bool{}
	for _, line := range strings.Split(src, "\n") {
		m := makeTarget.FindStringSubmatch(line)
		if m == nil || seen[m[1]] {
			continue
		}
		name := m[1]
		seen[name] = true
		c := &Candidate{Name: "make-" + name, Cmd: "make " + name, Source: "Makefile#" + name, Confidence: 40}
		switch {
		case name == "dev" || name == "run" || name == "serve" || name == "start":
			p.service(c)
			continue
		case name == "install" || name == "deps" || name == "setup":
			c.Kind = "install"
		default:
			c.Kind = classify(name)
		}
		if c.Kind != "" && c.Kind != "service" {
			p.step(c)
		}
	}
}

func detectProcfile(dir string, p *Proposal) {
	for _, f := range []string{"Procfile.dev", "Procfile"} {
		src := readFile(dir, f)
		if src == "" {
			continue
		}
		for _, line := range strings.Split(src, "\n") {
			name, cmd, ok := strings.Cut(line, ":")
			name, cmd = strings.TrimSpace(name), strings.TrimSpace(cmd)
			if !ok || name == "" || cmd == "" || strings.HasPrefix(name, "#") || strings.Contains(name, " ") {
				continue
			}
			c := &Candidate{Name: name, Cmd: cmd, Source: f + "#" + name, Confidence: 60}
			if name == "web" {
				c.Confidence = 75
			}
			c.Port = scriptPort(cmd)
			if strings.Contains(cmd, "$PORT") || strings.Contains(cmd, "${PORT") {
				c.PortEnv = "PORT"
			}
			p.service(c)
		}
		return // Procfile.dev wins over Procfile
	}
}

func detectPython(dir string, p *Proposal) {
	var install string
	switch {
	case exists(dir, "uv.lock"):
		install = "uv sync"
	case exists(dir, "poetry.lock"):
		install = "poetry install"
	case exists(dir, "Pipfile.lock"):
		install = "pipenv install"
	case exists(dir, "requirements.txt"):
		install = "pip install -r requirements.txt"
	case exists(dir, "pyproject.toml"):
		install = "pip install -e ."
	}
	if install != "" {
		p.step(&Candidate{Name: "py-install", Cmd: install, Kind: "install", Source: "python (" + strings.Fields(install)[0] + ")", Confidence: 75})
	}
	if exists(dir, "manage.py") {
		run := "python manage.py"
		if strings.HasPrefix(install, "uv") {
			run = "uv run python manage.py"
		} else if strings.HasPrefix(install, "poetry") {
			run = "poetry run python manage.py"
		}
		// Django's runserver default is :8000; the port is a positional arg
		p.service(&Candidate{Name: "runserver", Cmd: run + " runserver", Port: 8000, PortArgs: "{{port}}", Source: "manage.py (Django default :8000)", Confidence: 80})
		p.step(&Candidate{Name: "migrate", Cmd: run + " migrate", Kind: "migrate", Source: "manage.py", Confidence: 75})
	}
}

func goVersion(dir string) string {
	for _, line := range strings.Split(readFile(dir, "go.mod"), "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(line), "go "); ok {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func detectGo(dir string, p *Proposal) {
	if !exists(dir, "go.mod") {
		return
	}
	found := false
	entries, _ := os.ReadDir(filepath.Join(dir, "cmd"))
	for _, e := range entries {
		if !e.IsDir() || !hasGoMain(filepath.Join(dir, "cmd", e.Name())) {
			continue
		}
		found = true
		p.service(&Candidate{Name: e.Name(), Cmd: "go run ./cmd/" + e.Name(), Source: "go.mod → cmd/" + e.Name(), Confidence: 60})
	}
	if !found && hasGoMain(dir) {
		p.service(&Candidate{Name: filepath.Base(dir), Cmd: "go run .", Source: "go.mod → main package", Confidence: 70})
	}
}

func hasGoMain(dir string) bool {
	files, _ := filepath.Glob(filepath.Join(dir, "*.go"))
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		if err == nil && regexp.MustCompile(`(?m)^package main\b`).Match(b) && strings.Contains(string(b), "func main()") {
			return true
		}
	}
	return false
}

func detectCargo(dir string, p *Proposal) {
	if !exists(dir, "Cargo.toml") {
		return
	}
	p.service(&Candidate{Name: "cargo-run", Cmd: "cargo run", Source: "Cargo.toml", Confidence: 60})
	p.step(&Candidate{Name: "cargo-build", Cmd: "cargo build", Kind: "build", Source: "Cargo.toml", Confidence: 50})
}

func detectRails(dir string, p *Proposal) {
	if !exists(dir, "Gemfile") || !exists(dir, "config", "application.rb") {
		return
	}
	p.step(&Candidate{Name: "bundle", Cmd: "bundle install", Kind: "install", Source: "Gemfile", Confidence: 80})
	// Rails' server default is :3000; -p overrides it
	p.service(&Candidate{Name: "rails", Cmd: "bin/rails s", Port: 3000, PortArgs: "-p {{port}}", Source: "config/application.rb (Rails default :3000)", Confidence: 80})
	p.step(&Candidate{Name: "db:migrate", Cmd: "bin/rails db:migrate", Kind: "migrate", Source: "config/application.rb", Confidence: 75})
	p.step(&Candidate{Name: "db:reset", Cmd: "bin/rails db:reset", Kind: "reset", Source: "config/application.rb", Confidence: 50})
}

var envPortKey = regexp.MustCompile(`(?im)^\s*(?:export\s+)?([A-Z0-9_]*PORT)\s*=\s*"?([0-9]*)"?`)

// applyPortEnvHints reads PORT-ish keys from .env.example / .env.* and
// attaches them to the service they name ("API_PORT" → a service with
// "api" in its name; bare "PORT" → a lone port-less node service).
func applyPortEnvHints(dir string, p *Proposal) {
	files, _ := filepath.Glob(filepath.Join(dir, ".env*"))
	for _, f := range files {
		base := filepath.Base(f)
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		for _, m := range envPortKey.FindAllStringSubmatch(string(b), -1) {
			k, v := m[1], m[2]
			for _, s := range p.Services {
				if s.PortEnv != "" || s.PortArgs != "" {
					continue
				}
				prefix := strings.TrimSuffix(strings.TrimSuffix(k, "PORT"), "_")
				match := (prefix == "" && len(p.Services) == 1) ||
					(prefix != "" && strings.Contains(strings.ToUpper(s.Name), prefix))
				if !match {
					continue
				}
				s.PortEnv = k
				if s.Port == 0 {
					if port := atoiPort(v); port > 0 {
						s.Port = port
						s.Source += " (port from " + base + ")"
					}
				}
			}
		}
	}
}

package envdetect

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func write(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for rel, body := range files {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func byName(list []*Candidate, name string) *Candidate {
	for _, c := range list {
		if c.Name == name {
			return c
		}
	}
	return nil
}

// An Nx monorepo of Vite apps with inferred serve targets and literal
// server.port values, plus Storybook.
func TestDetectNxViteApps(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"package.json":         `{"packageManager":"pnpm@10.33.0","scripts":{"storybook":"nx storybook ui","lint":"nx run-many -t lint"}}`,
		"pnpm-lock.yaml":       "",
		".nvmrc":               "v24.18.0\n",
		"nx.json":              `{"plugins":[{"plugin":"@nx/vite/plugin","options":{"buildTargetName":"build","serveTargetName":"serve"}},"@nx/eslint/plugin"]}`,
		"libs/ui/project.json": `{"name":"ui","targets":{"storybook":{"options":{"port":6006}}}}`,
	}
	apps := map[string]int{"harmony": 3000, "arrow": 3001, "ashoka": 3002, "partner": 3003, "employer": 3004}
	for app, port := range apps {
		files["apps/"+app+"/project.json"] = `{"name":"` + app + `"}`
		files["apps/"+app+"/vite.config.ts"] = "export default defineConfig({\n  server: {\n    port: " + strconv.Itoa(port) + ",\n    host: 'localhost',\n  },\n})\n"
	}
	write(t, dir, files)

	p := Detect("nuna", dir)
	if p.Toolchain != "pnpm@10.33.0 node@24.18.0" {
		t.Errorf("toolchain = %q", p.Toolchain)
	}
	for app, port := range apps {
		c := byName(p.Services, app)
		if c == nil {
			t.Fatalf("missing service %s; got %+v", app, p.Services)
		}
		if c.Port != port || c.Cmd != "pnpm nx serve "+app || !c.Accept || c.PortArgs == "" {
			t.Errorf("%s = %+v", app, c)
		}
		if !strings.Contains(c.Source, "vite.config.ts") {
			t.Errorf("%s source %q lacks provenance", app, c.Source)
		}
	}
	if sb := byName(p.Services, "storybook"); sb == nil {
		t.Errorf("storybook script not proposed")
	}
	if inst := byName(p.Steps, "install"); inst == nil || inst.Cmd != "pnpm install" || !inst.Accept {
		t.Errorf("install = %+v", inst)
	}
	if byName(p.Steps, "lint") != nil || byName(p.Services, "lint") != nil {
		t.Errorf("lint should be ignored")
	}
}

// A pnpm API with migrate/codegen/reset scripts and a compose file of
// shared infra.
func TestDetectNodeAPIWithCompose(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, map[string]string{
		"package.json": `{"packageManager":"pnpm@10.33.0","scripts":{
			"start":"PORT=5000 node dist/main.js",
			"db:migrate":"node scripts/migrate.js",
			"codegen":"graphql-codegen",
			"db:reset":"node scripts/reset.js",
			"test":"vitest"}}`,
		"pnpm-lock.yaml": "",
		"docker/docker-compose.yml": `services:
  postgres:
    image: postgis/postgis:17
    ports:
      - "5432:5432"
  rabbitmq:
    image: rabbitmq:3-management
    ports:
      - 5672:5672
      - "15672:15672"
  opensearch:
    ports:
      - target: 9200
        published: 9200
volumes:
  pg: {}
`,
	})
	p := Detect("feather", dir)
	start := byName(p.Services, "start")
	if start == nil || start.Port != 5000 || start.Cmd != "pnpm start" || !start.Accept {
		t.Fatalf("start = %+v", start)
	}
	if start.PortEnv != "PORT" {
		t.Errorf("start.PortEnv = %q", start.PortEnv)
	}
	reset := byName(p.Steps, "db:reset")
	if reset == nil || !reset.Destructive || len(reset.Supersedes) != 1 || reset.Supersedes[0] != "db:migrate" {
		t.Errorf("db:reset = %+v", reset)
	}
	for _, n := range []string{"install", "db:migrate", "codegen"} {
		if byName(p.Steps, n) == nil {
			t.Errorf("missing step %s", n)
		}
	}
	infra := byName(p.Steps, "infra")
	if infra == nil || infra.Kind != "infra" || p.Compose != "docker/docker-compose.yml" {
		t.Fatalf("infra = %+v compose=%q", infra, p.Compose)
	}
	notes := strings.Join(p.Notes, "\n")
	for _, want := range []string{"postgres: container on :5432", "rabbitmq: container on :5672, :15672", "opensearch: container on :9200"} {
		if !strings.Contains(notes, want) {
			t.Errorf("notes missing %q:\n%s", want, notes)
		}
	}
	if byName(p.Services, "postgres") != nil {
		t.Errorf("compose containers must not become services")
	}
}

func TestDetectGoModule(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, map[string]string{
		"go.mod":          "module x\n\ngo 1.25.0\n",
		"cmd/api/main.go": "package main\n\nfunc main() {}\n",
		"cmd/lib/lib.go":  "package lib\n",
		"internal/x/x.go": "package x\n",
	})
	p := Detect("x", dir)
	if c := byName(p.Services, "api"); c == nil || c.Cmd != "go run ./cmd/api" {
		t.Fatalf("services = %+v", p.Services)
	}
	if byName(p.Services, "lib") != nil {
		t.Errorf("non-main cmd dir proposed")
	}
	if !strings.Contains(p.Toolchain, "go@1.25.0") {
		t.Errorf("toolchain = %q", p.Toolchain)
	}
}

func TestDetectRails(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, map[string]string{"Gemfile": "gem 'rails'\n", "config/application.rb": "module App; end\n"})
	p := Detect("r", dir)
	c := byName(p.Services, "rails")
	if c == nil || c.Port != 3000 || c.PortArgs != "-p {{port}}" {
		t.Fatalf("rails = %+v", c)
	}
	if r := byName(p.Steps, "db:reset"); r == nil || !r.Destructive || r.Accept {
		t.Errorf("db:reset = %+v", r)
	}
}

func TestDetectNeverGuessesPort(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, map[string]string{"package.json": `{"scripts":{"dev":"node server.js"}}`})
	p := Detect("x", dir)
	c := byName(p.Services, "dev")
	if c == nil || c.Port != 0 {
		t.Fatalf("dev = %+v", c)
	}
	if c.Cmd != "npm run dev" {
		t.Errorf("cmd = %q", c.Cmd)
	}
	if !strings.Contains(strings.Join(p.Notes, "\n"), "dev: no port found") {
		t.Errorf("missing no-port note: %v", p.Notes)
	}
}

func TestDetectProcfileAndMakefile(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, map[string]string{
		"Procfile": "web: bundle exec puma -p $PORT\nworker: bundle exec sidekiq\n",
		"Makefile": ".PHONY: dev\ndev:\n\tgo run .\nmigrate:\n\t./migrate\n%.o: %.c\n\tcc\nVAR := 1\n",
	})
	p := Detect("x", dir)
	if w := byName(p.Services, "web"); w == nil || w.PortEnv != "PORT" || !w.Accept {
		t.Errorf("web = %+v", w)
	}
	if byName(p.Services, "make-dev") == nil || byName(p.Steps, "make-migrate") == nil {
		t.Errorf("makefile: services=%+v steps=%+v", p.Services, p.Steps)
	}
}

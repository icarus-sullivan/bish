package langext

import "testing"

func TestAllParsesEveryManifest(t *testing.T) {
	if len(All()) == 0 {
		t.Fatal("expected at least one language definition")
	}
	for _, d := range All() {
		if d.ID == "" || d.Name == "" {
			t.Fatalf("definition missing id/name: %+v", d)
		}
		if len(d.Extensions) == 0 {
			t.Fatalf("%s: no extensions declared", d.ID)
		}
	}
}

func TestGetAndForExtension(t *testing.T) {
	def, ok := Get("py")
	if !ok || def.Name != "Python" {
		t.Fatalf("Get(py) = %+v, %v", def, ok)
	}
	if def.Server == nil || def.Formatter == nil {
		t.Fatal("python should declare both a server and a dedicated formatter")
	}

	def, ok = ForExtension(".py")
	if !ok || def.ID != "py" {
		t.Fatalf("ForExtension(.py) = %+v, %v", def, ok)
	}

	if _, ok := Get("nonexistent"); ok {
		t.Fatal("Get(nonexistent) should report not-found")
	}
}

func TestBuiltinFormattersHaveNoProcesses(t *testing.T) {
	for _, id := range []string{"json", "yaml", "sql", "xml", "toml", "html", "markdown", "csv"} {
		def, ok := Get(id)
		if !ok {
			t.Fatalf("missing builtin-formatter definition %s", id)
		}
		if !def.BuiltinFormatter {
			t.Errorf("%s: expected BuiltinFormatter=true", id)
		}
		if def.Server != nil || def.Formatter != nil {
			t.Errorf("%s: builtin-formatter language should have no external process defs", id)
		}
	}
}

package app

import "testing"

// runs against this repo itself — cheap end-to-end check of the porcelain parsers
func TestGitBlame(t *testing.T) {
	a := &App{}
	lines := a.GitBlame("app.go") // any committed file; untracked files blame to nil
	if lines == nil {
		t.Skip("not in a git checkout")
	}
	if len(lines) == 0 {
		t.Fatal("no blame lines")
	}
	for i, l := range lines[:3] {
		if l.SHA == "" || l.Author == "" {
			t.Fatalf("line %d missing sha/author: %+v", i, l)
		}
	}
}

func TestGitBlameNotRepo(t *testing.T) {
	a := &App{}
	if got := a.GitBlame("/tmp/definitely-not-a-repo-file.txt"); got != nil {
		t.Fatalf("want nil for non-repo, got %v", got)
	}
}

func TestParseUnifiedDiff(t *testing.T) {
	// added (old count 0), modified (both>0), deleted (new count 0)
	out := "diff --git a/f b/f\n" +
		"@@ -0,0 +5,2 @@\n" +      // 2 lines added at 5,6
		"@@ -10,1 +12,1 @@\n" +    // line 12 modified
		"@@ -20,3 +21,0 @@\n"      // deletion anchored at 21
	got := parseUnifiedDiff(out)
	want := []DiffLine{
		{Line: 5, Type: "added"}, {Line: 6, Type: "added"},
		{Line: 12, Type: "modified"},
		{Line: 21, Type: "deleted"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("line %d: got %+v, want %+v", i, got[i], want[i])
		}
	}
}

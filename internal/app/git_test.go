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

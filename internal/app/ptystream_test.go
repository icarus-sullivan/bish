package app

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"unicode/utf8"
)

func TestIncompleteTail(t *testing.T) {
	box := []byte("─")   // 3-byte rune
	emoji := []byte("😀") // 4-byte rune
	cases := []struct {
		in   []byte
		want int
	}{
		{[]byte("plain"), 0},
		{nil, 0},
		{box, 0},
		{box[:1], 1},
		{box[:2], 2},
		{append([]byte("ab"), emoji[:3]...), 3},
		{append([]byte("ab"), emoji...), 0},
	}
	for _, c := range cases {
		if got := incompleteTail(c.in); got != c.want {
			t.Errorf("incompleteTail(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestAppendBacklogBounded(t *testing.T) {
	s := &ptyStream{}
	line := bytes.Repeat([]byte("─"), 40)
	line = append(line, '\n')
	for i := 0; i < ptyBacklogMax/len(line)*3; i++ {
		s.appendBacklog(line)
	}
	if len(s.backlog) > 2*ptyBacklogMax {
		t.Fatalf("backlog %d exceeds bound %d", len(s.backlog), 2*ptyBacklogMax)
	}
	if !utf8.Valid(s.backlog) {
		t.Fatal("trimmed backlog is not valid UTF-8")
	}
	if !bytes.HasPrefix(s.backlog, []byte("─")) {
		t.Fatal("trimmed backlog should start on a line boundary")
	}
}

func TestWriteFileAtomicKeepsModeAndSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "script.sh")
	if err := os.WriteFile(target, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.sh")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if err := writeFileAtomic(link, []byte("new")); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Lstat(link); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("symlink was replaced by a regular file")
	}
	got, _ := os.ReadFile(target)
	if string(got) != "new" {
		t.Fatalf("target content = %q", got)
	}
	if fi, _ := os.Stat(target); fi.Mode().Perm() != 0o755 {
		t.Fatalf("mode = %v, want 0755", fi.Mode().Perm())
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 2 {
		t.Fatalf("temp file left behind: %d entries", len(entries))
	}
}

func TestWriteFileCheckedConflict(t *testing.T) {
	a := &App{}
	p := filepath.Join(t.TempDir(), "f.txt")
	if err := os.WriteFile(p, []byte("v1"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := a.StatMtime(p)
	m2, err := a.WriteFileChecked(p, "v2", m)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.WriteFileChecked(p, "v3", m); err == nil || err.Error() != ErrFileConflict {
		t.Fatalf("stale mtime should conflict, got %v", err)
	}
	if _, err := a.WriteFileChecked(p, "v3", m2); err != nil {
		t.Fatalf("current mtime should save: %v", err)
	}
}

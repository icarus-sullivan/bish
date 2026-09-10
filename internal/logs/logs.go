package logs

import (
	"regexp"
	"sync"
)

const maxLines = 1000

// ansiEscape matches ANSI CSI sequences ("\x1b[...<letter>") — the color/style
// codes tools like npm, vite, and next wrap their output in. The process log
// viewer renders lines as flat text, so left unstripped these show up as
// garbage (the ESC control char followed by literal text like "[2m[32m")
// instead of being interpreted as color.
var ansiEscape = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

type LogBuffer struct {
	mu    sync.Mutex
	lines []string
	head  int
	count int
}

func NewBuffer() *LogBuffer {
	return &LogBuffer{lines: make([]string, maxLines)}
}

func (b *LogBuffer) Write(line string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.lines[b.head] = ansiEscape.ReplaceAllString(line, "")
	b.head = (b.head + 1) % maxLines
	if b.count < maxLines {
		b.count++
	}
}

// Lines returns the last n lines in order.
func (b *LogBuffer) Lines(n int) []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	if n > b.count {
		n = b.count
	}
	out := make([]string, n)
	start := (b.head - n + maxLines) % maxLines
	for i := 0; i < n; i++ {
		out[i] = b.lines[(start+i)%maxLines]
	}
	return out
}

package logs

import "sync"

const maxLines = 1000

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
	b.lines[b.head] = line
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

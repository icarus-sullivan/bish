package lsp

import (
	"fmt"
	"io"
	"strings"
	"testing"
)

func TestReadFramesRoundTrip(t *testing.T) {
	msgs := []string{
		`{"jsonrpc":"2.0","id":1,"result":{}}`,
		`{"jsonrpc":"2.0","method":"textDocument/publishDiagnostics","params":{"uri":"file:///x.go"}}`,
	}
	var buf strings.Builder
	for _, m := range msgs {
		fmt.Fprintf(&buf, "Content-Length: %d\r\n\r\n%s", len(m), m)
	}
	var got []string
	err := readFrames(strings.NewReader(buf.String()), func(b []byte) {
		got = append(got, string(b))
	})
	if err != io.EOF {
		t.Fatalf("want EOF, got %v", err)
	}
	if len(got) != len(msgs) {
		t.Fatalf("want %d messages, got %d", len(msgs), len(got))
	}
	for i := range msgs {
		if got[i] != msgs[i] {
			t.Errorf("msg %d: want %q, got %q", i, msgs[i], got[i])
		}
	}
}

func TestReadFramesOversize(t *testing.T) {
	in := fmt.Sprintf("Content-Length: %d\r\n\r\n", maxMessage+1)
	err := readFrames(strings.NewReader(in), func([]byte) { t.Fatal("no message expected") })
	if err == nil || err == io.EOF {
		t.Fatalf("want oversize error, got %v", err)
	}
}

func TestSendFraming(t *testing.T) {
	pr, pw := io.Pipe()
	m := NewManager(func(string, ...interface{}) {})
	m.servers["go"] = &server{stdin: pw, cmd: nil}
	msg := `{"jsonrpc":"2.0","id":1,"method":"initialize"}`
	go func() {
		if err := m.Send("go", msg); err != nil {
			t.Errorf("Send: %v", err)
		}
		pw.Close()
	}()
	out, _ := io.ReadAll(pr)
	want := fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(msg), msg)
	if string(out) != want {
		t.Errorf("want %q, got %q", want, out)
	}
}

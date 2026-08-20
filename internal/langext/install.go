package langext

import (
	"bufio"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// RunInstaller runs argv, streaming each output line to emit (typically
// wired to an "…:install-output:<id>" event) and returning an error that
// folds in the last non-blank output line — cmd.Wait()'s own error is
// usually just "exit status 1", the real reason lives in the stream.
// Shared by lsp.Manager.Install and FormatterManager.Install so the two
// installer paths (server, formatter) don't duplicate this logic.
func RunInstaller(argv []string, emit func(line string)) error {
	if _, err := exec.LookPath(argv[0]); err != nil {
		return fmt.Errorf("%s not found on PATH", argv[0])
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	pr, pw := io.Pipe()
	cmd.Stdout = pw
	cmd.Stderr = pw
	if err := cmd.Start(); err != nil {
		pw.Close()
		return err
	}
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
		pw.Close()
	}()
	sc := bufio.NewScanner(pr)
	sc.Buffer(make([]byte, 4<<10), 1<<20)
	var lastLine string
	for sc.Scan() {
		line := sc.Text()
		if strings.TrimSpace(line) != "" {
			lastLine = line
		}
		emit(line)
	}
	if err := <-done; err != nil {
		if lastLine != "" {
			return fmt.Errorf("%s: %s", lastLine, err)
		}
		return err
	}
	return nil
}

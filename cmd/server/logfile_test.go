package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func TestRotatingLogAppendsAndCreatesDir(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nudge", "nudge.log")
	w, err := openRotatingLog(path, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("one\n")); err != nil {
		t.Fatal(err)
	}
	w.Close()
	w, err = openRotatingLog(path, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("two\n")); err != nil {
		t.Fatal(err)
	}
	w.Close()
	if got := readFile(t, path); got != "one\ntwo\n" {
		t.Fatalf("log = %q, want appended lines", got)
	}
	if _, err := os.Stat(path + ".1"); !os.IsNotExist(err) {
		t.Fatalf("unexpected backup: %v", err)
	}
}

func TestRotatingLogRotatesToSingleBackup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nudge.log")
	w, err := openRotatingLog(path, 10)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	for _, line := range []string{"aaaaaaaa\n", "bbbbbbbb\n", "cccccccc\n"} {
		if _, err := w.Write([]byte(line)); err != nil {
			t.Fatal(err)
		}
	}
	if got := readFile(t, path); got != "cccccccc\n" {
		t.Fatalf("current log = %q, want only the newest line", got)
	}
	if got := readFile(t, path+".1"); got != "bbbbbbbb\n" {
		t.Fatalf("backup = %q, want the previous line only (one backup kept)", got)
	}
	if _, err := os.Stat(path + ".2"); !os.IsNotExist(err) {
		t.Fatalf("second backup exists: %v", err)
	}
}

func TestRotatingLogRotatesOversizedFileAtOpen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nudge.log")
	if err := os.WriteFile(path, []byte(strings.Repeat("x", 50)), 0o600); err != nil {
		t.Fatal(err)
	}
	w, err := openRotatingLog(path, 10)
	if err != nil {
		t.Fatal(err)
	}
	w.Write([]byte("new\n"))
	w.Close()
	if got := readFile(t, path); got != "new\n" {
		t.Fatalf("log = %q, want fresh file", got)
	}
	if got := readFile(t, path+".1"); got != strings.Repeat("x", 50) {
		t.Fatalf("backup = %q", got)
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("no console") }

func TestLogOutputWritesFileEvenWhenStderrFails(t *testing.T) {
	var file bytes.Buffer
	w := logOutput(&file, failingWriter{})
	if n, err := w.Write([]byte("hello\n")); err != nil || n != 6 {
		t.Fatalf("Write = %d, %v; want 6, nil", n, err)
	}
	if file.String() != "hello\n" {
		t.Fatalf("file = %q", file.String())
	}
	var console bytes.Buffer
	file.Reset()
	w = logOutput(&file, &console)
	w.Write([]byte("both\n"))
	if file.String() != "both\n" || console.String() != "both\n" {
		t.Fatalf("file = %q, console = %q; want both", file.String(), console.String())
	}
	file.Reset()
	w = logOutput(&file, nil)
	w.Write([]byte("only\n"))
	if file.String() != "only\n" {
		t.Fatalf("file = %q", file.String())
	}
}

func TestControlSocketFailureIsFatalOnlyOffWindows(t *testing.T) {
	if !controlFailureFatal("linux") {
		t.Fatal("linux: control socket failure must stay fatal")
	}
	if controlFailureFatal("windows") {
		t.Fatal("windows: control socket failure must not stop the server")
	}
}

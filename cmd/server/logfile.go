package main

import (
	"io"
	"os"
	"path/filepath"
	"sync"
)

// maxLogBytes caps nudge.log; the previous file is kept as nudge.log.1.
const maxLogBytes = 2 << 20

// rotatingLog appends to a file and, before a write would push it past max
// bytes, moves it to path+".1" (replacing the old backup) and starts again.
// The file is closed before renaming so this also works on Windows.
type rotatingLog struct {
	mu   sync.Mutex
	path string
	max  int64
	file *os.File
	size int64
}

func openRotatingLog(path string, max int64) (*rotatingLog, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	w := &rotatingLog{path: path, max: max}
	if info, err := os.Stat(path); err == nil && info.Size() > max {
		_ = w.rotate()
	}
	if err := w.open(); err != nil {
		return nil, err
	}
	return w, nil
}

func (w *rotatingLog) open() error {
	f, err := os.OpenFile(w.path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return err
	}
	w.file, w.size = f, info.Size()
	return nil
}

func (w *rotatingLog) rotate() error {
	backup := w.path + ".1"
	if err := os.Remove(backup); err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.Rename(w.path, backup)
}

func (w *rotatingLog) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file != nil && w.size > 0 && w.size+int64(len(p)) > w.max {
		w.file.Close()
		w.file = nil
		_ = w.rotate() // on failure keep appending to the same file
		if err := w.open(); err != nil {
			return 0, err
		}
	}
	if w.file == nil {
		if err := w.open(); err != nil {
			return 0, err
		}
	}
	n, err := w.file.Write(p)
	w.size += int64(n)
	return n, err
}

func (w *rotatingLog) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return nil
	}
	err := w.file.Close()
	w.file = nil
	return err
}

// teeWriter writes every line to all writers and never fails because one of
// them does (a vanished console must not stop file logging).
type teeWriter []io.Writer

func (t teeWriter) Write(p []byte) (int, error) {
	for _, w := range t {
		_, _ = w.Write(p)
	}
	return len(p), nil
}

// logOutput returns the log destination: the file, plus the console when
// one is usable (console may be nil).
func logOutput(file io.Writer, console io.Writer) io.Writer {
	if console == nil {
		return file
	}
	return teeWriter{file, console}
}

// controlFailureFatal reports whether a control socket that cannot listen
// stops the server. On Windows AF_UNIX may be unavailable or the path too
// long; pairing still works through the in-process dialog, so the server
// keeps serving HTTP there.
func controlFailureFatal(goos string) bool {
	return goos != "windows"
}

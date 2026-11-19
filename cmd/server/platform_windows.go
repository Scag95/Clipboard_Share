//go:build windows

package main

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"unsafe"

	"nudge/internal/pair"
)

var (
	user32          = syscall.NewLazyDLL("user32.dll")
	procMessageBoxW = user32.NewProc("MessageBoxW")
)

// MessageBoxW flags. Deliberately no MB_TOPMOST / MB_SETFOREGROUND: the
// dialog must not steal focus, and the default button is No so a stray
// Enter or Space denies.
const (
	mbYesNo        = 0x00000004
	mbIconQuestion = 0x00000020
	mbDefButton2   = 0x00000100
)

// stderrUsable reports whether this process has a real standard error
// handle. nudge-background.exe (-H=windowsgui) normally has none.
func stderrUsable() bool {
	if os.Stderr == nil {
		return false
	}
	h, err := syscall.GetStdHandle(syscall.STD_ERROR_HANDLE)
	if err != nil || h == 0 || h == syscall.InvalidHandle {
		return false
	}
	kind, err := syscall.GetFileType(h)
	return err == nil && kind != syscall.FILE_TYPE_UNKNOWN
}

// setupLogging always logs to %APPDATA%\nudge\nudge.log on Windows (size
// capped, one backup) and also to the console when there is one.
func setupLogging() {
	var console io.Writer
	if stderrUsable() {
		console = os.Stderr
	}
	path := filepath.Join(pair.ConfigDir(), "nudge.log")
	file, err := openRotatingLog(path, maxLogBytes)
	if err != nil {
		if console == nil {
			log.SetOutput(io.Discard)
			return
		}
		log.Printf("warning: cannot open log file %s: %v", path, err)
		return
	}
	log.SetOutput(logOutput(file, console))
	log.Printf("log file: %s", path)
}

// messageBox shows a modal Yes/No question and returns the MessageBoxW
// result (IDYES, IDNO, or 0 on failure).
func messageBox(title, text string) int {
	if err := procMessageBoxW.Find(); err != nil {
		log.Printf("pairing dialog unavailable: %v", err)
		return 0
	}
	textPtr, err := syscall.UTF16PtrFromString(text)
	if err != nil {
		return 0
	}
	titlePtr, err := syscall.UTF16PtrFromString(title)
	if err != nil {
		return 0
	}
	// The dialog's window and message loop belong to the calling thread.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	ret, _, callErr := procMessageBoxW.Call(0,
		uintptr(unsafe.Pointer(textPtr)),
		uintptr(unsafe.Pointer(titlePtr)),
		mbYesNo|mbIconQuestion|mbDefButton2)
	if ret == 0 {
		log.Printf("pairing dialog failed: %v", callErr)
	}
	return int(ret)
}

// pairRequestHandler queues a native Yes/No dialog per new pairing request
// (one at a time, capped); `nudge pair` keeps working as an alternative.
func pairRequestHandler(b *pair.Broker, logf func(string, ...any)) func(pair.RequestInfo) {
	q := newApprovalQueue(b, messageBox, logf, maxPendingDialogs)
	q.Start()
	return func(req pair.RequestInfo) {
		logf("pair request %s from %s; answer the dialog or run: nudge pair --last", shortID(req.RequestID), req.IP)
		q.Enqueue(req)
	}
}

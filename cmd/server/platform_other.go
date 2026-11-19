//go:build !windows

package main

import (
	"os/exec"

	"nudge/internal/pair"
)

// setupLogging keeps the standard logger on stderr (journald / terminal).
func setupLogging() {}

// pairRequestHandler shows a best-effort desktop notification: headless
// systems and sessions without DISPLAY still approve through the CLI or the
// tray panel.
func pairRequestHandler(b *pair.Broker, _ func(string, ...any)) func(pair.RequestInfo) {
	return func(req pair.RequestInfo) {
		_, code := b.Verification()
		title, body := pairNotification(req.Name, code)
		go func() {
			_ = exec.Command("notify-send", title, body).Run()
		}()
	}
}

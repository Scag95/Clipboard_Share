//go:build !windows

package main

import "nudge/internal/pair"

// startTray is a no-op outside Windows: Linux keeps its own tray panel.
func startTray(string, *pair.Broker, func(string, ...any)) trayHandle {
	return noTray{}
}

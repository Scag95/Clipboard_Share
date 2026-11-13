//go:build linux

package clipboard

import "os"

func newBackend() clipboardBackend {
	if os.Getenv("WAYLAND_DISPLAY") != "" {
		if _, err := lookPath("wl-copy"); err == nil {
			return wlClipboard{}
		}
	}
	return libraryBackend{}
}

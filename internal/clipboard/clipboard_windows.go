//go:build windows

package clipboard

func newBackend() clipboardBackend {
	return libraryBackend{}
}

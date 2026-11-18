//go:build !windows

package pair

import "os"

// replaceFile atomically replaces newPath with oldPath (rename(2)).
func replaceFile(oldPath, newPath string) error {
	return os.Rename(oldPath, newPath)
}

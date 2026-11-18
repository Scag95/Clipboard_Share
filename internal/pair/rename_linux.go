//go:build linux

package pair

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

// renameNoReplace renames oldPath to newPath atomically and fails with an
// os.ErrExist-compatible error when newPath already exists, even as an empty
// directory (which plain rename(2) would replace).
func renameNoReplace(oldPath, newPath string) error {
	err := unix.Renameat2(unix.AT_FDCWD, oldPath, unix.AT_FDCWD, newPath, unix.RENAME_NOREPLACE)
	if errors.Is(err, unix.EINVAL) || errors.Is(err, unix.ENOSYS) {
		// Filesystem or kernel without RENAME_NOREPLACE.
		return renameIfAbsent(oldPath, newPath)
	}
	if err != nil {
		return &os.LinkError{Op: "rename", Old: oldPath, New: newPath, Err: err}
	}
	return nil
}

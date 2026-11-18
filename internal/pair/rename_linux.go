//go:build linux

package pair

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

// renameat2 performs rename(2) with RENAME_NOREPLACE; tests replace it to
// exercise the fallback.
var renameat2 = func(oldPath, newPath string) error {
	return unix.Renameat2(unix.AT_FDCWD, oldPath, unix.AT_FDCWD, newPath, unix.RENAME_NOREPLACE)
}

// renameNoReplace renames oldPath to newPath atomically and fails with an
// os.ErrExist-compatible error when newPath already exists, even as an empty
// directory (which plain rename(2) would replace).
func renameNoReplace(oldPath, newPath string) error {
	err := renameat2(oldPath, newPath)
	if errors.Is(err, unix.EINVAL) || errors.Is(err, unix.ENOSYS) {
		// Filesystem or kernel without RENAME_NOREPLACE.
		return renameIfAbsent(oldPath, newPath)
	}
	if err != nil {
		return &os.LinkError{Op: "rename", Old: oldPath, New: newPath, Err: err}
	}
	return nil
}

package pair

import "os"

// renameIfAbsent is the non-atomic fallback: it refuses an existing newPath
// before renaming. A concurrent creator can still race the check.
func renameIfAbsent(oldPath, newPath string) error {
	if _, err := os.Lstat(newPath); err == nil {
		return &os.LinkError{Op: "rename", Old: oldPath, New: newPath, Err: os.ErrExist}
	} else if !os.IsNotExist(err) {
		return err
	}
	return os.Rename(oldPath, newPath)
}

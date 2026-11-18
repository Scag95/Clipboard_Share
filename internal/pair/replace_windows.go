//go:build windows

package pair

import (
	"errors"
	"os"
	"syscall"
	"time"
)

const errorSharingViolation syscall.Errno = 32 // ERROR_SHARING_VIOLATION

// replaceFile replaces newPath with oldPath. os.Rename uses MoveFileEx with
// MOVEFILE_REPLACE_EXISTING, which fails with ACCESS_DENIED or
// SHARING_VIOLATION while another process (antivirus, indexer, an editor)
// briefly holds state.json open without FILE_SHARE_DELETE, so a few short
// retries are made before giving up.
func replaceFile(oldPath, newPath string) error {
	var err error
	for attempt := 0; attempt < 5; attempt++ {
		if err = os.Rename(oldPath, newPath); err == nil {
			return nil
		}
		if !errors.Is(err, syscall.ERROR_ACCESS_DENIED) && !errors.Is(err, errorSharingViolation) {
			return err
		}
		time.Sleep(time.Duration(attempt+1) * 20 * time.Millisecond)
	}
	return err
}

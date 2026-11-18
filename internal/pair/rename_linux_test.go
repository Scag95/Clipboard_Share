//go:build linux

package pair

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestRenameNoReplaceFallsBackWithoutRenameat2Support(t *testing.T) {
	for _, errno := range []error{unix.EINVAL, unix.ENOSYS} {
		t.Run(errno.Error(), func(t *testing.T) {
			prev := renameat2
			calls := 0
			renameat2 = func(string, string) error { calls++; return errno }
			t.Cleanup(func() { renameat2 = prev })

			dir := t.TempDir()
			src := filepath.Join(dir, "src")
			writeStateDir(t, src, "src")
			empty := filepath.Join(dir, "empty")
			if err := os.Mkdir(empty, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := renameNoReplace(src, empty); !errors.Is(err, os.ErrExist) {
				t.Fatalf("existing target: err = %v, want ErrExist", err)
			}
			assertStateDir(t, src, "src")

			dst := filepath.Join(dir, "dst")
			if err := renameNoReplace(src, dst); err != nil {
				t.Fatalf("absent target: err = %v", err)
			}
			assertStateDir(t, dst, "src")
			assertMissing(t, src)
			if calls != 2 {
				t.Fatalf("renameat2 calls = %d, want 2", calls)
			}
		})
	}
}

func TestRenameNoReplaceReportsOtherErrors(t *testing.T) {
	prev := renameat2
	renameat2 = func(string, string) error { return unix.EXDEV }
	t.Cleanup(func() { renameat2 = prev })
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	writeStateDir(t, src, "src")
	err := renameNoReplace(src, filepath.Join(dir, "dst"))
	var linkErr *os.LinkError
	if !errors.As(err, &linkErr) || !errors.Is(err, unix.EXDEV) {
		t.Fatalf("err = %v, want LinkError wrapping EXDEV", err)
	}
	assertStateDir(t, src, "src")
}

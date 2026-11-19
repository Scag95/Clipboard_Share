package pair

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// tempHome points the home directory (HOME, and USERPROFILE for Windows) at a
// fresh directory and clears every path override so the default locations are
// exercised without touching the real config. It pins the Linux path rules,
// which these tests assert on every host; Windows tests re-pin with
// fakePlatform.
func tempHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	prevOS := hostOS
	hostOS = "linux"
	t.Cleanup(func() { hostOS = prevOS })
	for _, name := range []string{"NUDGE_STATE", "NUDGE_CTL_SOCK"} {
		t.Setenv(name, "")
	}
	return home
}

func TestDefaultPathsUseNudgeConfigDir(t *testing.T) {
	home := tempHome(t)
	dir := filepath.Join(home, ".config", "nudge")
	if got := ConfigDir(); got != dir {
		t.Fatalf("ConfigDir() = %q, want %q", got, dir)
	}
	if got, want := StatePath(), filepath.Join(dir, "state.json"); got != want {
		t.Fatalf("StatePath() = %q, want %q", got, want)
	}
	if got, want := ControlPath(), filepath.Join(dir, "ctl.sock"); got != want {
		t.Fatalf("ControlPath() = %q, want %q", got, want)
	}
}

func TestPathOverridesUseNudgeNames(t *testing.T) {
	cases := []struct {
		suffix string
		get    func() string
	}{
		{"STATE", StatePath},
		{"CTL_SOCK", ControlPath},
	}
	for _, c := range cases {
		t.Run(c.suffix, func(t *testing.T) {
			tempHome(t)
			t.Setenv("NUDGE_"+c.suffix, "/tmp/new-value")
			if got := c.get(); got != "/tmp/new-value" {
				t.Fatalf("got %q, want /tmp/new-value", got)
			}
		})
	}
}

func TestEnvOverride(t *testing.T) {
	t.Setenv("NUDGE_PORT", "")
	if got := EnvOverride("PORT"); got != "" {
		t.Fatalf("unset: got %q", got)
	}
	t.Setenv("NUDGE_PORT", "9100")
	if got := EnvOverride("PORT"); got != "9100" {
		t.Fatalf("set: got %q, want 9100", got)
	}
}

func writeStateDir(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "state.json"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func assertStateDir(t *testing.T, dir, content string) {
	t.Helper()
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat %s: %v", dir, err)
	}
	assertPerm(t, dir, info, 0o700)
	file := filepath.Join(dir, "state.json")
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("read %s: %v", file, err)
	}
	if string(data) != content {
		t.Fatalf("%s = %q, want %q", file, data, content)
	}
	finfo, err := os.Stat(file)
	if err != nil {
		t.Fatal(err)
	}
	assertPerm(t, file, finfo, 0o600)
}

// assertPerm checks Unix permission bits; Windows has no such bits (os.Chmod
// only toggles read-only), so it is skipped there.
func assertPerm(t *testing.T, path string, info os.FileInfo, want os.FileMode) {
	t.Helper()
	if runtime.GOOS == "windows" {
		return
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("%s mode = %v, want %v", path, got, want)
	}
}

func assertMissing(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("%s exists (err=%v), want missing", path, err)
	}
}

// Resolving the default paths never creates the config directory.
func TestDefaultPathsAreSideEffectFree(t *testing.T) {
	home := tempHome(t)
	dir := filepath.Join(home, ".config", "nudge")
	if got := StatePath(); got != filepath.Join(dir, "state.json") {
		t.Fatalf("StatePath() = %q, want under %q", got, dir)
	}
	if got := ControlPath(); got != filepath.Join(dir, "ctl.sock") {
		t.Fatalf("ControlPath() = %q, want under %q", got, dir)
	}
	assertMissing(t, filepath.Join(home, ".config"))
}

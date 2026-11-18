package pair

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// fakePlatform pretends to run on goos with the given user config dir result,
// restoring the real seams when the test ends.
func fakePlatform(t *testing.T, goos, appData string, appDataErr error) {
	t.Helper()
	prevOS, prevDir := hostOS, userConfigDir
	hostOS = goos
	userConfigDir = func() (string, error) { return appData, appDataErr }
	t.Cleanup(func() { hostOS, userConfigDir = prevOS, prevDir })
}

func TestWindowsConfigRootIsAppData(t *testing.T) {
	tempHome(t)
	appData := filepath.Join(t.TempDir(), "AppData", "Roaming")
	fakePlatform(t, "windows", appData, nil)

	dir := filepath.Join(appData, "nudge")
	if got := ConfigDir(); got != dir {
		t.Fatalf("ConfigDir() = %q, want %q", got, dir)
	}
	if got := EffectiveConfigDir(); got != dir {
		t.Fatalf("EffectiveConfigDir() = %q, want %q", got, dir)
	}
	if got, want := StatePath(), filepath.Join(dir, "state.json"); got != want {
		t.Fatalf("StatePath() = %q, want %q", got, want)
	}
	if got, want := ControlPath(), filepath.Join(dir, "ctl.sock"); got != want {
		t.Fatalf("ControlPath() = %q, want %q", got, want)
	}
	t.Setenv("NUDGE_STATE", "/tmp/override-state.json")
	if got := StatePath(); got != "/tmp/override-state.json" {
		t.Fatalf("override StatePath() = %q", got)
	}
}

func TestWindowsConfigRootFallsBackToHomeAppData(t *testing.T) {
	home := tempHome(t)
	fakePlatform(t, "windows", "", errors.New("%AppData% is not defined"))
	want := filepath.Join(home, "AppData", "Roaming", "nudge")
	if got := ConfigDir(); got != want {
		t.Fatalf("ConfigDir() = %q, want %q", got, want)
	}
}

func TestWindowsIgnoresLegacyConfigDir(t *testing.T) {
	tempHome(t)
	appData := t.TempDir()
	fakePlatform(t, "windows", appData, nil)
	oldDir := filepath.Join(appData, "clipboard-share")
	newDir := filepath.Join(appData, "nudge")
	writeStateDir(t, oldDir, "old")

	if got := EffectiveConfigDir(); got != newDir {
		t.Fatalf("EffectiveConfigDir() = %q, want %q (no legacy fallback on Windows)", got, newDir)
	}
	moved, err := MigrateLegacyConfig()
	if err != nil || moved {
		t.Fatalf("MigrateLegacyConfig() = %v, %v; want false, nil on Windows", moved, err)
	}
	assertStateDir(t, oldDir, "old")
	assertMissing(t, newDir)
}

func TestLinuxConfigRootStaysDotConfig(t *testing.T) {
	home := tempHome(t)
	fakePlatform(t, "linux", filepath.Join(t.TempDir(), "xdg"), nil)
	want := filepath.Join(home, ".config", "nudge")
	if got := ConfigDir(); got != want {
		t.Fatalf("ConfigDir() = %q, want %q", got, want)
	}
	oldDir := filepath.Join(home, ".config", "clipboard-share")
	writeStateDir(t, oldDir, "old")
	if got := EffectiveConfigDir(); got != oldDir {
		t.Fatalf("EffectiveConfigDir() = %q, want legacy %q on Linux", got, oldDir)
	}
}

func listenWithFailingChmod(t *testing.T, goos string) (*ControlServer, error, string) {
	t.Helper()
	b := newTestBroker(t)
	path := filepath.Join(t.TempDir(), "ctl.sock")
	t.Setenv("CLIPBOARD_SHARE_CTL_SOCK", path)
	prevOS, prevChmod := hostOS, chmodPath
	hostOS = goos
	chmodPath = func(string, os.FileMode) error { return errors.New("chmod unsupported") }
	t.Cleanup(func() { hostOS, chmodPath = prevOS, prevChmod })
	control, err := ListenControl(b)
	return control, err, path
}

func TestListenControlToleratesChmodFailureOnWindows(t *testing.T) {
	control, err, path := listenWithFailingChmod(t, "windows")
	if err != nil {
		t.Fatalf("ListenControl on windows with failing chmod: %v", err)
	}
	defer control.Close()
	if info, err := os.Lstat(path); err != nil || info.Mode()&os.ModeSocket == 0 {
		t.Fatalf("socket not listening at %s: %v", path, err)
	}
}

func TestListenControlFailsOnChmodErrorOnLinux(t *testing.T) {
	control, err, path := listenWithFailingChmod(t, "linux")
	if err == nil {
		control.Close()
		t.Fatal("ListenControl on linux with failing chmod succeeded, want error")
	}
	assertMissing(t, path)
}

func TestControlPathIsSocketLike(t *testing.T) {
	cases := []struct {
		goos string
		mode fs.FileMode
		want bool
	}{
		{"linux", fs.ModeSocket, true},
		{"linux", 0, false},
		{"linux", fs.ModeIrregular, false},
		{"windows", fs.ModeSocket, true},
		{"windows", fs.ModeIrregular, true},
		{"windows", 0, false},
		{"windows", fs.ModeDir, false},
	}
	for _, c := range cases {
		if got := isSocketLike(c.goos, c.mode); got != c.want {
			t.Errorf("isSocketLike(%s, %v) = %v, want %v", c.goos, c.mode, got, c.want)
		}
	}
}

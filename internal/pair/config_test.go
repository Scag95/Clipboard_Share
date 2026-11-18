package pair

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// tempHome points HOME at a fresh directory and clears every path override so
// the default locations are exercised without touching the real ~/.config.
func tempHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, name := range []string{"NUDGE_STATE", "CLIPBOARD_SHARE_STATE", "NUDGE_CTL_SOCK", "CLIPBOARD_SHARE_CTL_SOCK"} {
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

func TestPathOverridesPreferNudgeAndFallBackToOldNames(t *testing.T) {
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
			t.Setenv("CLIPBOARD_SHARE_"+c.suffix, "/tmp/old-value")
			if got := c.get(); got != "/tmp/old-value" {
				t.Fatalf("old name only: got %q, want /tmp/old-value", got)
			}
			t.Setenv("NUDGE_"+c.suffix, "/tmp/new-value")
			if got := c.get(); got != "/tmp/new-value" {
				t.Fatalf("both names set: got %q, want /tmp/new-value", got)
			}
			t.Setenv("CLIPBOARD_SHARE_"+c.suffix, "")
			if got := c.get(); got != "/tmp/new-value" {
				t.Fatalf("new name only: got %q, want /tmp/new-value", got)
			}
		})
	}
}

func TestEnvOverride(t *testing.T) {
	t.Setenv("NUDGE_PORT", "")
	t.Setenv("CLIPBOARD_SHARE_PORT", "")
	if got := EnvOverride("PORT"); got != "" {
		t.Fatalf("neither set: got %q", got)
	}
	t.Setenv("CLIPBOARD_SHARE_PORT", "9000")
	if got := EnvOverride("PORT"); got != "9000" {
		t.Fatalf("old only: got %q, want 9000", got)
	}
	t.Setenv("NUDGE_PORT", "9100")
	if got := EnvOverride("PORT"); got != "9100" {
		t.Fatalf("both: got %q, want 9100", got)
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
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("%s mode = %v, want 0700", dir, info.Mode().Perm())
	}
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
	if finfo.Mode().Perm() != 0o600 {
		t.Fatalf("%s mode = %v, want 0600", file, finfo.Mode().Perm())
	}
}

func assertMissing(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("%s exists (err=%v), want missing", path, err)
	}
}

func TestMigrateLegacyConfigMovesOldDir(t *testing.T) {
	home := tempHome(t)
	oldDir := filepath.Join(home, ".config", "clipboard-share")
	newDir := filepath.Join(home, ".config", "nudge")
	writeStateDir(t, oldDir, `{"devices":[]}`)

	moved, err := MigrateLegacyConfig()
	if err != nil || !moved {
		t.Fatalf("MigrateLegacyConfig() = %v, %v; want true, nil", moved, err)
	}
	assertStateDir(t, newDir, `{"devices":[]}`)
	assertMissing(t, oldDir)

	// Once: a second run finds the new dir and does nothing.
	moved, err = MigrateLegacyConfig()
	if err != nil || moved {
		t.Fatalf("second MigrateLegacyConfig() = %v, %v; want false, nil", moved, err)
	}
	assertStateDir(t, newDir, `{"devices":[]}`)
}

func TestMigrateLegacyConfigKeepsExistingNewDir(t *testing.T) {
	home := tempHome(t)
	newDir := filepath.Join(home, ".config", "nudge")
	writeStateDir(t, newDir, "new")

	moved, err := MigrateLegacyConfig()
	if err != nil || moved {
		t.Fatalf("MigrateLegacyConfig() = %v, %v; want false, nil", moved, err)
	}
	assertStateDir(t, newDir, "new")
	assertMissing(t, filepath.Join(home, ".config", "clipboard-share"))
}

func TestMigrateLegacyConfigNeverOverwritesWhenBothExist(t *testing.T) {
	home := tempHome(t)
	oldDir := filepath.Join(home, ".config", "clipboard-share")
	newDir := filepath.Join(home, ".config", "nudge")
	writeStateDir(t, oldDir, "old")
	writeStateDir(t, newDir, "new")

	moved, err := MigrateLegacyConfig()
	if err != nil || moved {
		t.Fatalf("MigrateLegacyConfig() = %v, %v; want false, nil", moved, err)
	}
	assertStateDir(t, newDir, "new")
	assertStateDir(t, oldDir, "old")
}

func TestMigrateLegacyConfigNeverOverwritesEmptyNewDir(t *testing.T) {
	home := tempHome(t)
	oldDir := filepath.Join(home, ".config", "clipboard-share")
	newDir := filepath.Join(home, ".config", "nudge")
	writeStateDir(t, oldDir, "old")
	if err := os.Mkdir(newDir, 0o700); err != nil {
		t.Fatal(err)
	}

	moved, err := MigrateLegacyConfig()
	if err != nil || moved {
		t.Fatalf("MigrateLegacyConfig() = %v, %v; want false, nil", moved, err)
	}
	assertStateDir(t, oldDir, "old")
	entries, err := os.ReadDir(newDir)
	if err != nil || len(entries) != 0 {
		t.Fatalf("new dir entries = %v, err = %v; want empty", entries, err)
	}
}

func TestMigrateLegacyConfigWithNeitherCreatesNothing(t *testing.T) {
	home := tempHome(t)

	moved, err := MigrateLegacyConfig()
	if err != nil || moved {
		t.Fatalf("MigrateLegacyConfig() = %v, %v; want false, nil", moved, err)
	}
	assertMissing(t, filepath.Join(home, ".config"))
}

func TestMigrateLegacyConfigRenameFailureLeavesOldDir(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	home := tempHome(t)
	configDir := filepath.Join(home, ".config")
	oldDir := filepath.Join(configDir, "clipboard-share")
	writeStateDir(t, oldDir, "old")
	if err := os.Chmod(configDir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(configDir, 0o700) })

	moved, err := MigrateLegacyConfig()
	if err == nil || moved {
		t.Fatalf("MigrateLegacyConfig() = %v, %v; want false and an error", moved, err)
	}
	if err := os.Chmod(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	assertStateDir(t, oldDir, "old")
	assertMissing(t, filepath.Join(configDir, "nudge"))
}

func TestEffectiveConfigDirRules(t *testing.T) {
	cases := []struct {
		name        string
		makeNew     bool
		makeOld     bool
		wantDirName string
	}{
		{"neither", false, false, "nudge"},
		{"only new", true, false, "nudge"},
		{"only legacy", false, true, "clipboard-share"},
		{"both", true, true, "nudge"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := tempHome(t)
			newDir := filepath.Join(home, ".config", "nudge")
			oldDir := filepath.Join(home, ".config", "clipboard-share")
			if c.makeNew {
				writeStateDir(t, newDir, "new")
			}
			if c.makeOld {
				writeStateDir(t, oldDir, "old")
			}
			want := filepath.Join(home, ".config", c.wantDirName)
			if got := EffectiveConfigDir(); got != want {
				t.Fatalf("EffectiveConfigDir() = %q, want %q", got, want)
			}
			if got := StatePath(); got != filepath.Join(want, "state.json") {
				t.Fatalf("StatePath() = %q, want under %q", got, want)
			}
			if got := ControlPath(); got != filepath.Join(want, "ctl.sock") {
				t.Fatalf("ControlPath() = %q, want under %q", got, want)
			}
			// Resolution is side-effect free.
			if !c.makeNew {
				assertMissing(t, newDir)
			}
			if !c.makeOld {
				assertMissing(t, oldDir)
			}
			if c.makeNew {
				assertStateDir(t, newDir, "new")
			}
			if c.makeOld {
				assertStateDir(t, oldDir, "old")
			}
		})
	}
}

func TestEffectiveConfigDirOverridesKeepPriority(t *testing.T) {
	home := tempHome(t)
	writeStateDir(t, filepath.Join(home, ".config", "clipboard-share"), "old")
	t.Setenv("NUDGE_STATE", "/tmp/override-state.json")
	t.Setenv("CLIPBOARD_SHARE_CTL_SOCK", "/tmp/override.sock")
	if got := StatePath(); got != "/tmp/override-state.json" {
		t.Fatalf("StatePath() = %q", got)
	}
	if got := ControlPath(); got != "/tmp/override.sock" {
		t.Fatalf("ControlPath() = %q", got)
	}
}

func TestMigrateLegacyConfigFailedMoveKeepsLegacyEffective(t *testing.T) {
	home := tempHome(t)
	oldDir := filepath.Join(home, ".config", "clipboard-share")
	newDir := filepath.Join(home, ".config", "nudge")
	writeStateDir(t, oldDir, "old")
	boom := errors.New("boom")
	prev := renameDir
	renameDir = func(string, string) error { return boom }
	t.Cleanup(func() { renameDir = prev })

	moved, err := MigrateLegacyConfig()
	if moved || !errors.Is(err, boom) {
		t.Fatalf("MigrateLegacyConfig() = %v, %v; want false and boom", moved, err)
	}
	assertStateDir(t, oldDir, "old")
	assertMissing(t, newDir)
	if got := EffectiveConfigDir(); got != oldDir {
		t.Fatalf("EffectiveConfigDir() = %q, want %q", got, oldDir)
	}
	if got := StatePath(); got != filepath.Join(oldDir, "state.json") {
		t.Fatalf("StatePath() = %q, want inside the legacy dir", got)
	}
	if got := NewBroker().path; got != filepath.Join(oldDir, "state.json") {
		t.Fatalf("broker path = %q, want the legacy state file", got)
	}
}

func TestMigrateLegacyConfigSkipsWhenStateOverrideInsideLegacyDir(t *testing.T) {
	for _, name := range []string{"NUDGE_STATE", "CLIPBOARD_SHARE_STATE"} {
		for _, rel := range []string{"state.json", "sub/state.json"} {
			t.Run(name+"/"+rel, func(t *testing.T) {
				home := tempHome(t)
				oldDir := filepath.Join(home, ".config", "clipboard-share")
				newDir := filepath.Join(home, ".config", "nudge")
				writeStateDir(t, oldDir, "old")
				t.Setenv(name, filepath.Join(oldDir, rel))

				moved, err := MigrateLegacyConfig()
				if moved || !errors.Is(err, ErrStateInLegacyDir) {
					t.Fatalf("MigrateLegacyConfig() = %v, %v; want false, ErrStateInLegacyDir", moved, err)
				}
				assertStateDir(t, oldDir, "old")
				assertMissing(t, newDir)
			})
		}
	}
}

func TestMigrateLegacyConfigMovesWhenStateOverrideOutsideLegacyDir(t *testing.T) {
	home := tempHome(t)
	oldDir := filepath.Join(home, ".config", "clipboard-share")
	writeStateDir(t, oldDir, "old")
	// A sibling whose name merely starts with the legacy name is outside it.
	t.Setenv("NUDGE_STATE", oldDir+"-other/state.json")

	moved, err := MigrateLegacyConfig()
	if err != nil || !moved {
		t.Fatalf("MigrateLegacyConfig() = %v, %v; want true, nil", moved, err)
	}
	assertStateDir(t, filepath.Join(home, ".config", "nudge"), "old")
	assertMissing(t, oldDir)
}

func TestRenameIfAbsent(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	writeStateDir(t, src, "src")

	existing := filepath.Join(dir, "existing")
	writeStateDir(t, existing, "existing")
	if err := renameIfAbsent(src, existing); !errors.Is(err, os.ErrExist) {
		t.Fatalf("existing target: err = %v, want ErrExist", err)
	}
	assertStateDir(t, existing, "existing")

	empty := filepath.Join(dir, "empty")
	if err := os.Mkdir(empty, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := renameIfAbsent(src, empty); !errors.Is(err, os.ErrExist) {
		t.Fatalf("empty target: err = %v, want ErrExist", err)
	}
	assertStateDir(t, src, "src")

	dst := filepath.Join(dir, "dst")
	if err := renameIfAbsent(src, dst); err != nil {
		t.Fatalf("absent target: err = %v", err)
	}
	assertStateDir(t, dst, "src")
	assertMissing(t, src)
}

func TestMigrateLegacyConfigSkipsWhenStateOverrideReachesLegacyDirViaSymlink(t *testing.T) {
	home := tempHome(t)
	oldDir := filepath.Join(home, ".config", "clipboard-share")
	writeStateDir(t, oldDir, "old")
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(oldDir, link); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NUDGE_STATE", filepath.Join(link, "state.json"))

	moved, err := MigrateLegacyConfig()
	if moved || !errors.Is(err, ErrStateInLegacyDir) {
		t.Fatalf("MigrateLegacyConfig() = %v, %v; want false, ErrStateInLegacyDir", moved, err)
	}
	assertStateDir(t, oldDir, "old")
	assertMissing(t, filepath.Join(home, ".config", "nudge"))
}

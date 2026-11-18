package pair

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	configDirName       = "nudge"
	legacyConfigDirName = "clipboard-share"
)

// EnvOverride returns NUDGE_<suffix>, falling back to the pre-rename
// CLIPBOARD_SHARE_<suffix>. An empty value counts as unset.
func EnvOverride(suffix string) string {
	if value := os.Getenv("NUDGE_" + suffix); value != "" {
		return value
	}
	return os.Getenv("CLIPBOARD_SHARE_" + suffix)
}

func userConfigRoot() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".config")
}

// ConfigDir returns the new per-user state directory, ~/.config/nudge.
// Use EffectiveConfigDir to find where state lives right now.
func ConfigDir() string {
	return filepath.Join(userConfigRoot(), configDirName)
}

// LegacyConfigDir returns the pre-rename state directory,
// ~/.config/clipboard-share.
func LegacyConfigDir() string {
	return filepath.Join(userConfigRoot(), legacyConfigDirName)
}

// EffectiveConfigDir returns the directory everyone uses for state and the
// control socket: ~/.config/nudge if it exists, else ~/.config/clipboard-share
// if it exists (used as is, not moved), else ~/.config/nudge. It never creates
// or moves anything; only the server migrates, via MigrateLegacyConfig.
func EffectiveConfigDir() string {
	newDir := ConfigDir()
	if _, err := os.Lstat(newDir); err == nil {
		return newDir
	}
	if oldDir := LegacyConfigDir(); pathExists(oldDir) {
		return oldDir
	}
	return newDir
}

func pathExists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

// StatePath returns NUDGE_STATE (or CLIPBOARD_SHARE_STATE) or state.json in
// the effective config dir.
func StatePath() string {
	if path := EnvOverride("STATE"); path != "" {
		return path
	}
	return filepath.Join(EffectiveConfigDir(), "state.json")
}

// ControlPath returns NUDGE_CTL_SOCK (or CLIPBOARD_SHARE_CTL_SOCK) or
// ctl.sock in the effective config dir.
func ControlPath() string {
	if path := EnvOverride("CTL_SOCK"); path != "" {
		return path
	}
	return filepath.Join(EffectiveConfigDir(), "ctl.sock")
}

// ErrStateInLegacyDir reports that MigrateLegacyConfig left the old directory
// in place because NUDGE_STATE or CLIPBOARD_SHARE_STATE points inside it.
var ErrStateInLegacyDir = errors.New("state override points inside the legacy config dir")

// renameDir is the no-overwrite directory rename; tests replace it.
var renameDir = renameNoReplace

// isWithin reports whether path is dir or lies below it, comparing both the
// lexical paths and, where they resolve, their symlink-free forms.
func isWithin(path, dir string) bool {
	within := func(p, d string) bool {
		rel, err := filepath.Rel(d, p)
		return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return false
	}
	if within(absPath, absDir) {
		return true
	}
	realDir, err := filepath.EvalSymlinks(absDir)
	if err != nil {
		return false
	}
	for parent := filepath.Dir(absPath); ; parent = filepath.Dir(parent) {
		if realParent, err := filepath.EvalSymlinks(parent); err == nil {
			rest, _ := filepath.Rel(parent, absPath)
			return within(filepath.Join(realParent, rest), realDir)
		}
		if next := filepath.Dir(parent); next == parent {
			return false
		}
	}
}

// MigrateLegacyConfig moves ~/.config/clipboard-share to ~/.config/nudge when
// only the old directory exists, so already paired devices keep working. Only
// the server calls it, before loading state. An existing new directory is
// never overwritten, nothing is created when neither exists, and the move is
// skipped (ErrStateInLegacyDir) when the state override points inside the old
// directory. On error the old directory is left untouched and moved is false;
// callers should log the error and continue with EffectiveConfigDir, which
// then still resolves to the old directory.
func MigrateLegacyConfig() (moved bool, err error) {
	oldDir, newDir := LegacyConfigDir(), ConfigDir()
	if _, err := os.Lstat(newDir); err == nil {
		return false, nil
	} else if !os.IsNotExist(err) {
		return false, fmt.Errorf("inspect %s: %w", newDir, err)
	}
	if _, err := os.Lstat(oldDir); os.IsNotExist(err) {
		return false, nil
	} else if err != nil {
		return false, fmt.Errorf("inspect %s: %w", oldDir, err)
	}
	if state := EnvOverride("STATE"); state != "" && isWithin(state, oldDir) {
		return false, fmt.Errorf("%w: %s", ErrStateInLegacyDir, state)
	}
	if err := renameDir(oldDir, newDir); err != nil {
		if errors.Is(err, os.ErrExist) {
			// Another process created the new dir first; keep both as-is.
			return false, nil
		}
		return false, fmt.Errorf("move %s to %s: %w", oldDir, newDir, err)
	}
	return true, nil
}

package pair

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
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

// ConfigDir returns the per-user state directory, ~/.config/nudge.
func ConfigDir() string {
	return filepath.Join(userConfigRoot(), configDirName)
}

// LegacyConfigDir returns the pre-rename state directory,
// ~/.config/clipboard-share.
func LegacyConfigDir() string {
	return filepath.Join(userConfigRoot(), legacyConfigDirName)
}

// StatePath returns NUDGE_STATE (or CLIPBOARD_SHARE_STATE) or the default
// per-user state file.
func StatePath() string {
	if path := EnvOverride("STATE"); path != "" {
		return path
	}
	return filepath.Join(ConfigDir(), "state.json")
}

// ControlPath returns NUDGE_CTL_SOCK (or CLIPBOARD_SHARE_CTL_SOCK) or the
// default control socket.
func ControlPath() string {
	if path := EnvOverride("CTL_SOCK"); path != "" {
		return path
	}
	return filepath.Join(ConfigDir(), "ctl.sock")
}

// MigrateLegacyConfig moves ~/.config/clipboard-share to ~/.config/nudge when
// only the old directory exists, so already paired devices keep working. An
// existing new directory is never overwritten, and nothing is created when
// neither exists. On error the old directory is left untouched and moved is
// false; callers should log the error and continue.
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
	if err := renameNoReplace(oldDir, newDir); err != nil {
		if errors.Is(err, os.ErrExist) {
			// Another process created the new dir first; keep both as-is.
			return false, nil
		}
		return false, fmt.Errorf("move %s to %s: %w", oldDir, newDir, err)
	}
	return true, nil
}

package pair

import (
	"os"
	"path/filepath"
	"runtime"
)

const configDirName = "nudge"

// EnvOverride returns NUDGE_<suffix>. An empty value counts as unset.
func EnvOverride(suffix string) string {
	return os.Getenv("NUDGE_" + suffix)
}

// Platform seams; tests replace them to exercise the Windows rules on Linux.
var (
	hostOS        = runtime.GOOS
	userConfigDir = os.UserConfigDir
	chmodPath     = os.Chmod
)

// userConfigRoot is ~/.config on Linux (and other Unix systems), and
// %APPDATA% (os.UserConfigDir) on Windows, falling back to
// <home>\AppData\Roaming when APPDATA is unset.
func userConfigRoot() string {
	if hostOS == "windows" {
		if dir, err := userConfigDir(); err == nil && dir != "" {
			return dir
		}
		home, err := os.UserHomeDir()
		if err != nil || home == "" {
			return "."
		}
		return filepath.Join(home, "AppData", "Roaming")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".config")
}

// ConfigDir returns the per-user state directory: ~/.config/nudge on
// Linux, %APPDATA%\nudge on Windows.
func ConfigDir() string {
	return filepath.Join(userConfigRoot(), configDirName)
}

// StatePath returns NUDGE_STATE or state.json in the config dir.
func StatePath() string {
	if path := EnvOverride("STATE"); path != "" {
		return path
	}
	return filepath.Join(ConfigDir(), "state.json")
}

// ControlPath returns NUDGE_CTL_SOCK or ctl.sock in the config dir.
func ControlPath() string {
	if path := EnvOverride("CTL_SOCK"); path != "" {
		return path
	}
	return filepath.Join(ConfigDir(), "ctl.sock")
}

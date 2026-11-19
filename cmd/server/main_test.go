package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"nudge/internal/pair"
)

func TestInstanceNameFallsBackToNudge(t *testing.T) {
	cases := []struct {
		host string
		err  error
		want string
	}{
		{"", errors.New("no hostname"), "nudge"},
		{"  ", nil, "nudge"},
		{" omarchy\n", nil, "omarchy"},
		{strings.Repeat("a", 70), nil, strings.Repeat("a", 60)},
	}
	for _, c := range cases {
		if got := instanceNameFrom(c.host, c.err); got != c.want {
			t.Errorf("instanceNameFrom(%q, %v) = %q, want %q", c.host, c.err, got, c.want)
		}
	}
}

func TestServerPortPrefersNudgeAndFallsBack(t *testing.T) {
	t.Setenv("NUDGE_PORT", "")
	t.Setenv("CLIPBOARD_SHARE_PORT", "")
	if got := serverPort(); got != "8765" {
		t.Fatalf("default port = %q, want 8765", got)
	}
	t.Setenv("CLIPBOARD_SHARE_PORT", "9000")
	if got := serverPort(); got != "9000" {
		t.Fatalf("old name only = %q, want 9000", got)
	}
	t.Setenv("NUDGE_PORT", "9100")
	if got := serverPort(); got != "9100" {
		t.Fatalf("both names = %q, want 9100", got)
	}
}

func TestPairNotificationUsesNudge(t *testing.T) {
	title, body := pairNotification("Pixel 8", "6B86-B273-FF34")
	if title != "Nudge" {
		t.Fatalf("title = %q, want Nudge", title)
	}
	if want := "Pair request from Pixel 8, code 6B86-B273-FF34 — nudge pair --last"; body != want {
		t.Fatalf("body = %q, want %q", body, want)
	}
}

func TestPairClientUsageUsesNudge(t *testing.T) {
	var stdout, stderr bytes.Buffer
	t.Setenv("HOME", t.TempDir())
	code := runPairClient([]string{"bogus"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	want := "nudge pair: invalid arguments\nusage: nudge pair list | --last | deny --request <id> | revoke --device <id>\n"
	if stderr.String() != want {
		t.Fatalf("stderr = %q, want %q", stderr.String(), want)
	}
}

func TestPairClientUnreachableServerMessage(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "ctl.sock")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("NUDGE_CTL_SOCK", sock)
	var stdout, stderr bytes.Buffer
	if code := runPairClient([]string{"list"}, &stdout, &stderr); code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	prefix := "nudge pair: cannot connect to server at " + sock + ": "
	if !strings.HasPrefix(stderr.String(), prefix) || !strings.HasSuffix(stderr.String(), "(is the server running?)\n") {
		t.Fatalf("stderr = %q, want prefix %q", stderr.String(), prefix)
	}
}

func legacyOnlyHome(t *testing.T) (home, oldDir string) {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("the legacy clipboard-share config only exists on Linux")
	}
	home = t.TempDir()
	t.Setenv("HOME", home)
	for _, name := range []string{"NUDGE_STATE", "CLIPBOARD_SHARE_STATE", "NUDGE_CTL_SOCK", "CLIPBOARD_SHARE_CTL_SOCK"} {
		t.Setenv(name, "")
	}
	oldDir = filepath.Join(home, ".config", "clipboard-share")
	if err := os.MkdirAll(oldDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(oldDir, "state.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	return home, oldDir
}

// The CLI never migrates: it reaches the socket of the effective (legacy)
// dir, so a not-yet-migrated or still-running old server stays reachable.
func TestPairClientDoesNotMigrateAndUsesLegacySocket(t *testing.T) {
	home, oldDir := legacyOnlyHome(t)
	var stdout, stderr bytes.Buffer
	if code := runPairClient([]string{"list"}, &stdout, &stderr); code != 1 {
		t.Fatalf("exit code = %d, want 1 (no server)", code)
	}
	data, err := os.ReadFile(filepath.Join(oldDir, "state.json"))
	if err != nil || string(data) != "{}" {
		t.Fatalf("legacy state = %q, err = %v; want untouched", data, err)
	}
	newDir := filepath.Join(home, ".config", "nudge")
	if _, err := os.Lstat(newDir); !os.IsNotExist(err) {
		t.Fatalf("%s exists (err=%v), want missing", newDir, err)
	}
	if got := pair.EffectiveConfigDir(); got != oldDir {
		t.Fatalf("EffectiveConfigDir() = %q, want %q", got, oldDir)
	}
	want := "nudge pair: cannot connect to server at " + filepath.Join(oldDir, "ctl.sock") + ": "
	if !strings.HasPrefix(stderr.String(), want) {
		t.Fatalf("stderr = %q, want prefix %q", stderr.String(), want)
	}
}

func TestServerMigrateConfigMovesLegacyDir(t *testing.T) {
	home, oldDir := legacyOnlyHome(t)
	var logs []string
	migrateConfig(func(format string, args ...any) { logs = append(logs, fmt.Sprintf(format, args...)) })
	newDir := filepath.Join(home, ".config", "nudge")
	data, err := os.ReadFile(filepath.Join(newDir, "state.json"))
	if err != nil || string(data) != "{}" {
		t.Fatalf("migrated state = %q, err = %v", data, err)
	}
	if _, err := os.Lstat(oldDir); !os.IsNotExist(err) {
		t.Fatalf("old dir still present: %v", err)
	}
	if want := "nudge: moved " + oldDir + " to " + newDir + "\n"; len(logs) != 1 || logs[0] != want {
		t.Fatalf("logs = %q, want [%q]", logs, want)
	}
}

func TestServerMigrateConfigSkipsStateOverrideInLegacyDir(t *testing.T) {
	home, oldDir := legacyOnlyHome(t)
	t.Setenv("CLIPBOARD_SHARE_STATE", filepath.Join(oldDir, "state.json"))
	var logs []string
	migrateConfig(func(format string, args ...any) { logs = append(logs, fmt.Sprintf(format, args...)) })
	if _, err := os.Stat(filepath.Join(oldDir, "state.json")); err != nil {
		t.Fatalf("legacy state moved: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(home, ".config", "nudge")); !os.IsNotExist(err) {
		t.Fatalf("new dir created: %v", err)
	}
	if len(logs) != 1 || !strings.HasPrefix(logs[0], "nudge: config migration skipped, "+oldDir+" left untouched: ") {
		t.Fatalf("logs = %q", logs)
	}
}

// Existing CLI parsing is unchanged by the rename.
func TestPairClientCommandUnchanged(t *testing.T) {
	cases := map[string]map[string]string{
		"list":               {"cmd": "list"},
		"--last":             {"cmd": "approve-last"},
		"deny --request r1":  {"cmd": "deny", "request": "r1"},
		"revoke --device d1": {"cmd": "revoke", "device": "d1"},
	}
	for args, want := range cases {
		got, err := pairClientCommand(strings.Fields(args))
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("pairClientCommand(%q) = %v, %v; want %v", args, got, err, want)
		}
	}
	if _, err := pairClientCommand([]string{"deny", "--request"}); err == nil {
		t.Error("incomplete deny accepted")
	}
}

// S5: `nudge pair list` shows the verification code first, then the lists
// exactly as before.
func TestPairListPrintsTheVerificationCodeFirst(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("NUDGE_STATE", filepath.Join(dir, "state.json"))
	t.Setenv("NUDGE_CTL_SOCK", filepath.Join(dir, "ctl.sock"))
	b := pair.NewBroker()
	b.SetVerification("6b86b273ff34fce19d6b804eff5a3f5747ada4eaa22f1d49c01e52ddb7875b4b", "6B86-B273-FF34")
	control, err := pair.ListenControl(b)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { control.Close() })

	var stdout, stderr bytes.Buffer
	if code := runPairClient([]string{"list"}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit code = %d, stderr %q", code, stderr.String())
	}
	want := "Verification code: 6B86-B273-FF34\nPending requests:\n  (none)\nApproved devices:\n  (none)\n"
	if stdout.String() != want || stderr.Len() != 0 {
		t.Fatalf("stdout = %q, stderr = %q; want %q", stdout.String(), stderr.String(), want)
	}

	// Other commands print only their message, as before.
	stdout.Reset()
	if code := runPairClient([]string{"--last"}, &stdout, &stderr); code != 1 {
		t.Fatalf("--last with nothing pending: exit %d, want 1", code)
	}
	if stdout.Len() != 0 || stderr.String() != "nudge pair: "+pair.ErrNotFound.Error()+"\n" {
		t.Fatalf("--last stdout = %q, stderr = %q", stdout.String(), stderr.String())
	}
}

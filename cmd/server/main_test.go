package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
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
	title, body := pairNotification("Pixel 8")
	if title != "Nudge" {
		t.Fatalf("title = %q, want Nudge", title)
	}
	if want := "Pair request from Pixel 8 — nudge pair --last"; body != want {
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

func TestPairClientMigratesLegacyConfigDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("NUDGE_CTL_SOCK", "")
	t.Setenv("CLIPBOARD_SHARE_CTL_SOCK", "")
	oldDir := filepath.Join(home, ".config", "clipboard-share")
	if err := os.MkdirAll(oldDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(oldDir, "state.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	runPairClient([]string{"list"}, &stdout, &stderr)
	data, err := os.ReadFile(filepath.Join(home, ".config", "nudge", "state.json"))
	if err != nil || string(data) != "{}" {
		t.Fatalf("migrated state = %q, err = %v", data, err)
	}
	if _, err := os.Lstat(oldDir); !os.IsNotExist(err) {
		t.Fatalf("old dir still present: %v", err)
	}
	if !strings.Contains(stderr.String(), filepath.Join(home, ".config", "nudge", "ctl.sock")) {
		t.Fatalf("stderr = %q, want the new control socket path", stderr.String())
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

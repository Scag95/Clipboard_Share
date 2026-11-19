package main

import (
	"bytes"
	"errors"
	"path/filepath"
	"reflect"
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

func TestServerPortReadsNudgePort(t *testing.T) {
	t.Setenv("NUDGE_PORT", "")
	if got := serverPort(); got != "8765" {
		t.Fatalf("default port = %q, want 8765", got)
	}
	t.Setenv("NUDGE_PORT", "9100")
	if got := serverPort(); got != "9100" {
		t.Fatalf("NUDGE_PORT = %q, want 9100", got)
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

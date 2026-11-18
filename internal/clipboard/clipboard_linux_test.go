//go:build linux

package clipboard

import (
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"os/exec"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestNewBackend(t *testing.T) {
	originalLookPath := lookPath
	t.Cleanup(func() { lookPath = originalLookPath })

	t.Run("Linux without Wayland keeps the library backend", func(t *testing.T) {
		t.Setenv("WAYLAND_DISPLAY", "")
		lookPath = func(string) (string, error) {
			t.Fatal("LookPath must not be called without Wayland")
			return "", nil
		}

		backend := newBackend()
		if _, ok := backend.(libraryBackend); !ok {
			t.Fatalf("newBackend() returned %T, want libraryBackend", backend)
		}
	})

	t.Run("Wayland without wl-copy keeps the library backend", func(t *testing.T) {
		t.Setenv("WAYLAND_DISPLAY", "wayland-test")
		lookPath = func(name string) (string, error) {
			if name != "wl-copy" {
				t.Fatalf("LookPath called with %q, want wl-copy", name)
			}
			return "", exec.ErrNotFound
		}

		backend := newBackend()
		if _, ok := backend.(libraryBackend); !ok {
			t.Fatalf("newBackend() returned %T, want libraryBackend", backend)
		}
	})

	t.Run("Wayland with wl-copy uses native commands", func(t *testing.T) {
		t.Setenv("WAYLAND_DISPLAY", "wayland-test")
		lookPath = func(name string) (string, error) {
			if name != "wl-copy" {
				t.Fatalf("LookPath called with %q, want wl-copy", name)
			}
			return "/usr/bin/wl-copy", nil
		}

		backend := newBackend()
		if _, ok := backend.(wlClipboard); !ok {
			t.Fatalf("newBackend() returned %T, want wlClipboard", backend)
		}
	})
}

func TestWaylandTextCommands(t *testing.T) {
	t.Run("read", func(t *testing.T) {
		useFakeWayland(t, fakeCommand{
			name:   "wl-paste",
			args:   []string{"--no-newline"},
			stdout: []byte("hello from Wayland"),
		})

		got, err := GetText()
		if err != nil {
			t.Fatalf("GetText() error = %v", err)
		}
		if got != "hello from Wayland" {
			t.Fatalf("GetText() = %q, want %q", got, "hello from Wayland")
		}
	})

	t.Run("write", func(t *testing.T) {
		useFakeWayland(t, fakeCommand{
			name:  "wl-copy",
			stdin: []byte("hello from Wayland"),
		})

		if err := SetText("hello from Wayland"); err != nil {
			t.Fatalf("SetText() error = %v", err)
		}
	})
}

func TestWaylandImageCommands(t *testing.T) {
	png := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}

	t.Run("read", func(t *testing.T) {
		useFakeWayland(t, fakeCommand{
			name:   "wl-paste",
			args:   []string{"--type", "image/png"},
			stdout: png,
		})

		got, err := GetImage()
		if err != nil {
			t.Fatalf("GetImage() error = %v", err)
		}
		if !reflect.DeepEqual(got, png) {
			t.Fatalf("GetImage() = %v, want %v", got, png)
		}
	})

	t.Run("write", func(t *testing.T) {
		useFakeWayland(t, fakeCommand{
			name:  "wl-copy",
			args:  []string{"-t", "image/png"},
			stdin: png,
		})

		if err := SetImage(png); err != nil {
			t.Fatalf("SetImage() error = %v", err)
		}
	})
}

func TestWaylandEmptyClipboard(t *testing.T) {
	for _, test := range []struct {
		name   string
		stderr string
		read   func() error
	}{
		{
			name:   "text",
			stderr: "wl-paste: Nothing is copied\n",
			read: func() error {
				got, err := GetText()
				if err == nil && got != "" {
					return fmt.Errorf("GetText() = %q, want empty", got)
				}
				return err
			},
		},
		{
			name:   "image with empty clipboard",
			stderr: "wl-paste: Nothing is copied\n",
			read: func() error {
				got, err := GetImage()
				if err == nil && got != nil {
					return fmt.Errorf("GetImage() = %v, want nil", got)
				}
				return err
			},
		},
		{
			name:   "image with no suitable type",
			stderr: "wl-paste: No suitable type of content copied\n",
			read: func() error {
				got, err := GetImage()
				if err == nil && got != nil {
					return fmt.Errorf("GetImage() = %v, want nil", got)
				}
				return err
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			command := fakeCommand{
				name:     "wl-paste",
				args:     []string{"--no-newline"},
				stderr:   test.stderr,
				exitCode: 1,
			}
			if strings.HasPrefix(test.name, "image") {
				command.args = []string{"--type", "image/png"}
			}
			useFakeWayland(t, command)

			if err := test.read(); err != nil {
				t.Fatalf("empty clipboard read error = %v, want nil", err)
			}
		})
	}
}

func TestWaylandPasteFailureIsReturned(t *testing.T) {
	useFakeWayland(t, fakeCommand{
		name:     "wl-paste",
		args:     []string{"--no-newline"},
		stderr:   "wl-paste: cannot connect to Wayland display\n",
		exitCode: 1,
	})

	if _, err := GetText(); err == nil {
		t.Fatal("GetText() error = nil, want wl-paste failure")
	}
}

func TestSetImageRejectsEmptyDataBeforeRunningCommand(t *testing.T) {
	originalBackend := systemBackend
	originalExecCommand := execCommand
	systemBackend = wlClipboard{}
	execCommand = func(string, ...string) *exec.Cmd {
		t.Fatal("execCommand called for empty image")
		return nil
	}
	t.Cleanup(func() {
		systemBackend = originalBackend
		execCommand = originalExecCommand
	})

	if err := SetImage(nil); err == nil || err.Error() != "empty image data" {
		t.Fatalf("SetImage(nil) error = %v, want empty image data", err)
	}
}

func TestWaylandTextRoundtrip(t *testing.T) {
	if os.Getenv("WAYLAND_DISPLAY") == "" {
		t.Skip("WAYLAND_DISPLAY is not set")
	}
	if _, err := exec.LookPath("wl-copy"); err != nil {
		t.Skipf("wl-copy is not installed: %v", err)
	}
	if _, err := exec.LookPath("wl-paste"); err != nil {
		t.Skipf("wl-paste is not installed: %v", err)
	}

	originalBackend := systemBackend
	systemBackend = newBackend()
	t.Cleanup(func() { systemBackend = originalBackend })
	if _, ok := systemBackend.(wlClipboard); !ok {
		t.Skipf("native Wayland backend unavailable, got %T", systemBackend)
	}

	if err := Init(); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	want := fmt.Sprintf("nudge Wayland roundtrip %d", time.Now().UnixNano())
	if err := SetText(want); err != nil {
		t.Fatalf("SetText() error = %v", err)
	}
	got, err := GetText()
	if err != nil {
		t.Fatalf("GetText() error = %v", err)
	}
	if got != want {
		t.Fatalf("GetText() = %q, want %q", got, want)
	}
}

type fakeCommand struct {
	name     string
	args     []string
	stdin    []byte
	stdout   []byte
	stderr   string
	exitCode int
	hang     bool
}

func useFakeWayland(t *testing.T, expected fakeCommand) {
	t.Helper()
	originalBackend := systemBackend
	originalExecCommand := execCommand
	systemBackend = wlClipboard{}
	execCommand = fakeExecCommand
	t.Cleanup(func() {
		systemBackend = originalBackend
		execCommand = originalExecCommand
	})

	t.Setenv("CLIPBOARD_TEST_COMMAND", expected.name)
	t.Setenv("CLIPBOARD_TEST_ARGS", strings.Join(expected.args, "\x1f"))
	t.Setenv("CLIPBOARD_TEST_STDIN", base64.StdEncoding.EncodeToString(expected.stdin))
	t.Setenv("CLIPBOARD_TEST_STDOUT", base64.StdEncoding.EncodeToString(expected.stdout))
	t.Setenv("CLIPBOARD_TEST_STDERR", expected.stderr)
	t.Setenv("CLIPBOARD_TEST_EXIT", fmt.Sprint(expected.exitCode))
	t.Setenv("CLIPBOARD_TEST_HANG", fmt.Sprint(expected.hang))
}

func TestWaylandPasteHangIsKilledAtDeadline(t *testing.T) {
	useFakeWayland(t, fakeCommand{name: "wl-paste", args: []string{"--no-newline"}, hang: true})
	start := time.Now()
	_, err := GetText()
	elapsed := time.Since(start)
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("GetText() error = %v, want timeout kill", err)
	}
	if elapsed > 2*wlProcessTimeout {
		t.Fatalf("hang was bounded by %v, want near %v", elapsed, wlProcessTimeout)
	}
}

func fakeExecCommand(command string, args ...string) *exec.Cmd {
	helperArgs := append([]string{"-test.run=TestClipboardHelperProcess", "--", command}, args...)
	cmd := exec.Command(os.Args[0], helperArgs...)
	cmd.Env = append(os.Environ(), "GO_WANT_CLIPBOARD_HELPER=1")
	return cmd
}

func TestClipboardHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_CLIPBOARD_HELPER") != "1" {
		return
	}

	separator := -1
	for i, arg := range os.Args {
		if arg == "--" {
			separator = i
			break
		}
	}
	if separator < 0 || separator+1 >= len(os.Args) {
		fmt.Fprintln(os.Stderr, "helper command is missing")
		os.Exit(2)
	}
	command := os.Args[separator+1]
	args := os.Args[separator+2:]
	if os.Getenv("CLIPBOARD_TEST_HANG") == "true" {
		select {} // never answers; the caller must kill us at the deadline
	}
	if command != os.Getenv("CLIPBOARD_TEST_COMMAND") {
		fmt.Fprintf(os.Stderr, "command = %q, want %q\n", command, os.Getenv("CLIPBOARD_TEST_COMMAND"))
		os.Exit(2)
	}
	var expectedArgs []string
	if value := os.Getenv("CLIPBOARD_TEST_ARGS"); value != "" {
		expectedArgs = strings.Split(value, "\x1f")
	}
	if !slices.Equal(args, expectedArgs) {
		fmt.Fprintf(os.Stderr, "args = %q, want %q\n", args, expectedArgs)
		os.Exit(2)
	}

	stdin, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read stdin: %v\n", err)
		os.Exit(2)
	}
	expectedStdin, err := base64.StdEncoding.DecodeString(os.Getenv("CLIPBOARD_TEST_STDIN"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "decode expected stdin: %v\n", err)
		os.Exit(2)
	}
	if !reflect.DeepEqual(stdin, expectedStdin) {
		fmt.Fprintf(os.Stderr, "stdin = %v, want %v\n", stdin, expectedStdin)
		os.Exit(2)
	}

	stdout, err := base64.StdEncoding.DecodeString(os.Getenv("CLIPBOARD_TEST_STDOUT"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "decode expected stdout: %v\n", err)
		os.Exit(2)
	}
	if _, err := os.Stdout.Write(stdout); err != nil {
		fmt.Fprintf(os.Stderr, "write stdout: %v\n", err)
		os.Exit(2)
	}
	fmt.Fprint(os.Stderr, os.Getenv("CLIPBOARD_TEST_STDERR"))
	if code := os.Getenv("CLIPBOARD_TEST_EXIT"); code != "0" {
		var exitCode int
		if _, err := fmt.Sscan(code, &exitCode); err != nil {
			fmt.Fprintf(os.Stderr, "invalid exit code: %v\n", err)
			os.Exit(2)
		}
		os.Exit(exitCode)
	}
	os.Exit(0)
}

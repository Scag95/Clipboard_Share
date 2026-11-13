//go:build linux

package clipboard

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// wlDaemonGrace bounds how long we wait for copied output pipes to close
// after wl-copy exits. wl-copy daemonizes to serve the new clipboard content
// and the daemon inherits our stderr pipe, so without a WaitDelay cmd.Run
// would block until the daemon loses clipboard ownership.
const wlDaemonGrace = 200 * time.Millisecond

var (
	execCommand = exec.Command
	lookPath    = exec.LookPath
)

var errWaylandNoData = errors.New("wayland clipboard has no data")

type wlClipboard struct{}

func (wlClipboard) Init() error {
	return nil
}

func (wlClipboard) GetText() (string, error) {
	data, err := runWlPaste("--no-newline")
	if errors.Is(err, errWaylandNoData) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (wlClipboard) SetText(text string) error {
	return runWlCopy([]byte(text))
}

func (wlClipboard) GetImage() ([]byte, error) {
	data, err := runWlPaste("--type", "image/png")
	if errors.Is(err, errWaylandNoData) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return data, nil
}

func (wlClipboard) SetImage(data []byte) error {
	return runWlCopy(data, "-t", "image/png")
}

func runWlCopy(data []byte, args ...string) error {
	cmd := execCommand("wl-copy", args...)
	cmd.Stdin = bytes.NewReader(data)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.WaitDelay = wlDaemonGrace
	err := cmd.Run()
	if err != nil {
		// The daemon keeps our stderr pipe open until it loses clipboard
		// ownership, which fires WaitDelay after a successful copy. Only a
		// non-zero wl-copy exit is a real failure.
		if errors.Is(err, exec.ErrWaitDelay) && cmd.ProcessState != nil &&
			cmd.ProcessState.ExitCode() == 0 {
			return nil
		}
		return commandError("wl-copy", err, stderr.String())
	}
	return nil
}

func runWlPaste(args ...string) ([]byte, error) {
	cmd := execCommand("wl-paste", args...)
	data, err := cmd.Output()
	if err == nil {
		return data, nil
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		stderr := string(exitErr.Stderr)
		if strings.Contains(stderr, "Nothing is copied") ||
			strings.Contains(stderr, "No suitable type of content copied") {
			return nil, errWaylandNoData
		}
		return nil, commandError("wl-paste", err, stderr)
	}
	return nil, fmt.Errorf("wl-paste: %w", err)
}

func commandError(name string, err error, stderr string) error {
	if message := strings.TrimSpace(stderr); message != "" {
		return fmt.Errorf("%s: %w: %s", name, err, message)
	}
	return fmt.Errorf("%s: %w", name, err)
}

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

// wlProcessTimeout bounds each wl-copy/wl-paste run. A Wayland clipboard
// owner can refuse to answer a data request forever, which would wedge an
// HTTP request; clipboard.go documents every OS clipboard call as bounded.
const wlProcessTimeout = 3 * time.Second

// runBounded waits for a started command, killing it at the deadline so a
// stalled Wayland peer cannot block the caller.
func runBounded(name string, cmd *exec.Cmd) error {
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		return err
	case <-time.After(wlProcessTimeout):
		_ = cmd.Process.Kill()
		err := <-done
		return fmt.Errorf("%s timed out after %s: %w", name, wlProcessTimeout, err)
	}
}

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
	err := runBounded("wl-copy", cmd)
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
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := runBounded("wl-paste", cmd)
	if err == nil {
		return stdout.Bytes(), nil
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		msg := stderr.String()
		if strings.Contains(msg, "Nothing is copied") ||
			strings.Contains(msg, "No suitable type of content copied") {
			return nil, errWaylandNoData
		}
		return nil, commandError("wl-paste", err, msg)
	}
	return nil, commandError("wl-paste", err, stderr.String())
}

func commandError(name string, err error, stderr string) error {
	if message := strings.TrimSpace(stderr); message != "" {
		return fmt.Errorf("%s: %w: %s", name, err, message)
	}
	return fmt.Errorf("%s: %w", name, err)
}

//go:build linux

package input

import (
	"fmt"
	"testing"

	"golang.org/x/sys/unix"
)

// flakyLinuxDevice rejects the first failAfter writes with a transient EAGAIN
// and then behaves like the normal fake device, recording what was accepted.
type flakyLinuxDevice struct {
	fakeLinuxDevice
	failAfter int
	attempts  int
}

func (d *flakyLinuxDevice) WriteEvent(event inputEvent) error {
	d.attempts++
	if d.attempts <= d.failAfter {
		return fmt.Errorf("write: %w", unix.EAGAIN)
	}
	return d.fakeLinuxDevice.WriteEvent(event)
}

func TestLinuxSenderRetriesTransientEAGAIN(t *testing.T) {
	device := &flakyLinuxDevice{failAfter: 2}
	sender := &LinuxSender{device: device}
	if err := sender.Key("c", []string{"ctrl"}); err != nil {
		t.Fatalf("Key error = %v, want nil after EAGAIN drains", err)
	}
	if len(device.events) != 8 {
		t.Fatalf("recorded events = %d, want the full ctrl+c gesture (8)", len(device.events))
	}
}

// releaseFailDevice fails only the write at failOn with a hard error,
// mimicking a burst where a middle event is rejected.
type releaseFailDevice struct {
	fakeLinuxDevice
	failOn int
	calls  int
}

func (d *releaseFailDevice) WriteEvent(event inputEvent) error {
	d.calls++
	if d.calls == d.failOn {
		return fmt.Errorf("write: %w", unix.EIO)
	}
	return d.fakeLinuxDevice.WriteEvent(event)
}

func TestLinuxSenderEmitsReleasesAfterPartialFailure(t *testing.T) {
	device := &releaseFailDevice{failOn: 4}
	sender := &LinuxSender{device: device}
	if err := sender.Key("c", []string{"ctrl"}); err == nil {
		t.Fatal("Key error = nil, want write failure reported")
	}
	// The gesture's final key event is the ctrl-release; it must still be
	// attempted after a middle write failed, so nothing stays held on the
	// desktop.
	var lastKey *inputEvent
	for i := range device.events {
		if device.events[i].Type == evKey {
			lastKey = &device.events[i]
		}
	}
	if lastKey == nil || lastKey.Code != keyLeftCtrl || lastKey.Value != 0 {
		t.Fatalf("last key event = %#v, want ctrl release (value 0)", lastKey)
	}
}

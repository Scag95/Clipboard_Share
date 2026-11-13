//go:build linux

package input

import "testing"

func TestLinuxSenderRejectsUnknownModifierBeforeEvents(t *testing.T) {
	inj, device := newFakeLinuxSender()
	command := KeyPress{Key: "c", Mods: []string{"ctrl", "bogus"}}
	err := command.Run(inj)
	if err == nil || err.Error() != `unknown modifier "bogus"` {
		t.Fatalf("Run() error = %v, want unknown modifier", err)
	}
	if len(device.events) != 0 {
		t.Fatalf("Run() emitted events before rejecting modifier: %#v", device.events)
	}
}

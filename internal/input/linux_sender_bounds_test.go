//go:build linux

package input

import (
	"math"
	"testing"
)

func TestLinuxSenderClampsMove(t *testing.T) {
	for _, test := range []struct {
		name   string
		dx, dy int
	}{
		{"just outside protocol", 1001, -1001},
		{"integer extremes", math.MaxInt, math.MinInt},
	} {
		t.Run(test.name, func(t *testing.T) {
			inj, device := newFakeLinuxSender()
			if err := inj.Move(test.dx, test.dy); err != nil {
				t.Fatal(err)
			}
			if device.events[0].Value != MaxPointerDelta || device.events[1].Value != -MaxPointerDelta {
				t.Fatalf("Move values = [%d %d], want [%d %d]", device.events[0].Value, device.events[1].Value, MaxPointerDelta, -MaxPointerDelta)
			}
		})
	}
}

func TestLinuxSenderClampsScroll(t *testing.T) {
	for _, test := range []struct {
		name   string
		dx, dy int
	}{
		{"just outside protocol", 101, -101},
		{"integer extremes", math.MaxInt, math.MinInt},
	} {
		t.Run(test.name, func(t *testing.T) {
			inj, device := newFakeLinuxSender()
			if err := inj.Scroll(test.dx, test.dy); err != nil {
				t.Fatal(err)
			}
			if device.events[0].Value != MaxScrollNotches || device.events[1].Value != -MaxScrollNotches {
				t.Fatalf("Scroll values = [%d %d], want [%d %d]", device.events[0].Value, device.events[1].Value, MaxScrollNotches, -MaxScrollNotches)
			}
		})
	}
}

func TestLinuxSenderCloseDestroysExactlyOnce(t *testing.T) {
	inj, device := newFakeLinuxSender()
	if err := inj.Close(); err != nil {
		t.Fatal(err)
	}
	if err := inj.Close(); err != nil {
		t.Fatal(err)
	}
	if device.closes != 1 {
		t.Fatalf("Close calls = %d, want 1", device.closes)
	}
}

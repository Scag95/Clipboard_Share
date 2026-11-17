//go:build linux

package input

import (
	"reflect"
	"testing"
)

type fakeLinuxDevice struct {
	events []inputEvent
	closes int
}

func (f *fakeLinuxDevice) WriteEvent(event inputEvent) error {
	f.events = append(f.events, event)
	return nil
}

func (f *fakeLinuxDevice) Close() error {
	f.closes++
	return nil
}

func newFakeLinuxSender() (*LinuxSender, *fakeLinuxDevice) {
	device := &fakeLinuxDevice{}
	return &LinuxSender{device: device}, device
}

func keyEvent(code uint16, value int32) []inputEvent {
	return []inputEvent{
		{Type: evKey, Code: code, Value: value},
		{Type: evSyn, Code: synReport},
	}
}

func appendEvents(groups ...[]inputEvent) []inputEvent {
	var out []inputEvent
	for _, group := range groups {
		out = append(out, group...)
	}
	return out
}

func TestLinuxKeyMappingCoversProtocolKeys(t *testing.T) {
	for name := range namedKeys {
		if code := linuxKeyCode(name); code == 0 {
			t.Errorf("linuxKeyCode(%q) = 0", name)
		}
	}
	for name := 'a'; name <= 'z'; name++ {
		if code := linuxKeyCode(string(name)); code == 0 {
			t.Errorf("linuxKeyCode(%q) = 0", name)
		}
	}
	for name := '0'; name <= '9'; name++ {
		if code := linuxKeyCode(string(name)); code == 0 {
			t.Errorf("linuxKeyCode(%q) = 0", name)
		}
	}

	explicit := map[string]uint16{
		"enter": 28, "esc": 1, "tab": 15, "backspace": 14,
		"delete": 111, "insert": 110, "arrowup": 103,
		"arrowdown": 108, "arrowleft": 105, "arrowright": 106,
		"home": 102, "end": 107, "pageup": 104, "pagedown": 109,
		"space": 57, "a": 30, "z": 44, "0": 11, "9": 10,
		"f1": 59, "f10": 68, "f11": 87, "f12": 88,
	}
	for name, want := range explicit {
		if got := linuxKeyCode(name); got != want {
			t.Errorf("linuxKeyCode(%q) = %d, want %d", name, got, want)
		}
	}
	if got := linuxKeyCode("f19"); got != 0 {
		t.Errorf("linuxKeyCode(f19) = %d, want 0", got)
	}

	mods := map[string]uint16{"ctrl": 29, "alt": 56, "shift": 42, "win": 125}
	for name, want := range mods {
		if got := linuxModCode(name); got != want {
			t.Errorf("linuxModCode(%q) = %d, want %d", name, got, want)
		}
	}
}

func TestLinuxTextMappingCoversPrintableASCII(t *testing.T) {
	for r := rune(0x20); r <= 0x7e; r++ {
		if code, _, ok := linuxTextKey(r); !ok || code == 0 {
			t.Errorf("linuxTextKey(%q) = (%d, _, %v), want a key", r, code, ok)
		}
	}
	for _, r := range []rune{'A', 'Z', '!', '+', '{', '"', '?', '~'} {
		if _, shift, ok := linuxTextKey(r); !ok || !shift {
			t.Errorf("linuxTextKey(%q) should require shift", r)
		}
	}
	for _, r := range []rune{'a', 'z', '0', '9', '-', '=', ',', ' '} {
		if _, shift, ok := linuxTextKey(r); !ok || shift {
			t.Errorf("linuxTextKey(%q) should not require shift", r)
		}
	}
	if _, _, ok := linuxTextKey('ñ'); ok {
		t.Error("non-ASCII text must not have a Linux key mapping")
	}
}

func TestLinuxSenderMoveAndScroll(t *testing.T) {
	sender, device := newFakeLinuxSender()
	var injector Injector = sender
	if err := injector.Move(4, -2); err != nil {
		t.Fatal(err)
	}
	wantMove := []inputEvent{
		{Type: evRel, Code: relX, Value: 4},
		{Type: evRel, Code: relY, Value: -2},
		{Type: evSyn, Code: synReport},
	}
	if !reflect.DeepEqual(device.events, wantMove) {
		t.Fatalf("Move events = %#v, want %#v", device.events, wantMove)
	}

	device.events = nil
	if err := injector.Scroll(3, 2); err != nil {
		t.Fatal(err)
	}
	wantScroll := []inputEvent{
		{Type: evRel, Code: relWheel, Value: -2},
		{Type: evRel, Code: relHWheel, Value: -3},
		{Type: evSyn, Code: synReport},
	}
	if !reflect.DeepEqual(device.events, wantScroll) {
		t.Fatalf("Scroll events = %#v, want %#v", device.events, wantScroll)
	}
}

func TestLinuxSenderClickModes(t *testing.T) {
	for _, test := range []struct {
		mode string
		want []inputEvent
	}{
		{"down", keyEvent(btnLeft, 1)},
		{"up", keyEvent(btnLeft, 0)},
		{"tap", appendEvents(keyEvent(btnLeft, 1), keyEvent(btnLeft, 0))},
	} {
		t.Run(test.mode, func(t *testing.T) {
			sender, device := newFakeLinuxSender()
			if err := sender.Click("left", test.mode); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(device.events, test.want) {
				t.Fatalf("Click events = %#v, want %#v", device.events, test.want)
			}
		})
	}
}

func TestLinuxSenderKeyHoldsModifiersInOrder(t *testing.T) {
	sender, device := newFakeLinuxSender()
	if err := sender.Key("c", []string{"ctrl", "shift", "win"}); err != nil {
		t.Fatal(err)
	}
	want := appendEvents(
		keyEvent(keyLeftCtrl, 1),
		keyEvent(keyLeftShift, 1),
		keyEvent(keyLeftMeta, 1),
		keyEvent(keyC, 1),
		keyEvent(keyC, 0),
		keyEvent(keyLeftMeta, 0),
		keyEvent(keyLeftShift, 0),
		keyEvent(keyLeftCtrl, 0),
	)
	if !reflect.DeepEqual(device.events, want) {
		t.Fatalf("Key events = %#v, want %#v", device.events, want)
	}
}

func TestLinuxSenderTextASCIIIgnoresNonASCII(t *testing.T) {
	sender, device := newFakeLinuxSender()
	if err := sender.Text("Añ"); err != nil {
		t.Fatalf("Text returned an error for non-ASCII input: %v", err)
	}
	want := appendEvents(
		keyEvent(keyLeftShift, 1),
		keyEvent(keyA, 1),
		keyEvent(keyA, 0),
		keyEvent(keyLeftShift, 0),
	)
	if !reflect.DeepEqual(device.events, want) {
		t.Fatalf("Text events = %#v, want %#v", device.events, want)
	}

	device.events = nil
	if err := sender.Text("ñ🙂"); err != nil {
		t.Fatalf("Text(non-ASCII) returned an error: %v", err)
	}
	if len(device.events) != 0 {
		t.Fatalf("Text(non-ASCII) emitted events: %#v", device.events)
	}
}

func TestLinuxSenderRejectsInvalidDirectCallsWithoutEvents(t *testing.T) {
	sender, device := newFakeLinuxSender()
	for _, call := range []func() error{
		func() error { return sender.Click("middle", "tap") },
		func() error { return sender.Click("left", "double") },
		func() error { return sender.Key("f19", nil) },
	} {
		if err := call(); err == nil {
			t.Fatal("invalid direct call returned nil error")
		}
	}
	if len(device.events) != 0 {
		t.Fatalf("invalid calls emitted events: %#v", device.events)
	}
}

func TestNewWithoutDisplayUsesRecorder(t *testing.T) {
	t.Setenv("WAYLAND_DISPLAY", "")
	t.Setenv("DISPLAY", "")
	injector := New()
	if _, ok := injector.(*Recorder); !ok {
		t.Fatalf("New() without a display = %T, want *Recorder", injector)
	}
}

func TestNewLinuxUinputWhenAvailable(t *testing.T) {
	injector, err := NewLinux()
	if err != nil {
		t.Skipf("/dev/uinput is unavailable: %v", err)
	}
	closer, ok := injector.(interface{ Close() error })
	if !ok {
		t.Fatal("Linux injector does not expose Close for UI_DEV_DESTROY")
	}
	if err := closer.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

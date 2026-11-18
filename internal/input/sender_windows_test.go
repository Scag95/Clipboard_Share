//go:build windows && (amd64 || arm64)

package input

import (
	"testing"
	"unsafe"
)

// Win32 INPUT on 64-bit Windows: DWORD type at 0, the union at offset 8,
// sizeof(INPUT) == 40 (MOUSEINPUT is 32 bytes with dwExtraInfo at 24).
// SendInput rejects any other cbSize, so these offsets must never drift.
func TestRawInputLayoutMatchesWin32(t *testing.T) {
	var m rawMouse
	var k rawKey
	checks := []struct {
		name      string
		got, want uintptr
	}{
		{"sizeof(rawMouse)", unsafe.Sizeof(m), 40},
		{"sizeof(rawKey)", unsafe.Sizeof(k), 40},
		{"rawMouse.Type", unsafe.Offsetof(m.Type), 0},
		{"rawMouse.Dx", unsafe.Offsetof(m.Dx), 8},
		{"rawMouse.Dy", unsafe.Offsetof(m.Dy), 12},
		{"rawMouse.MouseData", unsafe.Offsetof(m.MouseData), 16},
		{"rawMouse.Flags", unsafe.Offsetof(m.Flags), 20},
		{"rawMouse.Time", unsafe.Offsetof(m.Time), 24},
		{"rawMouse.Extra", unsafe.Offsetof(m.Extra), 32},
		{"rawKey.Type", unsafe.Offsetof(k.Type), 0},
		{"rawKey.Vk", unsafe.Offsetof(k.Vk), 8},
		{"rawKey.Scan", unsafe.Offsetof(k.Scan), 10},
		{"rawKey.Flags", unsafe.Offsetof(k.Flags), 12},
		{"rawKey.Time", unsafe.Offsetof(k.Time), 16},
		{"rawKey.Extra", unsafe.Offsetof(k.Extra), 24},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %d, want %d", c.name, c.got, c.want)
		}
	}
}

func TestWindowsKeyRejectsUnknownModifier(t *testing.T) {
	s := &Sender{}
	// Fails before SendInput is reached, so nothing is injected.
	if err := s.Key("v", []string{"ctrl", "hyper"}); err == nil {
		t.Fatal("Key with an unknown modifier succeeded, want error")
	}
}

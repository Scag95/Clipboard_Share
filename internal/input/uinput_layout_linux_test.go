//go:build linux

package input

import (
	"testing"
	"unsafe"
)

// TestUinputSetupMatchesKernelLayout pins uinputSetup to struct uinput_setup
// from linux/uinput.h:
//
//	struct uinput_setup {
//		struct input_id id;   // offset 0, 8 bytes
//		char name[80];        // offset 8
//		__u32 ff_effects_max; // offset 88
//	};                        // 92 bytes, the size encoded in UI_DEV_SETUP
//
// With the fields out of order the kernel reads the device name from byte 8
// of our name, so a name of 8 bytes or fewer is empty to it and UI_DEV_SETUP
// fails with EINVAL (remote input silently falls back to the recorder).
func TestUinputSetupMatchesKernelLayout(t *testing.T) {
	var s uinputSetup
	if got := unsafe.Offsetof(s.ID); got != 0 {
		t.Errorf("offset of ID = %d, want 0", got)
	}
	if got := unsafe.Offsetof(s.Name); got != 8 {
		t.Errorf("offset of Name = %d, want 8", got)
	}
	if got := unsafe.Offsetof(s.FFEffectsMax); got != 88 {
		t.Errorf("offset of FFEffectsMax = %d, want 88", got)
	}
	if got := unsafe.Sizeof(s); got != 92 {
		t.Errorf("size = %d, want 92", got)
	}
	if size := uintptr(uiDevSetup>>16) & 0x3fff; size != unsafe.Sizeof(s) {
		t.Errorf("UI_DEV_SETUP encodes size %d, struct is %d", size, unsafe.Sizeof(s))
	}
}

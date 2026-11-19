//go:build windows && amd64

package main

import (
	"testing"
	"unsafe"
)

// TestWin32StructSizesMatchTheSDK pins the hand-written Win32 structs to
// their amd64 sizes in the Windows SDK. A wrong field order or type here is
// silently accepted by the compiler and misread by Windows.
func TestWin32StructSizesMatchTheSDK(t *testing.T) {
	cases := []struct {
		name string
		got  uintptr
		want uintptr
	}{
		{"NOTIFYICONDATAW", unsafe.Sizeof(notifyIconDataW{}), 976},
		{"WNDCLASSEXW", unsafe.Sizeof(wndClassExW{}), 80},
		{"MSG", unsafe.Sizeof(msgW{}), 48},
		{"POINT", unsafe.Sizeof(point{}), 8},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("sizeof(%s) = %d, want %d", c.name, c.got, c.want)
		}
	}
}

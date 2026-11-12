//go:build windows

package input

import (
	"fmt"
	"sync"
	"syscall"
	"unicode/utf16"
	"unsafe"
)

// Windows SendInput constants (see MSWSDK).
const (
	inputMouse    = 0
	inputKeyboard = 1

	mouseEventfMove      = 0x0001
	mouseEventfLeftDown  = 0x0002
	mouseEventfLeftUp    = 0x0004
	mouseEventfRightDown = 0x0008
	mouseEventfRightUp   = 0x0010
	mouseEventfWheel     = 0x0800
	mouseEventfHWheel    = 0x1000
	keyEventfExtendedKey = 0x0001
	keyEventfKeyUp       = 0x0002
	keyEventfUnicode     = 0x0004
	wheelDelta           = 120
	maxInputsPerCall     = 128
)

var (
	user32        = syscall.NewLazyDLL("user32.dll")
	procSendInput = user32.NewProc("SendInput")
)

// rawMouse mirrors the INPUT struct with the MOUSEINPUT union member
// (40 bytes on 64-bit Windows).
type rawMouse struct {
	Type      uint32
	_         uint32
	Dx        int32
	Dy        int32
	MouseData uint32
	Flags     uint32
	Time      uint16
	_         [6]byte
	Extra     uintptr
}

// rawKey mirrors the INPUT struct with the KEYBDINPUT union member, padded to
// the same total size as rawMouse.
type rawKey struct {
	Type  uint32
	_     uint32
	Vk    uint16
	Scan  uint16
	Flags uint32
	Time  uint16
	_     [6]byte
	Extra uintptr
	_     [8]byte
}

// Sender injects events into the Windows session via user32!SendInput using
// the standard syscall package, without cgo (Spec S7).
type Sender struct {
	mu       sync.Mutex
	initOnce sync.Once
	initErr  error
}

// New returns the Windows injector.
func New() Injector { return &Sender{} }

func (s *Sender) init() {
	s.initOnce.Do(func() {
		if err := procSendInput.Find(); err != nil {
			s.initErr = fmt.Errorf("locate SendInput in user32.dll: %w", err)
		}
	})
}

func (s *Sender) sendMouse(items []rawMouse) error {
	if len(items) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.init()
	if s.initErr != nil {
		return s.initErr
	}
	r1, _, errno := syscall.SyscallN(procSendInput.Addr(),
		uintptr(len(items)),
		uintptr(unsafe.Pointer(&items[0])),
		unsafe.Sizeof(items[0]))
	if int(r1) != len(items) {
		return fmt.Errorf("SendInput injected %d/%d mouse events (errno %v)", r1, len(items), errno)
	}
	return nil
}

func (s *Sender) sendKey(items []rawKey) error {
	if len(items) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.init()
	if s.initErr != nil {
		return s.initErr
	}
	r1, _, errno := syscall.SyscallN(procSendInput.Addr(),
		uintptr(len(items)),
		uintptr(unsafe.Pointer(&items[0])),
		unsafe.Sizeof(items[0]))
	if int(r1) != len(items) {
		return fmt.Errorf("SendInput injected %d/%d keyboard events (errno %v)", r1, len(items), errno)
	}
	return nil
}

func (s *Sender) Move(dx, dy int) error {
	return s.sendMouse([]rawMouse{{
		Type:  inputMouse,
		Dx:    int32(dx),
		Dy:    int32(dy),
		Flags: mouseEventfMove,
	}})
}

func (s *Sender) Scroll(dx, dy int) error {
	var items []rawMouse
	// Positive dy (scroll content down) is a negative wheel delta on Windows.
	if dy != 0 {
		items = append(items, rawMouse{
			Type:      inputMouse,
			MouseData: uint32(int32(-dy * wheelDelta)),
			Flags:     mouseEventfWheel,
		})
	}
	if dx != 0 {
		items = append(items, rawMouse{
			Type:      inputMouse,
			MouseData: uint32(int32(-dx * wheelDelta)),
			Flags:     mouseEventfHWheel,
		})
	}
	return s.sendMouse(items)
}

func (s *Sender) Click(button, mode string) error {
	var down, up uint32
	switch button {
	case "left":
		down, up = mouseEventfLeftDown, mouseEventfLeftUp
	case "right":
		down, up = mouseEventfRightDown, mouseEventfRightUp
	default:
		return fmt.Errorf("invalid button %q", button)
	}
	var items []rawMouse
	switch mode {
	case "down":
		items = append(items, rawMouse{Type: inputMouse, Flags: down})
	case "up":
		items = append(items, rawMouse{Type: inputMouse, Flags: up})
	case "tap":
		items = append(items,
			rawMouse{Type: inputMouse, Flags: down},
			rawMouse{Type: inputMouse, Flags: up})
	default:
		return fmt.Errorf("invalid click mode %q", mode)
	}
	return s.sendMouse(items)
}

func keyDown(vk uint16, extended bool) rawKey {
	var flags uint32
	if extended {
		flags |= keyEventfExtendedKey
	}
	return rawKey{Type: inputKeyboard, Vk: vk, Flags: flags}
}

func keyUp(vk uint16, extended bool) rawKey {
	var flags uint32
	if extended {
		flags |= keyEventfExtendedKey
	}
	flags |= keyEventfKeyUp
	return rawKey{Type: inputKeyboard, Vk: vk, Flags: flags}
}

func (s *Sender) Key(key string, mods []string) error {
	vk := KeyVK(key)
	if vk == 0 {
		return fmt.Errorf("unknown key %q", key)
	}
	ext := IsExtended(key)
	modsVK := normalizeMods(mods)

	items := make([]rawKey, 0, 2+2*len(modsVK))
	for _, m := range modsVK {
		items = append(items, keyDown(m, m == 0x5B))
	}
	items = append(items, keyDown(vk, ext), keyUp(vk, ext))
	for i := len(modsVK) - 1; i >= 0; i-- {
		items = append(items, keyUp(modsVK[i], modsVK[i] == 0x5B))
	}
	return s.sendKey(items)
}

func (s *Sender) Text(chars string) error {
	if chars == "" {
		return nil
	}
	var items []rawKey
	flush := func() error {
		if err := s.sendKey(items); err != nil {
			return err
		}
		items = items[:0]
		return nil
	}
	for _, r := range chars {
		for _, u := range utf16.Encode([]rune{r}) {
			down := rawKey{Type: inputKeyboard, Scan: u, Flags: keyEventfUnicode}
			up := rawKey{Type: inputKeyboard, Scan: u, Flags: keyEventfUnicode | keyEventfKeyUp}
			items = append(items, down, up)
			if len(items) >= maxInputsPerCall {
				if err := flush(); err != nil {
					return err
				}
			}
		}
	}
	return flush()
}

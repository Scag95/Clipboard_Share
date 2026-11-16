//go:build linux

package input

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

// gestureWriteDeadline bounds EAGAIN retries across one gesture batch. The
// uinput queue is opened O_NONBLOCK, so a burst can transiently reject a
// write; the kernel drains the queue within milliseconds.
const gestureWriteDeadline = 500 * time.Millisecond

const eventWriteRetryInterval = 2 * time.Millisecond

// Linux input event types and codes from linux/input-event-codes.h.
const (
	evSyn = 0x00
	evKey = 0x01
	evRel = 0x02

	synReport = 0
	relX      = 0x00
	relY      = 0x01
	relHWheel = 0x06
	relWheel  = 0x08

	btnLeft  = 0x110
	btnRight = 0x111

	keyLeftCtrl  = 29
	keyLeftShift = 42
	keyLeftAlt   = 56
	keyLeftMeta  = 125
	keyC         = 46
	keyA         = 30
)

// uinput ioctl requests from linux/uinput.h. UI_DEV_SETUP names the device;
// UI_DEV_CREATE and UI_DEV_DESTROY control its lifetime.
const (
	uiDevCreate  = 0x5501
	uiDevDestroy = 0x5502
	uiDevSetup   = 0x405c5503
	uiSetEvbit   = 0x40045564
	uiSetKeybit  = 0x40045565
	uiSetRelbit  = 0x40045566
)

var linuxNamedKeys = map[string]uint16{
	"enter":      28,
	"esc":        1,
	"tab":        15,
	"backspace":  14,
	"delete":     111,
	"insert":     110,
	"arrowup":    103,
	"arrowdown":  108,
	"arrowleft":  105,
	"arrowright": 106,
	"home":       102,
	"end":        107,
	"pageup":     104,
	"pagedown":   109,
	"space":      57,
	"f1":         59,
	"f2":         60,
	"f3":         61,
	"f4":         62,
	"f5":         63,
	"f6":         64,
	"f7":         65,
	"f8":         66,
	"f9":         67,
	"f10":        68,
	"f11":        87,
	"f12":        88,
}

var linuxModifierKeys = map[string]uint16{
	"ctrl":  keyLeftCtrl,
	"alt":   keyLeftAlt,
	"shift": keyLeftShift,
	"win":   keyLeftMeta,
}

var linuxLetterKeys = [26]uint16{
	30, 48, 46, 32, 18, 33, 34, 35, 23, 36, 37, 38, 50,
	49, 24, 25, 16, 19, 31, 20, 22, 47, 17, 45, 21, 44,
}

var linuxDigitKeys = [10]uint16{11, 2, 3, 4, 5, 6, 7, 8, 9, 10}

type asciiKey struct {
	code  uint16
	shift bool
}

var linuxPunctuationKeys = map[rune]asciiKey{
	'`':  {code: 41},
	'~':  {code: 41, shift: true},
	'-':  {code: 12},
	'_':  {code: 12, shift: true},
	'=':  {code: 13},
	'+':  {code: 13, shift: true},
	'[':  {code: 26},
	'{':  {code: 26, shift: true},
	']':  {code: 27},
	'}':  {code: 27, shift: true},
	'\\': {code: 43},
	'|':  {code: 43, shift: true},
	';':  {code: 39},
	':':  {code: 39, shift: true},
	'\'': {code: 40},
	'"':  {code: 40, shift: true},
	',':  {code: 51},
	'<':  {code: 51, shift: true},
	'.':  {code: 52},
	'>':  {code: 52, shift: true},
	'/':  {code: 53},
	'?':  {code: 53, shift: true},
}

func linuxKeyCode(name string) uint16 {
	name = strings.ToLower(name)
	if code, ok := linuxNamedKeys[name]; ok {
		return code
	}
	if len(name) != 1 {
		return 0
	}
	c := name[0]
	switch {
	case c >= 'a' && c <= 'z':
		return linuxLetterKeys[c-'a']
	case c >= '0' && c <= '9':
		return linuxDigitKeys[c-'0']
	}
	return 0
}

func linuxModCode(name string) uint16 {
	return linuxModifierKeys[strings.ToLower(name)]
}

func linuxTextKey(r rune) (code uint16, shift bool, ok bool) {
	if r >= 'A' && r <= 'Z' {
		return linuxLetterKeys[r-'A'], true, true
	}
	if r >= 'a' && r <= 'z' {
		return linuxLetterKeys[r-'a'], false, true
	}
	if r >= '0' && r <= '9' {
		return linuxDigitKeys[r-'0'], false, true
	}
	switch r {
	case '!', '@', '#', '$', '%', '^', '&', '*', '(', ')':
		digit := map[rune]rune{
			'!': '1', '@': '2', '#': '3', '$': '4', '%': '5',
			'^': '6', '&': '7', '*': '8', '(': '9', ')': '0',
		}[r]
		return linuxDigitKeys[digit-'0'], true, true
	case ' ':
		return linuxNamedKeys["space"], false, true
	}
	key, ok := linuxPunctuationKeys[r]
	return key.code, key.shift, ok
}

// inputEvent matches struct input_event, including its native word size.
// A zero timestamp asks the kernel to timestamp the event.
type inputEvent struct {
	Time  unix.Timeval
	Type  uint16
	Code  uint16
	Value int32
}

type linuxInputID struct {
	BusType uint16
	Vendor  uint16
	Product uint16
	Version uint16
}

type uinputSetup struct {
	Name         [80]byte
	ID           linuxInputID
	FFEffectsMax uint32
}

type linuxEventDevice interface {
	WriteEvent(inputEvent) error
	Close() error
}

type uinputDevice struct {
	file *os.File
}

func (d *uinputDevice) setup() error {
	fd := int(d.file.Fd())
	for _, eventType := range []int{evSyn, evKey, evRel} {
		if err := unix.IoctlSetInt(fd, uiSetEvbit, eventType); err != nil {
			return fmt.Errorf("UI_SET_EVBIT(%d): %w", eventType, err)
		}
	}
	for _, axis := range []int{relX, relY, relWheel, relHWheel} {
		if err := unix.IoctlSetInt(fd, uiSetRelbit, axis); err != nil {
			return fmt.Errorf("UI_SET_RELBIT(%d): %w", axis, err)
		}
	}

	keys := map[uint16]bool{btnLeft: true, btnRight: true}
	for _, code := range linuxNamedKeys {
		keys[code] = true
	}
	for _, code := range linuxModifierKeys {
		keys[code] = true
	}
	for r := rune(0x20); r <= 0x7e; r++ {
		code, _, _ := linuxTextKey(r)
		keys[code] = true
	}
	for code := range keys {
		if err := unix.IoctlSetInt(fd, uiSetKeybit, int(code)); err != nil {
			return fmt.Errorf("UI_SET_KEYBIT(%d): %w", code, err)
		}
	}

	setup := uinputSetup{ID: linuxInputID{BusType: 0x03, Vendor: 1, Product: 1, Version: 1}}
	copy(setup.Name[:], "clipboard-share")
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), uiDevSetup, uintptr(unsafe.Pointer(&setup)))
	if errno != 0 {
		return fmt.Errorf("UI_DEV_SETUP: %w", errno)
	}
	if err := unix.IoctlSetInt(fd, uiDevCreate, 0); err != nil {
		return fmt.Errorf("UI_DEV_CREATE: %w", err)
	}
	return nil
}

func (d *uinputDevice) WriteEvent(event inputEvent) error {
	bytes := unsafe.Slice((*byte)(unsafe.Pointer(&event)), unsafe.Sizeof(event))
	n, err := d.file.Write(bytes)
	if err != nil {
		return err
	}
	if n != len(bytes) {
		return io.ErrShortWrite
	}
	return nil
}

func (d *uinputDevice) Close() error {
	destroyErr := unix.IoctlSetInt(int(d.file.Fd()), uiDevDestroy, 0)
	closeErr := d.file.Close()
	if destroyErr != nil {
		return fmt.Errorf("UI_DEV_DESTROY: %w", destroyErr)
	}
	return closeErr
}

// LinuxSender injects events through a virtual uinput device. Close destroys
// the virtual device; New and NewLinux keep it alive for the injector's life.
type LinuxSender struct {
	mu     sync.Mutex
	device linuxEventDevice
	closed bool
}

// NewLinux opens and configures /dev/uinput. Wayland has no protocol-level
// pointer injection, so Linux uses the kernel's input interface instead (S2).
func NewLinux() (Injector, error) {
	fd, err := unix.Open("/dev/uinput", unix.O_WRONLY|unix.O_CLOEXEC|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, fmt.Errorf("open /dev/uinput: %w (load uinput and grant the current user access to it)", err)
	}
	device := &uinputDevice{file: os.NewFile(uintptr(fd), "/dev/uinput")}
	if err := device.setup(); err != nil {
		// setup may have already created the device before failing; Close
		// destroys it when it exists and always releases the fd.
		_ = device.Close()
		return nil, fmt.Errorf("configure /dev/uinput: %w", err)
	}
	sender := &LinuxSender{device: device}
	runtime.SetFinalizer(sender, func(s *LinuxSender) { _ = s.Close() })
	return sender, nil
}

// New returns the real Linux injector when a graphical session and uinput are
// available. Otherwise it preserves Recorder for development and tests (S7),
// logging why so an operator never mistakes recording for injecting.
func New() Injector {
	if os.Getenv("WAYLAND_DISPLAY") != "" || os.Getenv("DISPLAY") != "" {
		injector, err := NewLinux()
		if err == nil {
			log.Printf("input: uinput injector active")
			return injector
		}
		log.Printf("input: falling back to recorder (no injection): %v", err)
	} else {
		log.Printf("input: no display session, using recorder (no injection)")
	}
	return &Recorder{}
}

// Close destroys the uinput device and releases its file descriptor.
func (s *LinuxSender) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	runtime.SetFinalizer(s, nil)
	return s.device.Close()
}

func (s *LinuxSender) emit(events []inputEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return fmt.Errorf("linux input injector is closed")
	}
	var firstErr error
	deadline := time.Now().Add(gestureWriteDeadline)
	for _, event := range events {
		err := s.device.WriteEvent(event)
		for errors.Is(err, unix.EAGAIN) && time.Now().Before(deadline) {
			time.Sleep(eventWriteRetryInterval)
			err = s.device.WriteEvent(event)
		}
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("write uinput event: %w", err)
			}
			// Keep writing the rest of the batch anyway: gesture sequences end
			// in modifier/button releases, and skipping them would leave keys
			// stuck on the desktop.
		}
	}
	return firstErr
}

func appendKeyEvent(events []inputEvent, code uint16, value int32) []inputEvent {
	return append(events,
		inputEvent{Type: evKey, Code: code, Value: value},
		inputEvent{Type: evSyn, Code: synReport},
	)
}

// clampDelta bounds a delta to the protocol range before the int32 event
// conversion, so out-of-range values cannot wrap into the opposite direction
// (parse.go already rejects them at the wire boundary; the sender keeps the
// invariant for every caller).
func clampDelta(v, limit int) int32 {
	if v > limit {
		v = limit
	}
	if v < -limit {
		v = -limit
	}
	return int32(v)
}

func (s *LinuxSender) Move(dx, dy int) error {
	events := make([]inputEvent, 0, 3)
	if v := clampDelta(dx, MaxPointerDelta); v != 0 {
		events = append(events, inputEvent{Type: evRel, Code: relX, Value: v})
	}
	if v := clampDelta(dy, MaxPointerDelta); v != 0 {
		events = append(events, inputEvent{Type: evRel, Code: relY, Value: v})
	}
	if len(events) == 0 {
		return nil
	}
	events = append(events, inputEvent{Type: evSyn, Code: synReport})
	return s.emit(events)
}

func (s *LinuxSender) Scroll(dx, dy int) error {
	events := make([]inputEvent, 0, 3)
	// As on Windows, positive deltas scroll content down.
	if v := clampDelta(dy, MaxScrollNotches); v != 0 {
		events = append(events, inputEvent{Type: evRel, Code: relWheel, Value: -v})
	}
	if v := clampDelta(dx, MaxScrollNotches); v != 0 {
		events = append(events, inputEvent{Type: evRel, Code: relHWheel, Value: -v})
	}
	if len(events) == 0 {
		return nil
	}
	events = append(events, inputEvent{Type: evSyn, Code: synReport})
	return s.emit(events)
}

func (s *LinuxSender) Click(button, mode string) error {
	var code uint16
	switch button {
	case "left":
		code = btnLeft
	case "right":
		code = btnRight
	default:
		return fmt.Errorf("invalid button %q", button)
	}
	var events []inputEvent
	switch mode {
	case "down":
		events = appendKeyEvent(events, code, 1)
	case "up":
		events = appendKeyEvent(events, code, 0)
	case "tap":
		events = appendKeyEvent(events, code, 1)
		events = appendKeyEvent(events, code, 0)
	default:
		return fmt.Errorf("invalid click mode %q", mode)
	}
	return s.emit(events)
}

func (s *LinuxSender) Key(key string, mods []string) error {
	code := linuxKeyCode(key)
	if code == 0 {
		return fmt.Errorf("unknown key %q", key)
	}
	modCodes := make([]uint16, 0, len(mods))
	for _, mod := range mods {
		modCode := linuxModCode(mod)
		if modCode == 0 {
			// Reject before emitting anything: silently dropping an unknown
			// modifier would inject Ctrl+V where Ctrl+something-else was asked.
			return fmt.Errorf("unknown modifier %q", mod)
		}
		modCodes = append(modCodes, modCode)
	}
	events := make([]inputEvent, 0, (2+2*len(modCodes))*2)
	for _, modCode := range modCodes {
		events = appendKeyEvent(events, modCode, 1)
	}
	events = appendKeyEvent(events, code, 1)
	events = appendKeyEvent(events, code, 0)
	for i := len(modCodes) - 1; i >= 0; i-- {
		events = appendKeyEvent(events, modCodes[i], 0)
	}
	return s.emit(events)
}

func (s *LinuxSender) Text(chars string) error {
	var events []inputEvent
	for _, r := range chars {
		code, shift, ok := linuxTextKey(r)
		if !ok {
			// S2 beta types basic text only: non-ASCII runes are ignored.
			continue
		}
		if shift {
			events = appendKeyEvent(events, keyLeftShift, 1)
		}
		events = appendKeyEvent(events, code, 1)
		events = appendKeyEvent(events, code, 0)
		if shift {
			events = appendKeyEvent(events, keyLeftShift, 0)
		}
	}
	return s.emit(events)
}

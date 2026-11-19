//go:build windows

package main

import (
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"nudge/internal/pair"
)

// Win32 notification-area icon. One goroutine, locked to its OS thread,
// owns an invisible top-level window (not message-only: those do not get
// the TaskbarCreated broadcast), the icon and the message loop. Other
// goroutines only post messages to it; the menu reads the broker through
// Broker.Snapshot, which is safe for concurrent use.

var (
	shell32               = syscall.NewLazyDLL("shell32.dll")
	kernel32              = syscall.NewLazyDLL("kernel32.dll")
	procShellNotifyIconW  = shell32.NewProc("Shell_NotifyIconW")
	procGetModuleHandleW  = kernel32.NewProc("GetModuleHandleW")
	procRegisterClassExW  = user32.NewProc("RegisterClassExW")
	procCreateWindowExW   = user32.NewProc("CreateWindowExW")
	procDestroyWindow     = user32.NewProc("DestroyWindow")
	procDefWindowProcW    = user32.NewProc("DefWindowProcW")
	procGetMessageW       = user32.NewProc("GetMessageW")
	procTranslateMessage  = user32.NewProc("TranslateMessage")
	procDispatchMessageW  = user32.NewProc("DispatchMessageW")
	procPostMessageW      = user32.NewProc("PostMessageW")
	procPostQuitMessage   = user32.NewProc("PostQuitMessage")
	procRegisterWindowMsg = user32.NewProc("RegisterWindowMessageW")
	procLoadImageW        = user32.NewProc("LoadImageW")
	procLoadIconW         = user32.NewProc("LoadIconW")
	procDestroyIcon       = user32.NewProc("DestroyIcon")
	procGetSystemMetrics  = user32.NewProc("GetSystemMetrics")
	procCreatePopupMenu   = user32.NewProc("CreatePopupMenu")
	procAppendMenuW       = user32.NewProc("AppendMenuW")
	procTrackPopupMenu    = user32.NewProc("TrackPopupMenu")
	procDestroyMenu       = user32.NewProc("DestroyMenu")
	procSetForegroundWnd  = user32.NewProc("SetForegroundWindow")
	procGetCursorPos      = user32.NewProc("GetCursorPos")
)

const (
	wmNull        = 0x0000
	wmDestroy     = 0x0002
	wmClose       = 0x0010
	wmContextMenu = 0x007B
	wmRButtonUp   = 0x0205
	wmApp         = 0x8000
	wmTrayCB      = wmApp + 1
	wmTrayNotice  = wmApp + 2

	nimAdd        = 0
	nimModify     = 1
	nimDelete     = 2
	nimSetVersion = 4

	nifMessage  = 0x01
	nifIcon     = 0x02
	nifTip      = 0x04
	nifInfo     = 0x10
	nifShowTip  = 0x80
	niifInfo    = 0x01
	nfiVersion4 = 4

	imageIcon       = 1
	lrDefaultColor  = 0
	idiApplication  = 32512
	smCxSmIcon      = 49
	smCySmIcon      = 50
	mfString        = 0x0000
	mfGrayed        = 0x0001
	mfSeparator     = 0x0800
	tpmRightButton  = 0x0002
	tpmReturnCmd    = 0x0100
	tpmNoNotify     = 0x0080
	trayIconID      = 1
	trayCloseWait   = 2 * time.Second
	trayWindowClass = "NudgeTrayWindow"
)

type wndClassExW struct {
	cbSize        uint32
	style         uint32
	lpfnWndProc   uintptr
	cbClsExtra    int32
	cbWndExtra    int32
	hInstance     uintptr
	hIcon         uintptr
	hCursor       uintptr
	hbrBackground uintptr
	lpszMenuName  *uint16
	lpszClassName *uint16
	hIconSm       uintptr
}

type point struct{ x, y int32 }

type msgW struct {
	hwnd     uintptr
	message  uint32
	wParam   uintptr
	lParam   uintptr
	time     uint32
	pt       point
	lPrivate uint32
}

type notifyIconDataW struct {
	cbSize           uint32
	hWnd             uintptr
	uID              uint32
	uFlags           uint32
	uCallbackMessage uint32
	hIcon            uintptr
	szTip            [128]uint16
	dwState          uint32
	dwStateMask      uint32
	szInfo           [256]uint16
	uVersion         uint32
	szInfoTitle      [64]uint16
	dwInfoFlags      uint32
	guidItem         [16]byte
	hBalloonIcon     uintptr
}

type winTray struct {
	port   string
	broker *pair.Broker
	logf   func(string, ...any)

	hwnd        atomic.Uintptr
	done        chan struct{}
	closeOnce   sync.Once
	mu          sync.Mutex
	notices     [][2]string
	taskbarMsg  uint32
	icon        uintptr
	ownsIcon    bool
	added       bool // tray thread only
	version4    bool // tray thread only
	everAddedOK bool // tray thread only
}

// Only one tray per process; the window procedure reaches it here.
var (
	activeTray     *winTray
	trayWndProcPtr = syscall.NewCallback(trayWndProc)
)

// startTray creates the tray icon for the server. Failure is logged and the
// server keeps running without it.
func startTray(port string, b *pair.Broker, logf func(string, ...any)) trayHandle {
	t := &winTray{port: port, broker: b, logf: logf, done: make(chan struct{})}
	ready := make(chan bool, 1)
	go t.run(ready)
	if !<-ready {
		return noTray{}
	}
	return t
}

func (t *winTray) run(ready chan<- bool) {
	runtime.LockOSThread()
	defer close(t.done)
	hwnd, err := t.createWindow()
	if err != nil {
		t.logf("tray icon unavailable, server keeps running: %v", err)
		ready <- false
		return
	}
	activeTray = t
	t.hwnd.Store(hwnd)
	t.loadIcon()
	t.add()
	ready <- true

	var m msgW
	for {
		r, _, callErr := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		switch int32(r) {
		case 0:
			return
		case -1:
			t.logf("tray message loop failed: %v", callErr)
			t.remove()
			return
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
}

func (t *winTray) createWindow() (uintptr, error) {
	for _, p := range []*syscall.LazyProc{procShellNotifyIconW, procCreateWindowExW, procRegisterClassExW, procTrackPopupMenu} {
		if err := p.Find(); err != nil {
			return 0, err
		}
	}
	hInst, _, _ := procGetModuleHandleW.Call(0)
	className, _ := syscall.UTF16PtrFromString(trayWindowClass)
	wc := wndClassExW{
		lpfnWndProc:   trayWndProcPtr,
		hInstance:     hInst,
		lpszClassName: className,
	}
	wc.cbSize = uint32(unsafe.Sizeof(wc))
	if atom, _, callErr := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); atom == 0 {
		return 0, errors.New("RegisterClassExW: " + callErr.Error())
	}
	title, _ := syscall.UTF16PtrFromString(trayTooltip)
	// Never shown (no WS_VISIBLE); only receives messages.
	hwnd, _, callErr := procCreateWindowExW.Call(0,
		uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(title)),
		0, 0, 0, 0, 0, 0, 0, hInst, 0)
	if hwnd == 0 {
		return 0, errors.New("CreateWindowExW: " + callErr.Error())
	}
	name, _ := syscall.UTF16PtrFromString("TaskbarCreated")
	msg, _, _ := procRegisterWindowMsg.Call(uintptr(unsafe.Pointer(name)))
	t.taskbarMsg = uint32(msg)
	return hwnd, nil
}

// loadIcon uses the exe's icon group resource 1 (release builds) and falls
// back to the stock application icon (plain go build has no resources).
func (t *winTray) loadIcon() {
	hInst, _, _ := procGetModuleHandleW.Call(0)
	cx, _, _ := procGetSystemMetrics.Call(smCxSmIcon)
	cy, _, _ := procGetSystemMetrics.Call(smCySmIcon)
	icon, _, _ := procLoadImageW.Call(hInst, 1, imageIcon, cx, cy, lrDefaultColor)
	if icon != 0 {
		t.icon, t.ownsIcon = icon, true
		return
	}
	icon, _, _ = procLoadIconW.Call(0, idiApplication)
	t.icon = icon
}

func (t *winTray) baseData() notifyIconDataW {
	var nid notifyIconDataW
	nid.cbSize = uint32(unsafe.Sizeof(nid))
	nid.hWnd = t.hwnd.Load()
	nid.uID = trayIconID
	return nid
}

func shellNotify(op uintptr, nid *notifyIconDataW) (bool, error) {
	r, _, err := procShellNotifyIconW.Call(op, uintptr(unsafe.Pointer(nid)))
	return r != 0, err
}

// add (re)adds the icon; called at start and on TaskbarCreated.
func (t *winTray) add() {
	nid := t.baseData()
	nid.uFlags = nifMessage | nifIcon | nifTip | nifShowTip
	nid.uCallbackMessage = wmTrayCB
	nid.hIcon = t.icon
	copyUTF16(nid.szTip[:], trayTooltip)
	ok, err := shellNotify(nimAdd, &nid)
	if !ok {
		// Explorer may still be starting; TaskbarCreated retries.
		t.added = false
		t.logf("tray icon could not be added (will retry when the taskbar starts), server keeps running: %v", err)
		return
	}
	t.added = true
	nid.uVersion = nfiVersion4
	t.version4, _ = shellNotify(nimSetVersion, &nid)
	if !t.everAddedOK {
		t.logf("tray icon added")
	} else {
		t.logf("tray icon re-added after taskbar restart")
	}
	t.everAddedOK = true
}

func (t *winTray) remove() {
	if t.added {
		nid := t.baseData()
		shellNotify(nimDelete, &nid)
		t.added = false
	}
	if t.ownsIcon && t.icon != 0 {
		procDestroyIcon.Call(t.icon)
		t.icon, t.ownsIcon = 0, false
	}
}

func (t *winTray) showNotices() {
	t.mu.Lock()
	notices := t.notices
	t.notices = nil
	t.mu.Unlock()
	for _, n := range notices {
		if !t.added {
			t.logf("tray notification skipped: icon not in the taskbar")
			continue
		}
		nid := t.baseData()
		nid.uFlags = nifInfo
		nid.dwInfoFlags = niifInfo
		copyUTF16(nid.szInfoTitle[:], n[0])
		copyUTF16(nid.szInfo[:], n[1])
		if ok, err := shellNotify(nimModify, &nid); !ok {
			t.logf("tray notification failed: %v", err)
		}
	}
}

func (t *winTray) showMenu() {
	devices, _ := t.broker.Snapshot()
	model := trayMenuModel(t.port, devices)
	menu, _, _ := procCreatePopupMenu.Call()
	if menu == 0 {
		t.logf("tray menu could not be created")
		return
	}
	defer procDestroyMenu.Call(menu)
	appendItem := func(flags uintptr, text string) {
		var ptr *uint16
		if text != "" {
			ptr, _ = syscall.UTF16PtrFromString(trayMenuLabel(text))
		}
		procAppendMenuW.Call(menu, flags, 0, uintptr(unsafe.Pointer(ptr)))
	}
	appendItem(mfString|mfGrayed, model.Status)
	appendItem(mfSeparator, "")
	for _, line := range model.Phones {
		appendItem(mfString|mfGrayed, line)
	}
	var pt point
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	hwnd := t.hwnd.Load()
	// Required so the menu closes when the user clicks elsewhere.
	procSetForegroundWnd.Call(hwnd)
	procTrackPopupMenu.Call(menu, tpmRightButton|tpmReturnCmd|tpmNoNotify,
		uintptr(pt.x), uintptr(pt.y), 0, hwnd, 0)
	procPostMessageW.Call(hwnd, wmNull, 0, 0)
}

func trayWndProc(hwnd, msg, wParam, lParam uintptr) uintptr {
	t := activeTray
	if t == nil || hwnd != t.hwnd.Load() {
		r, _, _ := procDefWindowProcW.Call(hwnd, msg, wParam, lParam)
		return r
	}
	switch uint32(msg) {
	case wmTrayCB:
		event := uint32(lParam & 0xFFFF)
		if (t.version4 && event == wmContextMenu) || (!t.version4 && event == wmRButtonUp) {
			t.showMenu()
		}
		return 0
	case wmTrayNotice:
		t.showNotices()
		return 0
	case wmClose:
		procDestroyWindow.Call(hwnd)
		return 0
	case wmDestroy:
		t.remove()
		procPostQuitMessage.Call(0)
		return 0
	}
	if t.taskbarMsg != 0 && uint32(msg) == t.taskbarMsg {
		t.add()
		return 0
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, msg, wParam, lParam)
	return r
}

// NotifyPairRequest queues a tray notification; it never blocks (it runs in
// the broker's request callback).
func (t *winTray) NotifyPairRequest(req pair.RequestInfo) {
	title, text := trayPairNotice(req)
	t.mu.Lock()
	if len(t.notices) < maxPendingDialogs {
		t.notices = append(t.notices, [2]string{title, text})
	}
	t.mu.Unlock()
	if hwnd := t.hwnd.Load(); hwnd != 0 {
		procPostMessageW.Call(hwnd, wmTrayNotice, 0, 0)
	}
}

// Close removes the icon and ends the tray thread (bounded wait).
func (t *winTray) Close() {
	t.closeOnce.Do(func() {
		hwnd := t.hwnd.Load()
		if hwnd == 0 {
			return
		}
		if r, _, _ := procPostMessageW.Call(hwnd, wmClose, 0, 0); r == 0 {
			return
		}
		select {
		case <-t.done:
		case <-time.After(trayCloseWait):
			t.logf("tray icon did not close in time")
		}
	})
}

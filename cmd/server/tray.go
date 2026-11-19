package main

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf16"

	"nudge/internal/pair"
)

// Notification-area (tray) icon content. Only the Windows server shows a
// tray icon (tray_windows.go); the text is built here, platform-neutral, so
// it is unit tested on Linux. LAN-supplied names and IPs are sanitized and
// truncated exactly like the pairing dialog.

const (
	trayTooltip      = "Nudge"
	trayNoticeTitle  = "Solicitud de emparejamiento"
	trayNoPhones     = "Ningún teléfono emparejado"
	trayNoName       = "(sin nombre)"
	trayNoIP         = "(IP desconocida)"
	maxTrayPhones    = 10
	maxTrayPortChars = 16
)

// trayHandle is what runServer needs from the tray: a pairing notice and a
// clean removal on shutdown. Both are safe to call from any goroutine.
type trayHandle interface {
	NotifyPairRequest(req pair.RequestInfo)
	Close()
}

// noTray is used where there is no tray (non-Windows, or creation failed).
type noTray struct{}

func (noTray) NotifyPairRequest(pair.RequestInfo) {}
func (noTray) Close()                             {}

// withTrayNotice shows the tray notice for a new request and then runs the
// platform handler unchanged (on Windows, the Yes/No dialog that approves).
func withTrayNotice(t trayHandle, next func(pair.RequestInfo)) func(pair.RequestInfo) {
	return func(req pair.RequestInfo) {
		t.NotifyPairRequest(req)
		next(req)
	}
}

// trayMenu is the informational, read-only content of the tray menu: the
// status line, the verification code line (when known), then (after a
// separator) one line per approved phone.
type trayMenu struct {
	Status string
	Code   string
	Phones []string
}

// trayMenuModel builds the menu from the current approved devices, ordered
// by name (case-insensitive), then IP, then ID, capped at maxTrayPhones
// with a final "y N más" line. code is the certificate verification code
// (approvalBroker.Verification); empty means no code line.
func trayMenuModel(port, code string, devices []pair.DeviceInfo) trayMenu {
	menu := trayMenu{Status: "Escuchando en el puerto " + sanitizeDialogField(port, maxTrayPortChars)}
	if code != "" {
		menu.Code = "Código: " + code
	}
	if len(devices) == 0 {
		menu.Phones = []string{trayNoPhones}
		return menu
	}
	type row struct{ name, ip, id string }
	rows := make([]row, 0, len(devices))
	for _, d := range devices {
		name := sanitizeDialogField(d.Name, maxDialogName)
		if name == "" {
			name = trayNoName
		}
		ip := sanitizeDialogField(d.IP, maxDialogIP)
		if ip == "" {
			ip = trayNoIP
		}
		rows = append(rows, row{name, ip, d.ID})
	}
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := strings.ToLower(rows[i].name), strings.ToLower(rows[j].name)
		if a != b {
			return a < b
		}
		if rows[i].ip != rows[j].ip {
			return rows[i].ip < rows[j].ip
		}
		return rows[i].id < rows[j].id
	})
	shown := rows
	if len(shown) > maxTrayPhones {
		shown = shown[:maxTrayPhones]
	}
	for _, r := range shown {
		menu.Phones = append(menu.Phones, r.name+" — "+r.ip)
	}
	if extra := len(rows) - len(shown); extra > 0 {
		menu.Phones = append(menu.Phones, fmt.Sprintf("y %d más", extra))
	}
	return menu
}

// trayMenuLabel escapes "&" so Win32 menus do not treat it as a mnemonic.
func trayMenuLabel(s string) string {
	return strings.ReplaceAll(s, "&", "&&")
}

// trayPairNotice is the tray notification for a new pairing request. It
// only informs: approval happens in the Yes/No dialog.
func trayPairNotice(req pair.RequestInfo) (title, text string) {
	name := sanitizeDialogField(req.Name, maxDialogName)
	if name == "" {
		name = trayNoName
	}
	return trayNoticeTitle, name + " quiere emparejarse. Respondé en la ventana de Nudge."
}

// copyUTF16 writes s into a fixed NUL-terminated UTF-16 buffer (as in
// NOTIFYICONDATAW), truncating without splitting a surrogate pair and
// zero-filling the rest.
func copyUTF16(dst []uint16, s string) {
	for i := range dst {
		dst[i] = 0
	}
	if len(dst) == 0 {
		return
	}
	n := 0
	for _, r := range s {
		units := utf16.Encode([]rune{r})
		if n+len(units) > len(dst)-1 {
			break
		}
		n += copy(dst[n:], units)
	}
}

package main

import (
	"fmt"
	"strings"
	"unicode"

	"nudge/internal/pair"
)

// Pairing approval by native dialog (used on Windows, where there is no tray
// panel). The logic lives here, platform-neutral, so it is unit tested on
// Linux with an injected showDialog; approval_windows.go supplies MessageBoxW.

const (
	approvalTitle     = "Nudge"
	maxDialogName     = 40
	maxDialogIP       = 64
	maxPendingDialogs = 5

	// MessageBoxW return values.
	idYes = 6
	idNo  = 7
)

// approvalBroker is the part of *pair.Broker the dialog needs.
type approvalBroker interface {
	Snapshot() ([]pair.DeviceInfo, []pair.RequestInfo)
	Approve(requestID string) error
	Deny(requestID string) error
}

// sanitizeDialogField makes LAN-supplied text safe to show: control and
// format characters (including bidi overrides) are removed, whitespace runs
// collapse to one space, and the result is capped at max runes with "…".
func sanitizeDialogField(s string, max int) string {
	var b strings.Builder
	space := false
	for _, r := range s {
		switch {
		case unicode.IsSpace(r):
			space = true
			continue
		case unicode.IsControl(r), unicode.Is(unicode.Cf, r), r == unicode.ReplacementChar:
			continue
		}
		if space && b.Len() > 0 {
			b.WriteByte(' ')
		}
		space = false
		b.WriteRune(r)
	}
	out := []rune(b.String())
	if len(out) > max {
		out = append(out[:max-1], '…')
	}
	return string(out)
}

// approvalText is the Spanish dialog body for a pairing request.
func approvalText(req pair.RequestInfo) string {
	name := sanitizeDialogField(req.Name, maxDialogName)
	if name == "" {
		name = "(sin nombre)"
	}
	ip := sanitizeDialogField(req.IP, maxDialogIP)
	if ip == "" {
		ip = "(desconocida)"
	}
	return fmt.Sprintf("Un teléfono quiere emparejarse con este equipo.\n\n"+
		"Nombre: %s\nIP: %s\n\n"+
		"¿Permitir que controle el portapapeles, el ratón y el teclado de este equipo?\n\n"+
		"Si no reconoces este dispositivo, pulsa No.", name, ip)
}

// approvalQueue shows one dialog at a time. Requests beyond the queue
// capacity are dropped (they can still be approved with `nudge pair`).
type approvalQueue struct {
	queue  chan pair.RequestInfo
	broker approvalBroker
	show   func(title, text string) int
	logf   func(format string, args ...any)
}

func newApprovalQueue(b approvalBroker, show func(title, text string) int, logf func(string, ...any), capacity int) *approvalQueue {
	return &approvalQueue{
		queue:  make(chan pair.RequestInfo, capacity),
		broker: b,
		show:   show,
		logf:   logf,
	}
}

// Start runs the single dialog worker for the life of the process.
func (q *approvalQueue) Start() {
	go func() {
		for req := range q.queue {
			q.handle(req)
		}
	}()
}

// Enqueue never blocks (it runs inside Broker.Request); it reports false and
// drops req when the queue is full.
func (q *approvalQueue) Enqueue(req pair.RequestInfo) bool {
	select {
	case q.queue <- req:
		return true
	default:
		q.logf("pairing dialog queue full, request %s from %s not shown (use nudge pair list)", shortID(req.RequestID), req.IP)
		return false
	}
}

func (q *approvalQueue) pending(requestID string) bool {
	_, requests := q.broker.Snapshot()
	for _, r := range requests {
		if r.RequestID == requestID {
			return true
		}
	}
	return false
}

// handle shows the dialog for a still-pending request and applies the
// answer only if the request is still pending afterwards. Yes approves;
// No, closing the dialog or a dialog failure denies.
func (q *approvalQueue) handle(req pair.RequestInfo) {
	if !q.pending(req.RequestID) {
		return
	}
	answer := q.show(approvalTitle, approvalText(req))
	if !q.pending(req.RequestID) {
		q.logf("pairing request %s no longer pending, dialog answer ignored", shortID(req.RequestID))
		return
	}
	var err error
	if answer == idYes {
		err = q.broker.Approve(req.RequestID)
		if err == nil {
			q.logf("pairing request %s approved from the dialog", shortID(req.RequestID))
		}
	} else {
		err = q.broker.Deny(req.RequestID)
		if err == nil {
			q.logf("pairing request %s denied from the dialog", shortID(req.RequestID))
		}
	}
	if err != nil {
		q.logf("pairing dialog answer for %s failed: %v", shortID(req.RequestID), err)
	}
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

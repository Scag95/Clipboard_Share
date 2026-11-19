package main

import (
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"nudge/internal/pair"
)

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
}

func newApprovalBroker(t *testing.T) (*pair.Broker, *fakeClock) {
	t.Helper()
	t.Setenv("NUDGE_STATE", filepath.Join(t.TempDir(), "state.json"))
	clock := &fakeClock{now: time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)}
	return pair.NewBrokerWithClock(clock.Now), clock
}

func requestPairing(t *testing.T, b *pair.Broker, id, name, ip string) pair.RequestInfo {
	t.Helper()
	res, err := b.Request(id, name, ip)
	if err != nil {
		t.Fatalf("Request(%s): %v", id, err)
	}
	_, reqs := b.Snapshot()
	for _, r := range reqs {
		if r.RequestID == res.RequestID {
			return r
		}
	}
	t.Fatalf("request %s not pending", res.RequestID)
	return pair.RequestInfo{}
}

func pendingIDs(b *pair.Broker) []string {
	_, reqs := b.Snapshot()
	ids := make([]string, 0, len(reqs))
	for _, r := range reqs {
		ids = append(ids, r.RequestID)
	}
	return ids
}

func approvedCount(b *pair.Broker) int {
	devices, _ := b.Snapshot()
	return len(devices)
}

func TestSanitizeDialogField(t *testing.T) {
	cases := []struct {
		in   string
		max  int
		want string
	}{
		{"Pixel 8", 40, "Pixel 8"},
		{"  Pixel \t\n 8  ", 40, "Pixel 8"},
		{"evil\x00\x07\x1b[31mname", 40, "evil[31mname"},
		{"a\u202Egnp.exe", 40, "agnp.exe"}, // bidi override removed
		{"line1\r\nline2", 40, "line1 line2"},
		{strings.Repeat("é", 50), 40, strings.Repeat("é", 39) + "…"},
		{strings.Repeat("x", 40), 40, strings.Repeat("x", 40)},
		{"\x00\x01  \t", 40, ""},
	}
	for _, c := range cases {
		if got := sanitizeDialogField(c.in, c.max); got != c.want {
			t.Errorf("sanitizeDialogField(%q, %d) = %q, want %q", c.in, c.max, got, c.want)
		}
	}
}

func TestApprovalTextIsSpanishWithSanitizedNameAndIP(t *testing.T) {
	text := approvalText(pair.RequestInfo{Name: "Pixel\n8\x00 de Ana", IP: "192.168.1.20"}, "6B86-B273-FF34")
	for _, want := range []string{"Pixel 8 de Ana", "192.168.1.20", "emparejar", "¿Permitir"} {
		if !strings.Contains(text, want) {
			t.Errorf("approval text %q does not contain %q", text, want)
		}
	}
	if strings.ContainsRune(text, 0) {
		t.Errorf("approval text contains NUL: %q", text)
	}
	long := approvalText(pair.RequestInfo{Name: strings.Repeat("N", 200), IP: "10.0.0.1"}, "6B86-B273-FF34")
	if strings.Contains(long, strings.Repeat("N", 41)) {
		t.Errorf("long name not truncated: %q", long)
	}
	empty := approvalText(pair.RequestInfo{Name: "\x01", IP: "10.0.0.1"}, "6B86-B273-FF34")
	if !strings.Contains(empty, "(sin nombre)") {
		t.Errorf("empty name placeholder missing: %q", empty)
	}
}

func TestApprovalTextFullWordingUsesVoseo(t *testing.T) {
	got := approvalText(pair.RequestInfo{Name: "Pixel 8 de Ana", IP: "192.168.1.20"}, "6B86-B273-FF34")
	want := "Un teléfono quiere emparejarse con este equipo.\n\n" +
		"Nombre: Pixel 8 de Ana\nIP: 192.168.1.20\nCódigo: 6B86-B273-FF34\n\n" +
		"Comprobá que el teléfono muestre el mismo código.\n\n" +
		"¿Permitir que controle el portapapeles, el ratón y el teclado de este equipo?\n\n" +
		"Si no reconocés este dispositivo, elegí No."
	if got != want {
		t.Errorf("approval text =\n%q\nwant\n%q", got, want)
	}
}

func TestApprovalDialogYesApprovesAndNoDenies(t *testing.T) {
	cases := []struct {
		answer      int
		wantApprove bool
	}{
		{idYes, true},
		{idNo, false},
		{0, false}, // MessageBoxW failure
		{2, false}, // IDCANCEL / closed
	}
	for _, c := range cases {
		b, _ := newApprovalBroker(t)
		b.SetVerification("6b86b273ff34fce19d6b804eff5a3f5747ada4eaa22f1d49c01e52ddb7875b4b", "6B86-B273-FF34")
		req := requestPairing(t, b, "dev", "Pixel", "10.0.0.2")
		var shown []string
		q := newApprovalQueue(b, func(title, text string) int {
			shown = append(shown, title+"|"+text)
			return c.answer
		}, t.Logf, maxPendingDialogs)
		q.handle(req)
		if len(shown) != 1 || !strings.HasPrefix(shown[0], "Nudge|") {
			t.Fatalf("answer %d: shown = %q, want one dialog titled Nudge", c.answer, shown)
		}
		if !strings.Contains(shown[0], "\nCódigo: 6B86-B273-FF34\n") {
			t.Fatalf("answer %d: dialog %q does not show the broker's verification code", c.answer, shown[0])
		}
		if got := len(pendingIDs(b)); got != 0 {
			t.Fatalf("answer %d: %d requests still pending, want resolved", c.answer, got)
		}
		if got := approvedCount(b) == 1; got != c.wantApprove {
			t.Fatalf("answer %d: approved = %v, want %v", c.answer, got, c.wantApprove)
		}
		status, err := b.Status(req.RequestID, req.IP)
		want := pair.StateDenied
		if c.wantApprove {
			want = pair.StateApproved
		}
		if err != nil || status.State != want {
			t.Fatalf("answer %d: status = %v, %v; want %s", c.answer, status.State, err, want)
		}
	}
}

func TestApprovalDialogSkipsRequestNoLongerPending(t *testing.T) {
	b, clock := newApprovalBroker(t)
	expired := requestPairing(t, b, "a", "A", "10.0.0.3")
	clock.Advance(pair.PendingTTL + time.Second)
	resolved := requestPairing(t, b, "b", "B", "10.0.0.3")
	if err := b.Deny(resolved.RequestID); err != nil {
		t.Fatal(err)
	}
	shows := 0
	q := newApprovalQueue(b, func(string, string) int { shows++; return idYes }, t.Logf, maxPendingDialogs)
	q.handle(expired)
	q.handle(resolved)
	if shows != 0 {
		t.Fatalf("dialog shown %d times for non-pending requests, want 0", shows)
	}
	if approvedCount(b) != 0 {
		t.Fatal("non-pending request was approved")
	}
}

func TestApprovalDialogAnswerAfterExpiryIsNotApplied(t *testing.T) {
	b, clock := newApprovalBroker(t)
	req := requestPairing(t, b, "a", "A", "10.0.0.4")
	q := newApprovalQueue(b, func(string, string) int {
		clock.Advance(pair.PendingTTL + time.Second) // user answered too late
		return idYes
	}, t.Logf, maxPendingDialogs)
	q.handle(req)
	if approvedCount(b) != 0 {
		t.Fatal("expired request approved by a late Yes")
	}
	if _, err := b.Status(req.RequestID, req.IP); !errors.Is(err, pair.ErrExpired) {
		t.Fatalf("status err = %v, want ErrExpired", err)
	}
}

func TestApprovalQueueDropsNewestBeyondCap(t *testing.T) {
	b, _ := newApprovalBroker(t)
	q := newApprovalQueue(b, func(string, string) int { return idNo }, t.Logf, 2)
	var accepted []bool
	for i := 0; i < 4; i++ {
		accepted = append(accepted, q.Enqueue(pair.RequestInfo{RequestID: string(rune('a' + i))}))
	}
	want := []bool{true, true, false, false}
	for i := range want {
		if accepted[i] != want[i] {
			t.Fatalf("Enqueue results = %v, want %v", accepted, want)
		}
	}
	first, second := <-q.queue, <-q.queue
	if first.RequestID != "a" || second.RequestID != "b" {
		t.Fatalf("queued = %q, %q; want the oldest a, b kept", first.RequestID, second.RequestID)
	}
}

func TestApprovalQueueShowsOneDialogAtATime(t *testing.T) {
	b, _ := newApprovalBroker(t)
	var reqs []pair.RequestInfo
	for i := 0; i < 3; i++ {
		reqs = append(reqs, requestPairing(t, b, string(rune('a'+i)), "Phone", "10.0.0.5"))
	}
	var open, maxOpen, total int32
	done := make(chan struct{})
	q := newApprovalQueue(b, func(string, string) int {
		n := atomic.AddInt32(&open, 1)
		for {
			m := atomic.LoadInt32(&maxOpen)
			if n <= m || atomic.CompareAndSwapInt32(&maxOpen, m, n) {
				break
			}
		}
		time.Sleep(5 * time.Millisecond)
		atomic.AddInt32(&open, -1)
		if atomic.AddInt32(&total, 1) == 3 {
			close(done)
		}
		return idNo
	}, t.Logf, maxPendingDialogs)
	q.Start()
	for _, r := range reqs {
		if !q.Enqueue(r) {
			t.Fatalf("Enqueue(%s) dropped", r.RequestID)
		}
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("dialogs not all shown")
	}
	if maxOpen != 1 {
		t.Fatalf("max concurrent dialogs = %d, want 1", maxOpen)
	}
}

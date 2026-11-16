package pair

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newTestBroker(t *testing.T) *Broker {
	t.Helper()
	t.Setenv("CLIPBOARD_SHARE_STATE", filepath.Join(t.TempDir(), "state.json"))
	return NewBroker()
}

func approvedDevice(t *testing.T, b *Broker, id, ip string) (requestID, token string) {
	t.Helper()
	req, err := b.Request(id, "Pixel 8", ip)
	if err != nil {
		t.Fatalf("Request: %v", err)
	}
	if req.RequestID == "" || req.Token != "" {
		t.Fatalf("unknown device result = %+v, want pending request", req)
	}
	if err := b.Approve(req.RequestID); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	status, err := b.Status(req.RequestID, ip)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if status.State != StateApproved || status.Token == "" {
		t.Fatalf("approved status = %+v", status)
	}
	return req.RequestID, status.Token
}

func TestRequestApproveStatusTokenOnce(t *testing.T) {
	b := newTestBroker(t)
	req, err := b.Request("device-1", "Pixel 8", "192.0.2.10")
	if err != nil {
		t.Fatal(err)
	}
	status, err := b.Status(req.RequestID, "192.0.2.10")
	if err != nil || status.State != StatePending || status.Token != "" {
		t.Fatalf("pending status = %+v, err = %v", status, err)
	}
	if _, err := b.Status(req.RequestID, "192.0.2.11"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("cross-IP status error = %v, want ErrForbidden", err)
	}
	if err := b.Approve(req.RequestID); err != nil {
		t.Fatal(err)
	}
	status, err = b.Status(req.RequestID, "192.0.2.10")
	if err != nil || status.State != StateApproved || status.Token == "" {
		t.Fatalf("approved status = %+v, err = %v", status, err)
	}
	if !b.Validate(status.Token) {
		t.Fatal("approved token does not validate")
	}
	second, err := b.Status(req.RequestID, "192.0.2.10")
	if err != nil || second.State != StateApproved || second.Token != "" {
		t.Fatalf("second approved status = %+v, err = %v; token must be sent once", second, err)
	}
	if _, err := b.Status("missing", "192.0.2.10"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown status error = %v, want ErrNotFound", err)
	}
}

func TestKnownReissuesTokenAndSurvivesReload(t *testing.T) {
	b := newTestBroker(t)
	_, oldToken := approvedDevice(t, b, "device-1", "192.0.2.10")
	known, err := b.Request("device-1", "Pixel 8 Pro", "192.0.2.20")
	if err != nil {
		t.Fatal(err)
	}
	if known.RequestID != "" || known.Token == "" || known.Token == oldToken {
		t.Fatalf("known device result = %+v", known)
	}
	if b.Validate(oldToken) || !b.Validate(known.Token) {
		t.Fatal("re-pair must revoke the old token and validate only the new token")
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Validate(oldToken) || !loaded.Validate(known.Token) {
		t.Fatal("persisted token set does not match the latest re-pair")
	}
	devices, requests := loaded.Snapshot()
	if len(devices) != 1 || len(requests) != 0 {
		t.Fatalf("loaded snapshot: devices=%d requests=%d", len(devices), len(requests))
	}
	if devices[0].ID != "device-1" || devices[0].Name != "Pixel 8 Pro" || devices[0].IP != "192.0.2.20" || devices[0].FirstApproved.IsZero() {
		t.Fatalf("loaded device = %+v", devices[0])
	}
	info, err := os.Stat(StatePath())
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("state mode = %o, want 600", got)
	}
	data, err := os.ReadFile(StatePath())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "requestId") || strings.Contains(string(data), "pending") {
		t.Fatalf("pending requests were persisted: %s", data)
	}
}

func TestKnownMethodAndRevokePersist(t *testing.T) {
	b := newTestBroker(t)
	_, first := approvedDevice(t, b, "device-1", "192.0.2.10")
	second, err := b.Known("device-1")
	if err != nil || second == "" || second == first {
		t.Fatalf("Known: token=%q err=%v", second, err)
	}
	if b.Validate(first) || !b.Validate(second) {
		t.Fatal("Known did not rotate the token")
	}
	if err := b.Revoke("device-1"); err != nil {
		t.Fatal(err)
	}
	if b.Validate(second) {
		t.Fatal("revoked token still validates")
	}
	loaded, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Validate(second) {
		t.Fatal("revocation was not persisted")
	}
	if err := b.Revoke("device-1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second Revoke error = %v, want ErrNotFound", err)
	}
}

func TestPendingLimitExpiryAndDeny(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	t.Setenv("CLIPBOARD_SHARE_STATE", filepath.Join(t.TempDir(), "state.json"))
	b := NewBrokerWithClock(func() time.Time { return now })

	requestIDs := make([]string, 0, MaxPendingPerIP)
	for i := 0; i < MaxPendingPerIP; i++ {
		req, err := b.Request(fmt.Sprintf("device-%d", i), "Phone", "192.0.2.10")
		if err != nil {
			t.Fatalf("request %d: %v", i, err)
		}
		requestIDs = append(requestIDs, req.RequestID)
	}
	if _, err := b.Request("device-overflow", "Phone", "192.0.2.10"); !errors.Is(err, ErrTooManyPending) {
		t.Fatalf("sixth request error = %v, want ErrTooManyPending", err)
	}
	if _, err := b.Request("device-other-ip", "Phone", "192.0.2.11"); err != nil {
		t.Fatalf("other IP must have an independent limit: %v", err)
	}
	if err := b.Deny(requestIDs[1]); err != nil {
		t.Fatalf("Deny: %v", err)
	}

	now = now.Add(PendingTTL)
	status, err := b.Status(requestIDs[0], "192.0.2.10")
	if !errors.Is(err, ErrExpired) || status.State != StateExpired || status.Token != "" {
		t.Fatalf("expired status = %+v, err = %v", status, err)
	}
	if err := b.Approve(requestIDs[0]); !errors.Is(err, ErrNotFound) {
		t.Fatalf("approving expired request error = %v, want ErrNotFound", err)
	}
	if _, err := b.Request("device-after-expiry", "Phone", "192.0.2.10"); err != nil {
		t.Fatalf("expired requests must not count toward the limit: %v", err)
	}
	status, err = b.Status(requestIDs[1], "192.0.2.10")
	if err != nil || status.State != StateDenied || status.Token != "" {
		t.Fatalf("denied status = %+v, err = %v", status, err)
	}
}

func TestApproveLastUsesNewestPending(t *testing.T) {
	b := newTestBroker(t)
	first, err := b.Request("device-1", "First", "192.0.2.10")
	if err != nil {
		t.Fatal(err)
	}
	second, err := b.Request("device-2", "Second", "192.0.2.10")
	if err != nil {
		t.Fatal(err)
	}
	if err := b.ApproveLast(); err != nil {
		t.Fatal(err)
	}
	if status, err := b.Status(first.RequestID, "192.0.2.10"); err != nil || status.State != StatePending {
		t.Fatalf("first request = %+v, err = %v; want pending", status, err)
	}
	if status, err := b.Status(second.RequestID, "192.0.2.10"); err != nil || status.State != StateApproved || status.Token == "" {
		t.Fatalf("newest request = %+v, err = %v; want approved", status, err)
	}
}

func TestRequestNotificationOnlyForNewDevices(t *testing.T) {
	b := newTestBroker(t)
	notified := make(chan RequestInfo, 2)
	b.OnRequest(func(req RequestInfo) { notified <- req })
	req, err := b.Request("device-1", "Pixel 8", "192.0.2.10")
	if err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-notified:
		if got.RequestID != req.RequestID || got.ID != "device-1" || got.Name != "Pixel 8" || got.IP != "192.0.2.10" {
			t.Fatalf("notification = %+v", got)
		}
	default:
		t.Fatal("new request did not trigger notification callback")
	}
	if err := b.Approve(req.RequestID); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Request("device-1", "Pixel 8", "192.0.2.10"); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-notified:
		t.Fatalf("known re-pair triggered notification: %+v", got)
	default:
	}
}

func TestControlSocketCommandsAndTokenPrivacy(t *testing.T) {
	b := newTestBroker(t)
	first, err := b.Request("device-1", "First", "192.0.2.10")
	if err != nil {
		t.Fatal(err)
	}
	second, err := b.Request("device-2", "Second", "192.0.2.11")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLIPBOARD_SHARE_CTL_SOCK", filepath.Join(t.TempDir(), "ctl.sock"))
	control, err := ListenControl(b)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { control.Close() })
	if info, err := os.Stat(ControlPath()); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("socket info=%v err=%v, want mode 600", info, err)
	}

	send := func(cmd string) map[string]any {
		t.Helper()
		conn, err := net.Dial("unix", ControlPath())
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
		if _, err := fmt.Fprintln(conn, cmd); err != nil {
			t.Fatal(err)
		}
		line, err := bufio.NewReader(conn).ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(line, "token") {
			t.Fatalf("socket response exposed a token field: %s", line)
		}
		var response map[string]any
		if err := json.Unmarshal([]byte(line), &response); err != nil {
			t.Fatalf("invalid socket response %q: %v", line, err)
		}
		return response
	}

	response := send(`{"cmd":"list"}`)
	if response["ok"] != true || len(response["requests"].([]any)) != 2 || len(response["devices"].([]any)) != 0 {
		t.Fatalf("initial list = %#v", response)
	}
	response = send(`{"cmd":"approve-last"}`)
	if response["ok"] != true {
		t.Fatalf("approve-last = %#v", response)
	}
	response = send(fmt.Sprintf(`{"cmd":"deny","request":%q}`, first.RequestID))
	if response["ok"] != true {
		t.Fatalf("deny = %#v", response)
	}
	status, err := b.Status(first.RequestID, "192.0.2.10")
	if err != nil || status.State != StateDenied {
		t.Fatalf("denied status = %+v, err = %v", status, err)
	}
	status, err = b.Status(second.RequestID, "192.0.2.11")
	if err != nil || status.State != StateApproved || status.Token == "" {
		t.Fatalf("approved status = %+v, err = %v", status, err)
	}
	response = send(`{"cmd":"list"}`)
	if len(response["requests"].([]any)) != 0 || len(response["devices"].([]any)) != 1 {
		t.Fatalf("resolved list = %#v", response)
	}
	response = send(`{"cmd":"revoke","device":"device-2"}`)
	if response["ok"] != true || b.Validate(status.Token) {
		t.Fatalf("revoke response = %#v; token still valid = %v", response, b.Validate(status.Token))
	}
	response = send(`{"cmd":"unknown"}`)
	if response["ok"] != false || response["error"] == "" {
		t.Fatalf("unknown command response = %#v", response)
	}
}

func TestListenControlRemovesStaleSocket(t *testing.T) {
	b := newTestBroker(t)
	path := filepath.Join(t.TempDir(), "ctl.sock")
	t.Setenv("CLIPBOARD_SHARE_CTL_SOCK", path)
	stale, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	stale.(*net.UnixListener).SetUnlinkOnClose(false)
	if err := stale.Close(); err != nil {
		t.Fatal(err)
	}
	control, err := ListenControl(b)
	if err != nil {
		t.Fatalf("ListenControl with stale socket: %v", err)
	}
	if err := control.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("socket remains after Close: %v", err)
	}
}

// Package pair implements device-identity pairing with explicit approval on
// the server. Approved identities and their current tokens are persisted;
// pending requests live only in memory.
package pair

import (
	"bufio"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

const (
	// PendingTTL is how long a new device request can be approved or polled.
	PendingTTL = 90 * time.Second
	// MaxPendingPerIP bounds unresolved requests from one source address.
	MaxPendingPerIP = 5
)

var (
	ErrNotFound       = errors.New("pair: request or device not found")
	ErrForbidden      = errors.New("pair: request belongs to a different IP")
	ErrTooManyPending = errors.New("pair: too many pending requests from this IP")
	ErrExpired        = errors.New("pair: request expired")
	ErrInvalid        = errors.New("pair: invalid device identity")
)

// State is the externally visible state of a pairing request.
type State string

const (
	StatePending  State = "pending"
	StateDenied   State = "denied"
	StateExpired  State = "expired"
	StateApproved State = "approved"
)

// RequestResult contains either RequestID for a new, pending identity or an
// immediately usable Token for an already approved identity.
type RequestResult struct {
	RequestID string `json:"requestId,omitempty"`
	Token     string `json:"token,omitempty"`
}

// StatusResult contains a request state. Token is non-empty only on the
// first same-IP status poll after approval.
type StatusResult struct {
	State State  `json:"state"`
	Token string `json:"token,omitempty"`
}

// RequestInfo is the token-free, externally visible form of a request.
type RequestInfo struct {
	RequestID   string    `json:"requestId"`
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	IP          string    `json:"ip"`
	RequestedAt time.Time `json:"requestedAt"`
}

// DeviceInfo is the token-free, externally visible form of an approved
// device. Persisted tokens never appear in control-socket responses.
type DeviceInfo struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	IP            string    `json:"ip"`
	FirstApproved time.Time `json:"firstApproved"`
}

type device struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Token         string    `json:"token"`
	IP            string    `json:"ip"`
	FirstApproved time.Time `json:"firstApproved"`
}

type pendingRequest struct {
	RequestInfo
	state State
	token string
	seq   uint64
}

// Broker owns pending requests and approved devices. It is safe for
// concurrent use. Save failures leave the previous in-memory state intact.
type Broker struct {
	mu        sync.Mutex
	path      string
	now       func() time.Time
	devices   map[string]device
	pending   map[string]*pendingRequest
	sequence  uint64
	onRequest func(RequestInfo)
}

// NewBroker returns an empty broker using StatePath for persistence. Use Load
// at server startup to restore approved devices.
func NewBroker() *Broker {
	return newBroker(StatePath(), time.Now)
}

// NewBrokerWithClock returns an empty broker with an injectable clock. It is
// useful for deterministic expiration tests.
func NewBrokerWithClock(now func() time.Time) *Broker {
	if now == nil {
		now = time.Now
	}
	return newBroker(StatePath(), now)
}

func newBroker(path string, now func() time.Time) *Broker {
	return &Broker{
		path:    path,
		now:     now,
		devices: make(map[string]device),
		pending: make(map[string]*pendingRequest),
	}
}

// Load restores approved devices from StatePath. A missing state file is an
// empty broker; pending requests are deliberately never restored.
func Load() (*Broker, error) {
	b := NewBroker()
	data, err := os.ReadFile(b.path)
	if os.IsNotExist(err) {
		return b, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load pairing state: %w", err)
	}
	var stored struct {
		Devices []device `json:"devices"`
	}
	if err := json.Unmarshal(data, &stored); err != nil {
		return nil, fmt.Errorf("decode pairing state: %w", err)
	}
	for _, d := range stored.Devices {
		if d.ID == "" || d.Token == "" {
			return nil, fmt.Errorf("decode pairing state: device id and token are required")
		}
		b.devices[d.ID] = d
	}
	return b, nil
}

// Save atomically persists approved devices with mode 0600. Pending requests
// are not included.
func (b *Broker) Save() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	return saveDevices(b.path, b.devices)
}

func saveDevices(path string, devices map[string]device) error {
	list := make([]device, 0, len(devices))
	for _, d := range devices {
		list = append(list, d)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].ID < list[j].ID })
	stored := struct {
		Devices []device `json:"devices"`
	}{Devices: list}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create pairing state directory: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".state-*.tmp")
	if err != nil {
		return fmt.Errorf("create pairing state temporary file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	// On Windows Chmod only toggles the read-only attribute (0600 keeps the
	// file writable); access control comes from the per-user %APPDATA% ACL.
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return fmt.Errorf("secure pairing state temporary file: %w", err)
	}
	encoder := json.NewEncoder(tmp)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(stored); err != nil {
		tmp.Close()
		return fmt.Errorf("encode pairing state: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close pairing state: %w", err)
	}
	if err := replaceFile(tmpName, path); err != nil {
		return fmt.Errorf("replace pairing state: %w", err)
	}
	return nil
}

func cloneDevices(devices map[string]device) map[string]device {
	cloned := make(map[string]device, len(devices))
	for id, d := range devices {
		cloned[id] = d
	}
	return cloned
}

// OnRequest registers a callback invoked outside the broker lock whenever a
// new, unknown device creates a pending request. Known re-pairs do not notify.
func (b *Broker) OnRequest(fn func(RequestInfo)) {
	b.mu.Lock()
	b.onRequest = fn
	b.mu.Unlock()
}

// Request starts pairing for an unknown identity. A known identity gets a new
// token immediately, atomically revoking and replacing its previous token.
func (b *Broker) Request(id, name, ip string) (RequestResult, error) {
	if id == "" || name == "" {
		return RequestResult{}, ErrInvalid
	}
	b.mu.Lock()
	if d, ok := b.devices[id]; ok {
		token := randomToken()
		d.Name = name
		d.IP = ip
		d.Token = token
		next := cloneDevices(b.devices)
		next[id] = d
		if err := saveDevices(b.path, next); err != nil {
			b.mu.Unlock()
			return RequestResult{}, err
		}
		b.devices = next
		b.mu.Unlock()
		return RequestResult{Token: token}, nil
	}

	now := b.now()
	b.expireLocked(now)
	count := 0
	for _, req := range b.pending {
		if req.IP == ip && req.state == StatePending {
			count++
		}
	}
	if count >= MaxPendingPerIP {
		b.mu.Unlock()
		return RequestResult{}, ErrTooManyPending
	}
	b.sequence++
	req := &pendingRequest{
		RequestInfo: RequestInfo{
			RequestID:   randomToken(),
			ID:          id,
			Name:        name,
			IP:          ip,
			RequestedAt: now,
		},
		state: StatePending,
		seq:   b.sequence,
	}
	b.pending[req.RequestID] = req
	info := req.RequestInfo
	callback := b.onRequest
	b.mu.Unlock()
	if callback != nil {
		callback(info)
	}
	return RequestResult{RequestID: info.RequestID}, nil
}

// Status polls a pending request from its source IP. Approval tokens are
// returned once; an expired request returns StateExpired together with
// ErrExpired so callers can map both the body and HTTP status explicitly.
func (b *Broker) Status(requestID, ip string) (StatusResult, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.expireLocked(b.now())
	req, ok := b.pending[requestID]
	if !ok {
		return StatusResult{}, ErrNotFound
	}
	if req.IP != ip {
		return StatusResult{}, ErrForbidden
	}
	if req.state == StateExpired {
		return StatusResult{State: StateExpired}, ErrExpired
	}
	result := StatusResult{State: req.state, Token: req.token}
	req.token = ""
	return result, nil
}

// Approve approves a specific pending request and persists the device.
func (b *Broker) Approve(requestID string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.expireLocked(b.now())
	req, ok := b.pending[requestID]
	if !ok || req.state != StatePending {
		return ErrNotFound
	}
	return b.approveLocked(req)
}

// ApproveLast approves the most recently created unresolved request.
func (b *Broker) ApproveLast() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.expireLocked(b.now())
	var newest *pendingRequest
	for _, req := range b.pending {
		if req.state == StatePending && (newest == nil || req.seq > newest.seq) {
			newest = req
		}
	}
	if newest == nil {
		return ErrNotFound
	}
	return b.approveLocked(newest)
}

func (b *Broker) approveLocked(req *pendingRequest) error {
	token := randomToken()
	d := device{
		ID:            req.ID,
		Name:          req.Name,
		Token:         token,
		IP:            req.IP,
		FirstApproved: b.now(),
	}
	next := cloneDevices(b.devices)
	next[d.ID] = d
	if err := saveDevices(b.path, next); err != nil {
		return err
	}
	b.devices = next
	req.state = StateApproved
	req.token = token
	return nil
}

// Deny rejects a pending request. Denial is in memory only.
func (b *Broker) Deny(requestID string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.expireLocked(b.now())
	req, ok := b.pending[requestID]
	if !ok || req.state != StatePending {
		return ErrNotFound
	}
	req.state = StateDenied
	req.token = ""
	return nil
}

// Known explicitly rotates a known device's current token.
func (b *Broker) Known(deviceID string) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	d, ok := b.devices[deviceID]
	if !ok {
		return "", ErrNotFound
	}
	token := randomToken()
	d.Token = token
	next := cloneDevices(b.devices)
	next[deviceID] = d
	if err := saveDevices(b.path, next); err != nil {
		return "", err
	}
	b.devices = next
	return token, nil
}

// Revoke removes a device and persists the removal, invalidating its token.
func (b *Broker) Revoke(deviceID string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, ok := b.devices[deviceID]; !ok {
		return ErrNotFound
	}
	next := cloneDevices(b.devices)
	delete(next, deviceID)
	if err := saveDevices(b.path, next); err != nil {
		return err
	}
	b.devices = next
	return nil
}

// Validate reports whether token belongs to any currently approved device.
// Its signature intentionally matches api.TokenValidator.
func (b *Broker) Validate(token string) bool {
	if token == "" {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	valid := false
	for _, d := range b.devices {
		if subtle.ConstantTimeCompare([]byte(d.Token), []byte(token)) == 1 {
			valid = true
		}
	}
	return valid
}

// Snapshot returns token-free pending requests and approved devices. Resolved
// requests are omitted; both lists have deterministic ordering.
func (b *Broker) Snapshot() (devices []DeviceInfo, requests []RequestInfo) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.expireLocked(b.now())
	devices = make([]DeviceInfo, 0, len(b.devices))
	for _, d := range b.devices {
		devices = append(devices, DeviceInfo{
			ID:            d.ID,
			Name:          d.Name,
			IP:            d.IP,
			FirstApproved: d.FirstApproved,
		})
	}
	sort.Slice(devices, func(i, j int) bool { return devices[i].ID < devices[j].ID })
	requests = make([]RequestInfo, 0)
	for _, req := range b.pending {
		if req.state == StatePending {
			requests = append(requests, req.RequestInfo)
		}
	}
	sort.Slice(requests, func(i, j int) bool {
		if requests[i].RequestedAt.Equal(requests[j].RequestedAt) {
			return requests[i].RequestID < requests[j].RequestID
		}
		return requests[i].RequestedAt.Before(requests[j].RequestedAt)
	})
	return devices, requests
}

func (b *Broker) expireLocked(now time.Time) {
	for _, req := range b.pending {
		if req.state == StatePending && !now.Before(req.RequestedAt.Add(PendingTTL)) {
			req.state = StateExpired
			req.token = ""
		}
	}
}

func randomToken() string {
	var buf [32]byte
	if _, err := rand.Read(buf[:]); err != nil {
		panic("pair: crypto/rand unavailable: " + err.Error())
	}
	return hex.EncodeToString(buf[:])
}

// ControlServer serves newline-delimited JSON commands on a user-only Unix
// socket. It never exposes device tokens.
type ControlServer struct {
	listener net.Listener
	path     string
	done     chan struct{}
	close    sync.Once
	closeErr error
}

type controlCommand struct {
	Cmd       string `json:"cmd"`
	Request   string `json:"request"`
	RequestID string `json:"requestId"`
	Device    string `json:"device"`
	DeviceID  string `json:"deviceId"`
}

type controlResponse struct {
	OK       bool          `json:"ok"`
	Error    string        `json:"error,omitempty"`
	Message  string        `json:"message,omitempty"`
	Requests []RequestInfo `json:"requests"`
	Devices  []DeviceInfo  `json:"devices"`
}

// isSocketLike reports whether an existing control path may be a (stale)
// socket we are allowed to replace. On Windows an AF_UNIX socket file is a
// reparse point that Go reports as ModeSocket; ModeIrregular (unknown reparse
// tag) is accepted too so a stale socket never blocks startup there.
func isSocketLike(goos string, mode os.FileMode) bool {
	if mode&os.ModeSocket != 0 {
		return true
	}
	return goos == "windows" && mode.Type() == os.ModeIrregular
}

// ListenControl removes a stale socket and starts serving ControlPath. An
// active socket is never removed. Close unlinks the socket created here.
// Windows 10 1803+ supports AF_UNIX; there os.Chmod is not meaningful for a
// socket (the per-user %APPDATA% ACL protects it), so a chmod failure is
// ignored on Windows and fatal elsewhere.
func ListenControl(b *Broker) (*ControlServer, error) {
	path := ControlPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create control socket directory: %w", err)
	}
	if info, err := os.Lstat(path); err == nil {
		if !isSocketLike(hostOS, info.Mode()) {
			return nil, fmt.Errorf("control path exists and is not a socket: %s", path)
		}
		conn, dialErr := net.DialTimeout("unix", path, 200*time.Millisecond)
		if dialErr == nil {
			conn.Close()
			return nil, fmt.Errorf("pairing control server is already running at %s", path)
		}
		if err := os.Remove(path); err != nil {
			return nil, fmt.Errorf("remove stale control socket: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("inspect control socket: %w", err)
	}
	listener, err := net.Listen("unix", path)
	if err != nil {
		return nil, fmt.Errorf("listen on control socket %s: %w", path, err)
	}
	if unixListener, ok := listener.(*net.UnixListener); ok {
		unixListener.SetUnlinkOnClose(false)
	}
	if err := chmodPath(path, 0o600); err != nil && hostOS != "windows" {
		listener.Close()
		_ = os.Remove(path)
		return nil, fmt.Errorf("secure control socket: %w", err)
	}
	server := &ControlServer{listener: listener, path: path, done: make(chan struct{})}
	go server.accept(b)
	return server, nil
}

func (s *ControlServer) accept(b *Broker) {
	defer close(s.done)
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		go serveControlConn(conn, b)
	}
}

// Close stops accepting commands and removes this server's socket.
func (s *ControlServer) Close() error {
	s.close.Do(func() {
		s.closeErr = s.listener.Close()
		<-s.done
		if err := os.Remove(s.path); err != nil && !os.IsNotExist(err) && s.closeErr == nil {
			s.closeErr = err
		}
	})
	return s.closeErr
}

func serveControlConn(conn net.Conn, b *Broker) {
	defer conn.Close()
	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 1024), 64*1024)
	writer := json.NewEncoder(conn)
	for scanner.Scan() {
		var command controlCommand
		response := controlResponse{}
		if err := json.Unmarshal(scanner.Bytes(), &command); err != nil {
			response.Error = "invalid JSON command: " + err.Error()
			_ = writer.Encode(response)
			continue
		}
		response = b.control(command)
		_ = writer.Encode(response)
	}
}

func (b *Broker) control(command controlCommand) controlResponse {
	switch command.Cmd {
	case "list":
		devices, requests := b.Snapshot()
		return controlResponse{OK: true, Requests: requests, Devices: devices}
	case "approve-last":
		if err := b.ApproveLast(); err != nil {
			return controlResponse{Error: err.Error()}
		}
		return controlResponse{OK: true, Message: "Approved newest pair request."}
	case "approve":
		id := command.Request
		if id == "" {
			id = command.RequestID
		}
		if err := b.Approve(id); err != nil {
			return controlResponse{Error: err.Error()}
		}
		return controlResponse{OK: true, Message: "Approved pair request."}
	case "deny":
		id := command.Request
		if id == "" {
			id = command.RequestID
		}
		if err := b.Deny(id); err != nil {
			return controlResponse{Error: err.Error()}
		}
		return controlResponse{OK: true, Message: "Denied pair request."}
	case "revoke":
		id := command.Device
		if id == "" {
			id = command.DeviceID
		}
		if err := b.Revoke(id); err != nil {
			return controlResponse{Error: err.Error()}
		}
		return controlResponse{OK: true, Message: "Revoked device."}
	default:
		return controlResponse{Error: fmt.Sprintf("unknown command %q", command.Cmd)}
	}
}

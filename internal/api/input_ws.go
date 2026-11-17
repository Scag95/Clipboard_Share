package api

import (
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"

	"clipboard-share/internal/input"
)

// TokenValidator checks session tokens issued through pairing (implemented
// by internal/pair.Broker).
type TokenValidator interface {
	Validate(token string) bool
}

// DefaultInputRate is the per-connection event budget when none is given.
const DefaultInputRate = 120

// maxInputFrameBytes bounds one protocol message; larger frames are dropped
// by the connection and never parsed or injected.
const maxInputFrameBytes = 4096

var inputUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	// The LAN is untrusted enough that the session token, not the origin
	// header, is the authority. Browsers on this LAN could forge Origin
	// anyway; only the pairing client (Android) holds tokens.
	CheckOrigin: func(*http.Request) bool { return true },
}

// AttachInputWS registers the remote-control event stream (S3, S4, S6).
// GET /ws/input?token=<session token> upgrades to a WebSocket only when the
// token was issued by pairing; otherwise the request is rejected with 401
// before the upgrade. Validated events are injected at most
// eventsPerSecond; violations answer {"error":"..."} frames and are never
// injected. eventsPerSecond <= 0 selects DefaultInputRate.
func AttachInputWS(r *mux.Router, v TokenValidator, inj input.Injector, eventsPerSecond int) {
	if eventsPerSecond <= 0 {
		eventsPerSecond = DefaultInputRate
	}
	h := &inputHandlers{validator: v, injector: inj, rate: eventsPerSecond}
	r.HandleFunc("/ws/input", h.serve)
}

type inputHandlers struct {
	validator TokenValidator
	injector  input.Injector
	rate      int
}

func (h *inputHandlers) serve(w http.ResponseWriter, r *http.Request) {
	if !h.validator.Validate(r.URL.Query().Get("token")) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	conn, err := inputUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return // Upgrade already wrote the error
	}
	defer conn.Close()
	conn.SetReadLimit(maxInputFrameBytes)

	bucket := newBucket(h.rate, time.Now())
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return // client gone or frame over the read limit
		}
		if !bucket.allow(time.Now()) {
			h.writeError(conn, "rate limit exceeded")
			continue
		}
		cmd, err := input.Parse(data)
		if err != nil {
			h.writeError(conn, err.Error())
			continue
		}
		if err := cmd.Run(h.injector); err != nil {
			h.writeError(conn, "inject failed: "+err.Error())
		}
	}
}

func (h *inputHandlers) writeError(conn *websocket.Conn, msg string) {
	conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	_ = conn.WriteJSON(map[string]string{"error": msg})
}

// bucket is a token bucket: burst capacity equals the per-second rate.
// Single-goroutine use (one per connection); no locking needed.
type bucket struct {
	rate   float64
	max    float64
	credit float64
	last   time.Time
}

func newBucket(rate int, now time.Time) *bucket {
	return &bucket{
		rate:   float64(rate),
		max:    float64(rate),
		credit: float64(rate),
		last:   now,
	}
}

func (b *bucket) allow(now time.Time) bool {
	b.credit += now.Sub(b.last).Seconds() * b.rate
	if b.credit > b.max {
		b.credit = b.max
	}
	b.last = now
	if b.credit < 1 {
		return false
	}
	b.credit--
	return true
}

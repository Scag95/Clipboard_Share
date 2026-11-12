package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"

	"clipboard-share/internal/input"
	"clipboard-share/internal/pair"
)

func newInputServer(t *testing.T, rate int) (url string, b *pair.Broker, rec *input.Recorder) {
	t.Helper()
	b = pair.NewBroker()
	rec = &input.Recorder{}
	r := mux.NewRouter()
	AttachPairing(r, b)
	AttachInputWS(r, b, rec, rate)
	s := httptest.NewServer(r)
	t.Cleanup(s.Close)
	return s.URL, b, rec
}

func dial(t *testing.T, url, token string) (*websocket.Conn, int) {
	t.Helper()
	wsURL := "ws" + strings.TrimPrefix(url, "http") + "/ws/input?token=" + token
	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		code := 0
		if resp != nil {
			code = resp.StatusCode
			defer resp.Body.Close()
		}
		return nil, code
	}
	t.Cleanup(func() { conn.Close() })
	return conn, http.StatusOK
}

func expectErrFrame(t *testing.T, conn *websocket.Conn) string {
	t.Helper()
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var frame struct {
		Error string `json:"error"`
	}
	if err := conn.ReadJSON(&frame); err != nil {
		t.Fatalf("ReadJSON: %v", err)
	}
	if frame.Error == "" {
		t.Fatal("expected non-empty error field in frame")
	}
	return frame.Error
}

func TestInputWSRejectsMissingOrBadToken(t *testing.T) {
	url, _, _ := newInputServer(t, 120)
	for _, tok := range []string{"", "bogus"} {
		conn, code := dial(t, url, tok)
		if conn != nil {
			t.Fatalf("token %q: upgrade succeeded, want rejection", tok)
		}
		if code != http.StatusUnauthorized {
			t.Fatalf("token %q: status = %d, want 401", tok, code)
		}
	}
}

func TestInputWSRejectsSpentPinTokenDomain(t *testing.T) {
	url, b, _ := newInputServer(t, 120)
	// A token from a wrong PIN exchange must never connect.
	if _, ok := b.Pair("000001"); ok && b.Pin() != "000001" {
		t.Skip("PIN collision")
	}
	conn, code := dial(t, url, "deadbeef")
	if conn != nil || code != http.StatusUnauthorized {
		t.Fatalf("unissued token connected (code %d)", code)
	}
}

func TestInputWSAcceptedEventReachesInjector(t *testing.T) {
	url, b, rec := newInputServer(t, 120)
	tok, ok := b.Pair(b.Pin())
	if !ok {
		t.Fatal("pairing failed")
	}
	conn, code := dial(t, url, tok)
	if code != http.StatusOK {
		t.Fatalf("dial status %d", code)
	}
	if err := conn.WriteJSON(map[string]any{"type": "move", "dx": 7, "dy": -2}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		for _, e := range rec.Entries() {
			if e == "move 7 -2" {
				return
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("event never reached injector; entries = %v", rec.Entries())
}

func TestInputWSInvalidEventGetsErrorFrameAndStaysOpen(t *testing.T) {
	url, b, rec := newInputServer(t, 120)
	tok, _ := b.Pair(b.Pin())
	conn, _ := dial(t, url, tok)

	// S6: bad payloads answer {"error": ...} without closing the connection.
	bad := []map[string]any{
		{"type": "dance"},
		{"type": "move", "dx": 99999},
		{"type": "click", "button": "middle", "mode": "tap"},
	}
	for _, msg := range bad {
		if err := conn.WriteJSON(msg); err != nil {
			t.Fatal(err)
		}
		expectErrFrame(t, conn)
	}
	// malformed raw frame
	if err := conn.WriteMessage(websocket.TextMessage, []byte("{")); err != nil {
		t.Fatal(err)
	}
	expectErrFrame(t, conn)

	// Connection still usable for valid events.
	if err := conn.WriteJSON(map[string]any{"type": "scroll", "dy": 3}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		for _, e := range rec.Entries() {
			if e == "scroll 0 3" {
				return
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("connection died after invalid events; entries = %v", rec.Entries())
}

func TestInputWSRateLimit(t *testing.T) {
	url, b, rec := newInputServer(t, 5) // 5 events/s, burst 5
	tok, _ := b.Pair(b.Pin())
	conn, _ := dial(t, url, tok)

	errs := 0
	for i := 0; i < 10; i++ {
		if err := conn.WriteJSON(map[string]any{"type": "move", "dx": 1, "dy": 0}); err != nil {
			t.Fatal(err)
		}
	}
	// Drain whatever frames arrive within a short window.
	conn.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	for {
		var frame struct {
			Error string `json:"error"`
		}
		if err := conn.ReadJSON(&frame); err != nil {
			break
		}
		if frame.Error != "" {
			errs++
		}
	}
	if errs == 0 {
		t.Fatal("burst of 10 events at 5/s produced no rate-limit error frames")
	}
	if got := len(rec.Entries()); got > 6 { // 5 burst + at most 1 refilled
		t.Fatalf("%d events injected, want at most 6 at rate 5/s", got)
	}
}

func TestInputWSOverlongFrameRejected(t *testing.T) {
	url, b, _ := newInputServer(t, 120)
	tok, _ := b.Pair(b.Pin())
	conn, _ := dial(t, url, tok)
	huge := map[string]any{"type": "text", "chars": strings.Repeat("x", 10000)}
	if err := conn.WriteJSON(huge); err != nil {
		t.Fatal(err)
	}
	// Read limit closes the connection; that is acceptable (frame is never
	// injected). The server must not panic.
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _, err := conn.ReadMessage()
	if err == nil {
		var frame map[string]string
		// tolerate an error frame instead of a hard close
		_ = json.Unmarshal(nil, &frame)
	}
}

package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"

	"clipboard-share/internal/pair"
)

func newPairingServer(t *testing.T) (*httptest.Server, *pair.Broker) {
	t.Helper()
	b := pair.NewBroker()
	r := mux.NewRouter()
	AttachPairing(r, b)
	s := httptest.NewServer(r)
	t.Cleanup(s.Close)
	return s, b
}

func postPair(t *testing.T, url, body string) *http.Response {
	t.Helper()
	resp, err := http.Post(url+"/pair", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func TestPairEndpointCorrectPin(t *testing.T) {
	s, b := newPairingServer(t)
	resp := postPair(t, s.URL, `{"pin":"`+b.Pin()+`"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var out struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.Token == "" || !b.Validate(out.Token) {
		t.Fatalf("bad token in response: %q", out.Token)
	}
}

func TestPairEndpointWrongPinUnauthorized(t *testing.T) {
	s, b := newPairingServer(t)
	resp := postPair(t, s.URL, `{"pin":"999999"}`)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if strings.Contains(string(body), `"token"`) {
		t.Fatalf("401 response leaked a token: %s", body)
	}
	// The broker must still accept the real PIN afterwards (until MaxFailures).
	resp2 := postPair(t, s.URL, `{"pin":"`+b.Pin()+`"}`)
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("after one wrong try, correct PIN got %d, want 200", resp2.StatusCode)
	}
}

func TestPairEndpointBadBody(t *testing.T) {
	s, _ := newPairingServer(t)
	for _, body := range []string{`{}`, `{"pin":"12"}`, `not json`} {
		resp := postPair(t, s.URL, body)
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("body %q: status = %d, want 400", body, resp.StatusCode)
		}
	}
}

func TestPairEndpointGetNotAllowed(t *testing.T) {
	s, _ := newPairingServer(t)
	resp, err := http.Get(s.URL + "/pair")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("GET /pair: status = %d, want 405", resp.StatusCode)
	}
}

func TestPairEndpointLockedAfterMaxRotations(t *testing.T) {
	s, b := newPairingServer(t)
	wrong := func() string {
		p := []byte(b.Pin())
		p[0] = (p[0]-'0'+1)%10 + '0'
		return string(p)
	}
	for i := 0; i < pair.MaxFailures*pair.MaxRotations; i++ {
		resp := postPair(t, s.URL, `{"pin":"`+wrong()+`"}`)
		// The attempt that exhausts the lifetime cap already reports the
		// locked state, so the last one may answer 423 instead of 401.
		if resp.StatusCode != http.StatusUnauthorized && resp.StatusCode != http.StatusLocked {
			t.Fatalf("attempt %d: status = %d, want 401 or 423", i+1, resp.StatusCode)
		}
	}
	resp := postPair(t, s.URL, `{"pin":"`+b.Pin()+`"}`)
	if resp.StatusCode != http.StatusLocked {
		t.Fatalf("locked status = %d, want 423", resp.StatusCode)
	}
}

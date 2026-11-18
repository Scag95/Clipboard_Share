package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"

	"nudge/internal/pair"
)

type pairingClient struct {
	router *mux.Router
	t      *testing.T
}

func newPairingClient(t *testing.T) (*httptest.Server, *pair.Broker) {
	t.Helper()
	t.Setenv("CLIPBOARD_SHARE_STATE", filepath.Join(t.TempDir(), "state.json"))
	return newPairingClientWithBroker(t, pair.NewBroker())
}

func newPairingClientWithBroker(t *testing.T, b *pair.Broker) (*httptest.Server, *pair.Broker) {
	t.Helper()
	r := mux.NewRouter()
	AttachPairing(r, b)
	s := httptest.NewServer(r)
	t.Cleanup(s.Close)
	return s, b
}

func (c *pairingClient) do(method, target, body, ip string) *http.Response {
	c.t.Helper()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.RemoteAddr = ip + ":12345"
	rec := httptest.NewRecorder()
	c.router.ServeHTTP(rec, req)
	resp := rec.Result()
	c.t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func directPairingClient(t *testing.T, b *pair.Broker) *pairingClient {
	t.Helper()
	r := mux.NewRouter()
	AttachPairing(r, b)
	return &pairingClient{router: r, t: t}
}

func postPairRequest(t *testing.T, url, body string) *http.Response {
	t.Helper()
	resp, err := http.Post(url+"/pair/request", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func decodePairResponse(t *testing.T, resp *http.Response) (requestID, token string) {
	t.Helper()
	var out struct {
		RequestID string `json:"requestId"`
		Token     string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out.RequestID, out.Token
}

func getPairStatus(t *testing.T, url, requestID string) (pair.StatusResult, *http.Response) {
	t.Helper()
	resp, err := http.Get(url + "/pair/status?id=" + requestID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	var out pair.StatusResult
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out, resp
}

func TestPairKnownDeviceGetsImmediateRotatedToken(t *testing.T) {
	s, b := newPairingClient(t)
	resp := postPairRequest(t, s.URL, `{"id":"device-1","name":"Pixel 8"}`)
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("initial status = %d, want 202", resp.StatusCode)
	}
	requestID, _ := decodePairResponse(t, resp)
	if err := b.Approve(requestID); err != nil {
		t.Fatal(err)
	}
	status, statusResp := getPairStatus(t, s.URL, requestID)
	if statusResp.StatusCode != http.StatusOK || status.Token == "" {
		t.Fatalf("approved status = %+v, HTTP %d", status, statusResp.StatusCode)
	}

	resp = postPairRequest(t, s.URL, `{"id":"device-1","name":"Pixel 8"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("known status = %d, want 200", resp.StatusCode)
	}
	knownRequestID, newToken := decodePairResponse(t, resp)
	if knownRequestID != "" || newToken == "" || newToken == status.Token {
		t.Fatalf("known response requestId=%q token=%q", knownRequestID, newToken)
	}
	if b.Validate(status.Token) || !b.Validate(newToken) {
		t.Fatal("known re-pair did not rotate the device token")
	}
}

func TestPairUnknownPendingThenApprovedTokenOnceAndCrossIPForbidden(t *testing.T) {
	s, b := newPairingClient(t)
	resp := postPairRequest(t, s.URL, `{"id":"device-1","name":"Pixel 8"}`)
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("request status = %d, want 202", resp.StatusCode)
	}
	requestID, token := decodePairResponse(t, resp)
	if requestID == "" || token != "" {
		t.Fatalf("pending response requestId=%q token=%q", requestID, token)
	}
	status, statusResp := getPairStatus(t, s.URL, requestID)
	if statusResp.StatusCode != http.StatusOK || status.State != pair.StatePending || status.Token != "" {
		t.Fatalf("pending status = %+v, HTTP %d", status, statusResp.StatusCode)
	}
	if err := b.Approve(requestID); err != nil {
		t.Fatal(err)
	}

	client := directPairingClient(t, b)
	crossResp := client.do(http.MethodGet, "/pair/status?id="+requestID, "", "203.0.113.99")
	if crossResp.StatusCode != http.StatusForbidden {
		t.Fatalf("cross-IP status = %d, want 403", crossResp.StatusCode)
	}
	status, statusResp = getPairStatus(t, s.URL, requestID)
	if statusResp.StatusCode != http.StatusOK || status.State != pair.StateApproved || status.Token == "" {
		t.Fatalf("first approved status = %+v, HTTP %d", status, statusResp.StatusCode)
	}
	status, statusResp = getPairStatus(t, s.URL, requestID)
	if statusResp.StatusCode != http.StatusOK || status.State != pair.StateApproved || status.Token != "" {
		t.Fatalf("second approved status = %+v, HTTP %d; token must be omitted", status, statusResp.StatusCode)
	}
}

func TestPairStatusExpiresAtNinetySeconds(t *testing.T) {
	now := time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)
	t.Setenv("CLIPBOARD_SHARE_STATE", filepath.Join(t.TempDir(), "state.json"))
	b := pair.NewBrokerWithClock(func() time.Time { return now })
	client := directPairingClient(t, b)
	resp := client.do(http.MethodPost, "/pair/request", `{"id":"device-1","name":"Pixel 8"}`, "192.0.2.10")
	requestID, _ := decodePairResponse(t, resp)
	now = now.Add(pair.PendingTTL)
	resp = client.do(http.MethodGet, "/pair/status?id="+requestID, "", "192.0.2.10")
	if resp.StatusCode != http.StatusGone {
		t.Fatalf("expired HTTP status = %d, want 410", resp.StatusCode)
	}
	var status pair.StatusResult
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		t.Fatal(err)
	}
	if status.State != pair.StateExpired || status.Token != "" {
		t.Fatalf("expired response = %+v", status)
	}
}

func TestPairSixthPendingRequestFromSameIPGets429(t *testing.T) {
	s, _ := newPairingClient(t)
	for i := 0; i < pair.MaxPendingPerIP; i++ {
		resp := postPairRequest(t, s.URL, `{"id":"device-`+string(rune('a'+i))+`","name":"Phone"}`)
		if resp.StatusCode != http.StatusAccepted {
			t.Fatalf("request %d status = %d, want 202", i+1, resp.StatusCode)
		}
	}
	resp := postPairRequest(t, s.URL, `{"id":"device-overflow","name":"Phone"}`)
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("sixth status = %d, want 429", resp.StatusCode)
	}
}

func TestPairDeniedStatus(t *testing.T) {
	s, b := newPairingClient(t)
	resp := postPairRequest(t, s.URL, `{"id":"device-1","name":"Pixel 8"}`)
	requestID, _ := decodePairResponse(t, resp)
	if err := b.Deny(requestID); err != nil {
		t.Fatal(err)
	}
	status, statusResp := getPairStatus(t, s.URL, requestID)
	if statusResp.StatusCode != http.StatusOK || status.State != pair.StateDenied || status.Token != "" {
		t.Fatalf("denied status = %+v, HTTP %d", status, statusResp.StatusCode)
	}
}

func TestPairRequestValidationAndUnknownStatus(t *testing.T) {
	s, _ := newPairingClient(t)
	for _, body := range []string{`{}`, `{"id":"","name":"Phone"}`, `{"id":"device-1","name":""}`, `not json`} {
		resp := postPairRequest(t, s.URL, body)
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("body %q: status = %d, want 400", body, resp.StatusCode)
		}
	}
	resp, err := http.Get(s.URL + "/pair/status?id=missing")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown request status = %d, want 404", resp.StatusCode)
	}
}

func TestLegacyPINPairEndpointIsGone(t *testing.T) {
	s, _ := newPairingClient(t)
	resp, err := http.Post(s.URL+"/pair", "application/json", strings.NewReader(`{"pin":"123456"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("POST /pair status = %d, want 404", resp.StatusCode)
	}
}

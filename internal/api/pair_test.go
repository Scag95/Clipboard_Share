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

// postPairRequestAuth posts /pair/request with the given Authorization header
// value (no header when auth is empty).
func postPairRequestAuth(t *testing.T, url, body, auth string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, url+"/pair/request", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

// pairApprovedOverHTTP pairs id through /pair/request + approval +
// /pair/status and returns the issued token.
func pairApprovedOverHTTP(t *testing.T, url string, b *pair.Broker, id, name string) string {
	t.Helper()
	resp := postPairRequest(t, url, `{"id":"`+id+`","name":"`+name+`"}`)
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("initial %s status = %d, want 202", id, resp.StatusCode)
	}
	requestID, _ := decodePairResponse(t, resp)
	if err := b.Approve(requestID); err != nil {
		t.Fatal(err)
	}
	status, statusResp := getPairStatus(t, url, requestID)
	if statusResp.StatusCode != http.StatusOK || status.Token == "" {
		t.Fatalf("approved %s status = %+v, HTTP %d", id, status, statusResp.StatusCode)
	}
	return status.Token
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

// S2: a known device presenting its own current token gets a rotated token
// at once; the old token stops validating and the PC is not asked.
func TestPairKnownDeviceWithCurrentTokenGetsImmediateRotatedToken(t *testing.T) {
	s, b := newPairingClient(t)
	oldToken := pairApprovedOverHTTP(t, s.URL, b, "device-1", "Pixel 8")
	notified := make(chan pair.RequestInfo, 1)
	b.OnRequest(func(req pair.RequestInfo) { notified <- req })

	resp := postPairRequestAuth(t, s.URL, `{"id":"device-1","name":"Pixel 8"}`, "Bearer "+oldToken)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("known status = %d, want 200", resp.StatusCode)
	}
	knownRequestID, newToken := decodePairResponse(t, resp)
	if knownRequestID != "" || newToken == "" || newToken == oldToken {
		t.Fatalf("known response requestId=%q token=%q", knownRequestID, newToken)
	}
	if b.Validate(oldToken) || !b.Validate(newToken) {
		t.Fatal("known re-pair did not rotate the device token")
	}
	if devices, requests := b.Snapshot(); len(devices) != 1 || len(requests) != 0 {
		t.Fatalf("snapshot devices=%+v requests=%+v; want one device, no pending", devices, requests)
	}
	select {
	case got := <-notified:
		t.Fatalf("token-authenticated re-pair asked the PC: %+v", got)
	default:
	}
}

// S2: a known device id without its own current token is treated like a new
// phone: 202 pending, no token, the old token keeps working until approval,
// the PC is notified, and approval updates the same device entry.
func TestPairKnownDeviceWithoutItsTokenIsPendingUntilApproved(t *testing.T) {
	cases := []struct {
		name string
		auth func(own, other string) string
	}{
		{"no header", func(own, other string) string { return "" }},
		{"scheme only", func(own, other string) string { return "Bearer" }},
		{"wrong scheme", func(own, other string) string { return "Basic " + own }},
		{"double space", func(own, other string) string { return "Bearer  " + own }},
		{"wrong token", func(own, other string) string { return "Bearer " + strings.Repeat("0", 64) }},
		{"another device's valid token", func(own, other string) string { return "Bearer " + other }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, b := newPairingClient(t)
			oldToken := pairApprovedOverHTTP(t, s.URL, b, "device-1", "Pixel 8")
			otherToken := pairApprovedOverHTTP(t, s.URL, b, "device-2", "Tablet")
			devicesBefore, _ := b.Snapshot()
			notified := make(chan pair.RequestInfo, 1)
			b.OnRequest(func(req pair.RequestInfo) { notified <- req })

			resp := postPairRequestAuth(t, s.URL, `{"id":"device-1","name":"Pixel 8 Pro"}`, tc.auth(oldToken, otherToken))
			if resp.StatusCode != http.StatusAccepted {
				t.Fatalf("status = %d, want 202", resp.StatusCode)
			}
			var body map[string]any
			if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			requestID, _ := body["requestId"].(string)
			if _, hasToken := body["token"]; hasToken || requestID == "" {
				t.Fatalf("pending body = %v; want requestId and no token", body)
			}
			if !b.Validate(oldToken) || !b.Validate(otherToken) {
				t.Fatal("tokens must stay valid until the request is approved")
			}
			devices, requests := b.Snapshot()
			if len(requests) != 1 || requests[0].RequestID != requestID || requests[0].ID != "device-1" || requests[0].Name != "Pixel 8 Pro" {
				t.Fatalf("pending requests = %+v", requests)
			}
			if len(devices) != 2 {
				t.Fatalf("devices before approval = %+v", devices)
			}
			select {
			case got := <-notified:
				if got.RequestID != requestID || got.ID != "device-1" {
					t.Fatalf("notification = %+v", got)
				}
			default:
				t.Fatal("pending re-pair did not notify the PC")
			}
			status, statusResp := getPairStatus(t, s.URL, requestID)
			if statusResp.StatusCode != http.StatusOK || status.State != pair.StatePending || status.Token != "" {
				t.Fatalf("pending status = %+v, HTTP %d", status, statusResp.StatusCode)
			}

			if err := b.Approve(requestID); err != nil {
				t.Fatal(err)
			}
			status, statusResp = getPairStatus(t, s.URL, requestID)
			if statusResp.StatusCode != http.StatusOK || status.State != pair.StateApproved || status.Token == "" {
				t.Fatalf("approved status = %+v, HTTP %d", status, statusResp.StatusCode)
			}
			again, _ := getPairStatus(t, s.URL, requestID)
			if again.Token != "" {
				t.Fatal("approval token must be returned once")
			}
			if b.Validate(oldToken) || !b.Validate(status.Token) || !b.Validate(otherToken) {
				t.Fatal("approval must replace device-1's token only")
			}
			devices, requests = b.Snapshot()
			if len(devices) != 2 || len(requests) != 0 {
				t.Fatalf("after approval devices=%+v requests=%+v; want same two devices", devices, requests)
			}
			if d := devices[0]; d.ID != "device-1" || d.Name != "Pixel 8 Pro" || d.IP != "127.0.0.1" || !d.FirstApproved.Equal(devicesBefore[0].FirstApproved) {
				t.Fatalf("updated device = %+v, before = %+v", d, devicesBefore[0])
			}
		})
	}
}

// S2: a new device stays pending even when it presents some valid token.
func TestPairNewDeviceWithAnotherDevicesTokenIsPending(t *testing.T) {
	s, b := newPairingClient(t)
	otherToken := pairApprovedOverHTTP(t, s.URL, b, "device-2", "Tablet")
	resp := postPairRequestAuth(t, s.URL, `{"id":"device-new","name":"Pixel 8"}`, "Bearer "+otherToken)
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", resp.StatusCode)
	}
	requestID, token := decodePairResponse(t, resp)
	if requestID == "" || token != "" {
		t.Fatalf("pending response requestId=%q token=%q", requestID, token)
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

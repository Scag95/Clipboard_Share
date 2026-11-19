package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"nudge/internal/pair"
)

// fakeStore is an in-memory Store for handler tests. calls counts every
// Store method invocation so auth tests can prove the clipboard was untouched.
type fakeStore struct {
	text     string
	image    []byte
	setImage error
	calls    int
}

func (f *fakeStore) GetText() (string, error)  { f.calls++; return f.text, nil }
func (f *fakeStore) SetText(s string) error    { f.calls++; f.text = s; return nil }
func (f *fakeStore) GetImage() ([]byte, error) { f.calls++; return f.image, nil }
func (f *fakeStore) SetImage(b []byte) error {
	f.calls++
	if f.setImage != nil {
		return f.setImage
	}
	f.image = b
	return nil
}

// testServer is the clipboard router backed by a fakeStore and a real pairing
// broker; token is a valid session token for an approved device.
type testServer struct {
	*httptest.Server
	store  *fakeStore
	broker *pair.Broker
	token  string
}

func newTestServer(t *testing.T) (*testServer, *fakeStore) {
	t.Helper()
	t.Setenv("NUDGE_STATE", filepath.Join(t.TempDir(), "state.json"))
	b := pair.NewBroker()
	store := &fakeStore{}
	srv := httptest.NewServer(NewRouter(store, b))
	t.Cleanup(srv.Close)
	ts := &testServer{Server: srv, store: store, broker: b, token: inputToken(t, b)}
	return ts, store
}

// do sends a request with the given Authorization header ("" sends none).
func (s *testServer) do(t *testing.T, method, path, auth, contentType string, body []byte) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, s.URL+path, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func (s *testServer) get(t *testing.T, path string) *http.Response {
	t.Helper()
	return s.do(t, http.MethodGet, path, "Bearer "+s.token, "", nil)
}

func (s *testServer) post(t *testing.T, path, contentType string, body []byte) *http.Response {
	t.Helper()
	return s.do(t, http.MethodPost, path, "Bearer "+s.token, contentType, body)
}

type clipboardRoute struct {
	method, path, contentType string
	body                      []byte
}

var clipboardRoutes = []clipboardRoute{
	{http.MethodGet, "/clipboard/text", "", nil},
	{http.MethodPost, "/clipboard/text", "application/json", []byte(`{"text":"intruso"}`)},
	{http.MethodGet, "/clipboard/image", "", nil},
	{http.MethodPost, "/clipboard/image", "image/png", []byte{0x89, 'P', 'N', 'G', 9}},
}

func assertUnauthorized(t *testing.T, resp *http.Response, store *fakeStore) {
	t.Helper()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
	if got := resp.Header.Get("WWW-Authenticate"); got != "Bearer" {
		t.Fatalf("WWW-Authenticate = %q, want %q", got, "Bearer")
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "unauthorized\n" {
		t.Fatalf("body = %q, want %q", body, "unauthorized\n")
	}
	if store.calls != 0 {
		t.Fatalf("store saw %d calls, want 0", store.calls)
	}
	if store.text != "previo" || string(store.image) != "img-previa" {
		t.Fatalf("store changed: text=%q image=%q", store.text, store.image)
	}
}

func TestClipboardRejectsMissingOrInvalidToken(t *testing.T) {
	cases := []struct{ name, auth string }{
		{"no header", ""},
		{"basic scheme", "Basic x"},
		{"bearer without token", "Bearer"},
		{"bearer with empty token", "Bearer "},
		{"token without scheme", "deadbeef"},
		{"two tokens", "Bearer a b"},
		{"double space", "Bearer  x"},
		{"unknown token", "Bearer 0123456789abcdef"},
	}
	for _, rt := range clipboardRoutes {
		for _, c := range cases {
			t.Run(rt.method+" "+rt.path+"/"+c.name, func(t *testing.T) {
				srv, store := newTestServer(t)
				store.text, store.image = "previo", []byte("img-previa")
				resp := srv.do(t, rt.method, rt.path, c.auth, rt.contentType, rt.body)
				assertUnauthorized(t, resp, store)
			})
		}
	}
}

func TestClipboardRejectsValidTokenInWrongPlace(t *testing.T) {
	for _, rt := range clipboardRoutes {
		t.Run(rt.method+" "+rt.path, func(t *testing.T) {
			srv, store := newTestServer(t)
			store.text, store.image = "previo", []byte("img-previa")
			resp := srv.do(t, rt.method, rt.path+"?token="+srv.token, "Basic "+srv.token, rt.contentType, rt.body)
			assertUnauthorized(t, resp, store)
		})
	}
}

func TestClipboardRejectsRevokedToken(t *testing.T) {
	for _, rt := range clipboardRoutes {
		t.Run(rt.method+" "+rt.path, func(t *testing.T) {
			srv, store := newTestServer(t)
			store.text, store.image = "previo", []byte("img-previa")
			if err := srv.broker.Revoke("input-device"); err != nil {
				t.Fatal(err)
			}
			resp := srv.do(t, rt.method, rt.path, "Bearer "+srv.token, rt.contentType, rt.body)
			assertUnauthorized(t, resp, store)
		})
	}
}

func TestClipboardAcceptsCaseInsensitiveScheme(t *testing.T) {
	srv, store := newTestServer(t)
	store.text = "hola"
	resp := srv.do(t, http.MethodGet, "/clipboard/text", "bearer "+srv.token, "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var p TextPayload
	if err := json.NewDecoder(resp.Body).Decode(&p); err != nil {
		t.Fatal(err)
	}
	if p.Text != "hola" {
		t.Fatalf("text = %q, want %q", p.Text, "hola")
	}
}

func TestHealth(t *testing.T) {
	srv, _ := newTestServer(t)
	resp, err := http.Get(srv.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "ok" {
		t.Fatalf("body = %v, want status ok", body)
	}
}

func TestGetText(t *testing.T) {
	srv, store := newTestServer(t)
	store.text = "hola"
	resp := srv.get(t, "/clipboard/text")
	var p TextPayload
	if err := json.NewDecoder(resp.Body).Decode(&p); err != nil {
		t.Fatal(err)
	}
	if p.Text != "hola" {
		t.Fatalf("text = %q, want %q", p.Text, "hola")
	}
}

func TestPostText(t *testing.T) {
	srv, store := newTestServer(t)
	body, _ := json.Marshal(TextPayload{Text: "desde android"})
	resp := srv.post(t, "/clipboard/text", "application/json", body)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", resp.StatusCode)
	}
	if store.text != "desde android" {
		t.Fatalf("store.text = %q, want %q", store.text, "desde android")
	}
}

func TestPostTextInvalidJSON(t *testing.T) {
	srv, _ := newTestServer(t)
	resp := srv.post(t, "/clipboard/text", "application/json", []byte("{nope"))
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestGetImageEmpty(t *testing.T) {
	srv, _ := newTestServer(t)
	resp := srv.get(t, "/clipboard/image")
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", resp.StatusCode)
	}
}

func TestPostImageRoundTrip(t *testing.T) {
	srv, store := newTestServer(t)
	png := []byte{0x89, 'P', 'N', 'G', 1, 2, 3}
	resp := srv.post(t, "/clipboard/image", "image/png", png)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", resp.StatusCode)
	}
	if !bytes.Equal(store.image, png) {
		t.Fatal("image not stored")
	}
	resp2 := srv.get(t, "/clipboard/image")
	if resp2.Header.Get("Content-Type") != "image/png" {
		t.Fatalf("content-type = %q, want image/png", resp2.Header.Get("Content-Type"))
	}
	if !bytes.Equal(mustRead(t, resp2), png) {
		t.Fatal("image mismatch on GET")
	}
}

func TestPostImageTooLargeRejected(t *testing.T) {
	srv, store := newTestServer(t)
	oversize := make([]byte, MaxImageBytes+1)
	resp := srv.post(t, "/clipboard/image", "image/png", oversize)
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", resp.StatusCode)
	}
	if store.image != nil {
		t.Fatal("oversize image must not reach the clipboard")
	}
}

func TestPostImageEmptyRejected(t *testing.T) {
	srv, store := newTestServer(t)
	resp := srv.post(t, "/clipboard/image", "image/png", nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
	if store.image != nil {
		t.Fatal("empty body must not reach the clipboard")
	}
}

func TestPostImageStoreFailure(t *testing.T) {
	srv, store := newTestServer(t)
	store.setImage = http.ErrServerClosed
	resp := srv.post(t, "/clipboard/image", "image/png", []byte("x"))
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", resp.StatusCode)
	}
}

func mustRead(t *testing.T, resp *http.Response) []byte {
	t.Helper()
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

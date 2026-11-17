package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeStore is an in-memory Store for handler tests.
type fakeStore struct {
	text     string
	image    []byte
	setImage error
}

func (f *fakeStore) GetText() (string, error)  { return f.text, nil }
func (f *fakeStore) SetText(s string) error    { f.text = s; return nil }
func (f *fakeStore) GetImage() ([]byte, error) { return f.image, nil }
func (f *fakeStore) SetImage(b []byte) error {
	if f.setImage != nil {
		return f.setImage
	}
	f.image = b
	return nil
}

func newTestServer(t *testing.T) (*httptest.Server, *fakeStore) {
	t.Helper()
	store := &fakeStore{}
	srv := httptest.NewServer(NewRouter(store))
	t.Cleanup(srv.Close)
	return srv, store
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
	resp, err := http.Get(srv.URL + "/clipboard/text")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
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
	resp, err := http.Post(srv.URL+"/clipboard/text", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", resp.StatusCode)
	}
	if store.text != "desde android" {
		t.Fatalf("store.text = %q, want %q", store.text, "desde android")
	}
}

func TestPostTextInvalidJSON(t *testing.T) {
	srv, _ := newTestServer(t)
	resp, err := http.Post(srv.URL+"/clipboard/text", "application/json", strings.NewReader("{nope"))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestGetImageEmpty(t *testing.T) {
	srv, _ := newTestServer(t)
	resp, err := http.Get(srv.URL + "/clipboard/image")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", resp.StatusCode)
	}
}

func TestPostImageRoundTrip(t *testing.T) {
	srv, store := newTestServer(t)
	png := []byte{0x89, 'P', 'N', 'G', 1, 2, 3}
	resp, err := http.Post(srv.URL+"/clipboard/image", "image/png", bytes.NewReader(png))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", resp.StatusCode)
	}
	if !bytes.Equal(store.image, png) {
		t.Fatal("image not stored")
	}
	resp2, err := http.Get(srv.URL + "/clipboard/image")
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
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
	resp, err := http.Post(srv.URL+"/clipboard/image", "image/png", bytes.NewReader(oversize))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", resp.StatusCode)
	}
	if store.image != nil {
		t.Fatal("oversize image must not reach the clipboard")
	}
}

func TestPostImageEmptyRejected(t *testing.T) {
	srv, store := newTestServer(t)
	resp, err := http.Post(srv.URL+"/clipboard/image", "image/png", bytes.NewReader(nil))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
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
	resp, err := http.Post(srv.URL+"/clipboard/image", "image/png", bytes.NewReader([]byte("x")))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
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

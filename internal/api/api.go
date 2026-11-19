package api

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/gorilla/mux"
)

// MaxImageBytes is the largest image accepted by POST /clipboard/image.
const MaxImageBytes = 10 << 20 // 10 MiB

// ProtocolVersion is the HTTP API protocol the server speaks. It is reported
// by /health, the pairing responses and the mDNS TXT record; bump it only on
// breaking API changes (clients refuse to pair or sync on a mismatch).
const ProtocolVersion = 1

// Store abstracts the OS clipboard so handlers stay testable.
type Store interface {
	GetText() (string, error)
	SetText(text string) error
	GetImage() ([]byte, error)
	SetImage(data []byte) error
}

type TextPayload struct {
	Text string `json:"text"`
}

// healthPayload is the /health body; field order fixes the JSON shape
// {"status":"ok","protocol":1,"version":"1.0.0"}.
type healthPayload struct {
	Status   string `json:"status"`
	Protocol int    `json:"protocol"`
	Version  string `json:"version"`
}

// NewRouter serves /health (public; reports ProtocolVersion and the server
// version string) and the clipboard routes, which require
// `Authorization: Bearer <token>` accepted by v.
func NewRouter(store Store, v TokenValidator, version string) *mux.Router {
	r := mux.NewRouter()
	h := &handlers{store: store, version: version}
	r.HandleFunc("/health", h.health).Methods("GET")
	r.HandleFunc("/clipboard/text", requireToken(v, h.getText)).Methods("GET")
	r.HandleFunc("/clipboard/text", requireToken(v, h.setText)).Methods("POST")
	r.HandleFunc("/clipboard/image", requireToken(v, h.getImage)).Methods("GET")
	r.HandleFunc("/clipboard/image", requireToken(v, h.setImage)).Methods("POST")
	return r
}

type handlers struct {
	store   Store
	version string
}

func (h *handlers) health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(healthPayload{Status: "ok", Protocol: ProtocolVersion, Version: h.version})
}

func (h *handlers) getText(w http.ResponseWriter, r *http.Request) {
	text, err := h.store.GetText()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(TextPayload{Text: text})
}

func (h *handlers) setText(w http.ResponseWriter, r *http.Request) {
	var p TextPayload
	if err := json.NewDecoder(io.LimitReader(r.Body, MaxImageBytes)).Decode(&p); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := h.store.SetText(p.Text); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *handlers) getImage(w http.ResponseWriter, r *http.Request) {
	data, err := h.store.GetImage()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if len(data) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	_, _ = w.Write(data)
}

func (h *handlers) setImage(w http.ResponseWriter, r *http.Request) {
	limited := io.LimitReader(r.Body, MaxImageBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if len(data) > MaxImageBytes {
		http.Error(w, "image too large", http.StatusRequestEntityTooLarge)
		return
	}
	if len(data) == 0 {
		http.Error(w, "empty image body", http.StatusBadRequest)
		return
	}
	if err := h.store.SetImage(data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

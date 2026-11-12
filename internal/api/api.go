package api

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/gorilla/mux"
)

// MaxImageBytes is the largest image accepted by POST /clipboard/image.
const MaxImageBytes = 10 << 20 // 10 MiB

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

func NewRouter(store Store) *mux.Router {
	r := mux.NewRouter()
	h := &handlers{store: store}
	r.HandleFunc("/health", h.health).Methods("GET")
	r.HandleFunc("/clipboard/text", h.getText).Methods("GET")
	r.HandleFunc("/clipboard/text", h.setText).Methods("POST")
	r.HandleFunc("/clipboard/image", h.getImage).Methods("GET")
	r.HandleFunc("/clipboard/image", h.setImage).Methods("POST")
	return r
}

type handlers struct {
	store Store
}

func (h *handlers) health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
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

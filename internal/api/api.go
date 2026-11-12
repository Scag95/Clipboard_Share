package api

import (
	"encoding/json"
	"io"
	"net/http"

	"clipboard-share/internal/clipboard"

	"github.com/gorilla/mux"
)

type TextPayload struct {
	Text string `json:"text"`
}

func NewRouter() *mux.Router {
	r := mux.NewRouter()
	r.HandleFunc("/health", healthHandler).Methods("GET")
	r.HandleFunc("/clipboard/text", getTextHandler).Methods("GET")
	r.HandleFunc("/clipboard/text", setTextHandler).Methods("POST")
	r.HandleFunc("/clipboard/image", getImageHandler).Methods("GET")
	r.HandleFunc("/clipboard/image", setImageHandler).Methods("POST")
	return r
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func getTextHandler(w http.ResponseWriter, r *http.Request) {
	text, err := clipboard.GetText()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(TextPayload{Text: text})
}

func setTextHandler(w http.ResponseWriter, r *http.Request) {
	var p TextPayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := clipboard.SetText(p.Text); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func getImageHandler(w http.ResponseWriter, r *http.Request) {
	data, err := clipboard.GetImage()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if data == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Write(data)
}

func setImageHandler(w http.ResponseWriter, r *http.Request) {
	data, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := clipboard.SetImage(data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

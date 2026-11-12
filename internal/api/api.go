package api

import (
	"encoding/json"
	"net/http"

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
	// TODO: leer del clipboard del SO.
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(TextPayload{Text: ""})
}

func setTextHandler(w http.ResponseWriter, r *http.Request) {
	var p TextPayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	// TODO: escribir al clipboard del SO.
	w.WriteHeader(http.StatusNoContent)
}

func getImageHandler(w http.ResponseWriter, r *http.Request) {
	// TODO: servir imagen del clipboard (PNG).
	w.Header().Set("Content-Type", "image/png")
	w.WriteHeader(http.StatusNotImplemented)
}

func setImageHandler(w http.ResponseWriter, r *http.Request) {
	// TODO: guardar imagen recibida en el clipboard del SO.
	w.WriteHeader(http.StatusNoContent)
}

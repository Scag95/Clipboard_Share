package api

import (
	"encoding/json"
	"io"
	"net/http"
	"regexp"

	"github.com/gorilla/mux"
)

// Pairer exchanges a displayed PIN for a session token (implemented by
// internal/pair.Broker).
type Pairer interface {
	Pair(pin string) (token string, ok bool)
	// Locked reports that pairing exhausted its lifetime failed-attempt cap
	// and refuses every PIN until the server restarts.
	Locked() bool
}

type pairRequest struct {
	Pin string `json:"pin"`
}

type pairResponse struct {
	Token string `json:"token"`
}

var pinPattern = regexp.MustCompile(`^[0-9]{6}$`)

// AttachPairing registers the pairing endpoint on r. POST /pair with
// {"pin":"<6 digits>"} returns 200 and {"token":"..."} when the PIN is
// correct, 401 when it is wrong or spent, 400 for a malformed request, and
// 423 once the lifetime failed-attempt cap is exhausted (pairing locked
// until restart; see pair.MaxRotations).
func AttachPairing(r *mux.Router, p Pairer) {
	h := &pairHandlers{pairer: p}
	r.HandleFunc("/pair", h.pair).Methods("POST")
}

type pairHandlers struct {
	pairer Pairer
}

func (h *pairHandlers) pair(w http.ResponseWriter, r *http.Request) {
	var req pairRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<10)).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if !pinPattern.MatchString(req.Pin) {
		http.Error(w, "pin must be 6 digits", http.StatusBadRequest)
		return
	}
	token, ok := h.pairer.Pair(req.Pin)
	if !ok {
		if h.pairer.Locked() {
			http.Error(w, "pairing locked: too many failed attempts; restart the server", http.StatusLocked)
			return
		}
		http.Error(w, "invalid pin", http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(pairResponse{Token: token})
}

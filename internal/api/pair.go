package api

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"

	"github.com/gorilla/mux"

	"nudge/internal/pair"
)

// Pairer starts device pairing and polls its state (implemented by
// internal/pair.Broker).
type Pairer interface {
	Request(id, name, ip, presentedToken string) (pair.RequestResult, error)
	Status(requestID, ip string) (pair.StatusResult, error)
}

type pairRequest struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// AttachPairing registers device-identity pairing endpoints. POST
// /pair/request returns 200 with a rotated token only for a known device that
// sends its own current token as `Authorization: Bearer <token>`; any other
// request (new device, or known device without its valid token) gets 202 with
// a requestId and waits for approval on the PC; 429 when a source IP has too
// many pending requests. GET /pair/status?id=... is source-IP bound and returns approval
// tokens only once.
func AttachPairing(r *mux.Router, p Pairer) {
	h := &pairHandlers{pairer: p}
	r.HandleFunc("/pair/request", h.request).Methods("POST")
	r.HandleFunc("/pair/status", h.status).Methods("GET")
}

type pairHandlers struct {
	pairer Pairer
}

func (h *pairHandlers) request(w http.ResponseWriter, r *http.Request) {
	var req pairRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<10)).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.ID == "" || req.Name == "" {
		http.Error(w, "id and name are required", http.StatusBadRequest)
		return
	}
	// Absent or malformed header -> "" -> the request needs approval.
	presented, _ := bearerToken(r.Header.Get("Authorization"))
	result, err := h.pairer.Request(req.ID, req.Name, remoteIP(r), presented)
	if errors.Is(err, pair.ErrTooManyPending) {
		http.Error(w, "too many pending pair requests from this IP", http.StatusTooManyRequests)
		return
	}
	if errors.Is(err, pair.ErrInvalid) {
		http.Error(w, "id and name are required", http.StatusBadRequest)
		return
	}
	if err != nil {
		http.Error(w, "could not create pair request", http.StatusInternalServerError)
		return
	}
	status := http.StatusAccepted
	if result.Token != "" {
		status = http.StatusOK
	}
	writePairJSON(w, status, result)
}

func (h *pairHandlers) status(w http.ResponseWriter, r *http.Request) {
	requestID := r.URL.Query().Get("id")
	if requestID == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}
	result, err := h.pairer.Status(requestID, remoteIP(r))
	switch {
	case errors.Is(err, pair.ErrExpired):
		writePairJSON(w, http.StatusGone, result)
	case errors.Is(err, pair.ErrForbidden):
		http.Error(w, "pair request belongs to a different IP", http.StatusForbidden)
	case errors.Is(err, pair.ErrNotFound):
		http.Error(w, "pair request not found", http.StatusNotFound)
	case err != nil:
		http.Error(w, "could not read pair request", http.StatusInternalServerError)
	default:
		writePairJSON(w, http.StatusOK, result)
	}
}

func remoteIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

func writePairJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

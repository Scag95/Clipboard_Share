package api

import (
	"net/http"
	"strings"
)

// TokenValidator checks session tokens issued through pairing (implemented
// by internal/pair.Broker). The clipboard routes and /ws/input share it.
type TokenValidator interface {
	Validate(token string) bool
}

// requireToken rejects requests without a valid `Authorization: Bearer
// <token>` header with 401 before next runs, so the clipboard is untouched.
func requireToken(v TokenValidator, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token, ok := bearerToken(r.Header.Get("Authorization"))
		if !ok || !v.Validate(token) {
			w.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

// bearerToken parses "Bearer <token>": case-insensitive scheme, one space,
// one non-empty token. Anything else is not a bearer credential.
func bearerToken(header string) (string, bool) {
	parts := strings.Split(header, " ")
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] == "" {
		return "", false
	}
	return parts[1], true
}

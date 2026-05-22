package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"

	"github.com/rego/argus/internal/store"
)

type ctxKey int

const ctxKeyDevice ctxKey = 1

// requireAuth resolves the bearer token (header or ?token=) to a device row.
// It rejects requests with no/invalid token and stores the device on the
// request context for downstream handlers.
func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := extractToken(r)
		if token == "" {
			writeErr(w, http.StatusUnauthorized, errors.New("missing bearer token"))
			return
		}
		d, err := s.store.GetDeviceByAPIToken(r.Context(), token)
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusUnauthorized, errors.New("invalid token"))
			return
		}
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		_ = s.store.TouchDevice(r.Context(), d.ID)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKeyDevice, &d)))
	})
}

// deviceFrom returns the device associated with the request, or nil if the
// request didn't pass requireAuth.
func deviceFrom(r *http.Request) *store.Device {
	d, _ := r.Context().Value(ctxKeyDevice).(*store.Device)
	return d
}

// extractToken accepts the token from either an Authorization: Bearer header
// or a ?token=<...> query parameter. The query form exists for browser <img>
// and <video> tags that can't set headers.
func extractToken(r *http.Request) string {
	if h := r.Header.Get("Authorization"); h != "" {
		const prefix = "Bearer "
		if len(h) > len(prefix) && strings.EqualFold(h[:len(prefix)], prefix) {
			return strings.TrimSpace(h[len(prefix):])
		}
	}
	return r.URL.Query().Get("token")
}

// newAPIToken returns a 32-byte hex API token.
func newAPIToken() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

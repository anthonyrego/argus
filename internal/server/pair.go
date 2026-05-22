package server

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"math/big"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/rego/argus/internal/store"
)

const (
	pairCodeTTL  = 5 * time.Minute
	pairCodeLen  = 6
	pairAttempts = 5               // max /pair/complete attempts per IP per window
	pairWindow   = 15 * time.Minute // sliding window for attempts
)

// startPairingResp is the body returned by POST /api/pair/start.
type startPairingResp struct {
	Code      string    `json:"code"`
	ExpiresAt time.Time `json:"expires_at"`
}

func (s *Server) startPairing(w http.ResponseWriter, r *http.Request) {
	// Best-effort cleanup so the codes table doesn't grow forever.
	_ = s.store.DeleteExpiredPairingCodes(r.Context())

	for attempt := 0; attempt < 6; attempt++ {
		code, err := randomDigits(pairCodeLen)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		pc, err := s.store.CreatePairingCode(r.Context(), code, pairCodeTTL)
		if err == nil {
			writeJSON(w, http.StatusCreated, startPairingResp{
				Code:      pc.Code,
				ExpiresAt: pc.ExpiresAt,
			})
			return
		}
		// UNIQUE collision (vanishingly rare with 10^6 codes and only a
		// handful active at once): retry.
		if !strings.Contains(err.Error(), "UNIQUE") {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
	}
	writeErr(w, http.StatusInternalServerError, errors.New("could not allocate a unique code"))
}

type completePairingReq struct {
	Code      string `json:"code"`
	Platform  string `json:"platform"`
	Name      string `json:"name"`
	APNsToken string `json:"apns_token"`
}

func (s *Server) completePairing(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	if !s.pairLimiter.allow(ip) {
		writeErr(w, http.StatusTooManyRequests, errors.New("too many pairing attempts; try again later"))
		return
	}

	var req completePairingReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if req.Code == "" || req.Platform == "" {
		writeErr(w, http.StatusBadRequest, errors.New("code and platform required"))
		return
	}

	if _, err := s.store.ConsumePairingCode(r.Context(), strings.TrimSpace(req.Code)); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusUnauthorized, errors.New("invalid or expired code"))
			return
		}
		writeErr(w, http.StatusInternalServerError, err)
		return
	}

	token, err := newAPIToken()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	d, err := s.store.CreateDevice(r.Context(), store.Device{
		APIToken:  token,
		Platform:  req.Platform,
		Name:      req.Name,
		APNsToken: req.APNsToken,
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	// Successful pair — reset this IP's bucket so a legitimate user isn't
	// punished if they typo'd the code a few times before getting it right.
	s.pairLimiter.reset(ip)
	writeJSON(w, http.StatusCreated, viewOf(d, true))
}

// randomDigits returns a string of n cryptographically-random decimal digits.
func randomDigits(n int) (string, error) {
	out := make([]byte, n)
	max := big.NewInt(10)
	for i := 0; i < n; i++ {
		v, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		out[i] = '0' + byte(v.Int64())
	}
	return string(out), nil
}

// clientIP extracts the requester's IP, honoring X-Forwarded-For if present.
// We use the first hop in the chain (the real client). Behind Tailscale Funnel
// or a reverse proxy the header is trustworthy; on a plain LAN it isn't set
// and we fall back to RemoteAddr.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.IndexByte(xff, ','); i >= 0 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// ---- in-memory attempt limiter ----

// attemptLimiter is a tiny per-key counter with a fixed window. Not a
// precise sliding window — we just reset the bucket after `window` elapses.
// Fine for personal-use defenses where the goal is to slow down brute force,
// not to enforce exact quotas.
type attemptLimiter struct {
	max     int
	window  time.Duration
	mu      sync.Mutex
	buckets map[string]*attemptBucket
}

type attemptBucket struct {
	count   int
	resetAt time.Time
}

func newAttemptLimiter(max int, window time.Duration) *attemptLimiter {
	return &attemptLimiter{
		max:     max,
		window:  window,
		buckets: make(map[string]*attemptBucket),
	}
}

func (l *attemptLimiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	b, ok := l.buckets[key]
	if !ok || now.After(b.resetAt) {
		l.buckets[key] = &attemptBucket{count: 1, resetAt: now.Add(l.window)}
		return true
	}
	if b.count >= l.max {
		return false
	}
	b.count++
	return true
}

func (l *attemptLimiter) reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.buckets, key)
}

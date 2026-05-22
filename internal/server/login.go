package server

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/rego/argus/internal/store"
)

// loginReq is the body of POST /api/login.
type loginReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Name     string `json:"name"` // optional friendly name for this session (e.g. "Tony's Mac browser")
}

// loginResp returns the per-session API token plus the must-change-password
// flag, which the UI uses to force a password change before proceeding.
type loginResp struct {
	APIToken           string `json:"api_token"`
	MustChangePassword bool   `json:"must_change_password"`
	Username           string `json:"username"`
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	if !s.loginLimiter.allow(ip) {
		writeErr(w, http.StatusTooManyRequests, errors.New("too many login attempts; try again later"))
		return
	}

	var req loginReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if req.Username == "" || req.Password == "" {
		writeErr(w, http.StatusBadRequest, errors.New("username and password required"))
		return
	}

	admin, err := s.store.VerifyAdminPassword(r.Context(), req.Username, req.Password)
	if err != nil {
		if errors.Is(err, store.ErrBadCredentials) || errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusUnauthorized, errors.New("invalid credentials"))
			return
		}
		writeErr(w, http.StatusInternalServerError, err)
		return
	}

	// Mint a per-session device row. Each browser login gets its own row so
	// the user can revoke individual browser sessions from the devices list.
	token, err := newAPIToken()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	name := req.Name
	if name == "" {
		name = browserName(r.UserAgent())
	}
	if _, err := s.store.CreateDevice(r.Context(), store.Device{
		APIToken: token,
		Platform: "web",
		Name:     name,
	}); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}

	s.loginLimiter.reset(ip)
	writeJSON(w, http.StatusOK, loginResp{
		APIToken:           token,
		MustChangePassword: admin.MustChangePassword,
		Username:           admin.Username,
	})
}

type changePasswordReq struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

func (s *Server) changeAdminPassword(w http.ResponseWriter, r *http.Request) {
	var req changePasswordReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if len(req.NewPassword) < 8 {
		writeErr(w, http.StatusBadRequest, errors.New("new password must be at least 8 characters"))
		return
	}
	admin, err := s.store.GetAdmin(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if _, err := s.store.VerifyAdminPassword(r.Context(), admin.Username, req.CurrentPassword); err != nil {
		writeErr(w, http.StatusUnauthorized, errors.New("current password is wrong"))
		return
	}
	if err := s.store.UpdateAdminPassword(r.Context(), req.NewPassword); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// browserName returns a short label like "Chrome on macOS" derived from the
// User-Agent, so the devices list shows something more useful than "Browser".
func browserName(ua string) string {
	browser := "Browser"
	if contains(ua, "Edg/") {
		browser = "Edge"
	} else if contains(ua, "Chrome/") {
		browser = "Chrome"
	} else if contains(ua, "Firefox/") {
		browser = "Firefox"
	} else if contains(ua, "Safari/") {
		browser = "Safari"
	}
	os := ""
	switch {
	case contains(ua, "Mac OS X"), contains(ua, "Macintosh"):
		os = "macOS"
	case contains(ua, "Windows"):
		os = "Windows"
	case contains(ua, "Linux"):
		os = "Linux"
	case contains(ua, "iPhone"):
		os = "iPhone"
	case contains(ua, "iPad"):
		os = "iPad"
	case contains(ua, "Android"):
		os = "Android"
	}
	if os == "" {
		return browser
	}
	return browser + " on " + os
}

func contains(haystack, needle string) bool {
	if len(needle) == 0 || len(haystack) < len(needle) {
		return false
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

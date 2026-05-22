package server

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/rego/argus/internal/store"
)

// deviceView is the public shape returned by device endpoints. The api_token
// is only present in the immediate response from login or pair/complete —
// never in lists.
type deviceView struct {
	ID         int64  `json:"id"`
	Platform   string `json:"platform"`
	Name       string `json:"name"`
	APITokenIn string `json:"api_token,omitempty"`
	HasAPNs    bool   `json:"has_apns_token"`
	CreatedAt  string `json:"created_at"`
	LastSeenAt string `json:"last_seen_at"`
}

func viewOf(d store.Device, includeToken bool) deviceView {
	v := deviceView{
		ID:         d.ID,
		Platform:   d.Platform,
		Name:       d.Name,
		HasAPNs:    d.APNsToken != "",
		CreatedAt:  d.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		LastSeenAt: d.LastSeenAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
	if includeToken {
		v.APITokenIn = d.APIToken
	}
	return v
}

func (s *Server) listDevices(w http.ResponseWriter, r *http.Request) {
	devs, err := s.store.ListDevices(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	out := make([]deviceView, 0, len(devs))
	for _, d := range devs {
		out = append(out, viewOf(d, false))
	}
	writeJSON(w, http.StatusOK, out)
}

type updateMyDeviceReq struct {
	Name      string `json:"name"`
	APNsToken string `json:"apns_token"`
}

func (s *Server) updateMyDevice(w http.ResponseWriter, r *http.Request) {
	me := deviceFrom(r)
	if me == nil {
		writeErr(w, http.StatusUnauthorized, errors.New("not authenticated"))
		return
	}
	var req updateMyDeviceReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	name := req.Name
	if name == "" {
		name = me.Name
	}
	d, err := s.store.UpdateDeviceAPNsToken(r.Context(), me.ID, name, req.APNsToken)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, viewOf(d, false))
}

func (s *Server) deleteDevice(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if err := s.store.DeleteDevice(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, err)
			return
		}
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

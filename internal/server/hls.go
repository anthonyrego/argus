package server

import (
	"net/http"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/rego/argus/internal/streamer"
)

// hlsFile serves the camera's HLS playlist or a single segment from the
// streamer's session directory. Requesting the playlist starts the ffmpeg
// session if it isn't already running; subsequent .ts fetches keep it alive.
func (s *Server) hlsFile(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	name, err := streamer.SafeFilename(chi.URLParam(r, "file"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}

	dir, err := s.stream.Open(r.Context(), id)
	if err != nil {
		s.log.Warn("hls open", "camera_id", id, "err", err)
		writeErr(w, http.StatusBadGateway, err)
		return
	}
	// Touch on every file request so the session stays alive while the
	// client is actively pulling segments.
	s.stream.Touch(id)

	path := filepath.Join(dir, name)
	if strings.HasSuffix(name, ".m3u8") {
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		w.Header().Set("Cache-Control", "no-store")
	} else {
		w.Header().Set("Content-Type", "video/mp2t")
		w.Header().Set("Cache-Control", "max-age=10")
	}
	http.ServeFile(w, r, path)
}

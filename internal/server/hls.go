package server

import (
	"net/http"
	"net/url"
	"os"
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
		// If the caller authenticated via ?token= (browser <video> or iOS
		// native HLS), propagate that token to each segment URI in the playlist
		// so the player can fetch segments without losing auth.
		if token := r.URL.Query().Get("token"); token != "" {
			data, err := os.ReadFile(path)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			_, _ = w.Write([]byte(rewritePlaylistWithToken(string(data), token)))
			return
		}
	} else {
		w.Header().Set("Content-Type", "video/mp2t")
		w.Header().Set("Cache-Control", "max-age=10")
	}
	http.ServeFile(w, r, path)
}

// rewritePlaylistWithToken appends ?token=<token> (or &token=) to every URI
// line in an HLS playlist. URI lines are non-empty lines that don't start with
// '#'. Tag lines that embed URIs (e.g. #EXT-X-MAP:URI="...") are left alone —
// our segmenter doesn't emit them; if it ever does, extend this.
func rewritePlaylistWithToken(body, token string) string {
	enc := url.QueryEscape(token)
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		t := strings.TrimRight(line, "\r")
		trimmed := strings.TrimSpace(t)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		sep := "?"
		if strings.Contains(t, "?") {
			sep = "&"
		}
		lines[i] = t + sep + "token=" + enc
	}
	return strings.Join(lines, "\n")
}

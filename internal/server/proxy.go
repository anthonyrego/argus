package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/icholy/digest"

	"github.com/rego/argus/internal/store"
)

// cameraProxy fetches frames and live MJPEG from cameras and pipes them to the
// browser. The camera's own HTTP server uses digest auth; the icholy/digest
// transport handles the challenge-response.
type cameraProxy struct {
	store *store.Store
	log   *slog.Logger

	mu      sync.Mutex
	clients map[int64]*http.Client // one per camera so digest nonce state is reused
}

func newCameraProxy(s *store.Store, log *slog.Logger) *cameraProxy {
	return &cameraProxy{store: s, log: log, clients: make(map[int64]*http.Client)}
}

func (p *cameraProxy) clientFor(c store.Camera) *http.Client {
	p.mu.Lock()
	defer p.mu.Unlock()
	if cl, ok := p.clients[c.ID]; ok {
		return cl
	}
	cl := &http.Client{
		Transport: &digest.Transport{
			Username: c.Username,
			Password: c.Password,
		},
		// No overall Timeout — MJPEG is a long-lived stream. Per-request
		// timeouts are enforced via context where appropriate.
	}
	p.clients[c.ID] = cl
	return cl
}

func (p *cameraProxy) invalidate(id int64) {
	p.mu.Lock()
	delete(p.clients, id)
	p.mu.Unlock()
}

// snapshot proxies /cgi-bin/snapshot.cgi back to the client as a single JPEG.
func (p *cameraProxy) snapshot(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	cam, err := p.store.GetCamera(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	url := fmt.Sprintf("http://%s/cgi-bin/snapshot.cgi?channel=1", cam.Host)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	resp, err := p.clientFor(cam).Do(req)
	if err != nil {
		p.log.Warn("snapshot fetch failed", "camera_id", cam.ID, "err", err)
		writeErr(w, http.StatusBadGateway, err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		p.invalidate(cam.ID)
		writeErr(w, http.StatusBadGateway, fmt.Errorf("camera returned %d", resp.StatusCode))
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = io.Copy(w, resp.Body)
}

// stream proxies the camera's MJPEG-over-HTTP feed straight to the browser.
// Browsers will render this in an <img> tag without any JS player.
func (p *cameraProxy) stream(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	cam, err := p.store.GetCamera(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}

	// MJPEG is only available on the sub-stream; the main stream is H.264/H.265.
	// Clients that want main quality should use the HLS endpoint instead.
	url := fmt.Sprintf("http://%s/cgi-bin/mjpg/video.cgi?channel=1&subtype=1", cam.Host)
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, url, nil)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	resp, err := p.clientFor(cam).Do(req)
	if err != nil {
		p.log.Warn("mjpeg stream failed", "camera_id", cam.ID, "err", err)
		writeErr(w, http.StatusBadGateway, err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		p.invalidate(cam.ID)
		writeErr(w, http.StatusBadGateway, fmt.Errorf("camera returned %d", resp.StatusCode))
		return
	}

	// Pass through the multipart content-type (boundary intact) so the browser
	// knows how to parse the MJPEG stream.
	if ct := resp.Header.Get("Content-Type"); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Connection", "close")

	flusher, _ := w.(http.Flusher)
	buf := make([]byte, 32*1024)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				return
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
		if rerr != nil {
			return
		}
	}
}

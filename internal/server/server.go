package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/rego/argus/internal/eventmgr"
	"github.com/rego/argus/internal/store"
	"github.com/rego/argus/internal/streamer"
)

// Server wires the HTTP API and the embedded React SPA together.
type Server struct {
	store    *store.Store
	mgr      *eventmgr.Manager
	stream   *streamer.Streamer
	log      *slog.Logger
	frontend fs.FS // built React assets; nil falls back to a "frontend not built" message
	proxy    *cameraProxy
}

func New(s *store.Store, m *eventmgr.Manager, st *streamer.Streamer, frontend fs.FS, log *slog.Logger) *Server {
	return &Server{
		store:    s,
		mgr:      m,
		stream:   st,
		log:      log,
		frontend: frontend,
		proxy:    newCameraProxy(s, log),
	}
}

func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(requestLogger(s.log))

	r.Route("/api", func(r chi.Router) {
		r.Get("/cameras", s.listCameras)
		r.Post("/cameras", s.createCamera)
		r.Route("/cameras/{id}", func(r chi.Router) {
			r.Get("/", s.getCamera)
			r.Put("/", s.updateCamera)
			r.Delete("/", s.deleteCamera)
			r.Get("/snapshot.jpg", s.proxy.snapshot)
			r.Get("/stream.mjpg", s.proxy.stream)
			r.Get("/hls/{file}", s.hlsFile)
		})
		r.Get("/events", s.listEvents)
		r.Get("/events/stream", s.eventStream)
	})

	if s.frontend != nil {
		fileServer := http.FileServer(http.FS(s.frontend))
		r.Handle("/assets/*", fileServer)
		r.Handle("/vite.svg", fileServer)
		r.Handle("/favicon.ico", fileServer)
		// SPA fallback: any non-API GET serves index.html so the client router takes over.
		r.Get("/*", s.serveIndex)
	} else {
		r.Get("/*", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte("frontend not built — run `npm run build` in ./web or use the Vite dev server"))
		})
	}

	return r
}

func (s *Server) serveIndex(w http.ResponseWriter, _ *http.Request) {
	data, err := fs.ReadFile(s.frontend, "index.html")
	if err != nil {
		http.Error(w, "index.html not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(data)
}

// ---- camera handlers ----

type cameraPayload struct {
	Name     string `json:"name"`
	Host     string `json:"host"`
	Username string `json:"username"`
	Password string `json:"password"`
	Enabled  bool   `json:"enabled"`
}

func (s *Server) listCameras(w http.ResponseWriter, r *http.Request) {
	cams, err := s.store.ListCameras(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if cams == nil {
		cams = []store.Camera{}
	}
	writeJSON(w, http.StatusOK, cams)
}

func (s *Server) getCamera(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	c, err := s.store.GetCamera(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (s *Server) createCamera(w http.ResponseWriter, r *http.Request) {
	var p cameraPayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if p.Name == "" || p.Host == "" {
		writeErr(w, http.StatusBadRequest, errors.New("name and host are required"))
		return
	}
	c, err := s.store.CreateCamera(r.Context(), store.Camera{
		Name: p.Name, Host: p.Host, Username: p.Username, Password: p.Password,
		Enabled: p.Enabled,
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	s.mgr.Sync()
	writeJSON(w, http.StatusCreated, c)
}

func (s *Server) updateCamera(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	var p cameraPayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	c, err := s.store.UpdateCamera(r.Context(), id, store.Camera{
		Name: p.Name, Host: p.Host, Username: p.Username, Password: p.Password,
		Enabled: p.Enabled,
	})
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	s.mgr.Sync()
	writeJSON(w, http.StatusOK, c)
}

func (s *Server) deleteCamera(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if err := s.store.DeleteCamera(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, err)
			return
		}
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	s.mgr.Sync()
	w.WriteHeader(http.StatusNoContent)
}

// ---- event handlers ----

func (s *Server) listEvents(w http.ResponseWriter, r *http.Request) {
	f := store.ListEventsFilter{Limit: 100}
	if v := r.URL.Query().Get("camera_id"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			writeErr(w, http.StatusBadRequest, fmt.Errorf("camera_id: %w", err))
			return
		}
		f.CameraID = id
	}
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 || n > 1000 {
			writeErr(w, http.StatusBadRequest, errors.New("limit must be 1..1000"))
			return
		}
		f.Limit = n
	}
	if v := r.URL.Query().Get("before"); v != "" {
		t, err := time.Parse(time.RFC3339Nano, v)
		if err != nil {
			writeErr(w, http.StatusBadRequest, fmt.Errorf("before: %w", err))
			return
		}
		f.Before = t
	}
	if v := r.URL.Query().Get("codes"); v != "" {
		f.Codes = strings.Split(v, ",")
	}
	evs, err := s.store.ListEvents(r.Context(), f)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if evs == nil {
		evs = []store.Event{}
	}
	writeJSON(w, http.StatusOK, evs)
}

func (s *Server) eventStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	ch, cancel := s.mgr.Subscribe()
	defer cancel()

	// Initial comment so the client sees a 200 immediately.
	_, _ = w.Write([]byte(": connected\n\n"))
	flusher.Flush()

	ping := time.NewTicker(20 * time.Second)
	defer ping.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ping.C:
			_, _ = w.Write([]byte(": ping\n\n"))
			flusher.Flush()
		case ev, ok := <-ch:
			if !ok {
				return
			}
			b, err := json.Marshal(ev)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "event: motion\ndata: %s\n\n", b)
			flusher.Flush()
		}
	}
}

// ---- helpers ----

func parseID(r *http.Request) (int64, error) {
	v := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(v, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid id %q", v)
	}
	return id, nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}

func requestLogger(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)
			// Skip noisy SSE / MJPEG long-lived requests after they finish.
			log.Debug("http",
				"method", r.Method, "path", r.URL.Path,
				"status", ww.Status(), "dur_ms", time.Since(start).Milliseconds())
		})
	}
}


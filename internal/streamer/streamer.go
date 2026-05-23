package streamer

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/rego/argus/internal/store"
)

// idleTimeout is how long after the last segment fetch we kill an ffmpeg
// session. Keep it short so unused sessions don't burn CPU; keep it long
// enough that brief network blips on the client don't kill the stream.
const idleTimeout = 30 * time.Second

// firstSegmentWait bounds how long /index.m3u8 will block waiting for ffmpeg
// to produce its first segment. Camera RTSP handshake + first GOP usually
// lands inside 2s on a healthy LAN.
const firstSegmentWait = 8 * time.Second

// Streamer manages on-demand HLS sessions, one per camera. Sessions are
// spawned the first time a client asks for the playlist and reaped after
// idleTimeout passes with no requests.
type Streamer struct {
	store *store.Store
	log   *slog.Logger

	baseDir string

	mu       sync.Mutex
	sessions map[int64]*session

	ctx    context.Context
	cancel context.CancelFunc
}

type session struct {
	cameraID  int64
	camHash   string // creds+host hash; if camera changes, session is recreated
	dir       string
	cmd       *exec.Cmd
	cancel    context.CancelFunc
	ready     chan struct{} // closed when first segment is written
	readyOnce sync.Once

	mu           sync.Mutex
	lastActivity time.Time
}

func New(ctx context.Context, s *store.Store, log *slog.Logger) (*Streamer, error) {
	base, err := os.MkdirTemp("", "argus-hls-")
	if err != nil {
		return nil, fmt.Errorf("create base hls dir: %w", err)
	}
	sctx, cancel := context.WithCancel(ctx)
	st := &Streamer{
		store:    s,
		log:      log,
		baseDir:  base,
		sessions: make(map[int64]*session),
		ctx:      sctx,
		cancel:   cancel,
	}
	go st.sweeper()
	return st, nil
}

// Close kills all running ffmpeg sessions and removes the working directory.
func (st *Streamer) Close() {
	st.cancel()
	st.mu.Lock()
	for id, sess := range st.sessions {
		sess.shutdown()
		delete(st.sessions, id)
	}
	st.mu.Unlock()
	_ = os.RemoveAll(st.baseDir)
}

// Open returns the session directory for the given camera, starting an
// ffmpeg process if one is not already running. Blocks up to firstSegmentWait
// for the playlist to exist on a cold start. The returned directory is owned
// by the streamer; callers must not modify it.
func (st *Streamer) Open(ctx context.Context, cameraID int64) (string, error) {
	cam, err := st.store.GetCamera(ctx, cameraID)
	if err != nil {
		return "", err
	}
	hash := connHash(cam)

	st.mu.Lock()
	sess, ok := st.sessions[cameraID]
	if ok && sess.camHash != hash {
		// Credentials/host changed since we started — rebuild.
		sess.shutdown()
		delete(st.sessions, cameraID)
		ok = false
	}
	if !ok {
		sess, err = st.startSession(cam, hash)
		if err != nil {
			st.mu.Unlock()
			return "", err
		}
		st.sessions[cameraID] = sess
	}
	sess.touch()
	st.mu.Unlock()

	// Wait for the first segment to appear (or the session to die).
	select {
	case <-sess.ready:
		return sess.dir, nil
	case <-time.After(firstSegmentWait):
		return "", fmt.Errorf("hls: first segment not produced within %s", firstSegmentWait)
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

// Touch records activity for the camera's session so it isn't reaped while
// the client is still pulling segments.
func (st *Streamer) Touch(cameraID int64) {
	st.mu.Lock()
	sess, ok := st.sessions[cameraID]
	st.mu.Unlock()
	if ok {
		sess.touch()
	}
}

func (st *Streamer) startSession(cam store.Camera, hash string) (*session, error) {
	dir := filepath.Join(st.baseDir, fmt.Sprintf("cam-%d", cam.ID))
	if err := os.RemoveAll(dir); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}

	rtspURL := buildRTSPURL(cam.Username, cam.Password, cam.Host, 0)
	ctx, cancel := context.WithCancel(st.ctx)

	// -c:v copy -an: pass through the camera's H.264/H.265 main stream and
	// drop audio. The HLS muxer just chunks the existing GOPs into .ts.
	// hls_flags=delete_segments keeps disk usage bounded; omit_endlist marks
	// the playlist as live (no #EXT-X-ENDLIST tag).
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-loglevel", "warning",
		"-rtsp_transport", "tcp",
		// Regenerate PTS and ignore source DTS — Dahua's RTSP feed occasionally
		// emits packets whose DTS jumps backwards, which causes the HLS muxer
		// to flatten timestamps and produce visible playback hitches.
		"-fflags", "+genpts+igndts",
		"-i", rtspURL,
		"-an",
		"-c:v", "copy",
		"-f", "hls",
		"-hls_time", "2",
		"-hls_list_size", "6",
		"-hls_flags", "delete_segments+independent_segments+omit_endlist",
		"-hls_segment_filename", filepath.Join(dir, "seg_%05d.ts"),
		filepath.Join(dir, "index.m3u8"),
	)
	// Make the ffmpeg stderr available for logs but don't keep a giant buffer.
	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("start ffmpeg: %w", err)
	}

	sess := &session{
		cameraID:     cam.ID,
		camHash:      hash,
		dir:          dir,
		cmd:          cmd,
		cancel:       cancel,
		ready:        make(chan struct{}),
		lastActivity: time.Now(),
	}
	st.log.Info("hls session started", "camera_id", cam.ID, "camera", cam.Name, "dir", dir)

	// Drain stderr to keep ffmpeg from blocking, and surface its warnings.
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := stderr.Read(buf)
			if n > 0 {
				line := strings.TrimSpace(string(buf[:n]))
				if line != "" {
					st.log.Debug("ffmpeg", "camera_id", cam.ID, "msg", line)
				}
			}
			if err != nil {
				return
			}
		}
	}()

	// Watch for the first segment.
	go st.waitForReady(sess)

	// Reap process exit.
	go func() {
		err := cmd.Wait()
		if err != nil && ctx.Err() == nil {
			st.log.Warn("ffmpeg exited", "camera_id", cam.ID, "err", err)
		}
		// Drop session from map if still ours.
		st.mu.Lock()
		if cur, ok := st.sessions[cam.ID]; ok && cur == sess {
			delete(st.sessions, cam.ID)
		}
		st.mu.Unlock()
		// Unblock any pending Open() callers.
		sess.readyOnce.Do(func() { close(sess.ready) })
		_ = os.RemoveAll(sess.dir)
	}()

	return sess, nil
}

func (st *Streamer) waitForReady(sess *session) {
	playlist := filepath.Join(sess.dir, "index.m3u8")
	deadline := time.Now().Add(firstSegmentWait + 2*time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(playlist); err == nil {
			// Playlist exists, but it might be empty until ffmpeg writes the
			// first segment. Read it and check.
			data, _ := os.ReadFile(playlist)
			if strings.Contains(string(data), ".ts") {
				sess.readyOnce.Do(func() { close(sess.ready) })
				return
			}
		}
		select {
		case <-st.ctx.Done():
			return
		case <-time.After(100 * time.Millisecond):
		}
	}
	st.log.Warn("hls session not ready in time", "camera_id", sess.cameraID)
}

// sweeper periodically kills idle sessions. Runs until the streamer's context
// is cancelled by Close().
func (st *Streamer) sweeper() {
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-st.ctx.Done():
			return
		case <-t.C:
			st.reapIdle()
		}
	}
}

func (st *Streamer) reapIdle() {
	now := time.Now()
	st.mu.Lock()
	defer st.mu.Unlock()
	for id, sess := range st.sessions {
		sess.mu.Lock()
		idle := now.Sub(sess.lastActivity)
		sess.mu.Unlock()
		if idle > idleTimeout {
			st.log.Info("hls session idle, reaping", "camera_id", id, "idle", idle)
			sess.shutdown()
			delete(st.sessions, id)
		}
	}
}

func (s *session) touch() {
	s.mu.Lock()
	s.lastActivity = time.Now()
	s.mu.Unlock()
}

func (s *session) shutdown() {
	s.cancel()
	// The cmd.Wait() goroutine handles cleanup.
}

// buildRTSPURL composes the camera URL with proper escaping. Amcrest passwords
// often contain '@' which would otherwise confuse the URL parser.
func buildRTSPURL(user, pass, host string, subtype int) string {
	u := url.URL{
		Scheme:   "rtsp",
		User:     url.UserPassword(user, pass),
		Host:     ensurePort(host, "554"),
		Path:     "/cam/realmonitor",
		RawQuery: fmt.Sprintf("channel=1&subtype=%d", subtype),
	}
	return u.String()
}

func ensurePort(host, defaultPort string) string {
	if host == "" {
		return ""
	}
	if strings.Contains(host, ":") {
		return host
	}
	return host + ":" + defaultPort
}

// connHash captures the bits of a Camera that, if changed, require restarting
// the ffmpeg session.
func connHash(c store.Camera) string {
	return c.Host + "\x00" + c.Username + "\x00" + c.Password
}

// SafeFilename rejects names that would let a request escape the session dir.
// Returns the cleaned name and an error if it's not safe.
func SafeFilename(name string) (string, error) {
	if name == "" || strings.Contains(name, "/") || strings.Contains(name, "\\") || name == "." || name == ".." {
		return "", errors.New("invalid file")
	}
	// Allow only .ts and .m3u8 — HLS produces nothing else.
	if !strings.HasSuffix(name, ".ts") && !strings.HasSuffix(name, ".m3u8") {
		return "", errors.New("invalid file")
	}
	return name, nil
}

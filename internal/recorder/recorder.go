// Package recorder turns motion events into saved MP4 clips.
//
// One ffmpeg per enabled camera runs continuously, remuxing the main RTSP
// stream into ~1s MPEG-TS segments in a per-camera working directory. The
// pre-buffer is whatever segments still exist on disk. When a Start event
// arrives the recorder marks a trigger time, lets segmenting continue past
// the configured post-roll, then concats the covering segments into a single
// MP4 with ffmpeg's concat demuxer.
package recorder

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/rego/argus/internal/events"
	"github.com/rego/argus/internal/store"
)

// Config controls clip timing and on-disk layout.
type Config struct {
	Dir      string        // base directory for finalized clips (work files go under <Dir>/.work)
	PreRoll  time.Duration // seconds of segments kept before the trigger
	PostRoll time.Duration // seconds of segments collected after the most-recent Start
	MaxClip  time.Duration // hard cap on a single clip's total length from trigger
}

// pollInterval is how often we scan the segment dir for new files and check
// whether any active clip is ready to finalize. Short enough that we notice
// segment boundaries quickly, long enough not to thrash the disk.
const pollInterval = 500 * time.Millisecond

// segmentBufferMargin keeps a small extra pre-roll buffer beyond the configured
// PreRoll. Segments are typically 1-2s long and only close when the next one
// starts, so we need at least one extra segment in the ring to cover the full
// pre-roll window after the trigger lands.
const segmentBufferMargin = 4 * time.Second

// Recorder owns one ffmpeg session per camera plus the goroutines that poll
// segments and assemble clips.
type Recorder struct {
	store *store.Store
	log   *slog.Logger
	cfg   Config

	finalDir string // <Dir>
	workDir  string // <Dir>/.work

	ctx    context.Context
	cancel context.CancelFunc

	mu       sync.Mutex
	sessions map[int64]*camSession

	// OnClipOpened, if non-nil, is invoked exactly once each time a new clip is
	// created (i.e. on the trigger event that opens it, not on subsequent Starts
	// that extend it). Used by the push notifier so one push fires per clip.
	OnClipOpened func(store.Event)

	// OnRecordingInserted, if non-nil, is invoked after a clip has been fully
	// written and its row inserted into the store. Used by the SSE bridge so
	// clients learn the recording is now playable.
	OnRecordingInserted func(store.Recording)
}

func New(ctx context.Context, s *store.Store, log *slog.Logger, cfg Config) (*Recorder, error) {
	if cfg.Dir == "" {
		return nil, fmt.Errorf("recorder: dir is required")
	}
	finalDir, err := filepath.Abs(cfg.Dir)
	if err != nil {
		return nil, fmt.Errorf("recorder: abs dir: %w", err)
	}
	workDir := filepath.Join(finalDir, ".work")
	if err := os.MkdirAll(finalDir, 0o755); err != nil {
		return nil, fmt.Errorf("recorder: mkdir final: %w", err)
	}
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return nil, fmt.Errorf("recorder: mkdir work: %w", err)
	}

	rctx, cancel := context.WithCancel(ctx)
	return &Recorder{
		store:    s,
		log:      log,
		cfg:      cfg,
		finalDir: finalDir,
		workDir:  workDir,
		ctx:      rctx,
		cancel:   cancel,
		sessions: make(map[int64]*camSession),
	}, nil
}

// Sync brings running sessions in line with the desired camera set. Cameras
// that are no longer present (or whose connection params changed) are stopped;
// missing ones are started. Safe to call from the event manager after any
// camera CRUD.
func (r *Recorder) Sync(cams []store.Camera) {
	want := make(map[int64]store.Camera, len(cams))
	for _, c := range cams {
		if c.Enabled {
			want[c.ID] = c
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	for id, sess := range r.sessions {
		w, ok := want[id]
		if !ok || sess.camHash != connHash(w) {
			sess.stop()
			delete(r.sessions, id)
		}
	}
	for id, c := range want {
		if _, ok := r.sessions[id]; ok {
			continue
		}
		sess, err := r.startSession(c)
		if err != nil {
			r.log.Error("recorder: start session", "camera_id", id, "err", err)
			continue
		}
		r.sessions[id] = sess
	}
}

// Close stops all sessions. Active clips in flight are abandoned (no segment
// past the deadline will ever land), which is the price of a clean shutdown.
func (r *Recorder) Close() {
	r.cancel()
	r.mu.Lock()
	for id, sess := range r.sessions {
		sess.stop()
		delete(r.sessions, id)
	}
	r.mu.Unlock()
}

// OnEvent is the recorder's input from the event pipeline. Only Start actions
// matter; everything else is ignored. The eventStored argument is the
// persisted store.Event so we can link the recording to it.
func (r *Recorder) OnEvent(eventStored store.Event) {
	if !strings.EqualFold(eventStored.Action, "Start") {
		return
	}
	// Global "home mode": when recording is switched off we leave the segmenter
	// running (so re-enabling is instant with full pre-roll) but don't open or
	// extend any clip. Fail open on a read error — don't silently stop recording.
	if st, err := r.store.GetSettings(r.ctx); err == nil && !st.RecordingEnabled {
		return
	}
	r.mu.Lock()
	sess, ok := r.sessions[eventStored.CameraID]
	r.mu.Unlock()
	if !ok {
		return
	}
	sess.trigger(eventStored)
}

// DefaultCodes returns the codes that trigger a clip. We keep this aligned
// with [events.DefaultCodes] so anything the listener subscribes to can also
// trigger a recording. Filtering happens at OnEvent time via the Action field
// rather than the Code so all motion variants are eligible.
func DefaultCodes() []string { return events.DefaultCodes }

// ---- per-camera session ----

type camSession struct {
	rec      *Recorder
	cameraID int64
	name     string
	camHash  string

	workDir  string
	finalDir string

	ctx    context.Context
	cancel context.CancelFunc

	mu       sync.Mutex
	segments []segmentInfo // sorted oldest -> newest
	active   *activeClip
	// pinned segments are being read by an in-flight concat and must not be
	// pruned. Map key is segment file name.
	pinned  map[string]int
	stopped bool
}

type segmentInfo struct {
	name       string
	path       string
	appearedAt time.Time
}

type activeClip struct {
	triggerEventID int64
	startedAt      time.Time // earliest segment to include (trigger - PreRoll)
	triggerAt      time.Time // when the first Start landed (for the hard cap)
	deadline       time.Time // segmenting cutoff; extended on repeated Starts; bounded by triggerAt + MaxClip
}

func (r *Recorder) startSession(c store.Camera) (*camSession, error) {
	workDir := filepath.Join(r.workDir, fmt.Sprintf("cam-%d", c.ID))
	finalDir := filepath.Join(r.finalDir, fmt.Sprintf("cam-%d", c.ID))
	if err := os.RemoveAll(workDir); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(finalDir, 0o755); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(r.ctx)
	sess := &camSession{
		rec:      r,
		cameraID: c.ID,
		name:     c.Name,
		camHash:  connHash(c),
		workDir:  workDir,
		finalDir: finalDir,
		ctx:      ctx,
		cancel:   cancel,
		pinned:   make(map[string]int),
	}
	r.log.Info("recorder session started", "camera_id", c.ID, "camera", c.Name, "work_dir", workDir)

	go sess.runFfmpegLoop(c)
	go sess.pollLoop()
	return sess, nil
}

// runFfmpegLoop keeps the segmenter ffmpeg alive across transient camera
// outages. The loop exits when the session context is cancelled (camera
// removed or recorder shutting down).
func (s *camSession) runFfmpegLoop(c store.Camera) {
	backoff := time.Second
	const maxBackoff = 30 * time.Second
	for {
		if s.ctx.Err() != nil {
			return
		}
		err := s.runOnceFfmpeg(c)
		if s.ctx.Err() != nil {
			return
		}
		s.rec.log.Warn("recorder ffmpeg exited; retrying",
			"camera_id", s.cameraID, "err", err, "retry_in", backoff)
		select {
		case <-s.ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff < maxBackoff {
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}
	}
}

func (s *camSession) runOnceFfmpeg(c store.Camera) error {
	rtspURL := buildRTSPURL(c.Username, c.Password, c.Host)

	// -c copy: pure remux of the camera's H.264/H.265 main stream into MPEG-TS.
	// segment_time=1 + reset_timestamps keeps each .ts file roughly bounded
	// (it'll still cut on keyframes, so 1-2s in practice on a Dahua main).
	cmd := exec.CommandContext(s.ctx, "ffmpeg",
		"-loglevel", "warning",
		"-rtsp_transport", "tcp",
		"-i", rtspURL,
		"-an",
		"-c", "copy",
		"-f", "segment",
		"-segment_time", "1",
		"-segment_format", "mpegts",
		"-reset_timestamps", "1",
		"-strftime", "0",
		filepath.Join(s.workDir, "seg-%06d.ts"),
	)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start ffmpeg: %w", err)
	}
	go drainStderr(s.rec.log, s.cameraID, stderr)
	return cmd.Wait()
}

func (s *camSession) stop() {
	s.mu.Lock()
	s.stopped = true
	s.mu.Unlock()
	s.cancel()
}

// pollLoop watches workDir for new segments, prunes old ones, and finalizes
// any active clip whose deadline has been crossed.
func (s *camSession) pollLoop() {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.scanNewSegments()
			s.maybeFinalize()
			s.prune()
		}
	}
}

func (s *camSession) scanNewSegments() {
	entries, err := os.ReadDir(s.workDir)
	if err != nil {
		return
	}
	now := time.Now()
	known := make(map[string]struct{}, len(s.segments))
	s.mu.Lock()
	for _, seg := range s.segments {
		known[seg.name] = struct{}{}
	}
	s.mu.Unlock()

	var fresh []segmentInfo
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), "seg-") || !strings.HasSuffix(e.Name(), ".ts") {
			continue
		}
		if _, ok := known[e.Name()]; ok {
			continue
		}
		fresh = append(fresh, segmentInfo{
			name:       e.Name(),
			path:       filepath.Join(s.workDir, e.Name()),
			appearedAt: now,
		})
	}
	if len(fresh) == 0 {
		return
	}
	// Sort by name so the segment index order is preserved (ffmpeg writes
	// them lexicographically with %06d).
	sort.Slice(fresh, func(i, j int) bool { return fresh[i].name < fresh[j].name })

	s.mu.Lock()
	s.segments = append(s.segments, fresh...)
	s.mu.Unlock()
}

// trigger is called when a Start event arrives for this camera. If a clip is
// already in flight we extend its deadline (bounded by the hard cap from the
// original trigger). Otherwise we start a new one.
func (s *camSession) trigger(ev store.Event) {
	now := time.Now()
	triggerTime := ev.OccurredAt
	if triggerTime.IsZero() {
		triggerTime = now
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.stopped {
		return
	}

	if s.active != nil {
		newDeadline := now.Add(s.rec.cfg.PostRoll)
		hardCap := s.active.triggerAt.Add(s.rec.cfg.MaxClip)
		if newDeadline.After(hardCap) {
			newDeadline = hardCap
		}
		if newDeadline.After(s.active.deadline) {
			s.active.deadline = newDeadline
			s.rec.log.Debug("recorder clip extended",
				"camera_id", s.cameraID, "code", ev.Code,
				"deadline", s.active.deadline)
		}
		return
	}

	s.active = &activeClip{
		triggerEventID: ev.ID,
		triggerAt:      triggerTime,
		startedAt:      triggerTime.Add(-s.rec.cfg.PreRoll),
		deadline:       now.Add(s.rec.cfg.PostRoll),
	}
	s.rec.log.Info("recorder clip started",
		"camera_id", s.cameraID, "camera", s.name,
		"code", ev.Code, "event_id", ev.ID,
		"trigger_at", triggerTime, "deadline", s.active.deadline)
	if hook := s.rec.OnClipOpened; hook != nil {
		go hook(ev)
	}
}

// maybeFinalize finalizes the active clip if (a) the deadline has passed AND
// (b) we have a segment that appeared after the deadline (so we know exactly
// where to cut). The "one segment past the deadline" rule guarantees we
// include the last covering segment in full.
func (s *camSession) maybeFinalize() {
	s.mu.Lock()
	if s.active == nil {
		s.mu.Unlock()
		return
	}
	clip := *s.active
	now := time.Now()
	if now.Before(clip.deadline) {
		s.mu.Unlock()
		return
	}
	// Find segments to include: those whose appearedAt is within
	// [clip.startedAt, clip.deadline]. Also confirm there's at least one
	// segment past the deadline so we know the run ended.
	var include []segmentInfo
	hasPastDeadline := false
	for _, seg := range s.segments {
		if seg.appearedAt.After(clip.deadline) {
			hasPastDeadline = true
			break
		}
		if !seg.appearedAt.Before(clip.startedAt) {
			include = append(include, seg)
		}
	}
	if !hasPastDeadline {
		s.mu.Unlock()
		return
	}
	// Capture state and clear active so subsequent triggers start a new clip.
	// Pin the segments we're about to concat so prune() doesn't delete them
	// out from under the ffmpeg process.
	for _, seg := range include {
		s.pinned[seg.name]++
	}
	s.active = nil
	s.mu.Unlock()

	if len(include) == 0 {
		s.rec.log.Warn("recorder clip had no segments to concat",
			"camera_id", s.cameraID, "event_id", clip.triggerEventID)
		return
	}
	go func() {
		defer s.unpin(include)
		s.finalizeClip(clip, include)
	}()
}

func (s *camSession) unpin(segs []segmentInfo) {
	s.mu.Lock()
	for _, seg := range segs {
		if s.pinned[seg.name] <= 1 {
			delete(s.pinned, seg.name)
		} else {
			s.pinned[seg.name]--
		}
	}
	s.mu.Unlock()
}

func (s *camSession) finalizeClip(clip activeClip, segs []segmentInfo) {
	startedAt := segs[0].appearedAt
	endedAt := clip.deadline
	if last := segs[len(segs)-1].appearedAt; last.After(endedAt) {
		endedAt = last
	}
	outName := fmt.Sprintf("%s_event-%d.mp4",
		clip.triggerAt.UTC().Format("20060102T150405Z"),
		clip.triggerEventID)
	outPath := filepath.Join(s.finalDir, outName)

	listPath := filepath.Join(s.workDir, fmt.Sprintf("concat-%d.txt", time.Now().UnixNano()))
	if err := writeConcatList(listPath, segs); err != nil {
		s.rec.log.Error("recorder: write concat list", "err", err)
		return
	}
	defer os.Remove(listPath)

	// -bsf:a aac_adtstoasc would be needed if we kept audio; we don't.
	cmd := exec.CommandContext(s.ctx, "ffmpeg",
		"-loglevel", "warning",
		"-f", "concat",
		"-safe", "0",
		"-i", listPath,
		"-c", "copy",
		"-movflags", "+faststart",
		outPath,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		s.rec.log.Error("recorder: ffmpeg concat failed",
			"camera_id", s.cameraID, "event_id", clip.triggerEventID,
			"err", err, "ffmpeg", strings.TrimSpace(string(out)))
		return
	}

	info, err := os.Stat(outPath)
	if err != nil {
		s.rec.log.Error("recorder: stat output", "path", outPath, "err", err)
		return
	}
	rec, err := s.rec.store.InsertRecording(s.rec.ctx, store.Recording{
		CameraID:   s.cameraID,
		EventID:    clip.triggerEventID,
		StartedAt:  startedAt,
		EndedAt:    endedAt,
		DurationMs: endedAt.Sub(startedAt).Milliseconds(),
		Path:       outPath,
		SizeBytes:  info.Size(),
	})
	if err != nil {
		s.rec.log.Error("recorder: insert recording row",
			"camera_id", s.cameraID, "path", outPath, "err", err)
		return
	}
	s.rec.log.Info("recorder clip saved",
		"camera_id", s.cameraID, "event_id", clip.triggerEventID,
		"recording_id", rec.ID, "path", outPath,
		"segments", len(segs), "size_bytes", info.Size())
	if hook := s.rec.OnRecordingInserted; hook != nil {
		rec.CameraName = s.name
		go hook(rec)
	}
}

// prune removes segments older than what the pre-roll could possibly need,
// except those that fall inside an active clip's window. Runs every poll tick.
func (s *camSession) prune() {
	s.mu.Lock()
	defer s.mu.Unlock()

	keepFloor := time.Now().Add(-(s.rec.cfg.PreRoll + segmentBufferMargin))
	if s.active != nil && s.active.startedAt.Before(keepFloor) {
		keepFloor = s.active.startedAt
	}

	// Always keep the most-recent segment regardless of age — it's still
	// being written to and we need a "boundary" entry to know any new
	// arrivals.
	if len(s.segments) <= 1 {
		return
	}
	cutoff := 0
	for i, seg := range s.segments[:len(s.segments)-1] {
		if seg.appearedAt.Before(keepFloor) {
			cutoff = i + 1
		} else {
			break
		}
	}
	if cutoff == 0 {
		return
	}
	// Drop pinned segments from the prune set: they belong to an in-flight
	// concat. They'll be cleaned up on the next prune after the clip ships.
	kept := s.segments[:0:0]
	for i, seg := range s.segments {
		if i < cutoff && s.pinned[seg.name] == 0 {
			_ = os.Remove(seg.path)
			continue
		}
		kept = append(kept, seg)
	}
	s.segments = kept
}

// ---- helpers ----

func writeConcatList(path string, segs []segmentInfo) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	for _, seg := range segs {
		// concat demuxer wants escaped single-quoted paths; our segment names
		// are seg-NNNNNN.ts so there's nothing to escape, but be defensive.
		fmt.Fprintf(f, "file '%s'\n", strings.ReplaceAll(seg.path, "'", `'\''`))
	}
	return nil
}

func drainStderr(log *slog.Logger, cameraID int64, r interface{ Read(p []byte) (int, error) }) {
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			line := strings.TrimSpace(string(buf[:n]))
			if line != "" {
				log.Debug("recorder ffmpeg", "camera_id", cameraID, "msg", line)
			}
		}
		if err != nil {
			return
		}
	}
}

func buildRTSPURL(user, pass, host string) string {
	u := url.URL{
		Scheme:   "rtsp",
		User:     url.UserPassword(user, pass),
		Host:     ensurePort(host, "554"),
		Path:     "/cam/realmonitor",
		RawQuery: "channel=1&subtype=0",
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

func connHash(c store.Camera) string {
	return c.Host + "\x00" + c.Username + "\x00" + c.Password
}

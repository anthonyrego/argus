package eventmgr

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"

	"github.com/rego/argus/internal/events"
	"github.com/rego/argus/internal/store"
)

// EventSink receives every persisted event. Unlike the Subscribe channel it
// is called synchronously and cannot drop, so use it for components that must
// see every event (e.g. the clip recorder).
type EventSink interface {
	OnEvent(store.Event)
}

// CameraSyncer is told the current desired camera set whenever it changes.
// Implemented by the recorder so it can start/stop per-camera ffmpegs in
// lock-step with the event listeners.
type CameraSyncer interface {
	Sync(cams []store.Camera)
}

// Manager supervises one Dahua event listener per enabled camera. It persists
// events to the store and fans them out to in-process subscribers (SSE) and
// to any registered sinks.
type Manager struct {
	store *store.Store
	log   *slog.Logger

	ctx    context.Context
	cancel context.CancelFunc

	mu       sync.Mutex
	runners  map[int64]*runner // camera_id -> running listener
	sinks    []EventSink
	syncers  []CameraSyncer
	subsMu   sync.RWMutex
	subs     map[int]chan store.Event
	recSubs  map[int]chan store.Recording
	subsNext int
}

type runner struct {
	cancel context.CancelFunc
	snap   store.Camera
}

func New(ctx context.Context, s *store.Store, log *slog.Logger) *Manager {
	mctx, cancel := context.WithCancel(ctx)
	return &Manager{
		store:   s,
		log:     log,
		ctx:     mctx,
		cancel:  cancel,
		runners: make(map[int64]*runner),
		subs:    make(map[int]chan store.Event),
		recSubs: make(map[int]chan store.Recording),
	}
}

// AddSink registers an EventSink to receive every persisted event.
// Must be called before Start.
func (m *Manager) AddSink(s EventSink) {
	m.mu.Lock()
	m.sinks = append(m.sinks, s)
	m.mu.Unlock()
}

// AddSyncer registers a CameraSyncer to be notified on every camera change.
// Must be called before Start.
func (m *Manager) AddSyncer(s CameraSyncer) {
	m.mu.Lock()
	m.syncers = append(m.syncers, s)
	m.mu.Unlock()
}

// Start launches a listener for every currently-enabled camera. Call once at
// boot; subsequent camera changes must go through Sync.
func (m *Manager) Start() error {
	cams, err := m.store.ListCameras(m.ctx)
	if err != nil {
		return err
	}
	m.mu.Lock()
	for _, c := range cams {
		if c.Enabled {
			m.startLocked(c)
		}
	}
	syncers := append([]CameraSyncer(nil), m.syncers...)
	m.mu.Unlock()
	for _, sy := range syncers {
		sy.Sync(cams)
	}
	return nil
}

// Sync reconciles running listeners with the current camera set. Call after
// any camera create/update/delete.
func (m *Manager) Sync() {
	cams, err := m.store.ListCameras(m.ctx)
	if err != nil {
		m.log.Error("sync: list cameras", "err", err)
		return
	}
	want := make(map[int64]store.Camera, len(cams))
	for _, c := range cams {
		if c.Enabled {
			want[c.ID] = c
		}
	}

	m.mu.Lock()

	// Stop runners that are no longer wanted, or whose connection params changed.
	for id, r := range m.runners {
		w, ok := want[id]
		if !ok || !sameConn(r.snap, w) {
			r.cancel()
			delete(m.runners, id)
		}
	}
	// Start runners that are wanted but not running.
	for id, c := range want {
		if _, ok := m.runners[id]; !ok {
			m.startLocked(c)
		}
	}
	syncers := append([]CameraSyncer(nil), m.syncers...)
	m.mu.Unlock()

	for _, sy := range syncers {
		sy.Sync(cams)
	}
}

// Close stops all listeners. Safe to call multiple times.
func (m *Manager) Close() {
	m.cancel()
	m.mu.Lock()
	for id, r := range m.runners {
		r.cancel()
		delete(m.runners, id)
	}
	m.mu.Unlock()
}

func sameConn(a, b store.Camera) bool {
	return a.Host == b.Host && a.Username == b.Username && a.Password == b.Password
}

// startLocked must be called with m.mu held.
func (m *Manager) startLocked(c store.Camera) {
	rctx, cancel := context.WithCancel(m.ctx)
	r := &runner{cancel: cancel, snap: c}
	m.runners[c.ID] = r

	src := events.NewDahuaSource(c.Host, c.Username, c.Password, events.DefaultCodes, m.log.With("camera_id", c.ID, "camera", c.Name))
	ch := make(chan events.Event, 32)

	go func() {
		if err := src.Run(rctx, ch); err != nil {
			m.log.Error("event source stopped", "camera_id", c.ID, "err", err)
		}
		close(ch)
	}()

	go func() {
		for ev := range ch {
			stored := m.persist(c.ID, c.Name, ev)
			m.dispatch(stored)
		}
	}()

	m.log.Info("event listener started", "camera_id", c.ID, "camera", c.Name, "host", c.Host)
}

func (m *Manager) persist(cameraID int64, cameraName string, ev events.Event) store.Event {
	var dataJSON string
	if ev.Data != nil {
		if b, err := json.Marshal(ev.Data); err == nil {
			dataJSON = string(b)
		}
	}
	row, err := m.store.InsertEvent(m.ctx, store.Event{
		CameraID:   cameraID,
		Code:       ev.Code,
		Action:     ev.Action,
		ChannelIdx: ev.Index,
		DataJSON:   dataJSON,
		OccurredAt: ev.Time,
	})
	if err != nil {
		m.log.Error("persist event", "camera_id", cameraID, "code", ev.Code, "err", err)
		// Fall back to an in-memory event so subscribers still see it.
		row = store.Event{
			CameraID:   cameraID,
			Code:       ev.Code,
			Action:     ev.Action,
			ChannelIdx: ev.Index,
			DataJSON:   dataJSON,
			OccurredAt: ev.Time,
		}
	}
	row.CameraName = cameraName
	return row
}

// Subscribe returns a buffered channel that receives every event after the
// call. The returned cancel function must be called to release the subscription.
func (m *Manager) Subscribe() (<-chan store.Event, func()) {
	ch := make(chan store.Event, 32)
	m.subsMu.Lock()
	id := m.subsNext
	m.subsNext++
	m.subs[id] = ch
	m.subsMu.Unlock()
	return ch, func() {
		m.subsMu.Lock()
		if c, ok := m.subs[id]; ok {
			delete(m.subs, id)
			close(c)
		}
		m.subsMu.Unlock()
	}
}

// SubscribeRecordings returns a buffered channel that receives every recording
// inserted after the call. Same semantics as Subscribe.
func (m *Manager) SubscribeRecordings() (<-chan store.Recording, func()) {
	ch := make(chan store.Recording, 32)
	m.subsMu.Lock()
	id := m.subsNext
	m.subsNext++
	m.recSubs[id] = ch
	m.subsMu.Unlock()
	return ch, func() {
		m.subsMu.Lock()
		if c, ok := m.recSubs[id]; ok {
			delete(m.recSubs, id)
			close(c)
		}
		m.subsMu.Unlock()
	}
}

// BroadcastRecording fans a newly-inserted recording out to SSE subscribers.
// Wired from the recorder's OnRecordingInserted hook.
func (m *Manager) BroadcastRecording(rec store.Recording) {
	m.subsMu.RLock()
	defer m.subsMu.RUnlock()
	for _, ch := range m.recSubs {
		select {
		case ch <- rec:
		default:
		}
	}
}

// dispatch fans out a persisted event to sinks (synchronous, no drops) and to
// SSE subscribers (best-effort with drop on slow consumer).
func (m *Manager) dispatch(e store.Event) {
	m.mu.Lock()
	sinks := append([]EventSink(nil), m.sinks...)
	m.mu.Unlock()
	for _, s := range sinks {
		s.OnEvent(e)
	}
	m.broadcast(e)
}

func (m *Manager) broadcast(e store.Event) {
	m.subsMu.RLock()
	defer m.subsMu.RUnlock()
	for _, ch := range m.subs {
		select {
		case ch <- e:
		default:
			// Slow consumer; drop to keep the producer moving.
		}
	}
}

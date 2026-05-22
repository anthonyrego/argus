// Package apns pushes motion notifications to registered iOS devices.
//
// The sender is wired to the recorder's "new clip opened" hook so we fire
// exactly one push per recorded clip (the recorder coalesces overlapping
// motion starts into a single clip; we follow the same coalescing). The
// notification body is text only — the lock-screen preview image is fetched
// by the iOS Notification Service Extension from the snapshot URL we include
// in the payload.
package apns

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"

	"github.com/sideshow/apns2"
	"github.com/sideshow/apns2/payload"
	"github.com/sideshow/apns2/token"

	"github.com/rego/argus/internal/store"
)

// Config controls the APNs client.
type Config struct {
	TeamID     string
	KeyID      string
	KeyPath    string // path to the .p8 auth key
	BundleID   string // e.g. ai.getaide.argus
	Production bool   // false = sandbox/development
}

// Sender pushes motion notifications to registered iOS devices.
type Sender struct {
	cfg    Config
	store  *store.Store
	log    *slog.Logger
	client *apns2.Client

	queue  chan store.Event
	wg     sync.WaitGroup
	ctx    context.Context
	cancel context.CancelFunc
}

func New(ctx context.Context, s *store.Store, log *slog.Logger, cfg Config) (*Sender, error) {
	keyBytes, err := os.ReadFile(cfg.KeyPath)
	if err != nil {
		return nil, fmt.Errorf("read APNs key: %w", err)
	}
	authKey, err := token.AuthKeyFromBytes(keyBytes)
	if err != nil {
		return nil, fmt.Errorf("parse APNs key: %w", err)
	}
	tok := &token.Token{
		AuthKey: authKey,
		KeyID:   cfg.KeyID,
		TeamID:  cfg.TeamID,
	}
	client := apns2.NewTokenClient(tok)
	if cfg.Production {
		client = client.Production()
	} else {
		client = client.Development()
	}

	sctx, cancel := context.WithCancel(ctx)
	snd := &Sender{
		cfg:    cfg,
		store:  s,
		log:    log,
		client: client,
		queue:  make(chan store.Event, 64),
		ctx:    sctx,
		cancel: cancel,
	}
	snd.wg.Add(1)
	go snd.worker()
	return snd, nil
}

func (s *Sender) Close() {
	s.cancel()
	s.wg.Wait()
}

// OnClipOpened is the recorder's hook. Enqueues a single push for this clip.
// Non-blocking: drops with a warning if the queue is full.
func (s *Sender) OnClipOpened(ev store.Event) {
	if s.ctx.Err() != nil {
		return
	}
	select {
	case s.queue <- ev:
	case <-s.ctx.Done():
	default:
		s.log.Warn("apns queue full, dropping push", "event_id", ev.ID)
	}
}

func (s *Sender) worker() {
	defer s.wg.Done()
	for {
		select {
		case <-s.ctx.Done():
			return
		case ev := <-s.queue:
			s.fanOut(ev)
		}
	}
}

func (s *Sender) fanOut(ev store.Event) {
	targets, err := s.store.ListPushTargets(s.ctx)
	if err != nil {
		s.log.Error("apns: list push targets", "err", err)
		return
	}
	if len(targets) == 0 {
		return
	}
	title := ev.CameraName
	if title == "" {
		title = "Camera"
	}

	p := payload.NewPayload().
		AlertTitle(title).
		AlertBody(readableCode(ev.Code)).
		Sound("default").
		MutableContent().
		ThreadID(fmt.Sprintf("camera-%d", ev.CameraID)).
		Custom("event_id", ev.ID).
		Custom("camera_id", ev.CameraID).
		Custom("camera_name", ev.CameraName).
		Custom("code", ev.Code).
		Custom("occurred_at", ev.OccurredAt.UTC().Format("2006-01-02T15:04:05Z")).
		Custom("snapshot_path", fmt.Sprintf("/api/cameras/%d/snapshot.jpg", ev.CameraID))

	for _, d := range targets {
		n := &apns2.Notification{
			DeviceToken: d.APNsToken,
			Topic:       s.cfg.BundleID,
			Payload:     p,
			Priority:    apns2.PriorityHigh,
		}
		res, err := s.client.PushWithContext(s.ctx, n)
		if err != nil {
			s.log.Error("apns push", "device_id", d.ID, "err", err)
			continue
		}
		switch {
		case res.Sent():
			s.log.Debug("apns sent", "device_id", d.ID, "event_id", ev.ID)
		case res.Reason == apns2.ReasonBadDeviceToken || res.Reason == apns2.ReasonUnregistered:
			s.log.Info("apns clearing rejected token",
				"device_id", d.ID, "reason", res.Reason)
			_ = s.store.ClearDeviceAPNsToken(context.Background(), d.ID)
		default:
			s.log.Warn("apns not sent",
				"device_id", d.ID, "status", res.StatusCode, "reason", res.Reason)
		}
	}
}

func readableCode(code string) string {
	switch code {
	case "SmartMotionHuman":
		return "Person detected"
	case "SmartMotionVehicle":
		return "Vehicle detected"
	case "CrossLineDetection":
		return "Crossed line"
	case "CrossRegionDetection":
		return "Entered region"
	case "VideoMotion":
		return "Motion"
	default:
		return code
	}
}

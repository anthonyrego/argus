package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/rego/argus/internal/camera"
	"github.com/rego/argus/internal/vision"
)

type Agent struct {
	cam              *camera.Camera
	analyzer         vision.Analyzer
	prompt           string
	interval         time.Duration
	observationsPath string
	log              *slog.Logger
}

type Observation struct {
	Timestamp   time.Time `json:"timestamp"`
	Description string    `json:"description"`
	DurationMs  int64     `json:"duration_ms"`
}

func New(
	cam *camera.Camera,
	analyzer vision.Analyzer,
	prompt string,
	interval time.Duration,
	observationsPath string,
	log *slog.Logger,
) *Agent {
	return &Agent{
		cam:              cam,
		analyzer:         analyzer,
		prompt:           prompt,
		interval:         interval,
		observationsPath: observationsPath,
		log:              log,
	}
}

// Run loops until ctx is cancelled, sampling and analyzing frames at the
// configured interval. Errors on individual ticks are logged and skipped — the
// loop continues so a brief outage doesn't kill the agent.
func (a *Agent) Run(ctx context.Context) error {
	f, err := os.OpenFile(a.observationsPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open observations log: %w", err)
	}
	defer f.Close()

	a.tick(ctx, f)

	ticker := time.NewTicker(a.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			a.tick(ctx, f)
		}
	}
}

func (a *Agent) tick(ctx context.Context, f *os.File) {
	start := time.Now()
	frame, err := a.cam.Grab(ctx)
	if err != nil {
		a.log.Error("grab frame", "err", err)
		return
	}
	desc, err := a.analyzer.Describe(ctx, frame, a.prompt)
	if err != nil {
		a.log.Error("analyze frame", "err", err)
		return
	}
	obs := Observation{
		Timestamp:   start,
		Description: desc,
		DurationMs:  time.Since(start).Milliseconds(),
	}
	line, _ := json.Marshal(obs)
	if _, err := f.Write(append(line, '\n')); err != nil {
		a.log.Error("write observation", "err", err)
		return
	}
	a.log.Info("observation", "desc", desc, "duration_ms", obs.DurationMs)
}

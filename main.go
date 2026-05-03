package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rego/argus/internal/agent"
	"github.com/rego/argus/internal/camera"
	"github.com/rego/argus/internal/config"
	"github.com/rego/argus/internal/vision"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to config file")
	flag.Parse()

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Error("load config", "err", err)
		os.Exit(1)
	}

	cam := camera.New(cfg.Camera.RTSPURL)
	analyzer := vision.NewOpenAI(cfg.Vision.BaseURL, cfg.Vision.APIKey, cfg.Vision.Model)
	a := agent.New(
		cam,
		analyzer,
		cfg.Vision.Prompt,
		time.Duration(cfg.Agent.IntervalSeconds)*time.Second,
		cfg.Agent.ObservationsPath,
		log,
	)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	log.Info("argus starting", "interval_s", cfg.Agent.IntervalSeconds, "model", cfg.Vision.Model)
	if err := a.Run(ctx); err != nil {
		log.Error("agent stopped", "err", err)
		os.Exit(1)
	}
	log.Info("argus stopped cleanly")
}

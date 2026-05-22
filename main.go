package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rego/argus/internal/config"
	"github.com/rego/argus/internal/eventmgr"
	"github.com/rego/argus/internal/notify/apns"
	"github.com/rego/argus/internal/recorder"
	"github.com/rego/argus/internal/server"
	"github.com/rego/argus/internal/store"
	"github.com/rego/argus/internal/streamer"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to config file")
	debug := flag.Bool("debug", false, "verbose logging")
	flag.Parse()

	level := slog.LevelInfo
	if *debug {
		level = slog.LevelDebug
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Error("load config", "err", err)
		os.Exit(1)
	}

	st, err := store.Open(cfg.Server.Database)
	if err != nil {
		log.Error("open store", "err", err)
		os.Exit(1)
	}
	defer st.Close()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	mgr := eventmgr.New(ctx, st, log)

	rec, err := recorder.New(ctx, st, log, recorder.Config{
		Dir:      cfg.Recordings.Dir,
		PreRoll:  time.Duration(cfg.Recordings.PreRollSec) * time.Second,
		PostRoll: time.Duration(cfg.Recordings.PostRollSec) * time.Second,
		MaxClip:  time.Duration(cfg.Recordings.MaxClipSec) * time.Second,
	})
	if err != nil {
		log.Error("start recorder", "err", err)
		os.Exit(1)
	}
	defer rec.Close()
	mgr.AddSink(rec)
	mgr.AddSyncer(rec)
	rec.OnRecordingInserted = mgr.BroadcastRecording

	if cfg.APNs.PushEnabled() {
		pusher, err := apns.New(ctx, st, log, apns.Config{
			TeamID:     cfg.APNs.TeamID,
			KeyID:      cfg.APNs.KeyID,
			KeyPath:    cfg.APNs.KeyPath,
			BundleID:   cfg.APNs.BundleID,
			Production: cfg.APNs.Environment == "production",
		})
		if err != nil {
			log.Error("start apns sender", "err", err)
			os.Exit(1)
		}
		defer pusher.Close()
		rec.OnClipOpened = pusher.OnClipOpened
		log.Info("apns push enabled", "bundle_id", cfg.APNs.BundleID, "env", cfg.APNs.Environment)
	} else {
		log.Info("apns push disabled (config incomplete)")
	}

	if err := mgr.Start(); err != nil {
		log.Error("start event manager", "err", err)
		os.Exit(1)
	}
	defer mgr.Close()

	hls, err := streamer.New(ctx, st, log)
	if err != nil {
		log.Error("start streamer", "err", err)
		os.Exit(1)
	}
	defer hls.Close()

	srv := server.New(st, mgr, hls, server.Frontend(), log)
	httpSrv := &http.Server{
		Addr:              cfg.Server.Addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		_ = httpSrv.Shutdown(shutdownCtx)
	}()

	log.Info("argus listening", "addr", cfg.Server.Addr, "db", cfg.Server.Database)
	if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error("http server", "err", err)
		os.Exit(1)
	}
	log.Info("argus stopped cleanly")
}

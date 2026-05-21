package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config holds runtime settings. Cameras themselves are stored in SQLite and
// managed via the UI; only server-level knobs live here.
type Config struct {
	Server     ServerConfig     `yaml:"server"`
	Recordings RecordingsConfig `yaml:"recordings"`
}

type ServerConfig struct {
	Addr     string `yaml:"addr"`      // e.g. ":8080" or "0.0.0.0:8080"
	Database string `yaml:"database"`  // path to the SQLite file
}

// RecordingsConfig controls the motion-event clip recorder. PreRollSec is the
// number of seconds before the event to include; MaxClipSec is the hard upper
// bound on a single clip (extension on repeated Starts is capped at this).
type RecordingsConfig struct {
	Dir         string `yaml:"dir"`           // base directory for saved clips
	PreRollSec  int    `yaml:"pre_roll_sec"`  // seconds of pre-buffer
	PostRollSec int    `yaml:"post_roll_sec"` // seconds after a Start before finalizing (per trigger)
	MaxClipSec  int    `yaml:"max_clip_sec"`  // hard cap on a single clip's total length
}

func Load(path string) (*Config, error) {
	c := &Config{
		Server: ServerConfig{Addr: ":8080", Database: "argus.db"},
		Recordings: RecordingsConfig{
			Dir:         "recordings",
			PreRollSec:  5,
			PostRollSec: 10,
			MaxClipSec:  30,
		},
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return c, nil // defaults are fine for a fresh install
		}
		return nil, fmt.Errorf("read config: %w", err)
	}
	if err := yaml.Unmarshal(data, c); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if c.Server.Addr == "" {
		c.Server.Addr = ":8080"
	}
	if c.Server.Database == "" {
		c.Server.Database = "argus.db"
	}
	if c.Recordings.Dir == "" {
		c.Recordings.Dir = "recordings"
	}
	if c.Recordings.PreRollSec <= 0 {
		c.Recordings.PreRollSec = 5
	}
	if c.Recordings.PostRollSec <= 0 {
		c.Recordings.PostRollSec = 10
	}
	if c.Recordings.MaxClipSec <= 0 {
		c.Recordings.MaxClipSec = 30
	}
	return c, nil
}

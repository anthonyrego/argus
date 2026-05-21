package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config holds runtime settings. Cameras themselves are stored in SQLite and
// managed via the UI; only server-level knobs live here.
type Config struct {
	Server ServerConfig `yaml:"server"`
}

type ServerConfig struct {
	Addr     string `yaml:"addr"`      // e.g. ":8080" or "0.0.0.0:8080"
	Database string `yaml:"database"`  // path to the SQLite file
}

func Load(path string) (*Config, error) {
	c := &Config{
		Server: ServerConfig{Addr: ":8080", Database: "argus.db"},
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
	return c, nil
}

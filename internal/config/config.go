package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

const defaultOpenAIBaseURL = "https://api.openai.com/v1"

type Config struct {
	Camera CameraConfig `yaml:"camera"`
	Vision VisionConfig `yaml:"vision"`
	Agent  AgentConfig  `yaml:"agent"`
}

type CameraConfig struct {
	RTSPURL string `yaml:"rtsp_url"`
}

type VisionConfig struct {
	BaseURL string `yaml:"base_url"`
	APIKey  string `yaml:"api_key"`
	Model   string `yaml:"model"`
	Prompt  string `yaml:"prompt"`
}

type AgentConfig struct {
	IntervalSeconds  int    `yaml:"interval_seconds"`
	ObservationsPath string `yaml:"observations_path"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var c Config
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if c.Camera.RTSPURL == "" {
		return nil, fmt.Errorf("camera.rtsp_url is required")
	}
	if c.Vision.BaseURL == "" {
		c.Vision.BaseURL = defaultOpenAIBaseURL
	}
	if c.Vision.APIKey == "" {
		c.Vision.APIKey = os.Getenv("OPENAI_API_KEY")
	}
	if c.Vision.Model == "" {
		return nil, fmt.Errorf("vision.model is required")
	}
	if c.Agent.IntervalSeconds <= 0 {
		c.Agent.IntervalSeconds = 10
	}
	if c.Agent.ObservationsPath == "" {
		c.Agent.ObservationsPath = "observations.jsonl"
	}
	return &c, nil
}

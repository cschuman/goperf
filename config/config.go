package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config defines .goperf.yml settings.
type Config struct {
	Rules       []string `yaml:"rules"`
	IgnorePaths []string `yaml:"ignore_paths"`
	FailOn      string   `yaml:"fail_on"`
	Format      string   `yaml:"format"`
	Context     int      `yaml:"context"`
	Verbose     bool     `yaml:"verbose"`
}

// LoadConfig reads .goperf.yml from the current working directory.
func LoadConfig() (Config, error) {
	const configFile = ".goperf.yml"

	data, err := os.ReadFile(configFile)
	if err != nil {
		if os.IsNotExist(err) {
			return Config{}, nil
		}
		return Config{}, fmt.Errorf("read %s: %w", configFile, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse %s: %w", configFile, err)
	}

	return cfg, nil
}

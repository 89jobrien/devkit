// cmd/devkit/config.go
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Config mirrors the structure of .devkit.toml.
type Config struct {
	Project struct {
		Name        string   `toml:"name"`
		Description string   `toml:"description"`
		Version     string   `toml:"version"`
		InstallDate string   `toml:"install_date"`
		CIPlatforms []string `toml:"ci_platforms"`
	} `toml:"project"`
	Context struct {
		Files []string `toml:"files"`
	} `toml:"context"`
	Review struct {
		Focus string `toml:"focus"`
	} `toml:"review"`
	Components struct {
		Council  bool `toml:"council"`
		Review   bool `toml:"review"`
		Meta     bool `toml:"meta"`
		CIAgent  bool `toml:"ci_agent"`
		Diagnose bool `toml:"diagnose"`
	} `toml:"components"`
	Diagnose struct {
		LogCmd  string `toml:"log_cmd"`
		Service string `toml:"service"`
	} `toml:"diagnose"`
	Providers struct {
		Primary           string `toml:"primary"`
		FastModel         string `toml:"fast_model"`
		BalancedModel     string `toml:"balanced_model"`
		LargeContextModel string `toml:"large_context_model"`
		CodingModel       string `toml:"coding_model"`
	} `toml:"providers"`
}

// LoadConfig finds and parses the nearest .devkit.toml walking up from cwd.
func LoadConfig() (*Config, error) {
	dir, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	for {
		path := filepath.Join(dir, ".devkit.toml")
		if _, err := os.Stat(path); err == nil {
			var cfg Config
			if _, err := toml.DecodeFile(path, &cfg); err != nil {
				return nil, fmt.Errorf("parsing %s: %w", path, err)
			}
			return &cfg, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return &Config{}, nil
}

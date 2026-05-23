package config

import (
	"os"
	"gopkg.in/yaml.v2"
)

type ServerConfig struct {
	Address    string `yaml:"address"`
	ApiKey     string `yaml:"api_key"`
	TimeoutSec int    `yaml:"timeout_seconds"`
}

type BlockchainConfig struct {
	Difficulty int    `yaml:"difficulty"`
	GenesisData string `yaml:"genesis_data"`
}

type PoHConfig struct {
	IntervalMs    int `yaml:"interval_ms"`
	VerifyWindow  int `yaml:"verify_window"`
}

type Config struct {
	Server     ServerConfig     `yaml:"server"`
	Blockchain BlockchainConfig `yaml:"blockchain"`
	PoH        PoHConfig        `yaml:"poh"`
}

func Load() *Config {
	data, err := os.ReadFile("config/config.yaml")
	if err != nil {
		panic("Failed to read config: " + err.Error())
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		panic("Failed to parse config: " + err.Error())
	}

	// Set defaults
	if cfg.Server.TimeoutSec == 0 {
		cfg.Server.TimeoutSec = 30
	}
	if cfg.PoH.IntervalMs == 0 {
		cfg.PoH.IntervalMs = 1000
	}

	return &cfg
}

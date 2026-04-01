package config

import (
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Token            string `toml:"token"`
	APIURL           string `toml:"api_url"`
	MetricsInterval  int    `toml:"metrics_interval"`
	HostnameOverride string `toml:"hostname_override"`
}

func DefaultConfig() Config {
	return Config{
		APIURL:          "https://api.plingpush.com",
		MetricsInterval: 60,
	}
}

func Load() (Config, error) {
	cfg := DefaultConfig()

	paths := []string{
		"/etc/pling-agent/config.toml",
		filepath.Join(homeDir(), ".config", "pling-agent", "config.toml"),
	}

	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			if _, err := toml.DecodeFile(p, &cfg); err != nil {
				return cfg, err
			}
			return cfg, nil
		}
	}

	if t := os.Getenv("PLING_TOKEN"); t != "" {
		cfg.Token = t
	}

	return cfg, nil
}

func (c Config) Hostname() string {
	if c.HostnameOverride != "" {
		return c.HostnameOverride
	}
	name, _ := os.Hostname()
	return name
}

func homeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	return "/root"
}

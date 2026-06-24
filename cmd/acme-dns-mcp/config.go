package main

import (
	"os"

	"github.com/BurntSushi/toml"
)

type mcpConfig struct {
	BaseURL    string `toml:"base_url"`
	AdminToken string `toml:"admin_token"`
	Username   string `toml:"username"`
	Password   string `toml:"password"`
}

// loadConfig reads from a TOML file (if path non-empty), then overrides with env vars.
func loadConfig(path string) mcpConfig {
	var cfg mcpConfig
	if path != "" {
		_, _ = toml.DecodeFile(path, &cfg)
	}
	if v := os.Getenv("ACMEDNS_BASE_URL"); v != "" {
		cfg.BaseURL = v
	}
	if v := os.Getenv("ACMEDNS_ADMIN_TOKEN"); v != "" {
		cfg.AdminToken = v
	}
	if v := os.Getenv("ACMEDNS_USERNAME"); v != "" {
		cfg.Username = v
	}
	if v := os.Getenv("ACMEDNS_PASSWORD"); v != "" {
		cfg.Password = v
	}
	return cfg
}

package main

import (
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
)

type mcpConfig struct {
	BaseURL    string `toml:"base_url"`
	AdminToken string `toml:"admin_token"`
	Username   string `toml:"username"`
	Password   string `toml:"password"`
}

// loadConfig reads from a TOML file (if path non-empty) or from env vars
func loadConfig(path string) mcpConfig {
	var cfg mcpConfig
	if path != "" {
		if _, err := toml.DecodeFile(path, &cfg); err != nil && !os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "acme-dns-mcp: failed to parse config file %s: %v\n", path, err)
		}
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

package main

import (
	"os"
	"testing"
)

func TestLoadConfigFromEnv(t *testing.T) {
	os.Setenv("ACMEDNS_BASE_URL", "https://acmedns.example.com")
	os.Setenv("ACMEDNS_ADMIN_TOKEN", "secret-admin")
	os.Setenv("ACMEDNS_USERNAME", "user-uuid")
	os.Setenv("ACMEDNS_PASSWORD", "user-pass")
	defer func() {
		os.Unsetenv("ACMEDNS_BASE_URL")
		os.Unsetenv("ACMEDNS_ADMIN_TOKEN")
		os.Unsetenv("ACMEDNS_USERNAME")
		os.Unsetenv("ACMEDNS_PASSWORD")
	}()

	cfg := loadConfig("")
	if cfg.BaseURL != "https://acmedns.example.com" {
		t.Errorf("BaseURL: got %q", cfg.BaseURL)
	}
	if cfg.AdminToken != "secret-admin" {
		t.Errorf("AdminToken: got %q", cfg.AdminToken)
	}
	if cfg.Username != "user-uuid" {
		t.Errorf("Username: got %q", cfg.Username)
	}
	if cfg.Password != "user-pass" {
		t.Errorf("Password: got %q", cfg.Password)
	}
}

func TestLoadConfigFromFile(t *testing.T) {
	f, _ := os.CreateTemp("", "mcp-cfg-*.toml")
	defer os.Remove(f.Name())
	f.WriteString(`
base_url = "https://local.example.com"
admin_token = "file-admin"
username = "file-user"
password = "file-pass"
`)
	f.Close()

	cfg := loadConfig(f.Name())
	if cfg.BaseURL != "https://local.example.com" {
		t.Errorf("BaseURL from file: got %q", cfg.BaseURL)
	}
}

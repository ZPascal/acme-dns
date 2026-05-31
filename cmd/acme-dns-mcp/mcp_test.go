package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestLoadConfigFromEnv(t *testing.T) {
	require := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	require(os.Setenv("ACMEDNS_BASE_URL", "https://acmedns.example.com"))
	require(os.Setenv("ACMEDNS_ADMIN_TOKEN", "secret-admin"))
	require(os.Setenv("ACMEDNS_USERNAME", "user-uuid"))
	require(os.Setenv("ACMEDNS_PASSWORD", "user-pass"))
	defer func() {
		_ = os.Unsetenv("ACMEDNS_BASE_URL")
		_ = os.Unsetenv("ACMEDNS_ADMIN_TOKEN")
		_ = os.Unsetenv("ACMEDNS_USERNAME")
		_ = os.Unsetenv("ACMEDNS_PASSWORD")
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
	f, err := os.CreateTemp("", "mcp-cfg-*.toml")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(f.Name()) }()
	if _, err := f.WriteString(`
base_url = "https://local.example.com"
admin_token = "file-admin"
username = "file-user"
password = "file-pass"
`); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	cfg := loadConfig(f.Name())
	if cfg.BaseURL != "https://local.example.com" {
		t.Errorf("BaseURL from file: got %q", cfg.BaseURL)
	}
}

func TestToolHealthCheck(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	cfg := mcpConfig{BaseURL: srv.URL}
	result, err := callTool(cfg, "health_check", map[string]interface{}{})
	if err != nil {
		t.Fatalf("health_check failed: %v", err)
	}
	if result["status"] != "ok" {
		t.Errorf("expected status ok, got %v", result)
	}
}

func TestToolListTools(t *testing.T) {
	tools := listTools()
	names := make(map[string]bool)
	for _, tool := range tools {
		names[tool.Name] = true
	}
	for _, expected := range []string{"register_subdomain", "update_txt_record", "list_dns_records", "create_dns_record", "update_dns_record", "delete_dns_record", "health_check"} {
		if !names[expected] {
			t.Errorf("missing tool: %s", expected)
		}
	}
}

func TestToolListRecords(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/admin/records" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[{"id":"test-id","name":"example.com","type":"A","value":"1.2.3.4","ttl":300,"created":0}]`))
		}
	}))
	defer srv.Close()

	cfg := mcpConfig{BaseURL: srv.URL, AdminToken: "test-token"}
	result, err := callTool(cfg, "list_dns_records", map[string]interface{}{})
	if err != nil {
		t.Fatalf("list_dns_records failed: %v", err)
	}
	records, ok := result["records"]
	if !ok {
		t.Fatalf("expected 'records' key in result, got %v", result)
	}
	arr, ok := records.([]interface{})
	if !ok {
		t.Fatalf("expected records to be array, got %T", records)
	}
	if len(arr) != 1 {
		t.Fatalf("expected 1 record, got %d", len(arr))
	}
}

package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
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
	result, isError, err := callTool(cfg, "health_check", map[string]interface{}{})
	if err != nil {
		t.Fatalf("health_check failed: %v", err)
	}
	if isError {
		t.Errorf("expected isError=false, got true (result: %v)", result)
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
	result, isError, err := callTool(cfg, "list_dns_records", map[string]interface{}{})
	if err != nil {
		t.Fatalf("list_dns_records failed: %v", err)
	}
	if isError {
		t.Errorf("expected isError=false, got true (result: %v)", result)
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

func TestToolListRecordsEscapesQueryParams(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	cfg := mcpConfig{BaseURL: srv.URL, AdminToken: "test-token"}
	_, _, err := callTool(cfg, "list_dns_records", map[string]interface{}{"name": "foo&type=TXT#frag"})
	if err != nil {
		t.Fatalf("list_dns_records failed: %v", err)
	}
	q, err := url.ParseQuery(gotQuery)
	if err != nil {
		t.Fatalf("server received unparseable query %q: %v", gotQuery, err)
	}
	if got := q.Get("name"); got != "foo&type=TXT#frag" {
		t.Errorf("expected name filter to survive intact, got %q (raw query: %q)", got, gotQuery)
	}
	if q.Get("type") != "" {
		t.Errorf("unescaped input injected an unintended type param: %q", gotQuery)
	}
}

func TestToolUpdateRecordEscapesID(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"weird"}`))
	}))
	defer srv.Close()

	cfg := mcpConfig{BaseURL: srv.URL, AdminToken: "test-token"}
	_, _, err := callTool(cfg, "update_dns_record", map[string]interface{}{
		"id": "abc?admin=true", "name": "x.example.com", "type": "A", "value": "1.2.3.4",
	})
	if err != nil {
		t.Fatalf("update_dns_record failed: %v", err)
	}
	if gotPath != "/admin/records/abc?admin=true" {
		t.Errorf("expected id to be escaped into the literal path, server saw path %q", gotPath)
	}
}

func TestToolCreateRecordSurfacesAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_ttl"}`))
	}))
	defer srv.Close()

	cfg := mcpConfig{BaseURL: srv.URL, AdminToken: "test-token"}
	result, isError, err := callTool(cfg, "create_dns_record", map[string]interface{}{
		"name": "x.example.com", "type": "A", "value": "1.2.3.4", "ttl": 0,
	})
	if err != nil {
		t.Fatalf("create_dns_record returned a transport error: %v", err)
	}
	if !isError {
		t.Fatalf("expected isError=true for a 400 response, got false (result: %v)", result)
	}
	if result["error"] != "invalid_ttl" {
		t.Errorf("expected the acme-dns error body to be surfaced, got %v", result)
	}
}

func TestToolMissingAdminTokenIsError(t *testing.T) {
	cfg := mcpConfig{BaseURL: "http://unused.invalid"}
	for _, name := range []string{"list_dns_records", "create_dns_record", "update_dns_record", "delete_dns_record"} {
		result, isError, err := callTool(cfg, name, map[string]interface{}{"id": "x"})
		if err != nil {
			t.Fatalf("%s: unexpected transport error: %v", name, err)
		}
		if !isError {
			t.Errorf("%s: expected isError=true when admin_token is not configured", name)
		}
		if result["error"] != "admin_token not configured" {
			t.Errorf("%s: expected admin_token error, got %v", name, result)
		}
	}
}

func TestToolHealthCheckUnhealthy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	cfg := mcpConfig{BaseURL: srv.URL}
	result, isError, err := callTool(cfg, "health_check", map[string]interface{}{})
	if err != nil {
		t.Fatalf("health_check returned a transport error: %v", err)
	}
	if !isError {
		t.Errorf("expected isError=true for a non-200 response, got false (result: %v)", result)
	}
	if result["status"] != "unhealthy" {
		t.Errorf("expected status unhealthy, got %v", result)
	}
	if result["code"] != http.StatusServiceUnavailable {
		t.Errorf("expected code %d, got %v", http.StatusServiceUnavailable, result["code"])
	}
}

func TestToolRegister(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/register" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"username":"u","password":"p","subdomain":"s","fulldomain":"s.example.org","allowfrom":[]}`))
	}))
	defer srv.Close()

	cfg := mcpConfig{BaseURL: srv.URL}
	result, isError, err := callTool(cfg, "register_subdomain", map[string]interface{}{})
	if err != nil {
		t.Fatalf("register_subdomain failed: %v", err)
	}
	if isError {
		t.Errorf("expected isError=false, got true (result: %v)", result)
	}
	if result["subdomain"] != "s" {
		t.Errorf("expected subdomain 's', got %v", result)
	}
}

func TestToolRegisterSurfacesAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_allowfrom_cidr"}`))
	}))
	defer srv.Close()

	cfg := mcpConfig{BaseURL: srv.URL}
	result, isError, err := callTool(cfg, "register_subdomain", map[string]interface{}{"allowfrom": []interface{}{"not-a-cidr"}})
	if err != nil {
		t.Fatalf("register_subdomain returned a transport error: %v", err)
	}
	if !isError {
		t.Fatalf("expected isError=true for a 400 response, got false (result: %v)", result)
	}
	if result["error"] != "invalid_allowfrom_cidr" {
		t.Errorf("expected the acme-dns error body to be surfaced, got %v", result)
	}
}

func TestToolUpdateTXT(t *testing.T) {
	var gotUser, gotKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUser = r.Header.Get("X-Api-User")
		gotKey = r.Header.Get("X-Api-Key")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"txt":"___validation_token_received_from_the_ca___"}`))
	}))
	defer srv.Close()

	cfg := mcpConfig{BaseURL: srv.URL, Username: "user-uuid", Password: "user-pass"}
	result, isError, err := callTool(cfg, "update_txt_record", map[string]interface{}{
		"subdomain": "s", "txt": "___validation_token_received_from_the_ca___",
	})
	if err != nil {
		t.Fatalf("update_txt_record failed: %v", err)
	}
	if isError {
		t.Errorf("expected isError=false, got true (result: %v)", result)
	}
	if gotUser != "user-uuid" || gotKey != "user-pass" {
		t.Errorf("expected credentials to be forwarded as headers, got user=%q key=%q", gotUser, gotKey)
	}
}

func TestToolUpdateTXTMissingCredentials(t *testing.T) {
	cfg := mcpConfig{BaseURL: "http://unused.invalid"}
	result, isError, err := callTool(cfg, "update_txt_record", map[string]interface{}{"subdomain": "s", "txt": "x"})
	if err != nil {
		t.Fatalf("unexpected transport error: %v", err)
	}
	if !isError {
		t.Errorf("expected isError=true when username/password are not configured")
	}
	if result["error"] != "username and password not configured" {
		t.Errorf("expected credentials error, got %v", result)
	}
}

func TestToolUpdateTXTSurfacesAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"bad_txt"}`))
	}))
	defer srv.Close()

	cfg := mcpConfig{BaseURL: srv.URL, Username: "u", Password: "p"}
	result, isError, err := callTool(cfg, "update_txt_record", map[string]interface{}{"subdomain": "s", "txt": "too-short"})
	if err != nil {
		t.Fatalf("update_txt_record returned a transport error: %v", err)
	}
	if !isError {
		t.Fatalf("expected isError=true for a 400 response, got false (result: %v)", result)
	}
	if result["error"] != "bad_txt" {
		t.Errorf("expected the acme-dns error body to be surfaced, got %v", result)
	}
}

func TestToolDeleteRecord(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/admin/records/test-id" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	cfg := mcpConfig{BaseURL: srv.URL, AdminToken: "test-token"}
	result, isError, err := callTool(cfg, "delete_dns_record", map[string]interface{}{"id": "test-id"})
	if err != nil {
		t.Fatalf("delete_dns_record failed: %v", err)
	}
	if isError {
		t.Errorf("expected isError=false, got true (result: %v)", result)
	}
	if result["status"] != "deleted" {
		t.Errorf("expected status deleted, got %v", result)
	}
}

func TestToolDeleteRecordNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"record_not_found"}`))
	}))
	defer srv.Close()

	cfg := mcpConfig{BaseURL: srv.URL, AdminToken: "test-token"}
	result, isError, err := callTool(cfg, "delete_dns_record", map[string]interface{}{"id": "missing"})
	if err != nil {
		t.Fatalf("delete_dns_record returned a transport error: %v", err)
	}
	if !isError {
		t.Fatalf("expected isError=true for a 404 response, got false (result: %v)", result)
	}
	if result["status"] != "error" || result["code"] != http.StatusNotFound {
		t.Errorf("expected status error with code 404, got %v", result)
	}
}

func TestToolUpdateRecordMissingID(t *testing.T) {
	cfg := mcpConfig{BaseURL: "http://unused.invalid", AdminToken: "test-token"}
	result, isError, err := callTool(cfg, "update_dns_record", map[string]interface{}{"name": "x", "type": "A", "value": "1.2.3.4"})
	if err != nil {
		t.Fatalf("unexpected transport error: %v", err)
	}
	if !isError {
		t.Errorf("expected isError=true when id is missing")
	}
	if result["error"] != "id is required" {
		t.Errorf("expected id-required error, got %v", result)
	}
}

func TestToolHealthCheckTransportError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	unreachable := srv.URL
	srv.Close() // closed immediately: nothing is listening on this address anymore

	cfg := mcpConfig{BaseURL: unreachable}
	result, isError, err := callTool(cfg, "health_check", map[string]interface{}{})
	if err == nil {
		t.Fatalf("expected a transport error when the server is unreachable, got result=%v isError=%v", result, isError)
	}
}

func TestCallToolUnknown(t *testing.T) {
	cfg := mcpConfig{BaseURL: "http://unused.invalid"}
	result, isError, err := callTool(cfg, "not_a_real_tool", map[string]interface{}{})
	if err == nil {
		t.Fatalf("expected a transport-level error for an unknown tool, got result=%v isError=%v", result, isError)
	}
	if result != nil {
		t.Errorf("expected nil result for an unknown tool, got %v", result)
	}
}

func TestAsResultMap(t *testing.T) {
	if m := asResultMap(map[string]interface{}{"a": 1}); m["a"] != 1 {
		t.Errorf("expected map passthrough, got %v", m)
	}
	arr := []interface{}{"x", "y"}
	wrapped := asResultMap(arr)
	if got, ok := wrapped["result"].([]interface{}); !ok || len(got) != 2 {
		t.Errorf("expected non-map results wrapped under 'result', got %v", wrapped)
	}
}

func TestLoadConfigMalformedFile(t *testing.T) {
	f, err := os.CreateTemp("", "mcp-cfg-bad-*.toml")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(f.Name()) }()
	if _, err := f.WriteString("this is not valid = = toml"); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	// A malformed file must not panic and must fall back to zero-value fields
	// (env vars still apply on top, exercised separately by TestLoadConfigFromEnv).
	cfg := loadConfig(f.Name())
	if cfg.BaseURL != "" || cfg.AdminToken != "" {
		t.Errorf("expected zero-value config from a malformed file, got %+v", cfg)
	}
}

func TestHandleRequestInitialize(t *testing.T) {
	resp := handleRequest(mcpConfig{}, jsonRPCRequest{JSONRPC: "2.0", ID: 1, Method: "initialize"})
	result, ok := resp.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected a result map, got %v", resp.Result)
	}
	if result["protocolVersion"] != "2024-11-05" {
		t.Errorf("expected protocolVersion 2024-11-05, got %v", result["protocolVersion"])
	}
	if resp.Error != nil {
		t.Errorf("expected no error, got %v", resp.Error)
	}
}

func TestHandleRequestToolsList(t *testing.T) {
	resp := handleRequest(mcpConfig{}, jsonRPCRequest{JSONRPC: "2.0", ID: 2, Method: "tools/list"})
	result, ok := resp.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected a result map, got %v", resp.Result)
	}
	tools, ok := result["tools"].([]tool)
	if !ok || len(tools) != 7 {
		t.Errorf("expected 7 tools, got %v", result["tools"])
	}
}

func TestHandleRequestUnknownMethod(t *testing.T) {
	resp := handleRequest(mcpConfig{}, jsonRPCRequest{JSONRPC: "2.0", ID: 3, Method: "not/a/method"})
	errMap, ok := resp.Error.(map[string]interface{})
	if !ok {
		t.Fatalf("expected an error map, got %v", resp.Error)
	}
	if errMap["code"] != -32601 {
		t.Errorf("expected JSON-RPC method-not-found code -32601, got %v", errMap["code"])
	}
}

func TestHandleRequestToolsCallSuccessAndFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	cfg := mcpConfig{BaseURL: srv.URL}

	resp := handleRequest(cfg, jsonRPCRequest{
		JSONRPC: "2.0", ID: 4, Method: "tools/call",
		Params: map[string]interface{}{"name": "health_check", "arguments": map[string]interface{}{}},
	})
	if resp.Error != nil {
		t.Fatalf("unexpected protocol error: %v", resp.Error)
	}
	result, ok := resp.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected a result map, got %v", resp.Result)
	}
	if result["isError"] != false {
		t.Errorf("expected isError=false for a successful tool call, got %v", result["isError"])
	}

	// An unknown tool name is a protocol-level error (JSON-RPC error field, not isError).
	resp2 := handleRequest(cfg, jsonRPCRequest{
		JSONRPC: "2.0", ID: 5, Method: "tools/call",
		Params: map[string]interface{}{"name": "does_not_exist"},
	})
	if resp2.Error == nil {
		t.Fatalf("expected a protocol error for an unknown tool name, got result=%v", resp2.Result)
	}
}

func TestHandleRequestToolsCallMissingArguments(t *testing.T) {
	// No "arguments" key at all — should default to an empty map, not panic.
	resp := handleRequest(mcpConfig{BaseURL: "http://unused.invalid", AdminToken: "t"}, jsonRPCRequest{
		JSONRPC: "2.0", ID: 6, Method: "tools/call",
		Params: map[string]interface{}{"name": "delete_dns_record"},
	})
	result, ok := resp.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected a result map, got %v", resp.Result)
	}
	if result["isError"] != true {
		t.Errorf("expected isError=true for delete_dns_record without an id, got %v", result["isError"])
	}
}

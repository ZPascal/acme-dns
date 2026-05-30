# AI Agent Support Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an OpenAPI 3.1 spec (served at `GET /openapi.json`) and a standalone MCP server binary (`cmd/acme-dns-mcp/`) that exposes acme-dns as structured tools for AI agents.

**Architecture:** The OpenAPI spec is a static `openapi.json` embedded via `go:embed` and served by the existing HTTP server. The MCP binary is a separate Go entrypoint in `cmd/acme-dns-mcp/` that reads config from a TOML file or environment variables, proxies calls to the acme-dns HTTP API, and communicates with AI agents via stdio using the MCP protocol.

**Dependency:** This branch builds on `feature/dns-api-extended`. The admin-related MCP tools and OpenAPI paths assume the admin API is present. Either merge that branch first, or create this branch from `feature/dns-api-extended`.

**Tech Stack:** Go 1.26, `go:embed`, `encoding/json`, `net/http`, `os`, `github.com/BurntSushi/toml` (already in `go.mod`), MCP stdio protocol (JSON-RPC 2.0 over stdin/stdout — no external MCP SDK needed, implement protocol directly)

---

## File Map

| File | Action | Responsibility |
|------|--------|----------------|
| `openapi.json` | Create | Static OpenAPI 3.1 spec for all endpoints |
| `api.go` | Modify | Add `GET /openapi.json` handler with `go:embed` |
| `main.go` | Modify | Register `GET /openapi.json` route |
| `cmd/acme-dns-mcp/main.go` | Create | MCP binary entry point — stdio loop |
| `cmd/acme-dns-mcp/config.go` | Create | Config loading from TOML file + env vars |
| `cmd/acme-dns-mcp/tools.go` | Create | Tool definitions, JSON schemas, HTTP proxy calls |
| `cmd/acme-dns-mcp/mcp_test.go` | Create | Unit tests for tool dispatch and config loading |
| `.goreleaser.yml` | Modify | Add second binary build for `acme-dns-mcp` |

---

## Task 1: Write the OpenAPI 3.1 spec

**Files:**
- Create: `openapi.json`

- [ ] **Step 1: Create `openapi.json`**

Create `openapi.json` in the repo root:

```json
{
  "openapi": "3.1.0",
  "info": {
    "title": "acme-dns",
    "version": "1.0.0",
    "description": "Simplified DNS server with HTTP API for ACME DNS challenge automation and general DNS record management."
  },
  "components": {
    "securitySchemes": {
      "ApiUser": {
        "type": "apiKey",
        "in": "header",
        "name": "X-Api-User",
        "description": "UUIDv4 username obtained from /register"
      },
      "ApiKey": {
        "type": "apiKey",
        "in": "header",
        "name": "X-Api-Key",
        "description": "40-character base64url password obtained from /register"
      },
      "AdminBearer": {
        "type": "http",
        "scheme": "bearer",
        "description": "Admin token configured in [api.admin].token"
      }
    },
    "schemas": {
      "Registration": {
        "type": "object",
        "properties": {
          "username": { "type": "string", "format": "uuid" },
          "password": { "type": "string" },
          "fulldomain": { "type": "string" },
          "subdomain": { "type": "string" },
          "allowfrom": { "type": "array", "items": { "type": "string" } }
        }
      },
      "TxtUpdate": {
        "type": "object",
        "required": ["subdomain", "txt"],
        "properties": {
          "subdomain": { "type": "string", "description": "UUID subdomain from registration" },
          "txt": { "type": "string", "minLength": 43, "maxLength": 43, "description": "ACME challenge token (exactly 43 characters)" }
        }
      },
      "DNSRecord": {
        "type": "object",
        "required": ["name", "type", "value"],
        "properties": {
          "id": { "type": "string", "format": "uuid" },
          "name": { "type": "string", "description": "Fully qualified domain name, e.g. sub.example.com" },
          "type": { "type": "string", "enum": ["A", "AAAA", "CNAME", "MX", "TXT", "NS", "SRV", "CAA", "PTR"] },
          "value": { "type": "string" },
          "ttl": { "type": "integer", "minimum": 1, "maximum": 86400, "default": 300 },
          "created": { "type": "integer", "description": "Unix timestamp" }
        }
      },
      "Error": {
        "type": "object",
        "properties": {
          "error": { "type": "string" }
        }
      }
    }
  },
  "paths": {
    "/register": {
      "post": {
        "summary": "Register a new subdomain",
        "description": "Creates a new user account with a UUID subdomain for ACME DNS challenge delegation. Optionally restrict access to specific IP CIDRs.",
        "requestBody": {
          "required": false,
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "properties": {
                  "allowfrom": {
                    "type": "array",
                    "items": { "type": "string" },
                    "description": "Optional list of CIDR ranges allowed to use this account, e.g. [\"192.168.1.0/24\"]"
                  }
                }
              }
            }
          }
        },
        "responses": {
          "201": {
            "description": "Account created",
            "content": { "application/json": { "schema": { "$ref": "#/components/schemas/Registration" } } }
          },
          "400": { "description": "Invalid CIDR or malformed JSON", "content": { "application/json": { "schema": { "$ref": "#/components/schemas/Error" } } } }
        }
      }
    },
    "/update": {
      "post": {
        "summary": "Update ACME TXT record",
        "description": "Updates the TXT record for your subdomain with a new ACME challenge token. Requires X-Api-User and X-Api-Key headers from registration.",
        "security": [{ "ApiUser": [], "ApiKey": [] }],
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": { "$ref": "#/components/schemas/TxtUpdate" }
            }
          }
        },
        "responses": {
          "200": { "description": "TXT record updated", "content": { "application/json": { "schema": { "type": "object", "properties": { "txt": { "type": "string" } } } } } },
          "400": { "description": "Invalid subdomain or TXT value" },
          "401": { "description": "Invalid credentials or IP not allowed" }
        }
      }
    },
    "/health": {
      "get": {
        "summary": "Health check",
        "description": "Returns 200 OK when the server is ready. Use for liveness and readiness probes.",
        "responses": {
          "200": { "description": "Server is healthy" }
        }
      }
    },
    "/admin/records": {
      "get": {
        "summary": "List DNS records",
        "description": "Returns all managed DNS records. Filter by type and/or name using query parameters.",
        "security": [{ "AdminBearer": [] }],
        "parameters": [
          { "name": "type", "in": "query", "schema": { "type": "string" }, "description": "Filter by record type, e.g. A" },
          { "name": "name", "in": "query", "schema": { "type": "string" }, "description": "Filter by exact domain name" }
        ],
        "responses": {
          "200": { "description": "List of records", "content": { "application/json": { "schema": { "type": "array", "items": { "$ref": "#/components/schemas/DNSRecord" } } } } },
          "401": { "description": "Invalid or missing admin token" }
        }
      },
      "post": {
        "summary": "Create DNS record",
        "description": "Creates a new managed DNS record. Supported types: A, AAAA, CNAME, MX, TXT, NS, SRV, CAA, PTR.",
        "security": [{ "AdminBearer": [] }],
        "requestBody": {
          "required": true,
          "content": { "application/json": { "schema": { "$ref": "#/components/schemas/DNSRecord" } } }
        },
        "responses": {
          "201": { "description": "Record created", "content": { "application/json": { "schema": { "$ref": "#/components/schemas/DNSRecord" } } } },
          "400": { "description": "Invalid record type, value, or TTL" },
          "401": { "description": "Invalid or missing admin token" }
        }
      }
    },
    "/admin/records/{id}": {
      "put": {
        "summary": "Update DNS record",
        "description": "Updates an existing managed DNS record by ID.",
        "security": [{ "AdminBearer": [] }],
        "parameters": [
          { "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }
        ],
        "requestBody": {
          "required": true,
          "content": { "application/json": { "schema": { "$ref": "#/components/schemas/DNSRecord" } } }
        },
        "responses": {
          "200": { "description": "Record updated", "content": { "application/json": { "schema": { "$ref": "#/components/schemas/DNSRecord" } } } },
          "400": { "description": "Invalid record data" },
          "401": { "description": "Invalid or missing admin token" }
        }
      },
      "delete": {
        "summary": "Delete DNS record",
        "description": "Deletes a managed DNS record by ID.",
        "security": [{ "AdminBearer": [] }],
        "parameters": [
          { "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }
        ],
        "responses": {
          "204": { "description": "Record deleted" },
          "401": { "description": "Invalid or missing admin token" }
        }
      }
    }
  }
}
```

- [ ] **Step 2: Validate the JSON is well-formed**

```bash
python3 -c "import json,sys; json.load(open('openapi.json')); print('valid JSON')" 2>&1
```
Expected: `valid JSON`

- [ ] **Step 3: Commit**

```bash
git add openapi.json
git commit -m "feat: add OpenAPI 3.1 spec for all acme-dns endpoints"
```

---

## Task 2: Serve `openapi.json` from the HTTP API

**Files:**
- Modify: `api.go`
- Modify: `main.go`

- [ ] **Step 1: Write failing test**

Add to `api_test.go`:

```go
func TestOpenAPIEndpoint(t *testing.T) {
	e := setupTestDB(t)
	resp := e.GET("/openapi.json").Expect()
	resp.Status(http.StatusOK)
	resp.Header("Content-Type").Contains("application/json")
	resp.JSON().Object().ContainsKey("openapi")
}
```

- [ ] **Step 2: Run to verify it fails**

```bash
go test ./... -run "TestOpenAPIEndpoint" 2>&1
```
Expected: FAIL — route not registered.

- [ ] **Step 3: Add `go:embed` and handler to `api.go`**

Add at the top of `api.go` after the `package main` line:

```go
import _ "embed"

//go:embed openapi.json
var openapiSpec []byte
```

Add the handler function:

```go
func serveOpenAPI(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(openapiSpec)
}
```

- [ ] **Step 4: Register the route in `main.go`**

In `startHTTPAPI()`, add after `api.GET("/health", healthCheck)`:

```go
api.GET("/openapi.json", serveOpenAPI)
```

- [ ] **Step 5: Run test to verify it passes**

```bash
go test ./... -run "TestOpenAPIEndpoint" 2>&1
```
Expected: PASS.

- [ ] **Step 6: Run full test suite**

```bash
go test ./... 2>&1
```
Expected: all tests pass.

- [ ] **Step 7: Commit**

```bash
git add api.go main.go api_test.go
git commit -m "feat: serve OpenAPI 3.1 spec at GET /openapi.json"
```

---

## Task 3: Create MCP binary — config loading

**Files:**
- Create: `cmd/acme-dns-mcp/config.go`
- Create: `cmd/acme-dns-mcp/mcp_test.go`

The MCP config is separate from the main acme-dns config. It tells the MCP binary where the acme-dns server is and what credentials to use.

- [ ] **Step 1: Write failing test for config loading**

Create `cmd/acme-dns-mcp/mcp_test.go`:

```go
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
```

- [ ] **Step 2: Run to verify it fails**

```bash
cd cmd/acme-dns-mcp && go test ./... 2>&1; cd ../..
```
Expected: compile error — package not created yet.

- [ ] **Step 3: Create `cmd/acme-dns-mcp/config.go`**

```bash
mkdir -p cmd/acme-dns-mcp
```

Create `cmd/acme-dns-mcp/config.go`:

```go
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
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd cmd/acme-dns-mcp && go test ./... -run "TestLoadConfig" 2>&1; cd ../..
```
Expected: both tests PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/acme-dns-mcp/
git commit -m "feat: add MCP server config loading (file + env vars)"
```

---

## Task 4: Create MCP binary — tool definitions and HTTP proxy

**Files:**
- Create: `cmd/acme-dns-mcp/tools.go`

The MCP protocol uses JSON-RPC 2.0 over stdio. Three methods matter:
- `initialize` — handshake, return server info
- `tools/list` — return all available tools with schemas
- `tools/call` — execute a tool by name

- [ ] **Step 1: Write failing test for tool dispatch**

Add to `cmd/acme-dns-mcp/mcp_test.go`:

```go
import (
	"net/http"
	"net/http/httptest"
)

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
```

- [ ] **Step 2: Run to verify it fails**

```bash
cd cmd/acme-dns-mcp && go test ./... 2>&1; cd ../..
```
Expected: compile error — `callTool`, `listTools` not defined.

- [ ] **Step 3: Create `cmd/acme-dns-mcp/tools.go`**

```go
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// tool describes one MCP tool
type tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

func listTools() []tool {
	return []tool{
		{
			Name:        "health_check",
			Description: "Check if the acme-dns server is reachable and healthy.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
		},
		{
			Name:        "register_subdomain",
			Description: "Register a new subdomain and receive credentials (username, password, subdomain) for ACME DNS challenge delegation.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"allowfrom":{"type":"array","items":{"type":"string"},"description":"Optional CIDR ranges allowed to use this account"}}}`),
		},
		{
			Name:        "update_txt_record",
			Description: "Update the ACME TXT challenge record for a registered subdomain. Requires username and password from registration.",
			InputSchema: json.RawMessage(`{"type":"object","required":["subdomain","txt"],"properties":{"subdomain":{"type":"string","description":"UUID subdomain from registration"},"txt":{"type":"string","description":"Exactly 43-character ACME challenge token"}}}`),
		},
		{
			Name:        "list_dns_records",
			Description: "List all managed DNS records. Optionally filter by type (e.g. A) or name.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"type":{"type":"string"},"name":{"type":"string"}}}`),
		},
		{
			Name:        "create_dns_record",
			Description: "Create a new managed DNS record. Supported types: A, AAAA, CNAME, MX, TXT, NS, SRV, CAA, PTR.",
			InputSchema: json.RawMessage(`{"type":"object","required":["name","type","value"],"properties":{"name":{"type":"string"},"type":{"type":"string"},"value":{"type":"string"},"ttl":{"type":"integer","default":300}}}`),
		},
		{
			Name:        "update_dns_record",
			Description: "Update an existing managed DNS record by ID.",
			InputSchema: json.RawMessage(`{"type":"object","required":["id","name","type","value"],"properties":{"id":{"type":"string"},"name":{"type":"string"},"type":{"type":"string"},"value":{"type":"string"},"ttl":{"type":"integer"}}}`),
		},
		{
			Name:        "delete_dns_record",
			Description: "Delete a managed DNS record by ID.",
			InputSchema: json.RawMessage(`{"type":"object","required":["id"],"properties":{"id":{"type":"string"}}}`),
		},
	}
}

// callTool executes a tool by name and returns the result as a map.
func callTool(cfg mcpConfig, name string, args map[string]interface{}) (map[string]interface{}, error) {
	switch name {
	case "health_check":
		return toolHealthCheck(cfg)
	case "register_subdomain":
		return toolRegister(cfg, args)
	case "update_txt_record":
		return toolUpdateTXT(cfg, args)
	case "list_dns_records":
		return toolListRecords(cfg, args)
	case "create_dns_record":
		return toolCreateRecord(cfg, args)
	case "update_dns_record":
		return toolUpdateRecord(cfg, args)
	case "delete_dns_record":
		return toolDeleteRecord(cfg, args)
	}
	return nil, fmt.Errorf("unknown tool: %s", name)
}

func doRequest(cfg mcpConfig, method, path string, body interface{}, headers map[string]string) (map[string]interface{}, int, error) {
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, 0, err
		}
		bodyReader = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, cfg.BaseURL+path, bodyReader)
	if err != nil {
		return nil, 0, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	var result map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&result)
	if result == nil {
		result = map[string]interface{}{}
	}
	return result, resp.StatusCode, nil
}

func toolHealthCheck(cfg mcpConfig) (map[string]interface{}, error) {
	_, status, err := doRequest(cfg, "GET", "/health", nil, nil)
	if err != nil {
		return nil, err
	}
	if status == http.StatusOK {
		return map[string]interface{}{"status": "ok"}, nil
	}
	return map[string]interface{}{"status": "unhealthy", "code": status}, nil
}

func toolRegister(cfg mcpConfig, args map[string]interface{}) (map[string]interface{}, error) {
	result, _, err := doRequest(cfg, "POST", "/register", args, nil)
	return result, err
}

func toolUpdateTXT(cfg mcpConfig, args map[string]interface{}) (map[string]interface{}, error) {
	if cfg.Username == "" || cfg.Password == "" {
		return map[string]interface{}{"error": "username and password not configured"}, nil
	}
	headers := map[string]string{
		"X-Api-User": cfg.Username,
		"X-Api-Key":  cfg.Password,
	}
	result, _, err := doRequest(cfg, "POST", "/update", args, headers)
	return result, err
}

func toolListRecords(cfg mcpConfig, args map[string]interface{}) (map[string]interface{}, error) {
	if cfg.AdminToken == "" {
		return map[string]interface{}{"error": "admin_token not configured"}, nil
	}
	path := "/admin/records"
	var parts []string
	if t, ok := args["type"].(string); ok && t != "" {
		parts = append(parts, "type="+t)
	}
	if n, ok := args["name"].(string); ok && n != "" {
		parts = append(parts, "name="+n)
	}
	if len(parts) > 0 {
		path += "?" + strings.Join(parts, "&")
	}
	headers := map[string]string{"Authorization": "Bearer " + cfg.AdminToken}
	result, _, err := doRequest(cfg, "GET", path, nil, headers)
	return result, err
}

func toolCreateRecord(cfg mcpConfig, args map[string]interface{}) (map[string]interface{}, error) {
	if cfg.AdminToken == "" {
		return map[string]interface{}{"error": "admin_token not configured"}, nil
	}
	headers := map[string]string{"Authorization": "Bearer " + cfg.AdminToken}
	result, _, err := doRequest(cfg, "POST", "/admin/records", args, headers)
	return result, err
}

func toolUpdateRecord(cfg mcpConfig, args map[string]interface{}) (map[string]interface{}, error) {
	if cfg.AdminToken == "" {
		return map[string]interface{}{"error": "admin_token not configured"}, nil
	}
	id, _ := args["id"].(string)
	if id == "" {
		return map[string]interface{}{"error": "id is required"}, nil
	}
	headers := map[string]string{"Authorization": "Bearer " + cfg.AdminToken}
	result, _, err := doRequest(cfg, "PUT", "/admin/records/"+id, args, headers)
	return result, err
}

func toolDeleteRecord(cfg mcpConfig, args map[string]interface{}) (map[string]interface{}, error) {
	if cfg.AdminToken == "" {
		return map[string]interface{}{"error": "admin_token not configured"}, nil
	}
	id, _ := args["id"].(string)
	if id == "" {
		return map[string]interface{}{"error": "id is required"}, nil
	}
	headers := map[string]string{"Authorization": "Bearer " + cfg.AdminToken}
	_, status, err := doRequest(cfg, "DELETE", "/admin/records/"+id, nil, headers)
	if err != nil {
		return nil, err
	}
	if status == http.StatusNoContent {
		return map[string]interface{}{"status": "deleted"}, nil
	}
	return map[string]interface{}{"status": "error", "code": status}, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd cmd/acme-dns-mcp && go test ./... 2>&1; cd ../..
```
Expected: `TestToolHealthCheck` and `TestToolListTools` PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/acme-dns-mcp/tools.go cmd/acme-dns-mcp/mcp_test.go
git commit -m "feat: add MCP tool definitions and HTTP proxy logic"
```

---

## Task 5: Create MCP binary — stdio entry point

**Files:**
- Create: `cmd/acme-dns-mcp/main.go`

The MCP protocol over stdio sends and receives newline-delimited JSON-RPC 2.0 messages. Each message has `jsonrpc`, `id`, `method`, and optionally `params`.

- [ ] **Step 1: Create `cmd/acme-dns-mcp/main.go`**

```go
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type jsonRPCRequest struct {
	JSONRPC string                 `json:"jsonrpc"`
	ID      interface{}            `json:"id"`
	Method  string                 `json:"method"`
	Params  map[string]interface{} `json:"params"`
}

type jsonRPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   interface{} `json:"error,omitempty"`
}

func main() {
	cfgPath := filepath.Join(os.Getenv("HOME"), ".acme-dns-mcp", "config.toml")
	if v := os.Getenv("ACMEDNS_MCP_CONFIG"); v != "" {
		cfgPath = v
	}
	cfg := loadConfig(cfgPath)

	scanner := bufio.NewScanner(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)

	for scanner.Scan() {
		line := scanner.Bytes()
		var req jsonRPCRequest
		if err := json.Unmarshal(line, &req); err != nil {
			continue
		}

		var resp jsonRPCResponse
		resp.JSONRPC = "2.0"
		resp.ID = req.ID

		switch req.Method {
		case "initialize":
			resp.Result = map[string]interface{}{
				"protocolVersion": "2024-11-05",
				"capabilities":    map[string]interface{}{"tools": map[string]interface{}{}},
				"serverInfo":      map[string]interface{}{"name": "acme-dns-mcp", "version": "1.0.0"},
			}
		case "tools/list":
			resp.Result = map[string]interface{}{"tools": listTools()}
		case "tools/call":
			toolName, _ := req.Params["name"].(string)
			args, _ := req.Params["arguments"].(map[string]interface{})
			if args == nil {
				args = map[string]interface{}{}
			}
			result, err := callTool(cfg, toolName, args)
			if err != nil {
				resp.Error = map[string]interface{}{"code": -32000, "message": err.Error()}
			} else {
				resultJSON, _ := json.Marshal(result)
				resp.Result = map[string]interface{}{
					"content": []map[string]interface{}{
						{"type": "text", "text": string(resultJSON)},
					},
				}
			}
		default:
			resp.Error = map[string]interface{}{"code": -32601, "message": fmt.Sprintf("method not found: %s", req.Method)}
		}

		_ = encoder.Encode(resp)
	}
}
```

- [ ] **Step 2: Build the binary to verify it compiles**

```bash
go build ./cmd/acme-dns-mcp/... 2>&1
```
Expected: builds with no errors, creates `acme-dns-mcp` binary.

- [ ] **Step 3: Smoke-test the stdio protocol**

```bash
echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}' | ./acme-dns-mcp 2>&1
```
Expected: JSON response containing `"protocolVersion":"2024-11-05"`.

```bash
echo '{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}' | ./acme-dns-mcp 2>&1
```
Expected: JSON response with array of 7 tools.

- [ ] **Step 4: Clean up binary**

```bash
rm -f acme-dns-mcp
```

- [ ] **Step 5: Commit**

```bash
git add cmd/acme-dns-mcp/main.go
git commit -m "feat: add MCP binary stdio entry point with JSON-RPC 2.0 protocol"
```

---

## Task 6: Update `.goreleaser.yml` to build MCP binary

**Files:**
- Modify: `.goreleaser.yml`

- [ ] **Step 1: Add second build entry**

Replace the `builds:` section in `.goreleaser.yml` with:

```yaml
builds:
  - id: acme-dns
    env:
      - CGO_ENABLED=0
    goos:
      - linux
      - darwin
  - id: acme-dns-mcp
    main: ./cmd/acme-dns-mcp
    binary: acme-dns-mcp
    env:
      - CGO_ENABLED=0
    goos:
      - linux
      - darwin
```

- [ ] **Step 2: Run full test suite**

```bash
go test ./... 2>&1
```
Expected: all tests pass.

- [ ] **Step 3: Build both binaries to verify**

```bash
go build ./... 2>&1
```
Expected: builds with no errors.

- [ ] **Step 4: Clean up**

```bash
rm -f acme-dns acme-dns-mcp
```

- [ ] **Step 5: Commit**

```bash
git add .goreleaser.yml
git commit -m "chore: add acme-dns-mcp to goreleaser build pipeline"
```

---

## Task 7: Final integration check

- [ ] **Step 1: Run full test suite**

```bash
go test ./... -v 2>&1 | tail -30
```
Expected: all tests pass.

- [ ] **Step 2: Vet all packages**

```bash
go vet ./... 2>&1
```
Expected: no issues.

- [ ] **Step 3: Verify module is tidy**

```bash
go mod tidy && git diff go.mod go.sum
```
Expected: no unexpected changes (BurntSushi/toml already in `go.mod`).

- [ ] **Step 4: Final commit if needed**

```bash
git status
```
If any files are unstaged, commit them:
```bash
git add -A
git commit -m "chore: finalize ai-agent-support feature"
```

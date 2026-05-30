# Design: feature/ai-agent-support

**Date:** 2026-05-30  
**Branch:** `feature/ai-agent-support`  
**Status:** Approved

## Overview

Add two AI agent integration surfaces to acme-dns:

1. **OpenAPI 3.1 spec** — served at `GET /openapi.json`, enables any OpenAPI-aware agent or tool to discover and call the API via function calling
2. **MCP server binary** — `cmd/acme-dns-mcp/`, a standalone stdio-based MCP server that wraps the acme-dns HTTP API as structured tools for Claude and other MCP-capable agents

The core acme-dns server is minimally changed. The MCP server is a separate binary that proxies to the HTTP API — it has no direct DB or DNS access.

**Dependency:** This branch builds on `feature/dns-api-extended`. The OpenAPI spec and MCP tools for admin record management (`list_dns_records`, `create_dns_record`, `update_dns_record`, `delete_dns_record`) require the admin API from that branch to be merged first, or this branch should be built on top of it.

## OpenAPI 3.1 Spec

A static `openapi.json` file is authored once and embedded into the binary via `go:embed`. It documents:

- All existing endpoints: `POST /register`, `POST /update`, `GET /health`
- All new admin endpoints from `feature/dns-api-extended`: `GET/POST /admin/records`, `PUT/DELETE /admin/records/{id}`

The spec is served at `GET /openapi.json` with no authentication required (it describes the API, not data). CORS headers allow cross-origin access so browser-based agents and Swagger UI can consume it.

### Spec Structure

```json
{
  "openapi": "3.1.0",
  "info": { "title": "acme-dns", "version": "1.0.0" },
  "components": {
    "securitySchemes": {
      "ApiUser": { "type": "apiKey", "in": "header", "name": "X-Api-User" },
      "ApiKey":  { "type": "apiKey", "in": "header", "name": "X-Api-Key" },
      "AdminBearer": { "type": "http", "scheme": "bearer" }
    }
  },
  "paths": { ... }
}
```

Each operation includes a `description` field written for LLM consumption — concise, action-oriented, with parameter semantics explained.

## MCP Server Binary

### Location

```
cmd/acme-dns-mcp/
  main.go       # entry point, stdio transport setup
  tools.go      # tool definitions, JSON schemas, HTTP proxy logic
  config.go     # config loading from file and env vars
```

### Transport

stdio (standard input/output), the canonical MCP transport. Compatible with Claude Desktop, Claude Code (`/mcp add`), and any MCP-capable agent. No network port is opened by the MCP server itself.

### Configuration

The MCP binary reads from a config file (default: `~/.acme-dns-mcp/config.toml`) or environment variables:

```toml
base_url    = "https://acmedns.example.com"
admin_token = ""           # for admin record management tools
username    = ""           # for update_txt_record tool
password    = ""           # for update_txt_record tool
```

Environment variable equivalents: `ACMEDNS_BASE_URL`, `ACMEDNS_ADMIN_TOKEN`, `ACMEDNS_USERNAME`, `ACMEDNS_PASSWORD`.

Credentials are never passed as MCP tool arguments — they are configuration, not runtime inputs.

### MCP Tools

| Tool name | HTTP call | Auth used | Description |
|-----------|-----------|-----------|-------------|
| `register_subdomain` | `POST /register` | none | Register a new subdomain and get credentials |
| `update_txt_record` | `POST /update` | username + password | Update the ACME TXT record for a subdomain |
| `list_dns_records` | `GET /admin/records` | admin token | List all managed DNS records |
| `create_dns_record` | `POST /admin/records` | admin token | Create a new DNS record |
| `update_dns_record` | `PUT /admin/records/{id}` | admin token | Update an existing DNS record |
| `delete_dns_record` | `DELETE /admin/records/{id}` | admin token | Delete a DNS record |
| `health_check` | `GET /health` | none | Check if acme-dns is reachable |

Each tool definition includes:
- `name` and `description` (LLM-facing, action-oriented)
- `inputSchema` (JSON Schema for arguments)
- HTTP method, path, and header injection logic

Tools requiring missing credentials (e.g. `admin_token` not configured) return a structured error: `{"error": "admin_token not configured"}`.

### Security Properties

- No credentials in MCP tool arguments or responses
- Admin token and user credentials only flow from config file / env vars into HTTP request headers
- stdio transport means no network attack surface from the MCP layer
- HTTP calls to acme-dns use the same TLS as any other client — no special trust

## Release Pipeline

`.goreleaser.yml` is updated to build and publish `acme-dns-mcp` as a second binary alongside `acme-dns` in each release.

## Files Added/Changed

| File | Change |
|------|--------|
| `openapi.json` | New static OpenAPI 3.1 spec |
| `api.go` | Add `GET /openapi.json` handler with `go:embed` |
| `cmd/acme-dns-mcp/main.go` | MCP binary entry point, stdio transport |
| `cmd/acme-dns-mcp/tools.go` | Tool definitions, JSON schemas, HTTP proxy |
| `cmd/acme-dns-mcp/config.go` | Config loading (file + env vars) |
| `.goreleaser.yml` | Add second binary to release builds |

## Testing

- Unit tests for each MCP tool's input validation and HTTP request construction (mock HTTP server)
- Integration test: start a real acme-dns instance, run MCP tool calls against it, verify responses
- Test that missing credentials produce correct structured errors
- Verify `GET /openapi.json` returns valid OpenAPI 3.1 (schema validation)

## Non-Goals

- WebSocket or SSE MCP transport (stdio covers all current MCP clients)
- Natural language parsing inside the MCP server (tools take structured inputs only)
- MCP server embedded in the main acme-dns binary
- Authentication of MCP clients (stdio transport is inherently local)

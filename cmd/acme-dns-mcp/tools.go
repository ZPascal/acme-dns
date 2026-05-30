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

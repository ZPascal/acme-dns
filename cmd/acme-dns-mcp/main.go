package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// maxRPCLineSize bounds a single JSON-RPC request line. bufio.Scanner's default
// (bufio.MaxScanTokenSize, 64KB) is too small for tool calls carrying larger
// payloads (e.g. batches of DNS records) and fails silently when exceeded.
const maxRPCLineSize = 10 * 1024 * 1024

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

// handleRequest processes one decoded JSON-RPC request and returns the response to encode.
func handleRequest(cfg mcpConfig, req jsonRPCRequest) jsonRPCResponse {
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
		result, isError, err := callTool(cfg, toolName, args)
		if err != nil {
			resp.Error = map[string]interface{}{"code": -32000, "message": err.Error()}
		} else {
			resultJSON, _ := json.Marshal(result)
			resp.Result = map[string]interface{}{
				"content": []map[string]interface{}{
					{"type": "text", "text": string(resultJSON)},
				},
				"isError": isError,
			}
		}
	default:
		resp.Error = map[string]interface{}{"code": -32601, "message": fmt.Sprintf("method not found: %s", req.Method)}
	}

	return resp
}

func main() {
	cfgPath := filepath.Join(os.Getenv("HOME"), ".acme-dns-mcp", "config.toml")
	if v := os.Getenv("ACMEDNS_MCP_CONFIG"); v != "" {
		cfgPath = v
	}
	cfg := loadConfig(cfgPath)

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 64*1024), maxRPCLineSize)
	encoder := json.NewEncoder(os.Stdout)

	for scanner.Scan() {
		line := scanner.Bytes()
		var req jsonRPCRequest
		if err := json.Unmarshal(line, &req); err != nil {
			continue
		}
		_ = encoder.Encode(handleRequest(cfg, req))
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "acme-dns-mcp: stdin read error: %v\n", err)
		os.Exit(1)
	}
}

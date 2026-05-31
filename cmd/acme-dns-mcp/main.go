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

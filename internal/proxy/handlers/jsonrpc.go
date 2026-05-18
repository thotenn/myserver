package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/thotenn/myserver/internal/config"
	"github.com/thotenn/myserver/internal/proxy"
)

// JSONRPCRequest represents a JSON-RPC 2.0 request.
type JSONRPCRequest struct {
	JSONRPC string        `json:"jsonrpc"`
	Method  string        `json:"method"`
	Params  []interface{} `json:"params"`
	ID      int           `json:"id"`
}

// JSONRPCResponse represents a JSON-RPC 2.0 response.
type JSONRPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	Result  interface{} `json:"result,omitempty"`
	Error   *RPCError   `json:"error,omitempty"`
	ID      int         `json:"id"`
}

// RPCError represents a JSON-RPC error.
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// JSONRPCProxyHandler handles widgets with JSON-RPC APIs (e.g., Deluge, Transmission).
func JSONRPCProxyHandler(ctx context.Context, widget *config.WidgetConfig, method string, params []interface{}) (interface{}, error) {
	rpcReq := JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
		ID:      1,
	}

	bodyBytes, err := json.Marshal(rpcReq)
	if err != nil {
		return nil, fmt.Errorf("marshaling JSON-RPC request: %w", err)
	}

	rpcHeaders := map[string]string{
		"Content-Type": "application/json",
	}

	// Add password/token if configured
	if widget.Password != "" {
		rpcHeaders["Authorization"] = "Basic " + encodeBasicAuth("", widget.Password)
	}

	result, err := proxy.Proxy(ctx, widget.URL, &proxy.Params{
		Method:  http.MethodPost,
		Headers: rpcHeaders,
		Body:    strings.NewReader(string(bodyBytes)),
	})
	if err != nil {
		return nil, err
	}

	if result.Status != http.StatusOK {
		return nil, fmt.Errorf("JSON-RPC returned status %d", result.Status)
	}

	var rpcResp JSONRPCResponse
	if err := json.Unmarshal(result.Body, &rpcResp); err != nil {
		return nil, fmt.Errorf("parsing JSON-RPC response: %w", err)
	}

	if rpcResp.Error != nil {
		return nil, fmt.Errorf("JSON-RPC error %d: %s", rpcResp.Error.Code, rpcResp.Error.Message)
	}

	return rpcResp.Result, nil
}

func encodeBasicAuth(username, password string) string {
	auth := username + ":" + password
	return base64.StdEncoding.EncodeToString([]byte(auth))
}

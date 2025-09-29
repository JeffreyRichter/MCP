package main

import (
	"fmt"
	"net"
	"net/http"
	"testing"

	"github.com/JeffreyRichter/internal/aids"
	"github.com/JeffreyRichter/mcpsvr/mcp"
	"github.com/JeffreyRichter/mcpsvr/mcp/toolcall"
)

var (
	_serverPortAndKey = McpServerPortAndKey{Port: "8080", Key: "0123456789abcdef0123456789abcdef"}
	client            = NewMCPClient("http://"+net.JoinHostPort("localhost", _serverPortAndKey.Port)+"/mcp", _serverPortAndKey.Key)
	tcp               = NewAppToolCallProcessor(false)
)

func TestToolsList(t *testing.T) {
	resp := aids.Must(client.Do("GET", "/tools", http.Header{"Accept": []string{"application/json"}}, nil))
	fmt.Println("Tools:")
	for _, tool := range unmarshalBody[mcp.ListToolsResult](resp.Body).Tools {
		fmt.Printf("  %10s: %s\n", tool.Name, *tool.Description)
	}
}

func TestToolCallEphemeral(t *testing.T) {
	tc := client.runToolCall("add", "ID-1", tcp, true, map[string]any{"x": 1, "y": 2})
	_ = tc
}

func TestToolCallServerProcessing(t *testing.T) {
	tc := client.runToolCall("count", "ID-2", tcp, true, map[string]any{"countto": 5})
	_ = tc
}

func TestToolCallServerProcessingAfterCrash(t *testing.T) {
	tc := client.runToolCall("count", "ID-1", tcp, false, nil)
	_ = tc
}

func TestToolCallServerProcessingCancel(t *testing.T) {
	resp := aids.Must(client.Do("POST", fmt.Sprintf("/tools/%v/calls/%v/cancel", "count", "ID-1"), http.Header{"Accept": []string{"application/json"}}, nil))
	tc := unmarshalBody[toolcall.ToolCallClient](resp.Body)
	fmt.Printf("Cancel response: %v\n", tc)
}

func TestToolCallElicitation(t *testing.T) {
	tc := client.runToolCall("pii", "ID-1", tcp, true, map[string]any{"key": "my secret key"})
	_ = tc
}

func TestToolCallStreaming(t *testing.T) {
	// Steaming output is the DEBUG CONSOLE
	tcp := NewAppToolCallProcessor(true)
	tc := client.runToolCall("stream", "ID-1", tcp, true, nil)
	_ = tc
}

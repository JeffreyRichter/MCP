package main

import (
	"fmt"
	"net"
	"net/http"
	"testing"

	"github.com/JeffreyRichter/internal/aids"
	"github.com/JeffreyRichter/mcp"
)

func init() {
	serverPortAndKey := McpServerPortAndKey{Port: "8080", Key: "ForDebuggingOnly"} // Default to debug the server
	serverPortAndKey, _ = SpawnMCPServer("../mcpsvr/mcpsvr.exe")                   // Comment out to debug the server
	client = NewMCPClient("http://"+net.JoinHostPort("localhost", serverPortAndKey.Port)+"/mcp", serverPortAndKey.Key)
}

var client *mcpClient

func TestToolsList(t *testing.T) {
	response := aids.Must(client.Do("GET", "/tools", http.Header{"Accept": []string{"application/json"}}, nil))
	fmt.Println("Tools:")
	for _, tool := range unmarshalBody[mcp.ListToolsResult](response.Body).Tools {
		fmt.Printf("  %10s: %s\n", tool.Name, *tool.Description)
	}
}

func TestToolCallEphemeral(t *testing.T) {
	tcp := NewAppToolCallProcessor(false)
	tc := client.runToolCall("add", "ID-1", tcp, true, map[string]any{"x": 1, "y": 2})
	_ = tc
}

func TestToolCallServerProcessing(t *testing.T) {
	tcp := NewAppToolCallProcessor(false)
	tc := client.runToolCall("count", "ID-1", tcp, true, map[string]any{"countto": 5})
	_ = tc
}

func TestToolCallServerProcessingAfterCrash(t *testing.T) {
	tcp := NewAppToolCallProcessor(false)
	tc := client.runToolCall("count", "ID-1", tcp, false, nil)
	_ = tc
}

func TestToolCallServerProcessingCancel(t *testing.T) {
	response := aids.Must(client.Do("POST", fmt.Sprintf("/tools/%v/calls/%v/cancel", "count", "ID-1"), http.Header{"Accept": []string{"application/json"}}, nil))
	tc := unmarshalBody[mcp.ToolCall](response.Body)
	fmt.Printf("Cancel response: %v\n", tc)
}

func TestToolCallElicitation(t *testing.T) {
	tcp := NewAppToolCallProcessor(false)
	tc := client.runToolCall("welcome", "ID-1", tcp, true, nil)
	_ = tc
}

func TestToolCallStreaming(t *testing.T) {
	// Steaming output is the DEBUG CONSOLE
	tcp := NewAppToolCallProcessor(true)
	tc := client.runToolCall("stream", "ID-1", tcp, true, nil)
	_ = tc
}

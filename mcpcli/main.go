package main

import (
	"fmt"
	"net"
	"net/http"

	"github.com/JeffreyRichter/internal/aids"
)

var (
	tcp    = NewAppToolCallProcessor(false)
	stream = NewAppToolCallProcessor(true)
)

func main() {
	c := FgBlue
	c.Print(
		`[1] Demo from start
[2] Recover from crash
[3] Local MCP Server

Which demo do you want to do (1, 2, or 3):`)

	var demo string // Action: accept, decline, cancel
	fmt.Scan(&demo)
	switch demo {
	case "1":
		client := NewMCPClient("http://localhost:8080/mcp", "")

		// ***** Listing tools *****
		/*client.dump = true
		response := aids.Must(client.Do("GET", "/tools", http.Header{"Accept": []string{"application/json"}}, nil))
		fmt.Println("Tools:")
		for _, tool := range unmarshalBody[mcp.ListToolsResult](response.Body).Tools {
			fmt.Printf("  %10s: %s\n", tool.Name, *tool.Description)
		}

		// ***** Simple ephemeral tool call  *****
		client.runToolCall("add", "ID-1", tcp, true, map[string]any{"x": 1, "y": 2})

		// ***** Long-running Server processing tool call *****
		client.runToolCall("count", "ID-1", tcp, true, map[string]any{"countto": 5})
		client.dump = false

		// ***** Streaming tool call  *****
		client.runToolCall("stream", "ID-1", stream, true, nil)

		// ***** No Server state tool call  *****
		*/
		// ***** Long-running elicitation tool call *****
		client.runToolCall("welcome", "ID-1", tcp, true, nil)

	case "2":
		client := NewMCPClient("http://localhost:8080/mcp", "")

		// ***** Crash recovery tool call *****
		client.runToolCall("welcome", "ID-1", tcp, false, nil)

	case "3":
		// ***** Local MCP Server *****
		serverPortAndKey, _ := SpawnMCPServer("../mcpsvr/mcpsvr.exe")
		client := NewMCPClient("http://"+net.JoinHostPort("localhost", serverPortAndKey.Port)+"/mcp", serverPortAndKey.Key)
		aids.Must(client.Do("GET", "/tools", http.Header{"Accept": []string{"application/json"}}, nil))
	}
}

func Foo() {
	FgHiBlack.And(BgWhite).Print("MCP Client started\n")

	FgBlack.Print("MCP Client started\n")
	FgYellow.And(BgBlue).Printf("MCP Client started\n")
	fmt.Printf("MCP Client started\n")

	FgCyan.Printf("MCP Client started\n")
	FgGreen.Printf("MCP Client started\n")
	FgRed.Printf("MCP Client started\n")
	FgMagenta.Printf("MCP Client started\n")
	FgBlue.Printf("MCP Client started\n")
	FgWhite.Printf("MCP Client started\n")
}

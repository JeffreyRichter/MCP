package main

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func main() {
	const mcpServerURL = "https://localhost:8080"
	client := http.DefaultClient

	r := must(client.Get(mcpServerURL + "/mcp/tools")) // Get list of all tools
	fmt.Println("Response status:", r.Status)

	// Create a tool call
	toolCallId := time.Now().UnixMicro()
	toolCallUrl := fmt.Sprintf(mcpServerURL+"/mcp/tools/add/%d", toolCallId)
	req := must(http.NewRequest("PUT", toolCallUrl, strings.NewReader(`{"x": 1, "y": 2}`)))
	r = must(client.Do(req))
	fmt.Println("Response status:", r.Status)

	// Poll the tool call's status
	r = must(client.Get(toolCallUrl))
	fmt.Println("Response status:", r.Status)

	// Advance the tool call
	r = must(client.Post(toolCallUrl+"/advance", "application/json", strings.NewReader(`{"x": 3, "y": 4}`)))

	// List all the 'add' tool calls
	r = must(client.Get(mcpServerURL + "/mcp/tools/add/calls"))

	// Cancel the tool call
	r = must(client.Post(toolCallUrl+"/cancel", "application/json", nil))
}

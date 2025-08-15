package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func waitForKeypress() {
	fmt.Print("\nPress Enter to continue...")
	bufio.NewReader(os.Stdin).ReadBytes('\n')
}

func main() {
	const mcpServerURL = "http://localhost:8080"
	client := &LoggingClient{http.DefaultClient}

	fmt.Println("🚀 Starting MCP Client Demo")
	fmt.Printf("Server URL: %s\n", mcpServerURL)

	fmt.Println("\n📋 Getting list of all available tools...")
	req := must(http.NewRequest("GET", mcpServerURL+"/mcp/tools", nil))
	must(client.Do(req))
	waitForKeypress()

	fmt.Println("\n➕ Creating a new tool call for 'add' operation...")
	toolCallId := time.Now().UnixMicro()
	toolCallUrl := fmt.Sprintf(mcpServerURL+"/mcp/tools/add/calls/%d", toolCallId)
	fmt.Printf("Generated Tool Call ID: %d\n", toolCallId)

	requestBody := `{"x": 1, "y": 2}`
	req = must(http.NewRequest("PUT", toolCallUrl, strings.NewReader(requestBody)))
	req.Header.Set("Content-Type", "application/json")
	must(client.Do(req))
	waitForKeypress()

	fmt.Println("\n🔍 Checking the status of the created tool call...")
	req = must(http.NewRequest("GET", toolCallUrl, nil))
	must(client.Do(req))
	waitForKeypress()

	fmt.Println("\n⏭️ Advancing the tool call with new parameters...")
	advanceBody := `{"x": 3, "y": 4}`
	req = must(http.NewRequest("POST", toolCallUrl+"/advance", strings.NewReader(advanceBody)))
	req.Header.Set("Content-Type", "application/json")
	must(client.Do(req))
	waitForKeypress()

	fmt.Println("\n📝 Listing all tool calls for the 'add' operation...")
	req = must(http.NewRequest("GET", mcpServerURL+"/mcp/tools/add/calls", nil))
	must(client.Do(req))
	waitForKeypress()

	fmt.Println("\n❌ Canceling the tool call...")
	req = must(http.NewRequest("POST", toolCallUrl+"/cancel", nil))
	req.Header.Set("Content-Type", "application/json")
	must(client.Do(req))
	waitForKeypress()

	fmt.Println("\n✅ MCP Client Demo completed!")
}

type LoggingClient struct {
	*http.Client
}

func (lc *LoggingClient) Do(req *http.Request) (*http.Response, error) {
	reqBody := ""
	if req.Body != nil {
		bodyBytes, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read request body: %w", err)
		}
		if len(bodyBytes) > 0 {
			reqBody = string(bodyBytes)
			req.Body = io.NopCloser(strings.NewReader(reqBody))
		}
	}
	fmt.Println("\n=== REQUEST ===")
	fmt.Println(req.Method, req.URL.String())
	if reqBody != "" {
		fmt.Println(reqBody)
	}

	resp, err := lc.Client.Do(req)
	if err != nil {
		return nil, err
	}

	fmt.Println("=== RESPONSE ===")
	fmt.Println(resp.Status)

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	resp.Body.Close()

	resBody := "(no content)"
	if len(b) > 0 {
		content := bytes.Buffer{}
		if err := json.Indent(&content, b, "", "  "); err != nil {
			content.Reset()
			content.Write(b)
		}
		resBody = content.String()
	}
	fmt.Println(resBody)
	fmt.Println("================")
	return resp, nil
}

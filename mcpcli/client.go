package main

import (
	"bufio"
	"context"
	"encoding/json/jsontext"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/JeffreyRichter/internal/aids"
	"github.com/JeffreyRichter/mcpsvr/mcp/toolcall"
)

var showJson = false

func NewMCPClient(urlPrefix string, key string) *mcpClient {
	return &mcpClient{
		Client:    &http.Client{}, //Timeout: 10 * time.Second},
		key:       key,
		urlPrefix: urlPrefix,
	}
}

type mcpClient struct {
	*http.Client
	key       string
	urlPrefix string
}

func (c *mcpClient) appendPath(path string) string { return c.urlPrefix + path }

func (c *mcpClient) Do(method string, pathSuffix string, header http.Header, body any) (*http.Response, error) {
	aids.Assert(header != nil, "header must be non-nil")
	var bodyReader io.Reader = nil
	if body != nil {
		bodyReader = strings.NewReader(string(aids.MustMarshal(body)))
	}
	r := aids.Must(http.NewRequest(method, c.appendPath(pathSuffix), bodyReader))
	r.Header = header
	r.Header.Add("SharedKey", c.key)
	return c.Client.Do(r)
}

// runToolCall runs a tool call to completion, processing any requests along the way
// If initiate is true, the tool call is initiated here; otherwise it is assumed to be already initiated (and request is ignored)
func (c *mcpClient) runToolCall(toolName, toolCallID string, tcp ToolCallProcessor, initiate bool, request any) toolcall.ToolCallClient {
	toolCallIDURL := "/tools/" + toolName + "/calls/" + toolCallID

	get := func() *http.Response {
		return aids.Must(c.Do("GET", toolCallIDURL, http.Header{"Accept": []string{"application/json"}}, nil))
	}

	response := (*http.Response)(nil)
	if initiate {
		response = aids.Must(c.Do("PUT", toolCallIDURL, http.Header{
			"Idempotency-Key": []string{"foobar"},
			"Content-Type":    []string{"application/json"},
			"Accept":          []string{"application/json"},
		}, request))
	} else {
		response = get()
	}
	aids.Assert(response.StatusCode == http.StatusOK || response.StatusCode == http.StatusCreated,
		fmt.Sprintf("Expected 200 or 201; got %d\n", response.StatusCode))
	tc := unmarshalBody[toolcall.ToolCallClient](response.Body)

	for !tc.Status.Terminated() {
		tcp.ShowProgress(tc)
		tcp.ShowPartialResults(tc)
		switch *tc.Status {
		case "awaitingSamplingResult":
			result := tcp.Sample(tc)
			response = aids.Must(c.Do("POST", toolCallIDURL+"/advance", http.Header{
				"Content-Type": []string{"application/json"},
				"Accept":       []string{"application/json"},
				"If-Match":     []string{tc.ETag.String()},
			}, result))
			if response.StatusCode == 412 {
				response = get()
			}
			tc = unmarshalBody[toolcall.ToolCallClient](response.Body)
		case "awaitingElicitationResult":
			result := tcp.Elicit(tc)
			response = aids.Must(c.Do("POST", toolCallIDURL+"/advance", http.Header{
				"Content-Type": []string{"application/json"},
				"Accept":       []string{"application/json"},
				"If-Match":     []string{tc.ETag.String()},
			}, result))
			if response.StatusCode == 412 {
				response = get()
			}
			tc = unmarshalBody[toolcall.ToolCallClient](response.Body)

		case "submitted", "running":
			// Optional: Pause execution
			time.Sleep(200 * time.Millisecond)
			response = get()
			tc = unmarshalBody[toolcall.ToolCallClient](response.Body)
		}
	}
	tcp.Terminated(tc)
	return tc
}

func unmarshalBody[T any](body io.ReadCloser) T {
	defer body.Close()
	jsonBody := aids.Must(io.ReadAll(body))
	printJson(jsonBody)
	return aids.MustUnmarshal[T](jsonBody)
}

func printJson(body jsontext.Value) {
	if showJson {
		fmt.Println((&body).Indent())
	}
}

type ToolCallProcessor interface {
	ShowProgress(tc toolcall.ToolCallClient)
	ShowPartialResults(tc toolcall.ToolCallClient)
	Sample(tc toolcall.ToolCallClient) any
	Elicit(tc toolcall.ToolCallClient) any
	Terminated(tc toolcall.ToolCallClient)
}

func SpawnMCPServer(path string) (McpServerPortAndKey, func() error) {
	cmd := exec.Command(path)
	stdout, err := cmd.StdoutPipe()
	if aids.IsError(err) {
		panic(err)
	}
	cmd.Stderr = cmd.Stdout
	//cmd.Env = []string{"MCPSVR_LOCAL=true"}
	if err := cmd.Start(); aids.IsError(err) {
		panic(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Second)
	defer cancel()
	reader := bufio.NewReader(stdout)
	lineCh := make(chan string, 1)
	go func() {
		line, _ := reader.ReadString('\n')
		lineCh <- strings.TrimSpace(line)
	}()
	select {
	case line := <-lineCh:
		fmt.Println(line)
		cxn := aids.MustUnmarshal[McpServerPortAndKey](([]byte)(line))
		return cxn, cmd.Process.Kill

	case <-ctx.Done():
		panic("timeout waiting for MCP server to start")
	}
}

type McpServerPortAndKey struct {
	Port string `json:"port"`
	Key  string `json:"key"`
}

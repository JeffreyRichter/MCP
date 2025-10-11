package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json/jsontext"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/JeffreyRichter/internal/aids"
	"github.com/JeffreyRichter/mcp"
)

var showJson = false

func NewMCPClient(urlPrefix string, sharedKey string) *mcpClient {
	return &mcpClient{
		Client:    &http.Client{}, //Timeout: 10 * time.Second},
		sharedKey: sharedKey,
		urlPrefix: urlPrefix,
	}
}

type mcpClient struct {
	*http.Client
	sharedKey string
	urlPrefix string
	dump      bool
}

func (c *mcpClient) appendPath(path string) string { return c.urlPrefix + path }

func (c *mcpClient) Do(method string, pathSuffix string, header http.Header, body any) (*http.Response, error) {
	aids.Assert(header != nil, "header must be non-nil")
	var bodyReader io.Reader = nil
	if body != nil {
		bodyReader = strings.NewReader(string(aids.MustMarshal(body)))
	}
	request := aids.Must(http.NewRequest(method, c.appendPath(pathSuffix), bodyReader))
	request.Header = header
	if c.sharedKey != "" {
		request.Header.Add("SharedKey", c.sharedKey)
	}

	dumpBody := func(b *io.ReadCloser) string {
		if *b == nil {
			return ""
		}
		copy := drainBody(b)
		text := jsontext.Value(aids.Must(io.ReadAll(copy)))
		text.Indent(jsontext.WithIndent("  "))
		return string(text)
	}
	if c.dump {
		fmt.Printf("-----> REQUEST:\n%v%v\n", string(aids.Must(httputil.DumpRequest(request, false))), dumpBody(&request.Body))
	}
	response, err := c.Client.Do(request)
	if c.dump {
		fmt.Printf("<-----RESPONSE:\n%v%v\n", string(aids.Must(httputil.DumpResponse(response, false))), dumpBody(&response.Body))
	}
	return response, err
}

// drainBody reads all of b to memory, sets *b to a copy and returns a copy
func drainBody(b *io.ReadCloser) (copy io.ReadCloser) {
	if *b == nil || *b == http.NoBody {
		return http.NoBody // No copying needed. Preserve the magic sentinel meaning of NoBody.
	}
	var buf bytes.Buffer
	buf.ReadFrom(*b)
	(*b).Close()
	*b = io.NopCloser(bytes.NewReader(buf.Bytes()))
	return io.NopCloser(&buf)
}

// runToolCall runs a tool call to completion, processing any requests along the way
// If initiate is true, the tool call is initiated here; otherwise it is assumed to be already initiated (and request is ignored)
func (c *mcpClient) runToolCall(toolName, toolCallID string, tcp ToolCallProcessor, initiate bool, request any) mcp.ToolCall {
	toolCallIDURL := "/tools/" + toolName + "/calls/" + toolCallID

	get := func() *http.Response {
		return aids.Must(c.Do("GET", toolCallIDURL, http.Header{"Accept": []string{"application/json"}}, nil))
	}

	response := (*http.Response)(nil)
	if initiate {
		response = aids.Must(c.Do("PUT", toolCallIDURL, http.Header{
			"Idempotency-Key": []string{time.Now().Format(time.RFC3339Nano)},
			"Content-Type":    []string{"application/json"},
			"Accept":          []string{"application/json"},
		}, request))
	} else {
		response = get()
	}
	aids.AssertHttpStatus(response, http.StatusOK, http.StatusCreated)
	tc := unmarshalBody[mcp.ToolCall](response.Body)

	for !tc.Status.Terminated() {
		tcp.ShowProgress(tc)
		tcp.ShowPartialResults(tc)
		switch *tc.Status {
		case "awaitingSamplingResult":
			result := tcp.Sample(tc)
			response = aids.Must(c.Do("POST", toolCallIDURL+"/advance", http.Header{
				"Content-Type": []string{"application/json"},
				"Accept":       []string{"application/json"},
				"If-Match":     []string{*tc.ETag},
			}, result))
			if response.StatusCode == 412 {
				response = get()
			}
			tc = unmarshalBody[mcp.ToolCall](response.Body)

		case "awaitingElicitationResult":
			result := tcp.Elicit(tc)
			response = aids.Must(c.Do("POST", toolCallIDURL+"/advance", http.Header{
				"Content-Type": []string{"application/json"},
				"Accept":       []string{"application/json"},
				"If-Match":     []string{*tc.ETag},
			}, result))
			if response.StatusCode == 412 {
				response = get()
			}
			tc = unmarshalBody[mcp.ToolCall](response.Body)

		case "submitted", "running":
			// Optional: Pause execution
			time.Sleep(200 * time.Millisecond)
			response = get()
			tc = unmarshalBody[mcp.ToolCall](response.Body)
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
	ShowProgress(tc mcp.ToolCall)
	ShowPartialResults(tc mcp.ToolCall)
	Sample(tc mcp.ToolCall) any
	Elicit(tc mcp.ToolCall) any
	Terminated(tc mcp.ToolCall)
}

func SpawnMCPServer(path string) (McpServerPortAndKey, func() error) {
	cmd := exec.Command(path, "-pid="+strconv.Itoa(os.Getpid()))
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

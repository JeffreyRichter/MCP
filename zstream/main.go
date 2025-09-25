package main

import (
	"bufio"
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/JeffreyRichter/internal/aids"
)

// StreamTextSimple streams text character by character at a constant rate
func StreamTextSimple(text string, charsPerSecond int) {
	delay := time.Second / time.Duration(charsPerSecond)
	for _, char := range text {
		fmt.Print(string(char))
		time.Sleep(delay)
	}
	fmt.Println()
}

type toolCall struct {
	ToolName           *string        `json:"toolname"`
	ID                 *string        `json:"id"` // Scoped within tenant & tool name
	Expiration         *time.Time     `json:"expiration,omitempty"`
	ETag               *string        `json:"etag"`
	Status             *Status        `json:"status,omitempty" enum:"running,awaitingSamplingResponse,awaitingElicitationResponse,success,failed,canceled"`
	Request            jsontext.Value `json:"request,omitempty"`
	SamplingRequest    jsontext.Value `json:"samplingRequest,omitempty"`
	ElicitationRequest jsontext.Value `json:"elicitationRequest,omitempty"`
	Progress           jsontext.Value `json:"progress,omitempty"`
	Result             jsontext.Value `json:"result,omitempty"`
	Error              jsontext.Value `json:"error,omitempty"`
}
type Status string

const (
	StatusSubmitted                 Status = "submitted"
	StatusRunning                   Status = "running"
	StatusAwaitingSamplingResult    Status = "awaitingSamplingResult"
	StatusAwaitingElicitationResult Status = "awaitingElicitationResult"
	StatusSuccess                   Status = "success"
	StatusFailed                    Status = "failed"
	StatusCanceled                  Status = "canceled"
)

func spawnMCPServer() mcpServerPortAndKey {
	cmd := exec.Command("../mcpsvr/mcpsvr.exe")
	stdout, err := cmd.StdoutPipe()
	if aids.IsError(err) {
		panic(err)
	}
	cmd.Stderr = cmd.Stdout
	//cmd.Env = []string{"MCPSVR_LOCAL=true"}
	if err := cmd.Start(); aids.IsError(err) {
		panic(err)
	}
	// pid := cmd.Process.Pid
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	reader := bufio.NewReader(stdout)
	lineCh := make(chan string, 1)
	go func() {
		line, _ := reader.ReadString('\n')
		lineCh <- strings.TrimSpace(line)
	}()
	select {
	case line := <-lineCh:
		var cxn mcpServerPortAndKey
		if err := json.Unmarshal([]byte(line), &cxn); err != nil {
			panic(err)
		}
		return cxn
	case <-ctx.Done():
		panic("timeout waiting for MCP server to start")
	}
}

type mcpServerPortAndKey struct {
	Port int    `json:"port"`
	Key  string `json:"key"`
}

func main() {
	cxn := spawnMCPServer()
	mcpsvrURL := fmt.Sprintf("http://localhost:%d", cxn.Port)
	client := http.Client{}
	toolcallURL := mcpsvrURL + "mcp/tools/stream/calls/test"
	req, _ := http.NewRequest(http.MethodPut, toolcallURL, nil)
	req.Header.Set("authorization", cxn.Key)
	req.Header.Set("idempotency-key", "foo")

	for t := 0; true; {
		var tc toolCall
		response, err := client.Do(req)
		if err != nil {
			panic(err)
		}
		body, err := io.ReadAll(response.Body)
		if err != nil {
			panic(err)
		}
		response.Body.Close()
		if err := json.Unmarshal(body, &tc); err != nil {
			panic(err)
		}

		type streamToolCallResult struct {
			Text []string `json:"text"`
		}
		var streamResult streamToolCallResult
		if err := json.Unmarshal(tc.Result, &streamResult); err != nil {
			panic(err)
		}
		for len(streamResult.Text) > t { // Something new in the result, stream it
			StreamTextSimple(streamResult.Text[t], 70)
			t++
		}
		if *tc.Status == StatusSuccess || *tc.Status == StatusFailed || *tc.Status == StatusCanceled {
			break // Tool call reached termination state
		}
		time.Sleep(100 * time.Millisecond) // Give the tool call more time to advance before checking on it again
		req, _ := http.NewRequest(http.MethodGet, toolcallURL, nil)
		req.Header.Set("authorization", cxn.Key)
	}
	/*
		text, done := []string{}, false
		go func() {
			for t := range 10 {
				time.Sleep(60 * time.Millisecond)
				text = append(text, fmt.Sprintf("%d: Hello! I'm an AI assistant. I can help you with a variety of tasks including answering questions, writing content, analyzing data, and much more. What would you like to work on today?", t))
			}
			done = true
		}()

		for t := 0; true; {
			if len(text) == t && !done {
				time.Sleep(100 * time.Millisecond)
				continue
			}
			StreamTextSimple(text[t], 70)
			t++
			if t == len(text) && done {
				break
			}
		}*/
}

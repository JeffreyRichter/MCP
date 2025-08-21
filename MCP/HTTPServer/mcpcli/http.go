package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

var (
	ctx     = context.Background()
	timeout = 10 * time.Second
)

// httpClient wraps http.Client with MCP-specific functionality
type httpClient struct {
	*http.Client
	serverURL  string
	apiVersion string
}

// getTools fetches the list of available tools from the server
func (c *httpClient) getTools() ([]ToolInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.serverURL+"/mcp/tools", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch tools: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned status %d", resp.StatusCode)
	}

	var result struct {
		Tools []ToolInfo `json:"tools"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode tools response: %w", err)
	}

	return result.Tools, nil
}

// createToolCall executes a tool call and returns the HTTP transaction
func (c *httpClient) createToolCall(toolName, callID string, params map[string]any) (*HTTPTransaction, error) {
	body, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal parameters: %w", err)
	}
	url := fmt.Sprintf("%s/mcp/tools/%s/calls/%s", c.serverURL, toolName, callID)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	start := time.Now()
	resp, err := c.Do(req)
	duration := time.Since(start)
	if err == nil {
		defer resp.Body.Close()
	}

	transaction := &HTTPTransaction{
		Method:      req.Method,
		URL:         req.URL.String(),
		RequestBody: string(body),
		Timestamp:   start,
		Duration:    duration,
		Error:       err,
	}

	if resp != nil {
		transaction.StatusCode = resp.StatusCode
		transaction.Headers = make(map[string]string)
		for k, v := range resp.Header {
			transaction.Headers[k] = strings.Join(v, ", ")
		}

		if respBody, readErr := io.ReadAll(resp.Body); readErr == nil {
			transaction.ResponseBody = string(respBody)
		}
	}

	if err != nil {
		return transaction, fmt.Errorf("request failed: %w", err)
	}

	return transaction, nil
}

// advanceElicitation sends the user's decision for a PII elicitation
func (c *httpClient) advanceElicitation(callID string, approved bool) (*HTTPTransaction, error) {
	// POST { "action": "accept", "content": { "approved": <bool> } }
	url := fmt.Sprintf("%s/mcp/tools/pii/calls/%s/advance", c.serverURL, callID)

	params := map[string]any{
		"action":  "accept",
		"content": map[string]any{"approved": approved},
	}

	body, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal elicitation response: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create elicitation request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	start := time.Now()
	resp, err := c.Do(req)
	duration := time.Since(start)
	if err == nil {
		defer resp.Body.Close()
	}

	transaction := &HTTPTransaction{
		Method:      req.Method,
		URL:         req.URL.String(),
		RequestBody: string(body),
		Timestamp:   start,
		Duration:    duration,
		Error:       err,
	}

	if resp != nil {
		transaction.StatusCode = resp.StatusCode
		transaction.Headers = make(map[string]string)
		for k, v := range resp.Header {
			transaction.Headers[k] = strings.Join(v, ", ")
		}

		if respBody, readErr := io.ReadAll(resp.Body); readErr == nil {
			transaction.ResponseBody = string(respBody)
		}
	}

	if err != nil {
		return transaction, fmt.Errorf("elicitation advance failed: %w", err)
	}

	return transaction, nil
}

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
	authKey    string
}

// getTools fetches the list of available tools from the server and returns an HTTPTransaction for UI display
func (c *httpClient) getTools() ([]ToolInfo, *HTTPTransaction, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.serverURL+"/mcp/tools", nil)
	if isError(err) {
		return nil, nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if c.authKey != "" {
		req.Header.Set("Authorization", c.authKey)
	}

	start := time.Now()
	resp, err := c.Do(req)
	duration := time.Since(start)

	txn := &HTTPTransaction{
		Method:         req.Method,
		URL:            req.URL.String(),
		RequestHeaders: req.Header.Clone(),
		Timestamp:      start,
		Duration:       duration,
		Error:          err,
	}

	if isError(err) {
		return nil, txn, fmt.Errorf("failed to fetch tools: %w", err)
	}
	defer resp.Body.Close()
	txn.StatusCode = resp.StatusCode
	txn.ResponseHeaders = resp.Header.Clone()
	if body, err := io.ReadAll(resp.Body); !isError(err) {
		// TODO: clarify responsibility for response formatting, remove the double read
		txn.ResponseBody = string(body)
		resp.Body = io.NopCloser(bytes.NewReader(body))
	}

	if resp.StatusCode != http.StatusOK {
		return nil, txn, fmt.Errorf("server returned status %d", resp.StatusCode)
	}

	var result struct {
		Tools []ToolInfo `json:"tools"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); isError(err) {
		return nil, txn, fmt.Errorf("failed to decode tools response: %w", err)
	}

	return result.Tools, txn, nil
}

// createToolCall executes a tool call and returns the HTTP transaction
func (c *httpClient) createToolCall(toolName, callID string, params map[string]any) (*HTTPTransaction, error) {
	body, err := json.Marshal(params)
	if isError(err) {
		return nil, fmt.Errorf("failed to marshal parameters: %w", err)
	}
	url := fmt.Sprintf("%s/mcp/tools/%s/calls/%s", c.serverURL, toolName, callID)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(body))
	if isError(err) {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.authKey != "" {
		req.Header.Set("Authorization", c.authKey)
	}

	start := time.Now()
	resp, err := c.Do(req)
	duration := time.Since(start)
	if !isError(err) {
		defer resp.Body.Close()
	}

	transaction := &HTTPTransaction{
		Method:         req.Method,
		URL:            req.URL.String(),
		RequestBody:    string(body),
		RequestHeaders: req.Header.Clone(),
		Timestamp:      start,
		Duration:       duration,
		Error:          err,
	}

	if resp != nil {
		transaction.StatusCode = resp.StatusCode
		transaction.ResponseHeaders = resp.Header.Clone()

		if respBody, readErr := io.ReadAll(resp.Body); readErr == nil {
			transaction.ResponseBody = string(respBody)
		}
	}

	if isError(err) {
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
	if isError(err) {
		return nil, fmt.Errorf("failed to marshal elicitation response: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if isError(err) {
		return nil, fmt.Errorf("failed to create elicitation request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.authKey != "" {
		req.Header.Set("Authorization", c.authKey)
	}

	start := time.Now()
	resp, err := c.Do(req)
	duration := time.Since(start)
	if !isError(err) {
		defer resp.Body.Close()
	}

	transaction := &HTTPTransaction{
		Method:         req.Method,
		URL:            req.URL.String(),
		RequestBody:    string(body),
		RequestHeaders: req.Header.Clone(),
		Timestamp:      start,
		Duration:       duration,
		Error:          err,
	}

	if resp != nil {
		transaction.StatusCode = resp.StatusCode
		transaction.ResponseHeaders = resp.Header.Clone()

		if respBody, readErr := io.ReadAll(resp.Body); readErr == nil {
			transaction.ResponseBody = string(respBody)
		}
	}

	if isError(err) {
		return transaction, fmt.Errorf("elicitation advance failed: %w", err)
	}

	return transaction, nil
}
func isError(err error) bool { return err != nil }

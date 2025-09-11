package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"time"

	"github.com/JeffreyRichter/internal/aids"
	"github.com/JeffreyRichter/mcpsvr/mcp/toolcall"
)

const baseURL = "http://localhost:8080"

// TODO: more tests
type TestSuite struct{}

func (TestSuite) TestToolCallAdd() error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	callID := "test-add"

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, baseURL+"/mcp/tools/add/calls/"+callID, strings.NewReader(`{"x":5,"y":3}`))
	if aids.IsError(err) {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if aids.IsError(err) {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return errors.New("unexpected response status " + resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if aids.IsError(err) {
		return err
	}
	var add struct {
		Result struct {
			Sum int `json:"sum"`
		} `json:"result"`
	}
	if err := json.Unmarshal(body, &add); aids.IsError(err) {
		return err
	}
	if add.Result.Sum != 8 {
		return fmt.Errorf("expected sum: 8, got %d", add.Result.Sum)
	}

	return nil
}

func (TestSuite) TestToolCallPIIElicitationApproved() error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	callID := "test-pii-elicitation-approved"

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, baseURL+"/mcp/tools/pii/calls/"+callID, strings.NewReader(`{"key":"test"}`))
	if aids.IsError(err) {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if aids.IsError(err) {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return errors.New("unexpected response status " + resp.Status)
	}

	advanceBody := `{"action":"accept","content":{"approved":true}}`

	req, err = http.NewRequestWithContext(ctx, http.MethodPost, req.URL.String()+"/advance", strings.NewReader(advanceBody))
	if aids.IsError(err) {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err = http.DefaultClient.Do(req)
	if aids.IsError(err) {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return errors.New("unexpected response status " + resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if aids.IsError(err) {
		return err
	}
	var tc struct {
		Status             *string `json:"status"`
		ElicitationRequest any     `json:"elicitationRequest"`
		Result             any     `json:"result"`
	}
	if err := json.Unmarshal(body, &tc); aids.IsError(err) {
		return err
	}
	if tc.Status == nil || *tc.Status != "success" {
		return fmt.Errorf("expected status 'success', got %v", tc.Status)
	}
	if tc.ElicitationRequest != nil {
		return err
	}
	if tc.Result == nil {
		return errors.New("expected result")
	}
	b, err := json.Marshal(tc.Result)
	if aids.IsError(err) {
		return err
	}
	var result struct {
		Data string `json:"data"`
	}
	if err := json.Unmarshal(b, &result); aids.IsError(err) {
		return err
	}
	if len(result.Data) == 0 {
		return errors.New("expected result data")
	}

	// trying to advance the completed tool call is a bad request
	req, err = http.NewRequestWithContext(ctx, http.MethodPost, req.URL.String(), strings.NewReader(advanceBody))
	if aids.IsError(err) {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if aids.IsError(err) {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		return fmt.Errorf("expected %d, got %d", http.StatusBadRequest, resp.StatusCode)
	}

	return nil
}

func (TestSuite) TestToolCallPIICreateIdempotent() error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	callID := "test-pii-create-idempotent"
	requestBody := `{"key":"test"}`

	responses := make([]*http.Response, 2)
	for i := range 2 {
		req, err := http.NewRequestWithContext(ctx, http.MethodPut, baseURL+"/mcp/tools/pii/calls/"+callID, strings.NewReader(requestBody))
		if aids.IsError(err) {
			return err
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if aids.IsError(err) {
			return err
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("request %d received unexpected response status %s", i, resp.Status)
		}

		responses[i] = resp
	}

	if responses[0].StatusCode != responses[1].StatusCode {
		return fmt.Errorf("HTTP status code mismatch: first=%d, second=%d", responses[0].StatusCode, responses[1].StatusCode)
	}

	toolCalls := make([]toolcall.ToolCall, 2)
	for i, resp := range responses {
		body, err := io.ReadAll(resp.Body)
		if aids.IsError(err) {
			return err
		}
		tc := toolcall.ToolCall{}
		if err := json.Unmarshal(body, &tc); aids.IsError(err) {
			return fmt.Errorf("failed to unmarshal response %d: %w", i, err)
		}
		toolCalls[i] = tc
	}

	// identical responses imply idempotence
	tc1, tc2 := toolCalls[0], toolCalls[1]
	if !reflect.DeepEqual(tc1, tc2) {
		tc1JSON, _ := json.MarshalIndent(tc1, "", "  ")
		tc2JSON, _ := json.MarshalIndent(tc2, "", "  ")
		return fmt.Errorf("responses are not identical:\n=== Response 1 ===\n%s\n=== Response 2 ===\n%s", tc1JSON, tc2JSON)
	}
	if tc1.Status == nil {
		return errors.New("status should not be nil")
	}
	if tc1.ElicitationRequest == nil {
		return errors.New("expected an elicitation request")
	}

	return nil
}

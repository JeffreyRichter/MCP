package v20250808

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/JeffreyRichter/mcpsvc/mcp/toolcalls"
)

type testClient struct {
	t   *testing.T
	url string
}

func newTestClient(t *testing.T) *testClient {
	srv := testServer(t)
	t.Cleanup(srv.Close)
	return &testClient{t: t, url: srv.URL}
}

func (c *testClient) Put(path string, headers http.Header, body io.Reader) *http.Response {
	return c.do(http.MethodPut, path, headers, body)
}

func (c *testClient) Post(path string, headers http.Header, body io.Reader) *http.Response {
	return c.do(http.MethodPost, path, headers, body)
}

func (c *testClient) Get(path string, headers http.Header) *http.Response {
	return c.do(http.MethodGet, path, headers, nil)
}

func (c *testClient) do(method, path string, headers http.Header, body io.Reader) *http.Response {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, method, c.url+path, body)
	if err != nil {
		c.t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		for _, val := range v {
			req.Header.Add(k, val)
		}
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	return resp
}

func TestToolCallPIICreate(t *testing.T) {
	client := newTestClient(t)

	resp := client.Put("/mcp/tools/pii/calls/"+t.Name(), http.Header{}, strings.NewReader(`{"key":"test"}`))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status %d, got %d", http.StatusOK, resp.StatusCode)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	tc := toolcalls.ToolCall{}
	err = json.Unmarshal(b, &tc)
	if err != nil {
		t.Fatal(err)
	}
	if tc.Status == nil || *tc.Status != toolcalls.ToolCallStatusAwaitingElicitationResult {
		t.Fatalf("expected status %q, got %v", toolcalls.ToolCallStatusAwaitingElicitationResult, tc.Status)
	}
	if tc.ElicitationRequest == nil {
		t.Fatal("expected elicitation request to be present")
	}
	if !strings.Contains(tc.ElicitationRequest.Message, "PII") {
		t.Fatalf("unexpected elicitation message %q", tc.ElicitationRequest.Message)
	}
}

func TestToolCallPIICreate_Error(t *testing.T) {
	t.Skip("TODO: service infra writes multiple objects to body")
	client := newTestClient(t)

	resp := client.Put("/mcp/tools/pii/calls/"+t.Name(), http.Header{}, nil)
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(string(b))
	resp.Body.Close()

	tc := toolcalls.ToolCall{}
	err = json.Unmarshal(b, &tc)
	if err != nil {
		t.Fatal(err)
	}
	if tc.Status == nil || *tc.Status != toolcalls.ToolCallStatusAwaitingElicitationResult {
		t.Fatalf("expected status %q, got %v", toolcalls.ToolCallStatusAwaitingElicitationResult, tc.Status)
	}
	if tc.ElicitationRequest == nil {
		t.Fatal("expected elicitation request to be present")
	}
	if tc.ElicitationRequest.Message != "approve PII" {
		t.Fatalf("expected elicitation message 'approve PII', got %q", tc.ElicitationRequest.Message)
	}
}

func TestToolCallPIIGet(t *testing.T) {
	client := newTestClient(t)

	resp := client.Put("/mcp/tools/pii/calls/"+t.Name(), http.Header{}, strings.NewReader(`{"key":"test"}`))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Create failed with status %d", resp.StatusCode)
	}
	resp.Body.Close()

	// get the tool call
	resp = client.Get("/mcp/tools/pii/calls/"+t.Name(), http.Header{})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status %d, got %d", http.StatusOK, resp.StatusCode)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	tc := toolcalls.ToolCall{}
	err = json.Unmarshal(b, &tc)
	if err != nil {
		t.Fatal(err)
	}
	if tc.Status == nil || *tc.Status != toolcalls.ToolCallStatusAwaitingElicitationResult {
		t.Fatalf("expected status %q, got %v", toolcalls.ToolCallStatusAwaitingElicitationResult, tc.Status)
	}
}

func TestToolCallPIIElicitationApproved(t *testing.T) {
	client := newTestClient(t)

	resp := client.Put("/mcp/tools/pii/calls/"+t.Name(), http.Header{}, strings.NewReader(`{"key":"test"}`))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Create failed with status %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp = client.Post("/mcp/tools/pii/calls/"+t.Name()+"/advance", http.Header{}, strings.NewReader(`{"action":"accept","content":{"approved":true}}`))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status %d, got %d", http.StatusOK, resp.StatusCode)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	tc := toolcalls.ToolCall{}
	err = json.Unmarshal(b, &tc)
	if err != nil {
		t.Fatal(err)
	}

	if tc.Status == nil || *tc.Status != toolcalls.ToolCallStatusSuccess {
		t.Fatalf("expected status %q, got %v", toolcalls.ToolCallStatusSuccess, tc.Status)
	}
	if tc.ElicitationRequest != nil {
		t.Fatal("expected elicitation request to be nil after processing")
	}
	if tc.Result == nil {
		t.Fatal("expected result to be present")
	}

	var result PIIToolCallResult
	err = json.Unmarshal(tc.Result, &result)
	if err != nil {
		t.Fatal(err)
	}
	if result.Data != "here's your PII" {
		t.Fatalf("expected result data 'here's your PII', got %q", result.Data)
	}
}

func TestToolCallPIIElicitationRejected(t *testing.T) {
	for _, action := range []string{"decline", "reject"} {
		t.Run(action, func(t *testing.T) {
			client := newTestClient(t)
			id := strings.ReplaceAll(t.Name(), "/", "-")
			resp := client.Put("/mcp/tools/pii/calls/"+id, http.Header{}, strings.NewReader(`{"key":"test"}`))
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("Create failed with status %d", resp.StatusCode)
			}
			resp.Body.Close()

			resp = client.Post("/mcp/tools/pii/calls/"+id+"/advance", http.Header{}, strings.NewReader(fmt.Sprintf(`{"action":%q}`, action)))
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("Expected status %d, got %d", http.StatusOK, resp.StatusCode)
			}

			b, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatal(err)
			}
			resp.Body.Close()

			tc := toolcalls.ToolCall{}
			err = json.Unmarshal(b, &tc)
			if err != nil {
				t.Fatal(err)
			}
			if tc.Status == nil || *tc.Status != toolcalls.ToolCallStatusCanceled {
				t.Fatalf("expected status %q, got %v", toolcalls.ToolCallStatusCanceled, tc.Status)
			}
			if tc.ElicitationRequest != nil {
				t.Fatal("expected nil elicitation request after processing")
			}
			if tc.Result != nil {
				t.Fatalf("expected nil result after %q action", action)
			}
		})
	}

	t.Run("disapprove", func(t *testing.T) {
		client := newTestClient(t)
		id := strings.ReplaceAll(t.Name(), "/", "-")
		resp := client.Put("/mcp/tools/pii/calls/"+id, http.Header{}, strings.NewReader(`{"key":"test"}`))
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Create failed with status %d", resp.StatusCode)
		}
		resp.Body.Close()

		resp = client.Post("/mcp/tools/pii/calls/"+id+"/advance", http.Header{}, strings.NewReader(`{"action":"accept","content":{"approved":false}}`))
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Expected status %d, got %d", http.StatusOK, resp.StatusCode)
		}

		b, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()

		tc := toolcalls.ToolCall{}
		err = json.Unmarshal(b, &tc)
		if err != nil {
			t.Fatal(err)
		}
		if tc.Status == nil || *tc.Status != toolcalls.ToolCallStatusCanceled {
			t.Fatalf("expected status %q, got %v", toolcalls.ToolCallStatusCanceled, tc.Status)
		}
		if tc.ElicitationRequest != nil {
			t.Fatal("expected elicitation request to be nil after processing")
		}
		if tc.Result != nil {
			t.Fatal("expected result to be nil for disapproved elicitation")
		}
	})
}

func TestToolCallPIICancel(t *testing.T) {
	client := newTestClient(t)

	resp := client.Put("/mcp/tools/pii/calls/"+t.Name(), http.Header{}, strings.NewReader(`{"key":"test"}`))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Create failed with status %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp = client.Post("/mcp/tools/pii/calls/"+t.Name()+"/cancel", http.Header{}, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status %d, got %d", http.StatusOK, resp.StatusCode)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	tc := toolcalls.ToolCall{}
	err = json.Unmarshal(b, &tc)
	if err != nil {
		t.Fatal(err)
	}
	if tc.Status == nil || *tc.Status != toolcalls.ToolCallStatusCanceled {
		t.Fatalf("expected status %q, got %v", toolcalls.ToolCallStatusCanceled, tc.Status)
	}
	if tc.ElicitationRequest != nil {
		t.Fatal("expected elicitation request to be nil after cancellation")
	}
	if tc.Result != nil {
		t.Fatal("expected result to be nil after cancellation")
	}
	if tc.Error != nil {
		t.Fatal("expected error to be nil after cancellation")
	}
}

func TestToolCallPIICancelAlreadyCompleted(t *testing.T) {
	client := newTestClient(t)

	resp := client.Put("/mcp/tools/pii/calls/"+t.Name(), http.Header{}, strings.NewReader(`{"key":"test"}`))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Create failed with status %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp = client.Post("/mcp/tools/pii/calls/"+t.Name()+"/advance", http.Header{}, strings.NewReader(`{"action":"accept","content":{"approved":true}}`))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Advance failed with status %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp = client.Post("/mcp/tools/pii/calls/"+t.Name()+"/cancel", http.Header{}, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status %d, got %d", http.StatusOK, resp.StatusCode)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	tc := toolcalls.ToolCall{}
	err = json.Unmarshal(b, &tc)
	if err != nil {
		t.Fatal(err)
	}
	if tc.Status == nil || *tc.Status != toolcalls.ToolCallStatusSuccess {
		t.Fatalf("expected status %q, got %v", toolcalls.ToolCallStatusSuccess, tc.Status)
	}
}

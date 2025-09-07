package v20250808

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/JeffreyRichter/mcpsvr/mcp/toolcalls"
)

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

func TestToolCallPIIGet(t *testing.T) {
	client := newTestClient(t)

	resp := client.Put("/mcp/tools/pii/calls/"+t.Name(), http.Header{}, strings.NewReader(`{"key":"test"}`))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Create failed with status %d", resp.StatusCode)
	}

	// get the tool call
	resp = client.Get("/mcp/tools/pii/calls/"+t.Name(), http.Header{})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status %d, got %d", http.StatusOK, resp.StatusCode)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}

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

			resp = client.Post("/mcp/tools/pii/calls/"+id+"/advance", http.Header{}, strings.NewReader(fmt.Sprintf(`{"action":%q}`, action)))
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("Expected status %d, got %d", http.StatusOK, resp.StatusCode)
			}

			b, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatal(err)
			}

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

		resp = client.Post("/mcp/tools/pii/calls/"+id+"/advance", http.Header{}, strings.NewReader(`{"action":"accept","content":{"approved":false}}`))
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Expected status %d, got %d", http.StatusOK, resp.StatusCode)
		}

		b, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}

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

	resp = client.Post("/mcp/tools/pii/calls/"+t.Name()+"/cancel", http.Header{}, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status %d, got %d", http.StatusOK, resp.StatusCode)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}

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

	resp = client.Post("/mcp/tools/pii/calls/"+t.Name()+"/advance", http.Header{}, strings.NewReader(`{"action":"accept","content":{"approved":true}}`))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Advance failed with status %d", resp.StatusCode)
	}

	resp = client.Post("/mcp/tools/pii/calls/"+t.Name()+"/cancel", http.Header{}, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status %d, got %d", http.StatusOK, resp.StatusCode)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}

	tc := toolcalls.ToolCall{}
	err = json.Unmarshal(b, &tc)
	if err != nil {
		t.Fatal(err)
	}
	if tc.Status == nil || *tc.Status != toolcalls.ToolCallStatusSuccess {
		t.Fatalf("expected status %q, got %v", toolcalls.ToolCallStatusSuccess, tc.Status)
	}
}

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/JeffreyRichter/internal/aids"
	"github.com/JeffreyRichter/mcp"
	"github.com/JeffreyRichter/mcpsvr/toolcall"
)

func TestToolCallWelcomeCreate(t *testing.T) {
	client := newTestClient(t)

	resp := client.Put("/mcp/tools/welcome/calls/"+t.Name(),
		http.Header{"Idempotency-Key": []string{time.Now().Format(time.RFC3339Nano)}},
		strings.NewReader(`{"key":"test"}`))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status %d, got %d", http.StatusOK, resp.StatusCode)
	}

	b, err := io.ReadAll(resp.Body)
	if aids.IsError(err) {
		t.Fatal(err)
	}

	tc := toolcall.Resource{}
	err = json.Unmarshal(b, &tc)
	if aids.IsError(err) {
		t.Fatal(err)
	}
	if tc.Status == nil || *tc.Status != mcp.StatusAwaitingElicitationResult {
		t.Fatalf("expected status %q, got %v", mcp.StatusAwaitingElicitationResult, tc.Status)
	}
	if tc.ElicitationRequest == nil {
		t.Fatal("expected elicitation request to be present")
	}
	if !strings.Contains(tc.ElicitationRequest.Message, "Need name for welcome message.") {
		t.Fatalf("unexpected elicitation message %q", tc.ElicitationRequest.Message)
	}
}

func TestToolCallWelcomeGet(t *testing.T) {
	client := newTestClient(t)

	resp := client.Put("/mcp/tools/welcome/calls/"+t.Name(),
		http.Header{"Idempotency-Key": []string{time.Now().Format(time.RFC3339Nano)}},
		strings.NewReader(`{"key":"test"}`))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Create failed with status %d", resp.StatusCode)
	}

	// get the tool call
	resp = client.Get("/mcp/tools/welcome/calls/"+t.Name(), http.Header{})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status %d, got %d", http.StatusOK, resp.StatusCode)
	}

	b, err := io.ReadAll(resp.Body)
	if aids.IsError(err) {
		t.Fatal(err)
	}

	tc := toolcall.Resource{}
	err = json.Unmarshal(b, &tc)
	if aids.IsError(err) {
		t.Fatal(err)
	}
	if tc.Status == nil || *tc.Status != mcp.StatusAwaitingElicitationResult {
		t.Fatalf("expected status %q, got %v", mcp.StatusAwaitingElicitationResult, tc.Status)
	}
}

func TestToolCallWelcomeElicitationApproved(t *testing.T) {
	client := newTestClient(t)

	resp := client.Put("/mcp/tools/welcome/calls/"+t.Name(),
		http.Header{"Idempotency-Key": []string{time.Now().Format(time.RFC3339Nano)}},
		strings.NewReader(`{"key":"test"}`))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Create failed with status %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp = client.Post("/mcp/tools/welcome/calls/"+t.Name()+"/advance", http.Header{},
		strings.NewReader(`{"action":"accept","content":{"name":"Jeffrey"}}`))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status %d, got %d", http.StatusOK, resp.StatusCode)
	}

	b, err := io.ReadAll(resp.Body)
	if aids.IsError(err) {
		t.Fatal(err)
	}
	tc := toolcall.Resource{}
	err = json.Unmarshal(b, &tc)
	if aids.IsError(err) {
		t.Fatal(err)
	}

	if tc.Status == nil || *tc.Status != mcp.StatusSuccess {
		t.Fatalf("expected status %q, got %v", mcp.StatusSuccess, tc.Status)
	}
	if tc.ElicitationRequest != nil {
		t.Fatal("expected elicitation request to be nil after processing")
	}
	if tc.Result == nil {
		t.Fatal("expected result to be present")
	}

	var result welcomeToolCallResult
	err = json.Unmarshal(tc.Result, &result)
	if aids.IsError(err) {
		t.Fatal(err)
	}
	if !strings.HasPrefix(result.Welcome, "Hello ") {
		t.Fatalf("expected \"Hello ...\", got %q", result.Welcome)
	}
}

func TestToolCallElicitationRejected(t *testing.T) {
	for _, action := range []string{"decline", "cancel"} {
		t.Run(action, func(t *testing.T) {
			client := newTestClient(t)
			id := strings.ReplaceAll(t.Name(), "/", "-")
			resp := client.Put("/mcp/tools/welcome/calls/"+id,
				http.Header{"Idempotency-Key": []string{time.Now().Format(time.RFC3339Nano)}},
				strings.NewReader(`{"key":"test"}`))
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("Create failed with status %d", resp.StatusCode)
			}

			resp = client.Post("/mcp/tools/welcome/calls/"+id+"/advance", http.Header{},
				strings.NewReader(fmt.Sprintf(`{"action":%q}`, action)))
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("Expected status %d, got %d", http.StatusOK, resp.StatusCode)
			}

			b, err := io.ReadAll(resp.Body)
			if aids.IsError(err) {
				t.Fatal(err)
			}

			tc := toolcall.Resource{}
			err = json.Unmarshal(b, &tc)
			if aids.IsError(err) {
				t.Fatal(err)
			}
			switch action {
			case "decline":
				if *tc.Status != mcp.StatusSuccess {
					t.Fatalf("expected %q, got %v", mcp.StatusSuccess, *tc.Status)
				}
				if tc.Result == nil {
					t.Fatal("expected non-nil Result")
				}
			case "cancel":
				if *tc.Status != mcp.StatusCanceled {
					t.Fatalf("expected %q, got %v", mcp.StatusCanceled, *tc.Status)
				}
				if tc.Result != nil {
					t.Fatal("expected nil result")
				}
			}
			if tc.ElicitationRequest != nil {
				t.Fatal("expected nil elicitation request after processing")
			}
		})
	}
}

func TestToolCallWelcomeCancel(t *testing.T) {
	client := newTestClient(t)

	resp := client.Put("/mcp/tools/welcome/calls/"+t.Name(),
		http.Header{"Idempotency-Key": []string{time.Now().Format(time.RFC3339Nano)}},
		strings.NewReader(`{"key":"test"}`))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Create failed with status %d", resp.StatusCode)
	}

	resp = client.Post("/mcp/tools/welcome/calls/"+t.Name()+"/cancel", http.Header{}, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status %d, got %d", http.StatusOK, resp.StatusCode)
	}

	b, err := io.ReadAll(resp.Body)
	if aids.IsError(err) {
		t.Fatal(err)
	}

	tc := toolcall.Resource{}
	err = json.Unmarshal(b, &tc)
	if aids.IsError(err) {
		t.Fatal(err)
	}
	if tc.Status == nil || *tc.Status != mcp.StatusCanceled {
		t.Fatalf("expected status %q, got %v", mcp.StatusCanceled, tc.Status)
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

func TestToolCallWelcomeCancelAlreadyCompleted(t *testing.T) {
	client := newTestClient(t)

	resp := client.Put("/mcp/tools/welcome/calls/"+t.Name(),
		http.Header{"Idempotency-Key": []string{time.Now().Format(time.RFC3339Nano)}},
		strings.NewReader(`{"key":"test"}`))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Create failed with status %d", resp.StatusCode)
	}

	resp = client.Post("/mcp/tools/welcome/calls/"+t.Name()+"/advance", http.Header{},
		strings.NewReader(`{"action":"accept","content":{"name":"Jeffrey"}}`))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Advance failed with status %d", resp.StatusCode)
	}

	resp = client.Post("/mcp/tools/welcome/calls/"+t.Name()+"/cancel", http.Header{}, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status %d, got %d", http.StatusOK, resp.StatusCode)
	}

	b, err := io.ReadAll(resp.Body)
	if aids.IsError(err) {
		t.Fatal(err)
	}

	tc := toolcall.Resource{}
	err = json.Unmarshal(b, &tc)
	if aids.IsError(err) {
		t.Fatal(err)
	}
	if tc.Status == nil || *tc.Status != mcp.StatusSuccess {
		t.Fatalf("expected status %q, got %v", mcp.StatusSuccess, *tc.Status)
	}
}

package main

import (
	"context"
	"fmt"
	"net/http"

	"github.com/JeffreyRichter/mcpsvr/mcp"
	"github.com/JeffreyRichter/mcpsvr/mcp/toolcall"
	"github.com/JeffreyRichter/svrcore"
)

// ToolInfo defines the interface for tool-specific operations
type ToolInfo interface {
	// Tool returns the tool metadata.
	Tool() *mcp.Tool

	// Create creates a brand new tool call ID resource (if-none-match: *),
	// optionally starts phase processing, and writes success/error to the client.
	Create(ctx context.Context, tc *toolcall.ToolCall, r *svrcore.ReqRes, pm toolcall.PhaseMgr) error

	// Get retrieves the tool call ID resource and writes success/error to the client.
	Get(ctx context.Context, tc *toolcall.ToolCall, r *svrcore.ReqRes) error

	// Advance advances the tool call to the next phase and writes success/error to the client.
	Advance(ctx context.Context, tc *toolcall.ToolCall, r *svrcore.ReqRes) error

	// Cancel cancels the tool call and writes success/error to the client.
	Cancel(ctx context.Context, tc *toolcall.ToolCall, r *svrcore.ReqRes) error

	// ProcessPhase processes the tool call resources's current phase; there is no client to write success/error to.
	ProcessPhase(ctx context.Context, pp toolcall.PhaseProcessor, tc *toolcall.ToolCall) error
}

// defaultToolInfo provides default implementations of ToolCaller methods that all return "NotAllowed" errors.
type defaultToolInfo struct{}

func (*defaultToolInfo) Tool() *mcp.Tool { return nil }
func (*defaultToolInfo) Create(ctx context.Context, tc *toolcall.ToolCall, r *svrcore.ReqRes, pm toolcall.PhaseMgr) error {
	return r.WriteError(http.StatusMethodNotAllowed, nil, nil, "NotAllowed", "PUT not implemented for tool '%s'", *tc.ToolName)
}
func (*defaultToolInfo) Get(ctx context.Context, tc *toolcall.ToolCall, r *svrcore.ReqRes) error {
	return r.WriteError(http.StatusMethodNotAllowed, nil, nil, "NotAllowed", "GET not implemented for tool '%s'", *tc.ToolName)
}
func (*defaultToolInfo) Advance(ctx context.Context, tc *toolcall.ToolCall, r *svrcore.ReqRes) error {
	return r.WriteError(http.StatusMethodNotAllowed, nil, nil, "NotAllowed", "POST /advance not implemented for tool '%s'", *tc.ToolName)
}
func (*defaultToolInfo) Cancel(ctx context.Context, tc *toolcall.ToolCall, r *svrcore.ReqRes) error {
	return r.WriteError(http.StatusMethodNotAllowed, nil, nil, "NotAllowed", "POST /cancel not implemented for tool '%s'", *tc.ToolName)
}
func (*defaultToolInfo) ProcessPhase(ctx context.Context, pp toolcall.PhaseProcessor, tc *toolcall.ToolCall) error {
	return fmt.Errorf("ProcessPhase not implemented for tool '%s'", *tc.ToolName)
}

var _ ToolInfo = (*defaultToolInfo)(nil)

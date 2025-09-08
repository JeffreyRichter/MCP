package main

import (
	"context"
	"fmt"
	"net/http"

	"github.com/JeffreyRichter/mcpsvr/mcp"
	"github.com/JeffreyRichter/mcpsvr/mcp/toolcall"
	"github.com/JeffreyRichter/svrcore"
)

type ToolCaller interface {
	Tool() *mcp.Tool
	Create(ctx context.Context, tc *toolcall.ToolCall, r *svrcore.ReqRes, pm toolcall.PhaseMgr) error
	Get(ctx context.Context, tc *toolcall.ToolCall, r *svrcore.ReqRes) error
	Advance(ctx context.Context, tc *toolcall.ToolCall, r *svrcore.ReqRes) error
	Cancel(ctx context.Context, tc *toolcall.ToolCall, r *svrcore.ReqRes) error
	ProcessPhase(ctx context.Context, tc *toolcall.ToolCall, pp toolcall.PhaseProcessor) error
}
type defaultToolCaller struct{}

func (*defaultToolCaller) Tool() *mcp.Tool { return nil }
func (*defaultToolCaller) Create(ctx context.Context, tc *toolcall.ToolCall, r *svrcore.ReqRes, pm toolcall.PhaseMgr) error {
	return r.WriteError(http.StatusMethodNotAllowed, nil, nil, "NotAllowed", "PUT not implemented for tool '%s'", *tc.ToolName)
}
func (*defaultToolCaller) Get(ctx context.Context, tc *toolcall.ToolCall, r *svrcore.ReqRes) error {
	return r.WriteError(http.StatusMethodNotAllowed, nil, nil, "NotAllowed", "GET not implemented for tool '%s'", *tc.ToolName)
}
func (*defaultToolCaller) Advance(ctx context.Context, tc *toolcall.ToolCall, r *svrcore.ReqRes) error {
	return r.WriteError(http.StatusMethodNotAllowed, nil, nil, "NotAllowed", "POST /advance not implemented for tool '%s'", *tc.ToolName)
}
func (*defaultToolCaller) Cancel(ctx context.Context, tc *toolcall.ToolCall, r *svrcore.ReqRes) error {
	return r.WriteError(http.StatusMethodNotAllowed, nil, nil, "NotAllowed", "POST /cancel not implemented for tool '%s'", *tc.ToolName)
}
func (*defaultToolCaller) ProcessPhase(ctx context.Context, tc *toolcall.ToolCall, pp toolcall.PhaseProcessor) error {
	return fmt.Errorf("ProcessPhase not implemented for tool '%s'", *tc.ToolName)
}

var _ ToolCaller = (*defaultToolCaller)(nil)

type ToolInfo struct {
	Tool   *mcp.Tool
	Caller ToolCaller
}

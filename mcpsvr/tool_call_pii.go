package main

import (
	"context"
	"net/http"

	"github.com/JeffreyRichter/internal/aids"
	"github.com/JeffreyRichter/mcpsvr/mcp"
	"github.com/JeffreyRichter/mcpsvr/mcp/toolcall"
	"github.com/JeffreyRichter/svrcore"
)

type piiToolInfo struct {
	defaultToolInfo
	ops *mcpPolicies
}

func (c *piiToolInfo) Tool() *mcp.Tool {
	return &mcp.Tool{
		BaseMetadata: mcp.BaseMetadata{
			Name:  "pii",
			Title: aids.New("Get PII"),
		},
		Description: aids.New("Get PII data with client confirmation"),
		InputSchema: mcp.JSONSchema{
			Type:       "object",
			Properties: &map[string]any{},
			Required:   []string{},
		},
		OutputSchema: &mcp.JSONSchema{
			Type: "object",
			Properties: &map[string]any{
				"data": map[string]any{
					"type":        "string",
					"Description": aids.New("The PII data"),
				},
			},
			Required: []string{"data"},
		},
		Annotations: &mcp.ToolAnnotations{
			Title:           aids.New("Get PII"),
			ReadOnlyHint:    aids.New(true),
			DestructiveHint: aids.New(false),
			IdempotentHint:  aids.New(true),
			OpenWorldHint:   aids.New(false),
		},
		Meta: mcp.Meta{"sensitive": "true"},
	}
}

// This type block defines the tool-specific tool call resource types
type (
	PIIToolCallRequest struct {
		Key string `json:"key"`
	}

	PIIToolCallResult struct {
		Data string `json:"data"`
	}
)

// TODO: client must specify elicitation capability
func (c *piiToolInfo) Create(ctx context.Context, tc *toolcall.ToolCall, r *svrcore.ReqRes, pm toolcall.PhaseMgr) bool {
	var request PIIToolCallRequest
	if stop := r.UnmarshalBody(&request); stop {
		return stop
	}
	if request.Key == "" {
		return r.WriteError(http.StatusBadRequest, nil, nil, "BadRequest", "key is required")
	}

	tc.Request = aids.MustMarshal(request)
	tc.ElicitationRequest = &toolcall.ElicitationRequest{
		Message: "The requested data contains personal information (PII). Please approve access to this data.",
		RequestedSchema: struct {
			Type       string                                   `json:"type"`
			Properties map[string]mcp.PrimitiveSchemaDefinition `json:"properties"`
			Required   []string                                 `json:"required,omitempty"`
		}{
			Type: "object",
			Properties: map[string]mcp.PrimitiveSchemaDefinition{
				"approved": mcp.BooleanSchema{
					Type:        "boolean",
					Title:       aids.New("Approval"),
					Description: aids.New("Whether to approve PII access"),
				},
			},
			Required: []string{"approved"},
		},
	}
	tc.Status = aids.New(toolcall.StatusAwaitingElicitationResult)

	if se := c.ops.store.Put(ctx, tc, svrcore.AccessConditions{IfNoneMatch: svrcore.ETagAnyPtr}); se != nil {
		return r.WriteServerError(se, &svrcore.ResponseHeader{ETag: tc.ETag}, nil)
	}
	return r.WriteSuccess(http.StatusOK, &svrcore.ResponseHeader{ETag: tc.ETag}, nil, tc.ToClient())
}

func (c *piiToolInfo) Get(ctx context.Context, tc *toolcall.ToolCall, r *svrcore.ReqRes) bool {
	return r.WriteSuccess(http.StatusOK, &svrcore.ResponseHeader{ETag: tc.ETag}, nil, tc.ToClient())
}

func (c *piiToolInfo) Advance(ctx context.Context, tc *toolcall.ToolCall, r *svrcore.ReqRes) bool {
	if *tc.Status != toolcall.StatusAwaitingElicitationResult {
		return r.WriteError(http.StatusBadRequest, nil, nil, "BadRequest", "not expecting an elicitation result for call with status %q", *tc.Status)
	}

	var er toolcall.ElicitationResult
	if stop := r.UnmarshalBody(&er); stop {
		return stop
	}
	// All responses must specify an action; "content" is required only for "action": "accept"
	if er.Action == "" {
		return r.WriteError(http.StatusBadRequest, nil, nil, "BadRequest", "elicitation result content is required")
	}
	var approved, ok bool
	if er.Action == "accept" {
		if er.Content == nil {
			return r.WriteError(http.StatusBadRequest, nil, nil, "BadRequest", "elicitation result content is required")
		}
		// Client accepted the elicitation request, so we expect "content": {"approved": ...}
		if approved, ok = (*er.Content)["approved"].(bool); !ok {
			return r.WriteError(http.StatusBadRequest, nil, nil, "BadRequest", `missing "approved" boolean in elicitation result content`)
		}
	}
	tc.Status = aids.New(toolcall.StatusCanceled)
	if approved {
		tc.Status = aids.New(toolcall.StatusSuccess)
		tc.Result = aids.MustMarshal(PIIToolCallResult{Data: "here's your PII"})
	}
	// Drop the elicitation request because it's been processed
	tc.ElicitationRequest = nil

	if se := c.ops.store.Put(ctx, tc, svrcore.AccessConditions{IfMatch: r.H.IfMatch, IfNoneMatch: r.H.IfNoneMatch}); se != nil {
		return r.WriteServerError(se, &svrcore.ResponseHeader{ETag: tc.ETag}, nil)
	}
	return r.WriteSuccess(http.StatusOK, &svrcore.ResponseHeader{ETag: tc.ETag}, nil, tc.ToClient())
}

func (c *piiToolInfo) Cancel(ctx context.Context, tc *toolcall.ToolCall, r *svrcore.ReqRes) bool {
	switch *tc.Status {
	case toolcall.StatusSuccess, toolcall.StatusFailed, toolcall.StatusCanceled:
		return r.WriteSuccess(http.StatusOK, &svrcore.ResponseHeader{ETag: tc.ETag}, nil, tc.ToClient())
	}

	tc.Status, tc.Phase, tc.Error, tc.Result, tc.ElicitationRequest = aids.New(toolcall.StatusCanceled), nil, nil, nil, nil
	if se := c.ops.store.Put(ctx, tc, svrcore.AccessConditions{IfMatch: r.H.IfMatch, IfNoneMatch: r.H.IfNoneMatch}); se != nil {
		return r.WriteServerError(se, &svrcore.ResponseHeader{ETag: tc.ETag}, nil)
	}
	return r.WriteSuccess(http.StatusOK, &svrcore.ResponseHeader{ETag: tc.ETag}, nil, tc.ToClient())
}

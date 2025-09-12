package main

import (
	"context"
	"net/http"

	"github.com/JeffreyRichter/internal/aids"
	"github.com/JeffreyRichter/mcpsvr/mcp"
	"github.com/JeffreyRichter/mcpsvr/mcp/toolcall"
	"github.com/JeffreyRichter/svrcore"
)

type piiToolCaller struct {
	defaultToolCaller
	ops *mcpPolicies
}

type PIIToolCallRequest struct {
	Key string `json:"key"`
}

type PIIToolCallResult struct {
	Data string `json:"data"`
}

func (c *piiToolCaller) Tool() *mcp.Tool {
	return &mcp.Tool{
		BaseMetadata: mcp.BaseMetadata{
			Name:  "pii",
			Title: svrcore.Ptr("Get PII"),
		},
		Description: svrcore.Ptr("Get PII data with client confirmation"),
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
					"Description": svrcore.Ptr("The PII data"),
				},
			},
			Required: []string{"data"},
		},
		Annotations: &mcp.ToolAnnotations{
			Title:           svrcore.Ptr("Get PII"),
			ReadOnlyHint:    svrcore.Ptr(true),
			DestructiveHint: svrcore.Ptr(false),
			IdempotentHint:  svrcore.Ptr(true),
			OpenWorldHint:   svrcore.Ptr(false),
		},
		Meta: mcp.Meta{"sensitive": "true"},
	}
}

// TODO: client must specify elicitation capability
func (c *piiToolCaller) Create(ctx context.Context, tc *toolcall.ToolCall, r *svrcore.ReqRes, pm toolcall.PhaseMgr) error {
	var request PIIToolCallRequest
	if err := r.UnmarshalBody(&request); aids.IsError(err) {
		return err
	}
	if request.Key == "" {
		return r.WriteError(http.StatusBadRequest, nil, nil, "BadRequest", "key is required")
	}

	tc.Request = aids.Marshal(request)
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
					Title:       svrcore.Ptr("Approval"),
					Description: svrcore.Ptr("Whether to approve PII access"),
				},
			},
			Required: []string{"approved"},
		},
	}
	tc.Status = svrcore.Ptr(toolcall.StatusAwaitingElicitationResult)

	err := c.ops.store.Put(ctx, tc, svrcore.AccessConditions{IfMatch: r.H.IfMatch, IfNoneMatch: r.H.IfNoneMatch})
	if aids.IsError(err) {
		return err
	}
	return r.WriteSuccess(http.StatusOK, &svrcore.ResponseHeader{ETag: tc.ETag}, nil, tc)
}

func (c *piiToolCaller) Get(ctx context.Context, tc *toolcall.ToolCall, r *svrcore.ReqRes) error {
	return r.WriteSuccess(http.StatusOK, &svrcore.ResponseHeader{ETag: tc.ETag}, nil, tc)
}

func (c *piiToolCaller) Advance(ctx context.Context, tc *toolcall.ToolCall, r *svrcore.ReqRes) error {
	if *tc.Status != toolcall.StatusAwaitingElicitationResult {
		return r.WriteError(http.StatusBadRequest, nil, nil, "BadRequest", "not expecting an elicitation result for call with status %q", *tc.Status)
	}

	var er toolcall.ElicitationResult
	if err := r.UnmarshalBody(&er); aids.IsError(err) {
		return err
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
	tc.Status = svrcore.Ptr(toolcall.StatusCanceled)
	if approved {
		tc.Status = svrcore.Ptr(toolcall.StatusSuccess)
		tc.Result = aids.Marshal(PIIToolCallResult{Data: "here's your PII"})
	}
	// Drop the elicitation request because it's been processed
	tc.ElicitationRequest = nil

	if err := c.ops.store.Put(ctx, tc, svrcore.AccessConditions{IfMatch: r.H.IfMatch, IfNoneMatch: r.H.IfNoneMatch}); aids.IsError(err) {
		return err
	}
	return r.WriteSuccess(http.StatusOK, &svrcore.ResponseHeader{ETag: tc.ETag}, nil, tc)
}

func (c *piiToolCaller) Cancel(ctx context.Context, tc *toolcall.ToolCall, r *svrcore.ReqRes) error {
	switch *tc.Status {
	case toolcall.StatusSuccess, toolcall.StatusFailed, toolcall.StatusCanceled:
		return r.WriteSuccess(http.StatusOK, &svrcore.ResponseHeader{ETag: tc.ETag}, nil, tc)
	}

	tc.Status, tc.Phase, tc.Error, tc.Result, tc.ElicitationRequest = svrcore.Ptr(toolcall.StatusCanceled), nil, nil, nil, nil
	if err := c.ops.store.Put(ctx, tc, svrcore.AccessConditions{IfMatch: r.H.IfMatch, IfNoneMatch: r.H.IfNoneMatch}); aids.IsError(err) {
		return r.WriteServerError(err.(*svrcore.ServerError), nil, nil)
	}
	return r.WriteSuccess(http.StatusOK, &svrcore.ResponseHeader{ETag: tc.ETag}, nil, tc)
}

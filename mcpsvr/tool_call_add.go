package main

import (
	"context"
	"net/http"

	"github.com/JeffreyRichter/internal/aids"
	"github.com/JeffreyRichter/mcpsvr/mcp"
	"github.com/JeffreyRichter/mcpsvr/mcp/toolcall"
	"github.com/JeffreyRichter/svrcore"
)

type addToolInfo struct {
	defaultToolInfo
	ops *mcpPolicies
}

func (c *addToolInfo) Tool() *mcp.Tool {
	return &mcp.Tool{
		BaseMetadata: mcp.BaseMetadata{
			Name:  "add",
			Title: svrcore.Ptr("Add two numbers"),
		},
		Description: svrcore.Ptr("Add two numbers"),
		InputSchema: mcp.JSONSchema{
			Type: "object",
			Properties: &map[string]any{
				"x": map[string]any{
					"type":        "integer",
					"Description": svrcore.Ptr("The first number"),
				},
				"y": map[string]any{
					"type":        "integer",
					"Description": svrcore.Ptr("The second number"),
				},
			},
			Required: []string{"x", "y"},
		},
		OutputSchema: &mcp.JSONSchema{
			Type: "object",
			Properties: &map[string]any{
				"result": map[string]any{
					"type":        "integer",
					"Description": svrcore.Ptr("The result of the addition"),
				},
			},
			Required: []string{"result"},
		},
		Annotations: &mcp.ToolAnnotations{
			Title:           svrcore.Ptr("Add two numbers"),
			ReadOnlyHint:    svrcore.Ptr(false),
			DestructiveHint: svrcore.Ptr(false),
			IdempotentHint:  svrcore.Ptr(true),
			OpenWorldHint:   svrcore.Ptr(true),
		},
		Meta: mcp.Meta{"foo": "bar", "baz": "qux"},
	}
}

// This type block defines the tool-specific tool call resource types
type (
	addToolCallRequest struct {
		X int `json:"x,omitempty"`
		Y int `json:"y,omitempty"`
	}

	addToolCallResult struct {
		Sum int `json:"sum,omitempty"`
	}
)

// Create creates a brand new tool call ID resource.
// It must ensure that an existing resource does not already exist (for HTTP, use "if-none-match: *")
// If a resource already exists, return 409-Conflict
func (c *addToolInfo) Create(ctx context.Context, tc *toolcall.ToolCall, r *svrcore.ReqRes, pm toolcall.PhaseMgr) error {
	var trequest addToolCallRequest
	if err := r.UnmarshalBody(&trequest); aids.IsError(err) {
		return err
	}
	tc.Request = aids.MustMarshal(trequest)
	tc.Status = svrcore.Ptr(toolcall.StatusSuccess)
	tc.Result = aids.MustMarshal(&addToolCallResult{Sum: trequest.X + trequest.Y})

	// Create the resource; on success, the ToolCall.ETag field is updated from the response ETag
	if err := c.ops.store.Put(ctx, tc, svrcore.AccessConditions{IfNoneMatch: svrcore.ETagAnyPtr}); aids.IsError(err) {
		return r.WriteServerError(err.(*svrcore.ServerError), nil, nil)
	}
	return r.WriteSuccess(http.StatusOK, &svrcore.ResponseHeader{ETag: tc.ETag}, nil, tc.ToClient())
}

func (c *addToolInfo) Get(ctx context.Context, tc *toolcall.ToolCall, r *svrcore.ReqRes) error {
	return r.WriteSuccess(http.StatusOK, &svrcore.ResponseHeader{ETag: tc.ETag}, nil, tc.ToClient())
}

func (c *addToolInfo) Advance(ctx context.Context, tc *toolcall.ToolCall, r *svrcore.ReqRes) error {
	err := c.ops.store.Get(ctx, tc, svrcore.AccessConditions{IfMatch: r.H.IfMatch, IfNoneMatch: r.H.IfNoneMatch})
	if aids.IsError(err) {
		return err
	}
	if err := r.CheckPreconditions(svrcore.ResourceValues{AllowedConditionals: svrcore.AllowedConditionalsMatch, ETag: tc.ETag}); aids.IsError(err) {
		return err
	}
	switch *tc.Status {
	case toolcall.StatusAwaitingElicitationResult:
		var er toolcall.ElicitationResult
		err := r.UnmarshalBody(&er)
		if aids.IsError(err) {
			return err
		}
		// TODO: Process the er, update progress?, update status, update result/error
		tc.Status = svrcore.Ptr(toolcall.StatusSuccess)

	case toolcall.StatusAwaitingSamplingResult:
		var sr toolcall.SamplingResult
		err := r.UnmarshalBody(&sr)
		if aids.IsError(err) {
			return err
		}
		// TODO: Process the sr, update progress?, update status, update result/error
	default:
		return r.WriteError(http.StatusBadRequest, nil, nil, "BadRequest", "tool call status is '%s'; not expecting a result", *tc.Status)
	}

	err = c.ops.store.Put(ctx, tc, svrcore.AccessConditions{IfMatch: r.H.IfMatch, IfNoneMatch: r.H.IfNoneMatch}) // Update the resource
	if aids.IsError(err) {
		return err
	}
	return r.WriteSuccess(http.StatusOK, &svrcore.ResponseHeader{ETag: tc.ETag}, nil, tc.ToClient())
}

func (c *addToolInfo) Cancel(ctx context.Context, tc *toolcall.ToolCall, r *svrcore.ReqRes) error {
	/*
		1.	GET ToolCall resource/etag from client-passed ID
		2.	If status==terminal, return
		3.	Set status=Cancelled
		4.	Update resource (if-match:etag)
		a.	If update succeeds, send notification to ToolCallNotification queue
		b.	Else, go to Step #1 to retry. The race is OK because of #2.

	*/
	body := any(nil)
	return r.WriteSuccess(http.StatusOK, &svrcore.ResponseHeader{ETag: c.ops.etag()}, nil, body)
}

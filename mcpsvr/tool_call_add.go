package main

import (
	"context"
	"net/http"

	"github.com/JeffreyRichter/internal/aids"
	"github.com/JeffreyRichter/mcp"
	"github.com/JeffreyRichter/mcpsvr/toolcall"
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
			Title: aids.New("Add two numbers"),
		},
		Description: aids.New("Add two numbers"),
		InputSchema: mcp.JSONSchema{
			Type: "object",
			Properties: &map[string]any{
				"x": map[string]any{
					"type":        "integer",
					"Description": aids.New("The first number"),
				},
				"y": map[string]any{
					"type":        "integer",
					"Description": aids.New("The second number"),
				},
			},
			Required: []string{"x", "y"},
		},
		OutputSchema: &mcp.JSONSchema{
			Type: "object",
			Properties: &map[string]any{
				"result": map[string]any{
					"type":        "integer",
					"Description": aids.New("The result of the addition"),
				},
			},
			Required: []string{"result"},
		},
		Annotations: &mcp.ToolAnnotations{
			Title:           aids.New("Add two numbers"),
			ReadOnlyHint:    aids.New(false),
			DestructiveHint: aids.New(false),
			IdempotentHint:  aids.New(true),
			OpenWorldHint:   aids.New(true),
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
func (c *addToolInfo) Create(ctx context.Context, tc *toolcall.Resource, r *svrcore.ReqRes, pm toolcall.PhaseMgr) bool {
	var trequest addToolCallRequest
	if stop := r.UnmarshalBody(&trequest); stop {
		return stop
	}
	tc.Request = aids.MustMarshal(trequest)
	tc.Status = aids.New(mcp.StatusSuccess)
	tc.Result = aids.MustMarshal(&addToolCallResult{Sum: trequest.X + trequest.Y})

	// Create the resource; on success, the ToolCall.ETag field is updated from the response ETag
	if se := c.ops.store.Put(ctx, tc, svrcore.AccessConditions{IfNoneMatch: svrcore.ETagAnyPtr}); se != nil {
		return r.WriteServerError(se, nil, nil)
	}
	return r.WriteSuccess(http.StatusOK, &svrcore.ResponseHeader{ETag: tc.ETag}, nil, tc.ToMCP())
}

func (c *addToolInfo) Get(ctx context.Context, tc *toolcall.Resource, r *svrcore.ReqRes) bool {
	return r.WriteSuccess(http.StatusOK, &svrcore.ResponseHeader{ETag: tc.ETag}, nil, tc.ToMCP())
}

func (c *addToolInfo) Advance(ctx context.Context, tc *toolcall.Resource, r *svrcore.ReqRes) bool {
	se := c.ops.store.Get(ctx, tc, svrcore.AccessConditions{IfMatch: r.H.IfMatch, IfNoneMatch: r.H.IfNoneMatch})
	if se != nil {
		return r.WriteServerError(se, &svrcore.ResponseHeader{ETag: tc.ETag}, nil)
	}
	if stop := r.CheckPreconditions(svrcore.ResourceValues{AllowedConditionals: svrcore.AllowedConditionalsMatch, ETag: tc.ETag}); stop {
		return stop
	}
	switch *tc.Status {
	case mcp.StatusAwaitingElicitationResult:
		var er mcp.ElicitationResult
		if stop := r.UnmarshalBody(&er); stop {
			return stop
		}
		// TODO: Process the er, update progress?, update status, update result/error
		tc.Status = aids.New(mcp.StatusSuccess)

	case mcp.StatusAwaitingSamplingResult:
		var sr mcp.SamplingResult
		if stop := r.UnmarshalBody(&sr); stop {
			return stop
		}
		// TODO: Process the sr, update progress?, update status, update result/error
	default:
		return r.WriteError(http.StatusBadRequest, nil, nil, "BadRequest", "tool call status is '%s'; not expecting a result", *tc.Status)
	}

	se = c.ops.store.Put(ctx, tc, svrcore.AccessConditions{IfMatch: r.H.IfMatch, IfNoneMatch: r.H.IfNoneMatch}) // Update the resource
	if se != nil {
		return r.WriteServerError(se, &svrcore.ResponseHeader{ETag: tc.ETag}, nil)
	}
	return r.WriteSuccess(http.StatusOK, &svrcore.ResponseHeader{ETag: tc.ETag}, nil, tc.ToMCP())
}

func (c *addToolInfo) Cancel(ctx context.Context, tc *toolcall.Resource, r *svrcore.ReqRes) bool {
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

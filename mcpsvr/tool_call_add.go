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
	tc.Expiration = nil
	tc.Result = aids.MustMarshal(&addToolCallResult{Sum: trequest.X + trequest.Y})
	// Add is a simple ephemeral tool call so we do NOT put it in the Store
	return r.WriteSuccess(http.StatusOK, &svrcore.ResponseHeader{ETag: tc.ETag}, nil, tc.ToMCP())
}

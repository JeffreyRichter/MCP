package main

import (
	"context"
	"fmt"
	"net/http"

	"github.com/JeffreyRichter/internal/aids"
	"github.com/JeffreyRichter/mcp"
	"github.com/JeffreyRichter/mcpsvr/toolcall"
	"github.com/JeffreyRichter/svrcore"
)

type welcomeToolInfo struct {
	defaultToolInfo
	ops *mcpPolicies
}

func (c *welcomeToolInfo) Tool() *mcp.Tool {
	return &mcp.Tool{
		BaseMetadata: mcp.BaseMetadata{
			Name:  "welcome",
			Title: aids.New("Send a welcome message"),
		},
		Description: aids.New("Creates a welcome message for a user, eliciting the user's name."),
		InputSchema: mcp.JSONSchema{}, // No input parameters
		OutputSchema: &mcp.JSONSchema{
			Type: "object",
			Properties: &map[string]any{
				"message": map[string]any{
					"type":        "string",
					"Description": aids.New("The welcome message"),
				},
			},
			Required: []string{"message"},
		},
	}
}

// This type block defines the tool-specific tool call resource types
type (
	welcomeToolCallResult struct {
		Welcome string `json:"welcome"`
	}
)

// TODO: client must specify elicitation capability
func (c *welcomeToolInfo) Create(ctx context.Context, tc *toolcall.Resource, r *svrcore.ReqRes, pm toolcall.PhaseMgr) bool {
	tc.ElicitationRequest = &mcp.ElicitationRequest{
		Message: "Need name for welcome message.",
		RequestedSchema: struct {
			Type       string                                   `json:"type"`
			Properties map[string]mcp.PrimitiveSchemaDefinition `json:"properties"`
			Required   []string                                 `json:"required,omitempty"`
		}{
			Type: "object",
			Properties: map[string]mcp.PrimitiveSchemaDefinition{
				"name": mcp.StringSchema{
					Type:        "string",
					Title:       aids.New("Name"),
					Description: aids.New("The name for the welcome message."),
					MinLength:   aids.New(1),
					MaxLength:   aids.New(100),
					Format:      aids.New("name"),
				},
			},
			Required: []string{"name"},
		},
	}
	tc.Status = aids.New(mcp.StatusAwaitingElicitationResult)

	if se := c.ops.store.Put(ctx, tc, svrcore.AccessConditions{IfNoneMatch: svrcore.ETagAnyPtr}); se != nil {
		return r.WriteServerError(se, &svrcore.ResponseHeader{ETag: tc.ETag}, nil)
	}
	return r.WriteSuccess(http.StatusOK, &svrcore.ResponseHeader{ETag: tc.ETag}, nil, tc.ToMCP())
}

func (c *welcomeToolInfo) Get(ctx context.Context, tc *toolcall.Resource, r *svrcore.ReqRes) bool {
	return r.WriteSuccess(http.StatusOK, &svrcore.ResponseHeader{ETag: tc.ETag}, nil, tc.ToMCP())
}

func (c *welcomeToolInfo) Advance(ctx context.Context, tc *toolcall.Resource, r *svrcore.ReqRes) bool {
	if *tc.Status != mcp.StatusAwaitingElicitationResult {
		return r.WriteError(http.StatusBadRequest, nil, nil, "BadRequest", "not expecting an elicitation result for call with status %q", *tc.Status)
	}

	var er mcp.ElicitationResult
	if stop := r.UnmarshalBody(&er); stop {
		return stop
	}

	switch er.Action {
	case "accept": // User explicitly approved and submitted with data
		if er.Content == nil {
			return r.WriteError(http.StatusBadRequest, nil, nil, "BadRequest", "elicitation result: missing content value")
		}
		// We expect "content": {"name": ...}
		if name, ok := (*er.Content)["name"].(string); !ok {
			return r.WriteError(http.StatusBadRequest, nil, nil, "BadRequest", `elicitationresult: missing content "name" string`)
		} else {
			result := welcomeToolCallResult{Welcome: fmt.Sprintf("Hello %v, nice to meet you!", name)}
			tc.Result = aids.MustMarshal(result)
		}
		tc.Status, tc.ElicitationRequest = aids.New(mcp.StatusSuccess), nil

	case "decline": // User explicitly declined the request
		result := welcomeToolCallResult{Welcome: "Hello anonymous, nice to meet you!"}
		tc.Result = aids.MustMarshal(result)
		tc.Status, tc.ElicitationRequest = aids.New(mcp.StatusSuccess), nil

	case "cancel": // User dismissed without making an explicit choice
		tc.Status, tc.ElicitationRequest, tc.Result = aids.New(mcp.StatusCanceled), nil, nil

	default:
		return r.WriteError(http.StatusBadRequest, nil, nil, "BadRequest", "elicitation result: invalid Action must be 'accept', 'reject', or 'decline'.")
	}

	if se := c.ops.store.Put(ctx, tc, svrcore.AccessConditions{IfMatch: r.H.IfMatch, IfNoneMatch: r.H.IfNoneMatch}); se != nil {
		return r.WriteServerError(se, &svrcore.ResponseHeader{ETag: tc.ETag}, nil)
	}
	return r.WriteSuccess(http.StatusOK, &svrcore.ResponseHeader{ETag: tc.ETag}, nil, tc.ToMCP())
}

func (c *welcomeToolInfo) Cancel(ctx context.Context, tc *toolcall.Resource, r *svrcore.ReqRes) bool {
	switch *tc.Status {
	case mcp.StatusSuccess, mcp.StatusFailed, mcp.StatusCanceled:
		return r.WriteSuccess(http.StatusOK, &svrcore.ResponseHeader{ETag: tc.ETag}, nil, tc.ToMCP())
	}

	tc.Status, tc.Phase, tc.Error, tc.Result, tc.ElicitationRequest = aids.New(mcp.StatusCanceled), nil, nil, nil, nil
	if se := c.ops.store.Put(ctx, tc, svrcore.AccessConditions{IfMatch: r.H.IfMatch, IfNoneMatch: r.H.IfNoneMatch}); se != nil {
		return r.WriteServerError(se, &svrcore.ResponseHeader{ETag: tc.ETag}, nil)
	}
	return r.WriteSuccess(http.StatusOK, &svrcore.ResponseHeader{ETag: tc.ETag}, nil, tc.ToMCP())
}

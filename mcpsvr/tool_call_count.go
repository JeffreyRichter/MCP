package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/JeffreyRichter/internal/aids"
	"github.com/JeffreyRichter/mcpsvr/mcp"
	"github.com/JeffreyRichter/mcpsvr/mcp/toolcall"
	"github.com/JeffreyRichter/svrcore"
)

type countToolInfo struct {
	defaultToolInfo
	ops *mcpPolicies
}

func (c *countToolInfo) Tool() *mcp.Tool {
	return &mcp.Tool{
		BaseMetadata: mcp.BaseMetadata{
			Name:  "count",
			Title: aids.New("Count up from an integer"),
		},
		Description: aids.New("Count from a starting value, adding 1 to it for the specified number of increments"),
		InputSchema: mcp.JSONSchema{
			Type: "object",
			Properties: &map[string]any{
				"start": map[string]any{
					"type":        "integer",
					"Description": aids.New("The starting value"),
				},
				"increments": map[string]any{
					"type":        "integer",
					"Description": aids.New("The number of increments to perform"),
				},
			},
			Required: []string{},
		},
		OutputSchema: &mcp.JSONSchema{
			Type: "object",
			Properties: &map[string]any{
				"n": map[string]any{
					"type":        "integer",
					"Description": aids.New("The final count"),
				},
				"text": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "string",
					},
					"Description": aids.New("The text output array"),
				},
			},
			Required: []string{"n", "text"},
		},
		Annotations: &mcp.ToolAnnotations{
			Title:           aids.New("Count a specified number of increments"),
			ReadOnlyHint:    aids.New(false),
			DestructiveHint: aids.New(false),
			IdempotentHint:  aids.New(true),
			OpenWorldHint:   aids.New(true),
		},
	}
}

// This type block defines the tool-specific tool call resource types
type (
	countToolCallRequest struct {
		CountTo int `json:"countto,omitempty"`
	}

	countToolCallResult struct {
		Count   int      `json:"count"`
		Updates []string `json:"updates"`
	}
)

func (c *countToolInfo) Create(ctx context.Context, tc *toolcall.ToolCall, r *svrcore.ReqRes, pm toolcall.PhaseMgr) bool {
	var request countToolCallRequest
	if stop := r.UnmarshalBody(&request); stop {
		return stop
	}
	tc.Request = aids.MustMarshal(request)
	tc.Status = aids.New(toolcall.StatusRunning)
	result := countToolCallResult{
		Count:   0,
		Updates: []string{fmt.Sprintf("Started: %s", time.Now().Format(time.DateTime))},
	}
	tc.Phase = aids.New(fmt.Sprintf("Phase-%c", 'A'+result.Count))
	tc.Result = aids.MustMarshal(result)
	se := c.ops.store.Put(ctx, tc, svrcore.AccessConditions{IfNoneMatch: svrcore.ETagAnyPtr})
	if se != nil {
		return r.WriteServerError(se, nil, nil)
	}
	if se := pm.StartPhase(ctx, tc); se != nil {
		return r.WriteServerError(se, nil, nil)
	}
	return r.WriteSuccess(http.StatusOK, &svrcore.ResponseHeader{ETag: tc.ETag}, nil, tc.ToClient())
}

func (c *countToolInfo) Get(ctx context.Context, tc *toolcall.ToolCall, r *svrcore.ReqRes) bool {
	return r.WriteSuccess(http.StatusOK, &svrcore.ResponseHeader{ETag: tc.ETag}, nil, tc.ToClient())
}

// Cancel the tool call if it is running; otherwise, do nothing
func (c *countToolInfo) Cancel(ctx context.Context, tc *toolcall.ToolCall, r *svrcore.ReqRes) bool {
	switch *tc.Status {
	case toolcall.StatusSuccess, toolcall.StatusFailed, toolcall.StatusCanceled:
		return r.WriteSuccess(http.StatusOK, &svrcore.ResponseHeader{ETag: tc.ETag}, nil, tc.ToClient())
	}

	tc.Status, tc.Phase, tc.Error, tc.Result, tc.ElicitationRequest = aids.New(toolcall.StatusCanceled), nil, nil, nil, nil
	if se := c.ops.store.Put(ctx, tc, svrcore.AccessConditions{IfMatch: r.H.IfMatch, IfNoneMatch: r.H.IfNoneMatch}); se != nil {
		return r.WriteServerError(se, nil, nil)
	}
	return r.WriteSuccess(http.StatusOK, &svrcore.ResponseHeader{ETag: tc.ETag}, nil, tc.ToClient())
}

// ProcessPhase advanced the tool call's current phase to its next phase.
// Return nil to have the updated tc persisted to the tool call Store.
func (c *countToolInfo) ProcessPhase(_ context.Context, _ toolcall.PhaseProcessor, tc *toolcall.ToolCall) {
	time.Sleep(150 * time.Millisecond) // Simulate doing work

	request := aids.MustUnmarshal[countToolCallRequest](tc.Request)
	result := aids.MustUnmarshal[countToolCallResult](tc.Result) // Update the result
	result.Count++
	result.Updates = append(result.Updates, fmt.Sprintf("Incremented: %s", time.Now().Format(time.DateTime)))
	tc.Result = aids.MustMarshal(result)
	tc.Phase = aids.New(fmt.Sprintf("Phase-%c", 'A'+result.Count))
	if result.Count >= request.CountTo {
		tc.Status, tc.Phase = aids.New(toolcall.StatusSuccess), nil
	}
	se := c.ops.store.Put(context.TODO(), tc, svrcore.AccessConditions{IfMatch: tc.ETag})
	aids.Assert(se == nil, fmt.Errorf("failed to put tool call resource: %w", se))
}

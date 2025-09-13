package main

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/JeffreyRichter/internal/aids"
	"github.com/JeffreyRichter/mcpsvr/mcp"
	"github.com/JeffreyRichter/mcpsvr/mcp/toolcall"
	"github.com/JeffreyRichter/svrcore"
)

type countToolCaller struct {
	defaultToolCaller
	ops *mcpPolicies
}

type CountToolCallRequest struct {
	Start      int `json:"start,omitempty"`
	Increments int `json:"increments,omitempty"`
}

// TODO
type CountToolCallProgress struct {
	Count int `json:"count,omitempty"`
	Max   int `json:"max,omitempty"`
}

type CountToolCallResult struct {
	N    int      `json:"n,omitempty"`
	Text []string `json:"text,omitempty"`
}

type CountToolCallError struct {
	Overflow bool `json:"overflowcode,omitempty"`
}

func (c *countToolCaller) Tool() *mcp.Tool {
	return &mcp.Tool{
		BaseMetadata: mcp.BaseMetadata{
			Name:  "count",
			Title: svrcore.Ptr("Count up from an integer"),
		},
		Description: svrcore.Ptr("Count from a starting value, adding 1 to it for the specified number of increments"),
		InputSchema: mcp.JSONSchema{
			Type: "object",
			Properties: &map[string]any{
				"start": map[string]any{
					"type":        "integer",
					"Description": svrcore.Ptr("The starting value"),
				},
				"increments": map[string]any{
					"type":        "integer",
					"Description": svrcore.Ptr("The number of increments to perform"),
				},
			},
			Required: []string{},
		},
		OutputSchema: &mcp.JSONSchema{
			Type: "object",
			Properties: &map[string]any{
				"n": map[string]any{
					"type":        "integer",
					"Description": svrcore.Ptr("The final count"),
				},
				"text": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "string",
					},
					"Description": svrcore.Ptr("The text output array"),
				},
			},
			Required: []string{"n", "text"},
		},
		Annotations: &mcp.ToolAnnotations{
			Title:           svrcore.Ptr("Count a specified number of increments"),
			ReadOnlyHint:    svrcore.Ptr(false),
			DestructiveHint: svrcore.Ptr(false),
			IdempotentHint:  svrcore.Ptr(true),
			OpenWorldHint:   svrcore.Ptr(true),
		},
	}
}

func (c *countToolCaller) Create(ctx context.Context, tc *toolcall.ToolCall, r *svrcore.ReqRes, pm toolcall.PhaseMgr) error {
	var request CountToolCallRequest
	if err := r.UnmarshalBody(&request); aids.IsError(err) {
		return err
	}
	tc.Request = aids.MustMarshal(request)
	tc.Status = svrcore.Ptr(toolcall.StatusRunning)
	tc.Phase = svrcore.Ptr(strconv.Itoa(request.Increments))
	tc.Result = aids.MustMarshal(CountToolCallResult{
		N:    request.Start,
		Text: []string{fmt.Sprintf("Starting at %d", request.Start)},
	})
	err := c.ops.store.Put(ctx, tc, svrcore.AccessConditions{IfMatch: r.H.IfMatch, IfNoneMatch: r.H.IfNoneMatch})
	if aids.IsError(err) {
		return r.WriteServerError(err.(*svrcore.ServerError), nil, nil)
	}
	if err := pm.StartPhase(ctx, tc); aids.IsError(err) {
		return r.WriteServerError(err.(*svrcore.ServerError), nil, nil)
	}
	return r.WriteSuccess(http.StatusOK, &svrcore.ResponseHeader{ETag: tc.ETag}, nil, tc)
}

func (c *countToolCaller) Get(ctx context.Context, tc *toolcall.ToolCall, r *svrcore.ReqRes) error {
	return r.WriteSuccess(http.StatusOK, &svrcore.ResponseHeader{ETag: tc.ETag}, nil, tc)
}

// Cancel the tool call if it is running; otherwise, do nothing
func (c *countToolCaller) Cancel(ctx context.Context, tc *toolcall.ToolCall, r *svrcore.ReqRes) error {
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

// ProcessPhase advanced the tool call's current phase to its next phase.
// Return nil to have the updated tc persisted to the tool call Store.
func (c *countToolCaller) ProcessPhase(_ context.Context, _ toolcall.PhaseProcessor, tc *toolcall.ToolCall) error {
	time.Sleep(17 * time.Millisecond) // Simulate doing work
	startPhase := aids.Must(strconv.Atoi(*tc.Phase))
	tc.Phase = svrcore.Ptr(strconv.Itoa(startPhase - 1))
	// If you need the request data: request := aids.Unmarshal[CountToolCallRequest](tc.Request)

	result := aids.MustUnmarshal[CountToolCallResult](tc.Result) // Update the result
	result.Text = append(result.Text, fmt.Sprintf("Phase advanced at %s", time.Now().Format(time.DateTime)))
	if startPhase <= 0 {
		tc.Status, tc.Phase, result.N = svrcore.Ptr(toolcall.StatusSuccess), nil, 42
	}
	tc.Result = aids.MustMarshal(result)
	return nil
}

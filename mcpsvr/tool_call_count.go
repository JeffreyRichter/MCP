package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

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
	N int `json:"n,omitempty"`
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
			},
			Required: []string{"n"},
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
	var trequest CountToolCallRequest
	if err := r.UnmarshalBody(&trequest); isError(err) {
		return err
	}
	tc.Request = must(json.Marshal(trequest))

	tc.Status = svrcore.Ptr(toolcall.StatusRunning)
	tc.Phase = svrcore.Ptr(strconv.Itoa(trequest.Increments))
	err := c.ops.store.Put(ctx, tc, svrcore.AccessConditions{IfMatch: r.H.IfMatch, IfNoneMatch: r.H.IfNoneMatch})
	if isError(err) {
		return err
	}
	go pm.StartPhase(context.TODO(), tc)
	return r.WriteSuccess(http.StatusOK, &svrcore.ResponseHeader{ETag: tc.ETag}, nil, tc)
}

func (c *countToolCaller) Get(ctx context.Context, tc *toolcall.ToolCall, r *svrcore.ReqRes) error {
	err := c.ops.store.Get(ctx, tc, svrcore.AccessConditions{IfMatch: r.H.IfMatch, IfNoneMatch: r.H.IfNoneMatch})
	// TODO: Fix up 304-Not Modified
	if isError(err) {
		return err
	}
	return r.WriteSuccess(http.StatusOK, &svrcore.ResponseHeader{ETag: tc.ETag}, nil, tc)
}

// TODO: could all tool calls use the same cancel method?
func (c *countToolCaller) Cancel(ctx context.Context, tc *toolcall.ToolCall, r *svrcore.ReqRes) error {
	err := c.ops.store.Get(ctx, tc, svrcore.AccessConditions{IfMatch: r.H.IfMatch, IfNoneMatch: r.H.IfNoneMatch})
	if isError(err) {
		return err
	}
	if err = r.CheckPreconditions(svrcore.ResourceValues{ETag: tc.ETag}); isError(err) {
		return err
	}
	if tc.Status == nil {
		return r.WriteError(http.StatusInternalServerError, nil, nil, "InternalServerError", "tool call status is nil; can't cancel")
	}
	switch *tc.Status {
	case toolcall.StatusSuccess, toolcall.StatusFailed, toolcall.StatusCanceled:
		return r.WriteSuccess(http.StatusOK, &svrcore.ResponseHeader{ETag: tc.ETag}, nil, tc)
	}

	tc.ElicitationRequest, tc.Error, tc.Result, tc.Status = nil, nil, nil, svrcore.Ptr(toolcall.StatusCanceled)
	if err = c.ops.store.Put(ctx, tc, svrcore.AccessConditions{IfMatch: r.H.IfMatch, IfNoneMatch: r.H.IfNoneMatch}); isError(err) {
		return err
	}
	return r.WriteSuccess(http.StatusOK, &svrcore.ResponseHeader{ETag: tc.ETag}, nil, tc)
}

// TODO: this belongs on v20250808.httpOperations or at least in v20250808 but can't be there now because
// the phase manager needs to reference this function at construction; see GetPhaseManager above. Overall
// this arrangement is awkward because the phase manager is nominally version-independent but in practice
// needs to know about specific tool calls
func (c *countToolCaller) ProcessPhase(_ context.Context, tc *toolcall.ToolCall, _ toolcall.PhaseProcessor) error {
	phase, err := strconv.Atoi(*tc.Phase)
	if isError(err) {
		return fmt.Errorf("invalid phase %q", *tc.Phase)
	}

	time.Sleep(17 * time.Millisecond) // Simulate doing work
	phase--
	tc.Phase = svrcore.Ptr(strconv.Itoa(phase))
	// TODO: if we needed data from the client request e.g. CountToolCallRequest here, we'd have to unmarshal it again
	if phase <= 0 {
		tc.Status = svrcore.Ptr(toolcall.StatusSuccess)
		tc.Result = must(json.Marshal(struct{ N int }{N: 42}))
	}
	return nil
}

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"time"

	"github.com/JeffreyRichter/mcpsvr/mcp"
	"github.com/JeffreyRichter/mcpsvr/mcp/toolcall"
	"github.com/JeffreyRichter/svrcore"
)

type addToolCaller struct {
	//defaultToolCaller
	ops *httpOps
}
type AddToolCallRequest struct {
	X int `json:"x,omitempty"`
	Y int `json:"y,omitempty"`
}

type AddToolCallProgress struct {
	Count int `json:"count,omitempty"`
	Max   int `json:"max,omitempty"`
}

type AddToolCallResult struct {
	Sum int `json:"sum,omitempty"`
}

type AddToolCallError struct {
	Overflow bool `json:"overflowcode,omitempty"`
}

func (c *addToolCaller) Tool() *mcp.Tool {
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

func (c *addToolCaller) Create(ctx context.Context, tc *toolcall.ToolCall, r *svrcore.ReqRes, pm toolcall.PhaseMgr) error {
	var trequest AddToolCallRequest
	if err := r.UnmarshalBody(&trequest); err != nil {
		return err
	}
	tc.Request = must(json.Marshal(trequest))

	tc.Status = svrcore.Ptr(toolcall.StatusSuccess)
	tresult := &AddToolCallResult{Sum: trequest.X + trequest.Y}
	tc.Result = must(json.Marshal(tresult))

	// simulate this tool call requiring some effort
	d := time.Duration(5 + rand.Intn(1500))
	fmt.Fprintf(os.Stderr, "[%s] blocking for %dms\n", *tc.ID, d)
	time.Sleep(d * time.Millisecond)

	err := c.ops.store.Put(ctx, tc, svrcore.AccessConditions{IfMatch: r.H.IfMatch, IfNoneMatch: r.H.IfNoneMatch}) // Create/replace the resource
	if err != nil {
		return err
	}
	return r.WriteSuccess(http.StatusOK, &svrcore.ResponseHeader{ETag: tc.ETag}, nil, tc)
}

func (c *addToolCaller) Get(ctx context.Context, tc *toolcall.ToolCall, r *svrcore.ReqRes) error {
	err := c.ops.store.Get(ctx, tc, svrcore.AccessConditions{IfMatch: r.H.IfMatch, IfNoneMatch: r.H.IfNoneMatch})
	// TODO: Fix up 304-Not Modified
	if err != nil {
		return err
	}
	return r.WriteSuccess(http.StatusOK, &svrcore.ResponseHeader{ETag: tc.ETag}, nil, tc)
}

func (c *addToolCaller) Advance(ctx context.Context, tc *toolcall.ToolCall, r *svrcore.ReqRes) error {
	err := c.ops.store.Get(ctx, tc, svrcore.AccessConditions{IfMatch: r.H.IfMatch, IfNoneMatch: r.H.IfNoneMatch})
	if err != nil {
		return err
	}
	if err := r.CheckPreconditions(svrcore.ResourceValues{AllowedConditionals: svrcore.AllowedConditionalsMatch, ETag: tc.ETag}); err != nil {
		return err
	}
	switch *tc.Status {
	case toolcall.StatusAwaitingElicitationResult:
		var er toolcall.ElicitationResult
		err := r.UnmarshalBody(&er)
		if err != nil {
			return err
		}
		// TODO: Process the er, update progress?, update status, update result/error
		tc.Status = svrcore.Ptr(toolcall.StatusSuccess)

	case toolcall.StatusAwaitingSamplingResult:
		var sr toolcall.SamplingResult
		err := r.UnmarshalBody(&sr)
		if err != nil {
			return err
		}
		// TODO: Process the sr, update progress?, update status, update result/error
	default:
		return r.WriteError(http.StatusBadRequest, nil, nil, "BadRequest", "tool call status is '%s'; not expecting a result", *tc.Status)
	}

	err = c.ops.store.Put(ctx, tc, svrcore.AccessConditions{IfMatch: r.H.IfMatch, IfNoneMatch: r.H.IfNoneMatch}) // Update the resource
	if err != nil {
		return err
	}
	return r.WriteSuccess(http.StatusOK, &svrcore.ResponseHeader{ETag: tc.ETag}, nil, tc)
}

func (c *addToolCaller) Cancel(ctx context.Context, tc *toolcall.ToolCall, r *svrcore.ReqRes) error {
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

/////////////////////////////////////////////////////////////////////////

func (c *addToolCaller) ProcessPhase(ctx context.Context, tc *toolcall.ToolCall, pp toolcall.PhaseProcessor) error {
	switch *tc.Phase {
	case "submitted":
		// Do work
		tc.Phase = svrcore.Ptr("one")
		tc.Status = svrcore.Ptr(toolcall.StatusRunning)
		return nil

	case "one":
		// Do work
		pp.ExtendTime(ctx, time.Millisecond*300)
		tc.Status = svrcore.Ptr(toolcall.StatusSuccess)
		tc.Phase = (*string)(tc.Status) // No more phases
		return nil
	}
	// TODO: Fix the error
	panic(fmt.Sprintf("Unknown phase: %s", *tc.Phase))
}

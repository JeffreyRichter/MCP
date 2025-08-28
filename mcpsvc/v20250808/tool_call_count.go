package v20250808

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/JeffreyRichter/mcpsvc/mcp/toolcalls"
	"github.com/JeffreyRichter/mcpsvc/resources"
	si "github.com/JeffreyRichter/serviceinfra"
)

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

func (ops *httpOperations) createToolCallCount(ctx context.Context, tc *toolcalls.ToolCall, r *si.ReqRes) error {
	var trequest CountToolCallRequest
	if err := r.UnmarshalBody(&trequest); err != nil {
		return err
	}
	tc.Request = must(json.Marshal(trequest))

	tc.Status = si.Ptr(toolcalls.ToolCallStatusRunning)
	tc.Phase = si.Ptr(strconv.Itoa(trequest.Increments))
	tc, err := ops.Put(ctx, tc, &toolcalls.AccessConditions{IfMatch: r.H.IfMatch, IfNoneMatch: r.H.IfNoneMatch})
	if err != nil {
		return err
	}
	pm := resources.GetPhaseManager()
	go pm.StartPhaseProcessing(context.TODO(), tc, 5*time.Second)
	return r.WriteResponse(&si.ResponseHeader{ETag: tc.ETag}, nil, http.StatusOK, tc)
}

func (ops *httpOperations) getToolCallCount(ctx context.Context, tc *toolcalls.ToolCall, r *si.ReqRes) error {
	tc, err := ops.Get(ctx, tc, &toolcalls.AccessConditions{IfMatch: r.H.IfMatch, IfNoneMatch: r.H.IfNoneMatch})
	if err != nil {
		return err
	}
	return r.WriteResponse(&si.ResponseHeader{ETag: tc.ETag}, nil, http.StatusOK, tc)
}

// TODO: could all tool calls use the same cancel method?
func (ops *httpOperations) cancelToolCallCount(ctx context.Context, tc *toolcalls.ToolCall, r *si.ReqRes) error {
	tc, err := ops.Get(ctx, tc, &toolcalls.AccessConditions{IfMatch: r.H.IfMatch, IfNoneMatch: r.H.IfNoneMatch})
	if err != nil {
		return err
	}
	if err = r.ValidatePreconditions(&si.PreconditionValues{ETag: tc.ETag}); err != nil {
		return err
	}
	if tc.Status == nil {
		return r.Error(http.StatusInternalServerError, "InternalServerError", "tool call status is nil; can't cancel")
	}
	switch *tc.Status {
	case toolcalls.ToolCallStatusSuccess, toolcalls.ToolCallStatusFailed, toolcalls.ToolCallStatusCanceled:
		return r.WriteResponse(&si.ResponseHeader{ETag: tc.ETag}, nil, http.StatusOK, tc)
	}

	tc.ElicitationRequest = nil
	tc.Error = nil
	tc.Result = nil
	tc.Status = si.Ptr(toolcalls.ToolCallStatusCanceled)
	if tc, err = ops.Put(ctx, tc, &toolcalls.AccessConditions{IfMatch: r.H.IfMatch, IfNoneMatch: r.H.IfNoneMatch}); err != nil {
		return err
	}
	return r.WriteResponse(&si.ResponseHeader{ETag: tc.ETag}, nil, http.StatusOK, tc)
}

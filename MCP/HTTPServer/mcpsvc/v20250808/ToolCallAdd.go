package v20250808

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/JeffreyRichter/mcpsvc/mcp/toolcalls"
	si "github.com/JeffreyRichter/serviceinfra"
)

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

const tenant = "sometenant"

func (ops *httpOperations) createToolCallAdd(ctx context.Context, tc *toolcalls.ToolCall, r *si.ReqRes) error {
	var trequest AddToolCallRequest
	if err := r.UnmarshalBody(&trequest); err != nil {
		return err
	}
	tc.Request = must(json.Marshal(trequest))

	tc.Status = si.Ptr(toolcalls.ToolCallStatusSuccess)
	tresult := &AddToolCallResult{Sum: trequest.X + trequest.Y}
	tc.Result = must(json.Marshal(tresult))

	tc, err := ops.Put(ctx, tenant, tc, &toolcalls.AccessConditions{IfMatch: r.H.IfMatch, IfNoneMatch: r.H.IfNoneMatch}) // Create/replace the resource
	if err != nil {
		return err
	}
	return r.WriteResponse(&si.ResponseHeader{ETag: tc.ETag}, nil, http.StatusOK, tc)
}

func (ops *httpOperations) getToolCallAdd(ctx context.Context, tc *toolcalls.ToolCall, r *si.ReqRes) error {
	tc, err := ops.Get(ctx, tenant, tc, &toolcalls.AccessConditions{IfMatch: r.H.IfMatch, IfNoneMatch: r.H.IfNoneMatch})
	// TODO: Fix up 304-Not Modified
	if err != nil {
		return err
	}
	return r.WriteResponse(&si.ResponseHeader{ETag: tc.ETag}, nil, http.StatusOK, tc)
}

func (ops *httpOperations) advanceToolCallAdd(ctx context.Context, tc *toolcalls.ToolCall, r *si.ReqRes) error {
	tc, err := ops.Get(ctx, tenant, tc, &toolcalls.AccessConditions{IfMatch: r.H.IfMatch, IfNoneMatch: r.H.IfNoneMatch})
	if err != nil {
		return err
	}
	if err := r.ValidatePreconditions(&si.PreconditionValues{ETag: tc.ETag}); err != nil {
		return err
	}
	switch *tc.Status {
	case toolcalls.ToolCallStatusAwaitingElicitationResult:
		var er toolcalls.ElicitationResult
		err := r.UnmarshalBody(&er)
		if err != nil {
			return err
		}
		// TODO: Process the er, update progress?, update status, update result/error
		tc.Status = si.Ptr(toolcalls.ToolCallStatusSuccess)

	case toolcalls.ToolCallStatusAwaitingSamplingResult:
		var sr toolcalls.SamplingResult
		err := r.UnmarshalBody(&sr)
		if err != nil {
			return err
		}
		// TODO: Process the sr, update progress?, update status, update result/error
	default:
		return r.Error(http.StatusBadRequest, "BadRequest", "tool call status is '%s'; not expecting a result", *tc.Status)
	}

	tc, err = ops.Put(ctx, tenant, tc, &toolcalls.AccessConditions{IfMatch: r.H.IfMatch, IfNoneMatch: r.H.IfNoneMatch}) // Update the resource
	if err != nil {
		return err
	}
	return r.WriteResponse(&si.ResponseHeader{ETag: tc.ETag}, nil, http.StatusOK, tc)
}

func (ops *httpOperations) cancelToolCallAdd(ctx context.Context, tc *toolcalls.ToolCall, r *si.ReqRes) error {
	/*
		1.	GET ToolCall resource/etag from client-passed ID
		2.	If status==terminal, return
		3.	Set status=Cancelled
		4.	Update resource (if-match:etag)
		a.	If update succeeds, send notification to ToolCallNotification queue
		b.	Else, go to Step #1 to retry. The race is OK because of #2.

	*/
	body := any(nil)
	return r.WriteResponse(&si.ResponseHeader{
		ETag: ops.etag(),
	}, nil, http.StatusOK, body)
}

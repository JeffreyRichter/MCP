package v20250808

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/JeffreyRichter/mcpsvc/mcp"
	"github.com/JeffreyRichter/mcpsvc/mcp/toolcalls"
	si "github.com/JeffreyRichter/serviceinfra"
)

type PIIToolCallRequest struct {
	Key string `json:"key"`
}

type PIIToolCallResult struct {
	Data string `json:"data"`
}

// TODO: client must specify elicitation capability
func (ops *httpOperations) createToolCallPII(ctx context.Context, tc *toolcalls.ToolCall, r *si.ReqRes) error {
	var trequest PIIToolCallRequest
	if err := r.UnmarshalBody(&trequest); err != nil {
		return err
	}
	tc.Request = must(json.Marshal(trequest))
	if trequest.Key == "" {
		return r.Error(http.StatusBadRequest, "BadRequest", "key is required")
	}

	tc.ElicitationRequest = &toolcalls.ElicitationRequest{
		Message: "The requested data contains personal information (PII). Please approve access to this data.",
		RequestedSchema: struct {
			Type       string                                   `json:"type"`
			Properties map[string]mcp.PrimitiveSchemaDefinition `json:"properties"`
			Required   []string                                 `json:"required,omitempty"`
		}{
			Type: "object",
			Properties: map[string]mcp.PrimitiveSchemaDefinition{
				"approved": mcp.BooleanSchema{
					Type:        "boolean",
					Title:       si.Ptr("Approval"),
					Description: si.Ptr("Whether to approve PII access"),
				},
			},
			Required: []string{"approved"},
		},
	}
	tc.Status = si.Ptr(toolcalls.ToolCallStatusAwaitingElicitationResult)

	tc, err := ops.Put(ctx, tenant, tc, &toolcalls.AccessConditions{IfMatch: r.H.IfMatch, IfNoneMatch: r.H.IfNoneMatch})
	if err != nil {
		return err
	}
	return r.WriteResponse(&si.ResponseHeader{ETag: tc.ETag}, nil, http.StatusOK, tc)
}

func (ops *httpOperations) getToolCallPII(ctx context.Context, tc *toolcalls.ToolCall, r *si.ReqRes) error {
	tc, err := ops.Get(ctx, tenant, tc, &toolcalls.AccessConditions{IfMatch: r.H.IfMatch, IfNoneMatch: r.H.IfNoneMatch})
	if err != nil {
		return err
	}
	return r.WriteResponse(&si.ResponseHeader{ETag: tc.ETag}, nil, http.StatusOK, tc)
}

func (ops *httpOperations) advanceToolCallPII(ctx context.Context, tc *toolcalls.ToolCall, r *si.ReqRes) error {
	tc, err := ops.Get(ctx, tenant, tc, &toolcalls.AccessConditions{IfMatch: r.H.IfMatch, IfNoneMatch: r.H.IfNoneMatch})
	if err != nil {
		return err
	}
	if err = r.ValidatePreconditions(&si.PreconditionValues{ETag: tc.ETag}); err != nil {
		return err
	}
	if tc.Status == nil {
		return r.Error(http.StatusInternalServerError, "InternalServerError", "can't advance because tool call has no status")
	}
	if *tc.Status != toolcalls.ToolCallStatusAwaitingElicitationResult {
		return r.Error(http.StatusBadRequest, "BadRequest", "not expecting an elicitation result for call with status %q", *tc.Status)
	}

	var er toolcalls.ElicitationResult
	err = r.UnmarshalBody(&er)
	if err != nil {
		return err
	}
	// all responses must specify an action; "content" is required only for "action": "accept"
	if er.Action == "" {
		return r.Error(http.StatusBadRequest, "BadRequest", "elicitation result content is required")
	}
	var approved, ok bool
	if er.Action == "accept" {
		if er.Content == nil {
			return r.Error(http.StatusBadRequest, "BadRequest", "elicitation result content is required")
		}
		// client accepted the elicitation request, so we expect "content": {"approved": ...}
		if approved, ok = (*er.Content)["approved"].(bool); !ok {
			return r.Error(http.StatusBadRequest, "BadRequest", `missing "approved" boolean in elicitation result content`)
		}
	}
	tc.Status = si.Ptr(toolcalls.ToolCallStatusCanceled)
	if approved {
		tc.Status = si.Ptr(toolcalls.ToolCallStatusSuccess)
		tresult := &PIIToolCallResult{Data: "here's your PII"}
		tc.Result = must(json.Marshal(tresult))
	}
	// drop the elicitation request because it's been processed
	tc.ElicitationRequest = nil

	if tc, err = ops.Put(ctx, tenant, tc, &toolcalls.AccessConditions{IfMatch: r.H.IfMatch, IfNoneMatch: r.H.IfNoneMatch}); err != nil {
		return err
	}
	return r.WriteResponse(&si.ResponseHeader{ETag: tc.ETag}, nil, http.StatusOK, tc)
}

func (ops *httpOperations) cancelToolCallPII(ctx context.Context, tc *toolcalls.ToolCall, r *si.ReqRes) error {
	tc, err := ops.Get(ctx, tenant, tc, &toolcalls.AccessConditions{IfMatch: r.H.IfMatch, IfNoneMatch: r.H.IfNoneMatch})
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
	if tc, err = ops.Put(ctx, tenant, tc, &toolcalls.AccessConditions{IfMatch: r.H.IfMatch, IfNoneMatch: r.H.IfNoneMatch}); err != nil {
		return err
	}
	return r.WriteResponse(&si.ResponseHeader{ETag: tc.ETag}, nil, http.StatusOK, tc)
}

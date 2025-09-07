package v20250808

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/JeffreyRichter/mcpsvr/mcp"
	"github.com/JeffreyRichter/mcpsvr/mcp/toolcalls"
	"github.com/JeffreyRichter/svrcore"
)

type piiToolCaller struct {
	defaultToolCaller
	ops *httpOperations
}

type PIIToolCallRequest struct {
	Key string `json:"key"`
}

type PIIToolCallResult struct {
	Data string `json:"data"`
}

func (c *piiToolCaller) Tool() *mcp.Tool {
	return &mcp.Tool{
		BaseMetadata: mcp.BaseMetadata{
			Name:  "pii",
			Title: svrcore.Ptr("Get PII"),
		},
		Description: svrcore.Ptr("Get PII data with client confirmation"),
		InputSchema: mcp.JSONSchema{
			Type:       "object",
			Properties: &map[string]any{},
			Required:   []string{},
		},
		OutputSchema: &mcp.JSONSchema{
			Type: "object",
			Properties: &map[string]any{
				"data": map[string]any{
					"type":        "string",
					"Description": svrcore.Ptr("The PII data"),
				},
			},
			Required: []string{"data"},
		},
		Annotations: &mcp.ToolAnnotations{
			Title:           svrcore.Ptr("Get PII"),
			ReadOnlyHint:    svrcore.Ptr(true),
			DestructiveHint: svrcore.Ptr(false),
			IdempotentHint:  svrcore.Ptr(true),
			OpenWorldHint:   svrcore.Ptr(false),
		},
		Meta: mcp.Meta{"sensitive": "true"},
	}
}

// TODO: client must specify elicitation capability
func (c *piiToolCaller) Create(ctx context.Context, tc *toolcalls.ToolCall, r *svrcore.ReqRes) error {
	var trequest PIIToolCallRequest
	if err := r.UnmarshalBody(&trequest); err != nil {
		return err
	}
	tc.Request = must(json.Marshal(trequest))
	if trequest.Key == "" {
		return r.WriteError(http.StatusBadRequest, nil, nil, "BadRequest", "key is required")
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
					Title:       svrcore.Ptr("Approval"),
					Description: svrcore.Ptr("Whether to approve PII access"),
				},
			},
			Required: []string{"approved"},
		},
	}
	tc.Status = svrcore.Ptr(toolcalls.ToolCallStatusAwaitingElicitationResult)

	err := c.ops.Put(ctx, tc, svrcore.AccessConditions{IfMatch: r.H.IfMatch, IfNoneMatch: r.H.IfNoneMatch})
	if err != nil {
		return err
	}
	return r.WriteSuccess(http.StatusOK, &svrcore.ResponseHeader{ETag: tc.ETag}, nil, tc)
}

func (c *piiToolCaller) Get(ctx context.Context, tc *toolcalls.ToolCall, r *svrcore.ReqRes) error {
	err := c.ops.Get(ctx, tc, svrcore.AccessConditions{IfMatch: r.H.IfMatch, IfNoneMatch: r.H.IfNoneMatch})
	if err != nil {
		return err
	}
	return r.WriteSuccess(http.StatusOK, &svrcore.ResponseHeader{ETag: tc.ETag}, nil, tc)
}

func (c *piiToolCaller) Advance(ctx context.Context, tc *toolcalls.ToolCall, r *svrcore.ReqRes) error {
	err := c.ops.Get(ctx, tc, svrcore.AccessConditions{IfMatch: r.H.IfMatch, IfNoneMatch: r.H.IfNoneMatch})
	if err != nil {
		return err
	}
	if err = r.ValidatePreconditions(svrcore.ResourceValues{AllowedConditionals: svrcore.AllowedConditionalsMatch, ETag: tc.ETag}); err != nil {
		return err
	}
	if tc.Status == nil {
		return r.WriteError(http.StatusInternalServerError, nil, nil, "InternalServerError", "can't advance because tool call has no status")
	}
	if *tc.Status != toolcalls.ToolCallStatusAwaitingElicitationResult {
		return r.WriteError(http.StatusBadRequest, nil, nil, "BadRequest", "not expecting an elicitation result for call with status %q", *tc.Status)
	}

	var er toolcalls.ElicitationResult
	err = r.UnmarshalBody(&er)
	if err != nil {
		return err
	}
	// all responses must specify an action; "content" is required only for "action": "accept"
	if er.Action == "" {
		return r.WriteError(http.StatusBadRequest, nil, nil, "BadRequest", "elicitation result content is required")
	}
	var approved, ok bool
	if er.Action == "accept" {
		if er.Content == nil {
			return r.WriteError(http.StatusBadRequest, nil, nil, "BadRequest", "elicitation result content is required")
		}
		// client accepted the elicitation request, so we expect "content": {"approved": ...}
		if approved, ok = (*er.Content)["approved"].(bool); !ok {
			return r.WriteError(http.StatusBadRequest, nil, nil, "BadRequest", `missing "approved" boolean in elicitation result content`)
		}
	}
	tc.Status = svrcore.Ptr(toolcalls.ToolCallStatusCanceled)
	if approved {
		tc.Status = svrcore.Ptr(toolcalls.ToolCallStatusSuccess)
		tresult := &PIIToolCallResult{Data: "here's your PII"}
		tc.Result = must(json.Marshal(tresult))
	}
	// drop the elicitation request because it's been processed
	tc.ElicitationRequest = nil

	if err = c.ops.Put(ctx, tc, svrcore.AccessConditions{IfMatch: r.H.IfMatch, IfNoneMatch: r.H.IfNoneMatch}); err != nil {
		return err
	}
	return r.WriteSuccess(http.StatusOK, &svrcore.ResponseHeader{ETag: tc.ETag}, nil, tc)
}

func (c *piiToolCaller) Cancel(ctx context.Context, tc *toolcalls.ToolCall, r *svrcore.ReqRes) error {
	err := c.ops.Get(ctx, tc, svrcore.AccessConditions{IfMatch: r.H.IfMatch, IfNoneMatch: r.H.IfNoneMatch})
	if err != nil {
		return err
	}
	if err = r.ValidatePreconditions(svrcore.ResourceValues{AllowedConditionals: svrcore.AllowedConditionalsMatch, ETag: tc.ETag}); err != nil {
		return err
	}
	if tc.Status == nil {
		return r.WriteError(http.StatusInternalServerError, nil, nil, "InternalServerError", "tool call status is nil; can't cancel")
	}
	switch *tc.Status {
	case toolcalls.ToolCallStatusSuccess, toolcalls.ToolCallStatusFailed, toolcalls.ToolCallStatusCanceled:
		return r.WriteSuccess(http.StatusOK, &svrcore.ResponseHeader{ETag: tc.ETag}, nil, tc)
	}

	tc.ElicitationRequest = nil
	tc.Error = nil
	tc.Result = nil
	tc.Status = svrcore.Ptr(toolcalls.ToolCallStatusCanceled)
	if err = c.ops.Put(ctx, tc, svrcore.AccessConditions{IfMatch: r.H.IfMatch, IfNoneMatch: r.H.IfNoneMatch}); err != nil {
		return err
	}
	return r.WriteSuccess(http.StatusOK, &svrcore.ResponseHeader{ETag: tc.ETag}, nil, tc)
}

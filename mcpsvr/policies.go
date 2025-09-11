package main

// For timeouts/cancellation, see https://ieftimov.com/posts/make-resilient-golang-net-http-servers-using-timeouts-deadlines-context-cancellation/

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/JeffreyRichter/internal/aids"
	"github.com/JeffreyRichter/mcpsvr/mcp"
	"github.com/JeffreyRichter/mcpsvr/mcp/toolcall"
	"github.com/JeffreyRichter/svrcore"
)

// Resource type & operations pattern:
// 1. Define Resource Type struct & define api-agnostic resource type operations on this type
// 2. Construct global singleton instance/variable of the Resource Type used to call #1 methods
// 3. Define api-version Resource Type Operations struct with field of #1 & define api-specific HTTP operations on this type
// 4. Construct global singleton instance/variable of #3 wrapping #2 & set api-version routes to these var/methods

// mcpPolicies wraps the version-agnostic resources (ToolCalls) with this specific api-version's HTTP operations: behavior wrapping state
type mcpPolicies struct {
	errorLogger *slog.Logger
	store       toolcall.Store
	pm          toolcall.PhaseMgr
	toolInfos   map[string]*ToolInfo
}

func (p *mcpPolicies) buildToolInfos() {
	p.toolInfos = map[string]*ToolInfo{}
	for _, tc := range []ToolCaller{
		&addToolCaller{ops: p},
		&countToolCaller{ops: p},
		&piiToolCaller{ops: p},
	} {
		if t := tc.Tool(); t != nil {
			p.toolInfos[t.Name] = &ToolInfo{Tool: t, Caller: tc}
		}
	}
}

// etag returns the ETag for this version's HTTP operations
func (p *mcpPolicies) etag() *svrcore.ETag { return svrcore.Ptr(svrcore.ETag("v20250808")) }

// lookupToolCall retrieves the ToolInfo and ToolCall from the given request URL (and authentication for tenant).
// Writes an HTTP error response and returns a *ServerError if the tool name or tool call ID is missing or invalid.
func (p *mcpPolicies) lookupToolCall(r *svrcore.ReqRes) (*ToolInfo, *toolcall.ToolCall, error) {
	tenant := "sometenant"
	toolName, toolCallID := r.R.PathValue("toolName"), r.R.PathValue("toolCallID")
	if toolName == "" {
		return nil, nil, r.WriteError(http.StatusBadRequest, nil, nil, "BadRequest", "Tool name required")
	}
	if toolCallID == "" {
		return nil, nil, r.WriteError(http.StatusBadRequest, nil, nil, "BadRequest", "Tool call ID required")
	}
	ti, ok := p.toolInfos[toolName]
	if !ok {
		return nil, nil, r.WriteError(http.StatusBadRequest, nil, nil, "BadRequest", "Tool '%s' not found", toolName)
	}
	return ti, toolcall.New(tenant, toolName, toolCallID), nil
}

// toolNameToProcessPhaseFunc converts a toolname to a function that knows how to advance the tool call's phase/state
func (p *mcpPolicies) toolNameToProcessPhaseFunc(toolName string) (toolcall.ProcessPhaseFunc, error) {
	ti, ok := p.toolInfos[toolName]
	if !ok {
		return nil, fmt.Errorf("tool '%s' not found", toolName)
	}
	return ti.Caller.ProcessPhase, nil
}

// putToolCallResource creates a new tool call resource (idempotently if a retry occurs).
// Writes an HTTP error response and returns a *ServerError if the tool name or tool call ID is missing or invalid.
func (p *mcpPolicies) putToolCallResource(ctx context.Context, r *svrcore.ReqRes) error {
	ti, tc, err := p.lookupToolCall(r)
	if aids.IsError(err) {
		return err
	}
	if err := r.CheckPreconditions(svrcore.ResourceValues{AllowedConditionals: svrcore.AllowedConditionalsNone, ETag: tc.ETag}); aids.IsError(err) {
		return err
	}

	// Calculate idempotency key based on something in the request that should be stable across retries
	// For example, the Date header (which must be present per RFC 7231)
	if tc.IdempotencyKey != nil { // PUT on an existing tool call ID
		if (tc.IdempotencyKey != nil) && (*tc.IdempotencyKey != *r.H.IdempotencyKey) { // Not a retry
			return r.WriteError(http.StatusConflict, nil, nil, "Conflict", "Tool call ID already exists")
		}
	}
	tc.IdempotencyKey = r.H.IdempotencyKey
	return ti.Caller.Create(ctx, tc, r, p.pm)
}

// preambleToolCallResource retrieves the ToolInfo and ToolCall from the given request URL (and authentication for tenant),
// then retrieves the ToolCall resource from storage and validates preconditions.
// Writes an HTTP error response and returns a *ServerError if the tool name or tool call ID is missing or invalid,
// the ToolCall resource is not found, or preconditions are not met.
// This method is used is called by GET & POST (not PUT) because it assumes the resource must already exist.
func (p *mcpPolicies) preambleToolCallResource(ctx context.Context, r *svrcore.ReqRes) (*ToolInfo, *toolcall.ToolCall, error) {
	ti, tc, err := p.lookupToolCall(r)
	if aids.IsError(err) {
		return nil, nil, err
	}
	err = p.store.Get(ctx, tc, svrcore.AccessConditions{IfMatch: r.H.IfMatch, IfNoneMatch: r.H.IfNoneMatch})
	if aids.IsError(err) {
		return nil, nil, r.WriteError(http.StatusNotFound, nil, nil, "NotFound", "Tool call not found")
	}
	if err = r.CheckPreconditions(svrcore.ResourceValues{AllowedConditionals: svrcore.AllowedConditionalsMatch, ETag: tc.ETag}); aids.IsError(err) {
		return nil, nil, err
	}
	return ti, tc, nil
}

// getToolCallResource retrieves the ToolCall resource from the request.
func (p *mcpPolicies) getToolCallResource(ctx context.Context, r *svrcore.ReqRes) error {
	ti, tc, err := p.preambleToolCallResource(ctx, r)
	if aids.IsError(err) {
		return err
	}
	return ti.Caller.Get(ctx, tc, r)
}

// postToolCallAdvance advances the state of a tool call using r's body (CreateMessageResult or ElicitResult)
func (p *mcpPolicies) postToolCallResourceAdvance(ctx context.Context, r *svrcore.ReqRes) error {
	ti, tc, err := p.preambleToolCallResource(ctx, r)
	if aids.IsError(err) {
		return err
	}
	return ti.Caller.Advance(ctx, tc, r)
}

// postToolCallCancelResource cancels a tool call.
func (p *mcpPolicies) postToolCallCancelResource(ctx context.Context, r *svrcore.ReqRes) error {
	ti, tc, err := p.preambleToolCallResource(ctx, r)
	if aids.IsError(err) {
		return err
	}
	return ti.Caller.Cancel(ctx, tc, r)
}

////////////////////////////////////////////////////////////////////////////////////////////////////////

// getToolList retrieves the list of tools.
func (p *mcpPolicies) getToolList(ctx context.Context, r *svrcore.ReqRes) error {
	if err := r.CheckPreconditions(svrcore.ResourceValues{AllowedConditionals: svrcore.AllowedConditionalsMatch, ETag: p.etag()}); aids.IsError(err) {
		return err
	}
	result := mcp.ListToolsResult{Tools: make([]mcp.Tool, 0, len(p.toolInfos))}
	for _, ti := range p.toolInfos {
		result.Tools = append(result.Tools, *ti.Tool)
	}
	return r.WriteSuccess(http.StatusOK, &svrcore.ResponseHeader{ETag: p.etag()}, nil, result)
}

// listToolCalls retrieves the list of tool calls.
func (p *mcpPolicies) listToolCalls(ctx context.Context, r *svrcore.ReqRes) error {
	body := any(nil)
	return r.WriteSuccess(http.StatusOK, &svrcore.ResponseHeader{ETag: p.etag()}, nil, body)
}

// getResources retrieves the list of resources.
func (p *mcpPolicies) getResources(ctx context.Context, r *svrcore.ReqRes) error {
	return r.WriteSuccess(http.StatusNoContent, &svrcore.ResponseHeader{}, nil, nil)
}

// getResourcesTemplates retrieves the list of resource templates.
func (p *mcpPolicies) getResourcesTemplates(ctx context.Context, r *svrcore.ReqRes) error {
	return r.WriteSuccess(http.StatusNoContent, &svrcore.ResponseHeader{}, nil, nil)
}

// getResource retrieves a specific resource by name.
func (p *mcpPolicies) getResource(ctx context.Context, r *svrcore.ReqRes) error {
	resourceName := r.R.PathValue("name")
	if resourceName == "" {
		return r.WriteError(http.StatusBadRequest, nil, nil, "BadRequest", "Resource name is required")
	}
	return r.WriteSuccess(http.StatusNoContent, &svrcore.ResponseHeader{}, nil, nil)
}

// getPrompts retrieves the list of prompts.
func (p *mcpPolicies) getPrompts(ctx context.Context, r *svrcore.ReqRes) error {
	return r.WriteSuccess(http.StatusNoContent, &svrcore.ResponseHeader{}, nil, nil)
}

// getPrompt retrieves a specific prompt by name.
func (p *mcpPolicies) getPrompt(ctx context.Context, r *svrcore.ReqRes) error {
	return r.WriteSuccess(http.StatusNoContent, &svrcore.ResponseHeader{}, nil, nil)
}

// putRoots updates the list of root resources.
func (p *mcpPolicies) putRoots(ctx context.Context, r *svrcore.ReqRes) error {
	return r.WriteSuccess(http.StatusNoContent, &svrcore.ResponseHeader{}, nil, nil)
}

// postCompletion returns a text completion.
func (p *mcpPolicies) postCompletion(ctx context.Context, r *svrcore.ReqRes) error {
	return r.WriteSuccess(http.StatusNoContent, &svrcore.ResponseHeader{}, nil, nil)
}

/*
Server-Deque event from ToolCallsInProgress:
1.	GET ToolCall resource from event ID
2.	If resource etag doesn’t match event’s etag, this event was already processed, just return.
a.	NOTE: This makes processing the same event idempotent
3.	If status=InputRequired/Completed/Canceled/Failed/Rejected, just return
a.	Else: status must be Submitted/Working
4.	Do ToolCall processing
a.	If client input needed, set status to InputRequired; otherwise set status to Completed/Failed/Rejected (processing was terminal)
b.	Update resource (if-match: etag) with new status
•	If unsuccessful, we lost race, just return
b.	Send status notification to ToolCallNotification queue

Server-Tool Call GC:
1.	For each ToolCall resource with expiration < NOW
a.	Delete resource’s ToolCallNotification queue (idempotent)
•	NOTE: Failure here means next GC will delete this resource
b.	Delete the resource

NOTES:
-	Whenever an event is queued to the ToolCallNotification queue, a webhook could also be called.
*/

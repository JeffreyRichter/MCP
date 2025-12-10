package main

// For timeouts/cancellation, see https://ieftimov.com/posts/make-resilient-golang-net-http-servers-using-timeouts-deadlines-context-cancellation/

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/JeffreyRichter/internal/aids"
	"github.com/JeffreyRichter/mcp"
	"github.com/JeffreyRichter/mcpsvr/toolcall"
	"github.com/JeffreyRichter/svrcore"
)

// Resource type & operations pattern:
// 1. Define Resource Type struct & define api-agnostic resource type operations on this type
// 2. Construct global singleton instance/variable of the Resource Type used to call #1 methods
// 3. Define api-version Resource Type Operations struct with field of #1 & define api-specific HTTP operations on this type
// 4. Construct global singleton instance/variable of #3 wrapping #2 & set api-version routes to these var/methods

// mcpStages wraps the version-agnostic resources (ToolCalls) with this specific api-version's HTTP operations: behavior wrapping state
type mcpStages struct {
	errorLogger *slog.Logger
	store       toolcall.Store
	pm          toolcall.PhaseMgr
	toolInfos   map[string]ToolInfo // ToolName to ToolCaller implementation
}

func (p *mcpStages) buildToolInfos() {
	p.toolInfos = map[string]ToolInfo{}
	for _, tc := range []ToolInfo{
		&addToolInfo{ops: p},
		&countToolInfo{ops: p},
		&welcomeToolInfo{ops: p},
		&streamToolInfo{ops: p},
	} {
		if t := tc.Tool(); t != nil {
			p.toolInfos[t.Name] = tc
		}
	}
}

// etag returns the ETag for this version's HTTP operations
func (p *mcpStages) etag() *svrcore.ETag { return aids.New(svrcore.ETag("v20250808")) }

// lookupToolCall retrieves the ToolInfo and ToolCall from the given request URL (and authentication for tenant).
// Writes an HTTP error response and returns a *ServerError if the tool name or tool call ID is missing or invalid.
func (p *mcpStages) lookupToolCall(r *svrcore.ReqRes) (ToolInfo, *toolcall.Resource, bool) {
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
	return ti, toolcall.New(tenant, toolName, toolCallID), false
}

// toolNameToProcessPhaseFunc converts a toolname to a function that knows how to advance the tool call's phase/state
func (p *mcpStages) toolNameToProcessPhaseFunc(toolName string) toolcall.ProcessPhaseFunc {
	ti, ok := p.toolInfos[toolName]
	aids.Assert(ok, fmt.Errorf("tool '%s' not found", toolName))
	return ti.ProcessPhase
}

// putToolCallResource creates a new tool call resource (idempotently if a retry occurs).
// Writes an HTTP error response and returns a *ServerError if the tool name or tool call ID is missing or invalid.
func (p *mcpStages) putToolCallResource(ctx context.Context, r *svrcore.ReqRes) bool {
	ti, tc, stop := p.lookupToolCall(r)
	if stop {
		return stop
	}
	// PUT does not support any conditional headers; return error if client specifies them
	if stop := r.CheckPreconditions(svrcore.ResourceValues{AllowedConditionals: svrcore.AllowedConditionalsNone}); stop {
		return stop
	}
	if r.H.IdempotencyKey == nil {
		return r.WriteError(http.StatusBadRequest, nil, nil, "BadRequest", "IdempotencyKey header required for PUT")
	}

	// Does the tool call ID already exist?
	toolCallIDFound := false
	if se := p.store.Get(ctx, tc, svrcore.AccessConditions{}); se == nil {
		toolCallIDFound = true // Found is OK; this is an existing tool call ID
	} else if se.StatusCode == http.StatusNotFound {
		// Not found is OK; this is a new tool call ID
	} else {
		return r.WriteError(http.StatusInternalServerError, nil, nil, "InternalServerError", "Failed to get tool call")
	}

	if !toolCallIDFound { // If tool call ID doesn't already exist, create it
		tc.IdempotencyKey = r.H.IdempotencyKey
		return ti.Create(ctx, tc, r, p.pm) // Create method must use "if-none-match: *"
	}

	// Tool call already exists
	if *tc.IdempotencyKey == *r.H.IdempotencyKey { // This is a retry, return existing resource & 200-OK via GET
		return ti.Get(ctx, tc, r)
	}
	return r.WriteError(http.StatusConflict, nil, nil, "Conflict", "Tool call ID already exists with different IdempotencyKey")
}

// preambleToolCallResource retrieves the ToolInfo and ToolCall from the given request URL (and authentication for tenant),
// then retrieves the ToolCall resource from storage and validates preconditions.
// Writes an HTTP error response and returns a *ServerError if the tool name or tool call ID is missing or invalid,
// the ToolCall resource is not found, or preconditions are not met.
// This method is used is called by GET & POST (not PUT) because it assumes the resource must already exist.
func (p *mcpStages) preambleToolCallResource(ctx context.Context, r *svrcore.ReqRes) (ToolInfo, *toolcall.Resource, bool) {
	ti, tc, stop := p.lookupToolCall(r)
	if stop {
		return nil, nil, stop
	}
	se := p.store.Get(ctx, tc, svrcore.AccessConditions{IfMatch: r.H.IfMatch, IfNoneMatch: r.H.IfNoneMatch})
	if se != nil {
		return nil, nil, r.WriteError(http.StatusNotFound, nil, nil, "NotFound", "Tool call not found")
	}
	if stop := r.CheckPreconditions(svrcore.ResourceValues{AllowedConditionals: svrcore.AllowedConditionalsMatch, ETag: tc.ETag}); stop {
		return nil, nil, stop
	}
	return ti, tc, false
}

// getToolCallResource retrieves the ToolCall resource from the request.
func (p *mcpStages) getToolCallResource(ctx context.Context, r *svrcore.ReqRes) bool {
	ti, tc, stop := p.preambleToolCallResource(ctx, r)
	if stop {
		return stop
	}
	return ti.Get(ctx, tc, r)
}

// postToolCallAdvance advances the state of a tool call using r's body (CreateMessageResult or ElicitResult)
func (p *mcpStages) postToolCallResourceAdvance(ctx context.Context, r *svrcore.ReqRes) bool {
	ti, tc, stop := p.preambleToolCallResource(ctx, r)
	if stop {
		return stop
	}
	return ti.Advance(ctx, tc, r)
}

// postToolCallCancelResource cancels a tool call.
func (p *mcpStages) postToolCallCancelResource(ctx context.Context, r *svrcore.ReqRes) bool {
	ti, tc, stop := p.preambleToolCallResource(ctx, r)
	if stop {
		return stop
	}
	return ti.Cancel(ctx, tc, r)
}

////////////////////////////////////////////////////////////////////////////////////////////////////////

// getToolList retrieves the list of tools.
func (p *mcpStages) getToolList(ctx context.Context, r *svrcore.ReqRes) bool {
	if stop := r.CheckPreconditions(svrcore.ResourceValues{AllowedConditionals: svrcore.AllowedConditionalsMatch, ETag: p.etag()}); stop {
		return true
	}
	result := mcp.ListToolsResult{Tools: make([]mcp.Tool, 0, len(p.toolInfos))}
	for _, ti := range p.toolInfos {
		result.Tools = append(result.Tools, *ti.Tool())
	}
	return r.WriteSuccess(http.StatusOK, &svrcore.ResponseHeader{ETag: p.etag()}, nil, result)
}

// listToolCalls retrieves the list of tool calls.
func (p *mcpStages) listToolCalls(ctx context.Context, r *svrcore.ReqRes) bool {
	body := any(nil)
	return r.WriteSuccess(http.StatusOK, &svrcore.ResponseHeader{ETag: p.etag()}, nil, body)
}

// getResources retrieves the list of resources.
func (p *mcpStages) getResources(ctx context.Context, r *svrcore.ReqRes) bool {
	return r.WriteSuccess(http.StatusNoContent, &svrcore.ResponseHeader{}, nil, nil)
}

// getResourcesTemplates retrieves the list of resource templates.
func (p *mcpStages) getResourcesTemplates(ctx context.Context, r *svrcore.ReqRes) bool {
	return r.WriteSuccess(http.StatusNoContent, &svrcore.ResponseHeader{}, nil, nil)
}

// getResource retrieves a specific resource by name.
func (p *mcpStages) getResource(ctx context.Context, r *svrcore.ReqRes) bool {
	resourceName := r.R.PathValue("name")
	if resourceName == "" {
		return r.WriteError(http.StatusBadRequest, nil, nil, "BadRequest", "Resource name is required")
	}
	return r.WriteSuccess(http.StatusNoContent, &svrcore.ResponseHeader{}, nil, nil)
}

// getPrompts retrieves the list of prompts.
func (p *mcpStages) getPrompts(ctx context.Context, r *svrcore.ReqRes) bool {
	return r.WriteSuccess(http.StatusNoContent, &svrcore.ResponseHeader{}, nil, nil)
}

// getPrompt retrieves a specific prompt by name.
func (p *mcpStages) getPrompt(ctx context.Context, r *svrcore.ReqRes) bool {
	return r.WriteSuccess(http.StatusNoContent, &svrcore.ResponseHeader{}, nil, nil)
}

// putRoots updates the list of root resources.
func (p *mcpStages) putRoots(ctx context.Context, r *svrcore.ReqRes) bool {
	return r.WriteSuccess(http.StatusNoContent, &svrcore.ResponseHeader{}, nil, nil)
}

// postCompletion returns a text completion.
func (p *mcpStages) postCompletion(ctx context.Context, r *svrcore.ReqRes) bool {
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

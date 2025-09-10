package main

// For timeouts/cancellation, see https://ieftimov.com/posts/make-resilient-golang-net-http-servers-using-timeouts-deadlines-context-cancellation/

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azqueue"
	"github.com/JeffreyRichter/mcpsvr/mcp"
	"github.com/JeffreyRichter/mcpsvr/mcp/toolcall"
	"github.com/JeffreyRichter/mcpsvr/resources/azresources"
	"github.com/JeffreyRichter/mcpsvr/resources/localresources"
	"github.com/JeffreyRichter/svrcore"
)

// Resource type & operations pattern:
// 1. Define Resource Type struct & define api-agnostic resource type operations on this type
// 2. Construct global singleton instance/variable of the Resource Type used to call #1 methods
// 3. Define api-version Resource Type Operations struct with field of #1 & define api-specific HTTP operations on this type
// 4. Construct global singleton instance/variable of #3 wrapping #2 & set api-version routes to these var/methods

func newLocalMcpServer(shutdownCtx context.Context, errorLogger *slog.Logger) *httpOps {
	ops := &httpOps{errorLogger: errorLogger, store: localresources.NewToolCallStore(shutdownCtx)}
	ops.pm = localresources.NewPhaseMgr(shutdownCtx, localresources.PhaseMgrConfig{ErrorLogger: errorLogger, ToolNameToProcessPhaseFunc: ops.toolNameToProcessPhaseFunc})
	ops.buildToolInfos()
	return ops
}

func newAzureMcpServer(shutdownCtx context.Context, errorLogger *slog.Logger, blobClient *azblob.Client, queueClient *azqueue.QueueClient) *httpOps {
	ops := &httpOps{errorLogger: errorLogger, store: azresources.NewToolCallStore(blobClient)}
	pm, err := azresources.NewPhaseMgr(shutdownCtx, queueClient, ops.store, azresources.PhaseMgrConfig{ErrorLogger: errorLogger, ToolNameToProcessPhaseFunc: ops.toolNameToProcessPhaseFunc})
	if isError(err) {
		panic(err)
	}
	ops.pm = pm
	ops.buildToolInfos()
	return ops
}

// httpOps wraps the version-agnostic resources (ToolCalls) with this specific api-version's HTTP operations: behavior wrapping state
type httpOps struct {
	errorLogger *slog.Logger
	store       toolcall.Store
	pm          toolcall.PhaseMgr
	toolInfos   map[string]*ToolInfo
}

func (ops *httpOps) buildToolInfos() {
	ops.toolInfos = map[string]*ToolInfo{}
	for _, tc := range []ToolCaller{
		&addToolCaller{ops: ops},
		&countToolCaller{ops: ops},
		&piiToolCaller{ops: ops},
	} {
		if t := tc.Tool(); t != nil {
			ops.toolInfos[t.Name] = &ToolInfo{Tool: t, Caller: tc}
		}
	}
}

// etag returns the ETag for this version's HTTP operations
func (ops *httpOps) etag() *svrcore.ETag { return svrcore.Ptr(svrcore.ETag("v20250808")) }

// lookupToolCall retrieves the ToolInfo and ToolCall from the given request URL (and authentication for tenant).
// Writes an HTTP error response and returns a *ServerError if the tool name or tool call ID is missing or invalid.
func (ops *httpOps) lookupToolCall(r *svrcore.ReqRes) (*ToolInfo, *toolcall.ToolCall, error) {
	tenant := "sometenant"
	toolName, toolCallID := r.R.PathValue("toolName"), r.R.PathValue("toolCallID")
	if toolName == "" {
		return nil, nil, r.WriteError(http.StatusBadRequest, nil, nil, "BadRequest", "Tool name required")
	}
	if toolCallID == "" {
		return nil, nil, r.WriteError(http.StatusBadRequest, nil, nil, "BadRequest", "Tool call ID required")
	}
	ti, ok := ops.toolInfos[toolName]
	if !ok {
		return nil, nil, r.WriteError(http.StatusBadRequest, nil, nil, "BadRequest", "Tool '%s' not found", toolName)
	}
	return ti, toolcall.New(tenant, toolName, toolCallID), nil
}

// toolNameToProcessPhaseFunc converts a toolname to a function that knows how to advance the tool call's phase/state
func (ops *httpOps) toolNameToProcessPhaseFunc(toolName string) (toolcall.ProcessPhaseFunc, error) {
	ti, ok := ops.toolInfos[toolName]
	if !ok {
		return nil, fmt.Errorf("tool '%s' not found", toolName)
	}
	return ti.Caller.ProcessPhase, nil
}

// putToolCallResource creates a new tool call resource (idempotently if a retry occurs).
// Writes an HTTP error response and returns a *ServerError if the tool name or tool call ID is missing or invalid.
func (ops *httpOps) putToolCallResource(ctx context.Context, r *svrcore.ReqRes) error {
	ti, tc, err := ops.lookupToolCall(r)
	if isError(err) {
		return err
	}
	if err := r.CheckPreconditions(svrcore.ResourceValues{AllowedConditionals: svrcore.AllowedConditionalsNone, ETag: tc.ETag}); isError(err) {
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
	return ti.Caller.Create(ctx, tc, r, ops.pm)
}

// preambleToolCallResource retrieves the ToolInfo and ToolCall from the given request URL (and authentication for tenant),
// then retrieves the ToolCall resource from storage and validates preconditions.
// Writes an HTTP error response and returns a *ServerError if the tool name or tool call ID is missing or invalid,
// the ToolCall resource is not found, or preconditions are not met.
// This method is used is called by GET & POST (not PUT) because it assumes the resource must already exist.
func (ops *httpOps) preambleToolCallResource(ctx context.Context, r *svrcore.ReqRes) (*ToolInfo, *toolcall.ToolCall, error) {
	ti, tc, err := ops.lookupToolCall(r)
	if isError(err) {
		return nil, nil, err
	}
	err = ops.store.Get(ctx, tc, svrcore.AccessConditions{IfMatch: r.H.IfMatch, IfNoneMatch: r.H.IfNoneMatch})
	if isError(err) {
		return nil, nil, r.WriteError(http.StatusNotFound, nil, nil, "NotFound", "Tool call not found")
	}
	if err = r.CheckPreconditions(svrcore.ResourceValues{AllowedConditionals: svrcore.AllowedConditionalsMatch, ETag: tc.ETag}); isError(err) {
		return nil, nil, err
	}
	return ti, tc, nil
}

// getToolCallResource retrieves the ToolCall resource from the request.
func (ops *httpOps) getToolCallResource(ctx context.Context, r *svrcore.ReqRes) error {
	ti, tc, err := ops.preambleToolCallResource(ctx, r)
	if isError(err) {
		return err
	}
	return ti.Caller.Get(ctx, tc, r)
}

// postToolCallAdvance advances the state of a tool call using r's body (CreateMessageResult or ElicitResult)
func (ops *httpOps) postToolCallResourceAdvance(ctx context.Context, r *svrcore.ReqRes) error {
	ti, tc, err := ops.preambleToolCallResource(ctx, r)
	if isError(err) {
		return err
	}
	return ti.Caller.Advance(ctx, tc, r)
}

// postToolCallCancelResource cancels a tool call.
func (ops *httpOps) postToolCallCancelResource(ctx context.Context, r *svrcore.ReqRes) error {
	ti, tc, err := ops.preambleToolCallResource(ctx, r)
	if isError(err) {
		return err
	}
	return ti.Caller.Cancel(ctx, tc, r)
}

////////////////////////////////////////////////////////////////////////////////////////////////////////

// getToolList retrieves the list of tools.
func (ops *httpOps) getToolList(ctx context.Context, r *svrcore.ReqRes) error {
	if err := r.CheckPreconditions(svrcore.ResourceValues{AllowedConditionals: svrcore.AllowedConditionalsMatch, ETag: ops.etag()}); isError(err) {
		return err
	}
	result := mcp.ListToolsResult{Tools: make([]mcp.Tool, 0, len(ops.toolInfos))}
	for _, ti := range ops.toolInfos {
		result.Tools = append(result.Tools, *ti.Tool)
	}
	return r.WriteSuccess(http.StatusOK, &svrcore.ResponseHeader{ETag: ops.etag()}, nil, result)
}

// listToolCalls retrieves the list of tool calls.
func (ops *httpOps) listToolCalls(ctx context.Context, r *svrcore.ReqRes) error {
	body := any(nil)
	return r.WriteSuccess(http.StatusOK, &svrcore.ResponseHeader{ETag: ops.etag()}, nil, body)
}

// getResources retrieves the list of resources.
func (ops *httpOps) getResources(ctx context.Context, r *svrcore.ReqRes) error {
	return r.WriteSuccess(http.StatusNoContent, &svrcore.ResponseHeader{}, nil, nil)
}

// getResourcesTemplates retrieves the list of resource templates.
func (ops *httpOps) getResourcesTemplates(ctx context.Context, r *svrcore.ReqRes) error {
	return r.WriteSuccess(http.StatusNoContent, &svrcore.ResponseHeader{}, nil, nil)
}

// getResource retrieves a specific resource by name.
func (ops *httpOps) getResource(ctx context.Context, r *svrcore.ReqRes) error {
	resourceName := r.R.PathValue("name")
	if resourceName == "" {
		return r.WriteError(http.StatusBadRequest, nil, nil, "BadRequest", "Resource name is required")
	}
	return r.WriteSuccess(http.StatusNoContent, &svrcore.ResponseHeader{}, nil, nil)
}

// getPrompts retrieves the list of prompts.
func (ops *httpOps) getPrompts(ctx context.Context, r *svrcore.ReqRes) error {
	return r.WriteSuccess(http.StatusNoContent, &svrcore.ResponseHeader{}, nil, nil)
}

// getPrompt retrieves a specific prompt by name.
func (ops *httpOps) getPrompt(ctx context.Context, r *svrcore.ReqRes) error {
	return r.WriteSuccess(http.StatusNoContent, &svrcore.ResponseHeader{}, nil, nil)
}

// putRoots updates the list of root resources.
func (ops *httpOps) putRoots(ctx context.Context, r *svrcore.ReqRes) error {
	return r.WriteSuccess(http.StatusNoContent, &svrcore.ResponseHeader{}, nil, nil)
}

// postCompletion returns a text completion.
func (ops *httpOps) postCompletion(ctx context.Context, r *svrcore.ReqRes) error {
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

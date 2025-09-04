package v20250808

// For timeouts/cancellation, see https://ieftimov.com/posts/make-resilient-golang-net-http-servers-using-timeouts-deadlines-context-cancellation/

import (
	"bytes"
	"context"
	"crypto/md5"
	"fmt"
	"net/http"
	"sync"

	"github.com/JeffreyRichter/mcpsvc/mcp"
	"github.com/JeffreyRichter/mcpsvc/mcp/toolcalls"
	"github.com/JeffreyRichter/mcpsvc/resources"
	"github.com/JeffreyRichter/svrcore"
)

const v20250808 = "v20250808"

// singletons are defined as function variables so tests can insert mocks
// TODO: reconsider this architecture; a hierarchy of singletons complicates testing
var (
	GetOps = sync.OnceValue(func() *httpOperations {
		store := resources.GetToolCallStore()
		return &httpOperations{ToolCallStore: store}
	})
	GetToolInfos = sync.OnceValue(buildToolInfosMap)
)

func buildToolInfosMap() map[string]*ToolInfo {
	ops := GetOps()
	return map[string]*ToolInfo{
		"add": {
			Create:       ops.createToolCallAdd,
			Get:          ops.getToolCallAdd,
			Advance:      ops.advanceToolCallAdd,
			Cancel:       ops.cancelToolCallAdd,
			ProcessPhase: ops.processPhaseToolCallAdd,
			Tool: &mcp.Tool{
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
			},
		},
		"count": {
			Cancel: ops.cancelToolCallCount,
			Create: ops.createToolCallCount,
			Get:    ops.getToolCallCount,
			Tool: &mcp.Tool{
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
			},
		},
		"pii": {
			Create:  ops.createToolCallPII,
			Get:     ops.getToolCallPII,
			Advance: ops.advanceToolCallPII,
			Cancel:  ops.cancelToolCallPII,
			Tool: &mcp.Tool{
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
			},
		},
	}
}

type toolOpFunc func(ctx context.Context, toolCall *toolcalls.ToolCall, r *svrcore.ReqRes) error

type ToolInfo struct {
	Tool         *mcp.Tool
	Create       toolOpFunc
	Get          toolOpFunc
	Advance      toolOpFunc
	Cancel       toolOpFunc
	ProcessPhase toolcalls.ProcessPhaseFunc
}

// httpOperations wraps the version-agnostic resources (ToolCalls) with this specific api-version's HTTP operations: behavior wrapping state
type httpOperations struct{ resources.ToolCallStore }

// etag returns the ETag for this version's HTTP operations
func (ops *httpOperations) etag() *svrcore.ETag {
	return svrcore.Ptr(svrcore.ETag("v20250808"))
}

// lookupToolCall retrieves the ToolInfo and ToolCall from the given request URL (and authentication for tenant).
func (ops *httpOperations) lookupToolCall(r *svrcore.ReqRes) (*ToolInfo, *toolcalls.ToolCall, error) {
	tenant := "sometenant"
	toolName, toolCallId := r.R.PathValue("toolName"), r.R.PathValue("toolCallId")
	if toolName == "" {
		return nil, nil, r.Error(http.StatusBadRequest, "BadRequest", "Tool name required")
	}
	if toolCallId == "" {
		return nil, nil, r.Error(http.StatusBadRequest, "BadRequest", "Tool call ID required")
	}
	infos := GetToolInfos()
	ti, ok := infos[toolName]
	if !ok {
		return nil, nil, r.Error(http.StatusBadRequest, "BadRequest", "Tool '%s' not found", toolName)
	}
	return ti, toolcalls.NewToolCall(tenant, toolName, toolCallId), nil
}

/* TODO: Fix for error below:
pm, err := resources.NewPhaseMgr(hutdownCtx, "", ops.toolNameToProcessPhaseFunc)
if err != nil {
	return nil, err
}*/

// toolNameToProcessPhaseFunc converts a toolname to a function that knows how to advance the tool call's phase/state
func (ops *httpOperations) toolNameToProcessPhaseFunc(toolName string) (toolcalls.ProcessPhaseFunc, error) {
	ti, ok := GetToolInfos()[toolName]
	if !ok {
		return nil, fmt.Errorf("tool '%s' not found", toolName)
	}
	return ti.ProcessPhase, nil
}

// putToolCallResource creates a new tool call resource (idempotently if a retry occurs).
func (ops *httpOperations) putToolCallResource(ctx context.Context, r *svrcore.ReqRes) error {
	ti, tc, err := ops.lookupToolCall(r)
	if err != nil {
		return err
	}
	if err := r.ValidatePreconditions(svrcore.ResourceValues{AllowedConditionals: svrcore.AllowedConditionalsNone, ETag: tc.ETag}); err != nil {
		return err
	}

	// Calculate idempotency key based on something in the request that should be stable across retries
	// For example, the Date header (which must be present per RFC 7231)
	calcIdempotencyKey := func(s string) []byte { ik := md5.Sum(([]byte)(s)); return ik[:] }
	k := "TODO: FIX ME"
	if r.H.Date != nil {
		k = r.H.Date.String()
	}
	incomingIK := calcIdempotencyKey(k) // TODO: Maybe improve key value?
	if tc.IdempotencyKey != nil {                       // PUT on an existing tool call ID
		if tc.IdempotencyKey != nil && !bytes.Equal(*tc.IdempotencyKey, incomingIK) { // Not a retry
			return r.Error(http.StatusConflict, "Conflict", "Tool call ID already exists")
		}
	}
	if err := r.UnmarshalBody(&tc); err != nil {
		return err
	}
	tc.IdempotencyKey = &incomingIK
	return ti.Create(ctx, tc, r)
}

// getToolCallResource retrieves the ToolCall resource from the request.
func (ops *httpOperations) getToolCallResource(ctx context.Context, r *svrcore.ReqRes) error {
	ti, tc, err := ops.lookupToolCall(r)
	if err != nil {
		return err
	}
	err = ops.Get(ctx, tc, svrcore.AccessConditions{IfMatch: r.H.IfMatch, IfNoneMatch: r.H.IfNoneMatch})
	if err != nil {
		return r.Error(http.StatusNotFound, "NotFound", "Tool call not found")
	}
	if err = r.ValidatePreconditions(svrcore.ResourceValues{AllowedConditionals: svrcore.AllowedConditionalsMatch, ETag: tc.ETag}); err != nil {
		return err
	}
	return ti.Get(ctx, tc, r)
}

// postToolCallAdvance advances the state of a tool call using r's body (CreateMessageResult or ElicitResult)
func (ops *httpOperations) postToolCallAdvance(ctx context.Context, r *svrcore.ReqRes) error {
	ti, tc, err := ops.lookupToolCall(r)
	if err != nil {
		return err
	}
	err = ops.Get(ctx, tc, svrcore.AccessConditions{IfMatch: r.H.IfMatch, IfNoneMatch: r.H.IfNoneMatch})
	if err != nil {
		return r.Error(http.StatusNotFound, "NotFound", "Tool call not found")
	}
	if err = r.ValidatePreconditions(svrcore.ResourceValues{AllowedConditionals: svrcore.AllowedConditionalsMatch, ETag: tc.ETag}); err != nil {
		return err
	}
	return ti.Advance(ctx, tc, r)
}

// postToolCallCancelResource cancels a tool call.
func (ops *httpOperations) postToolCallCancelResource(ctx context.Context, r *svrcore.ReqRes) error {
	ti, tc, err := ops.lookupToolCall(r)
	if err != nil {
		return err
	}
	err = ops.Get(ctx, tc, svrcore.AccessConditions{IfMatch: r.H.IfMatch, IfNoneMatch: r.H.IfNoneMatch})
	if err != nil {
		return r.Error(http.StatusNotFound, "NotFound", "Tool call not found")
	}
	if err = r.ValidatePreconditions(svrcore.ResourceValues{AllowedConditionals: svrcore.AllowedConditionalsMatch, ETag: tc.ETag}); err != nil {
		return err
	}
	return ti.Cancel(ctx, tc, r)
}

////////////////////////////////////////////////////////////////////////////////////////////////////////

// getToolList retrieves the list of tools.
func (ops *httpOperations) getToolList(ctx context.Context, r *svrcore.ReqRes) error {
	if err := r.ValidatePreconditions(svrcore.ResourceValues{AllowedConditionals: svrcore.AllowedConditionalsMatch, ETag: svrcore.Ptr(svrcore.ETag(v20250808))}); err != nil {
		return err
	}
	info := GetToolInfos()
	result := mcp.ListToolsResult{
		Tools: make([]mcp.Tool, 0, len(info)),
	}
	for _, ti := range info {
		result.Tools = append(result.Tools, *ti.Tool)
	}
	return r.WriteResponse(&svrcore.ResponseHeader{
		ETag: svrcore.Ptr(svrcore.ETag(v20250808)),
	}, nil, http.StatusOK, result)
}

// listToolCalls retrieves the list of tool calls.
func (ops *httpOperations) listToolCalls(ctx context.Context, r *svrcore.ReqRes) error {
	body := any(nil)
	return r.WriteResponse(&svrcore.ResponseHeader{
		ETag: ops.etag(),
	}, nil, http.StatusOK, body)
}

// getResources retrieves the list of resources.
func (ops *httpOperations) getResources(ctx context.Context, r *svrcore.ReqRes) error {
	return r.WriteResponse(&svrcore.ResponseHeader{}, nil, http.StatusNoContent, nil)
}

// getResourcesTemplates retrieves the list of resource templates.
func (ops *httpOperations) getResourcesTemplates(ctx context.Context, r *svrcore.ReqRes) error {
	return r.WriteResponse(&svrcore.ResponseHeader{}, nil, http.StatusNoContent, nil)
}

// getResource retrieves a specific resource by name.
func (ops *httpOperations) getResource(ctx context.Context, r *svrcore.ReqRes) error {
	resourceName := r.R.PathValue("name")
	if resourceName == "" {
		return r.Error(http.StatusBadRequest, "BadRequest", "Resource name is required")
	}
	return r.WriteResponse(&svrcore.ResponseHeader{}, nil, http.StatusNoContent, nil)
}

// getPrompts retrieves the list of prompts.
func (ops *httpOperations) getPrompts(ctx context.Context, r *svrcore.ReqRes) error {
	return r.WriteResponse(&svrcore.ResponseHeader{}, nil, http.StatusNoContent, nil)
}

// getPrompt retrieves a specific prompt by name.
func (ops *httpOperations) getPrompt(ctx context.Context, r *svrcore.ReqRes) error {
	return r.WriteResponse(&svrcore.ResponseHeader{}, nil, http.StatusNoContent, nil)
}

// putRoots updates the list of root resources.
func (ops *httpOperations) putRoots(ctx context.Context, r *svrcore.ReqRes) error {
	return r.WriteResponse(&svrcore.ResponseHeader{}, nil, http.StatusNoContent, nil)
}

// postCompletion returns a text completion.
func (ops *httpOperations) postCompletion(ctx context.Context, r *svrcore.ReqRes) error {
	return r.WriteResponse(&svrcore.ResponseHeader{}, nil, http.StatusNoContent, nil)
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

package v20250808

// For timeouts/cancellation, see https://ieftimov.com/posts/make-resilient-golang-net-http-servers-using-timeouts-deadlines-context-cancellation/

import (
	"context"
	"net/http"
	"sync"

	"github.com/JeffreyRichter/mcpsvc/mcp"
	"github.com/JeffreyRichter/mcpsvc/mcp/toolcalls"
	"github.com/JeffreyRichter/mcpsvc/resources"
	si "github.com/JeffreyRichter/serviceinfra"
)

// singletons are defined as function variables so tests can insert mocks
var (
	GetOps = sync.OnceValue(func() *httpOperations {
		store := resources.GetToolCallStore()
		return &httpOperations{ToolCallStore: store}
	})
	GetToolInfos = sync.OnceValue(func() map[string]*ToolInfo {
		ops := GetOps()
		return map[string]*ToolInfo{
			"add": {
				Create:  ops.createToolCallAdd,
				Get:     ops.getToolCallAdd,
				Advance: ops.advanceToolCallAdd,
				Cancel:  ops.cancelToolCallAdd,
				Tool: &mcp.Tool{
					BaseMetadata: mcp.BaseMetadata{
						Name:  "add",
						Title: si.Ptr("Add two numbers"),
					},
					Description: si.Ptr("Add two numbers"),
					InputSchema: mcp.JSONSchema{
						Type: "object",
						Properties: &map[string]any{
							"x": map[string]any{
								"type":        "integer",
								"Description": si.Ptr("The first number"),
							},
							"y": map[string]any{
								"type":        "integer",
								"Description": si.Ptr("The second number"),
							},
						},
						Required: []string{"x", "y"},
					},
					OutputSchema: &mcp.JSONSchema{
						Type: "object",
						Properties: &map[string]any{
							"result": map[string]any{
								"type":        "integer",
								"Description": si.Ptr("The result of the addition"),
							},
						},
						Required: []string{"result"},
					},
					Annotations: &mcp.ToolAnnotations{
						Title:           si.Ptr("Add two numbers"),
						ReadOnlyHint:    si.Ptr(false),
						DestructiveHint: si.Ptr(false),
						IdempotentHint:  si.Ptr(true),
						OpenWorldHint:   si.Ptr(true),
					},
					Meta: mcp.Meta{"foo": "bar", "baz": "qux"},
				},
			},
		}
	})
)

type toolOp func(ctx context.Context, toolCall *toolcalls.ToolCall, r *si.ReqRes) error

type ToolInfo struct {
	Tool    *mcp.Tool
	Create  toolOp
	Get     toolOp
	Advance toolOp
	Cancel  toolOp
}

// httpOperations wraps the version-agnostic resources (ToolCalls) with this specific api-version's HTTP operations: behavior wrapping state
type httpOperations struct{ resources.ToolCallStore }

func (ops *httpOperations) etag() *si.ETag { return si.Ptr(si.ETag("v20250808")) }

func (ops *httpOperations) lookupToolCall(r *si.ReqRes) (*ToolInfo, *toolcalls.ToolCall, error) {
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
	return ti, toolcalls.NewToolCall(toolName, toolCallId), nil
}

func (ops *httpOperations) putToolCallResource(ctx context.Context, r *si.ReqRes) error {
	ti, tc, err := ops.lookupToolCall(r)
	if err != nil {
		return err
	}

	// PUT only if if-match matches; PUT only if if-none-match doesn't match
	// if no if-match, then create a new tool call if it doesn't exist; if does exist & a retry, return current tool state; else 409-conflict (already exists)
	// if if-match matches, then it does exist; return 409-conflict
	// If no if-none-match, then it does not exist; create a new tool call

	/* TODO: Find existing tool call or create new tool call for retry idempotency & concurrency
	tc, err := ops.Get(ctx, tenant, tcc.ToolName, tcc.ToolCallId, &toolcalls.AccessConditions{IfMatch: r.H.IfMatch, IfNoneMatch: r.H.IfNoneMatch})
	resourceExists := err == nil
	if resourceExists {
		if err := r.ValidatePreconditions(&si.PreconditionValues{ETag: tc.ETag}); err != nil {
			return err
		}
	}
	*/

	return ti.Create(ctx, tc, r)
}

func (ops *httpOperations) getToolCallResource(ctx context.Context, r *si.ReqRes) error {
	ti, tc, err := ops.lookupToolCall(r)
	if err != nil {
		return err
	}
	tc, err = ops.Get(ctx, tenant, tc, &toolcalls.AccessConditions{IfMatch: r.H.IfMatch, IfNoneMatch: r.H.IfNoneMatch})
	if err != nil {
		return r.Error(http.StatusNotFound, "NotFound", "Tool call not found")
	}
	if err = r.ValidatePreconditions(&si.PreconditionValues{ETag: tc.ETag}); err != nil {
		return err
	}
	return ti.Get(ctx, tc, r)
}

// toolCallAdvance advances the state of a tool call. The last parameter is one of: CallToolRequestParams's arguments, CreateMessageResult, or ElicitResult
func (ops *httpOperations) postToolCallAdvance(ctx context.Context, r *si.ReqRes) error {
	ti, tc, err := ops.lookupToolCall(r)
	if err != nil {
		return err
	}
	tc, err = ops.Get(ctx, tenant, tc, &toolcalls.AccessConditions{IfMatch: r.H.IfMatch, IfNoneMatch: r.H.IfNoneMatch})
	if err != nil {
		return r.Error(http.StatusNotFound, "NotFound", "Tool call not found")
	}
	if err = r.ValidatePreconditions(&si.PreconditionValues{ETag: tc.ETag}); err != nil {
		return err
	}
	return ti.Advance(ctx, tc, r)
}

func (ops *httpOperations) postToolCallCancelResource(ctx context.Context, r *si.ReqRes) error {
	ti, tc, err := ops.lookupToolCall(r)
	if err != nil {
		return err
	}
	tc, err = ops.Get(ctx, tenant, tc, &toolcalls.AccessConditions{IfMatch: r.H.IfMatch, IfNoneMatch: r.H.IfNoneMatch})
	if err != nil {
		return r.Error(http.StatusNotFound, "NotFound", "Tool call not found")
	}
	if err = r.ValidatePreconditions(&si.PreconditionValues{ETag: tc.ETag}); err != nil {
		return err
	}
	return ti.Cancel(ctx, tc, r)
}

////////////////////////////////////////////////////////////////////////////////////////////////////////

func (ops *httpOperations) getToolList(ctx context.Context, r *si.ReqRes) error {
	body := any(nil)
	return r.WriteResponse(&si.ResponseHeader{
		ETag: ops.etag(),
	}, nil, http.StatusOK, body)
}

func (ops *httpOperations) listToolCalls(ctx context.Context, r *si.ReqRes) error {
	body := any(nil)
	return r.WriteResponse(&si.ResponseHeader{
		ETag: ops.etag(),
	}, nil, http.StatusOK, body)
}

func (ops *httpOperations) getResources(ctx context.Context, r *si.ReqRes) error {
	return r.WriteResponse(&si.ResponseHeader{}, nil, http.StatusNoContent, nil)
}
func (ops *httpOperations) getResourcesTemplates(ctx context.Context, r *si.ReqRes) error {
	return r.WriteResponse(&si.ResponseHeader{}, nil, http.StatusNoContent, nil)
}
func (ops *httpOperations) getResource(ctx context.Context, r *si.ReqRes) error {
	resourceName := r.R.PathValue("name")
	if resourceName == "" {
		return r.Error(http.StatusBadRequest, "BadRequest", "Resource name is required")
	}
	return r.WriteResponse(&si.ResponseHeader{}, nil, http.StatusNoContent, nil)
}

func (ops *httpOperations) getPrompts(ctx context.Context, r *si.ReqRes) error {
	return r.WriteResponse(&si.ResponseHeader{}, nil, http.StatusNoContent, nil)
}
func (ops *httpOperations) getPrompt(ctx context.Context, r *si.ReqRes) error {
	return r.WriteResponse(&si.ResponseHeader{}, nil, http.StatusNoContent, nil)
}

func (ops *httpOperations) putRoots(ctx context.Context, r *si.ReqRes) error {
	return r.WriteResponse(&si.ResponseHeader{}, nil, http.StatusNoContent, nil)
}
func (ops *httpOperations) postCompletion(ctx context.Context, r *si.ReqRes) error {
	return r.WriteResponse(&si.ResponseHeader{}, nil, http.StatusNoContent, nil)
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

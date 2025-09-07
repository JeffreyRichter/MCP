package v20250808

// For timeouts/cancellation, see https://ieftimov.com/posts/make-resilient-golang-net-http-servers-using-timeouts-deadlines-context-cancellation/

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"sync"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/JeffreyRichter/mcpsvr/config"
	"github.com/JeffreyRichter/mcpsvr/mcp"
	"github.com/JeffreyRichter/mcpsvr/mcp/toolcalls"
	"github.com/JeffreyRichter/mcpsvr/resources"
	"github.com/JeffreyRichter/mcpsvr/resources/azresources"
	"github.com/JeffreyRichter/mcpsvr/resources/localresources"
	"github.com/JeffreyRichter/svrcore"
)

const v20250808 = "v20250808"

// singletons are defined as function variables so tests can insert mocks
// TODO: reconsider this architecture; a hierarchy of singletons complicates testing
var (
	GetOps = sync.OnceValue(func() *httpOperations {
		store := GetToolCallStore()
		return &httpOperations{ToolCallStore: store}
	})

	GetToolInfos = sync.OnceValue(buildToolInfosMap)

	// GetToolCallStore returns a singleton ToolCallStore. It's an exported variable so offline tests can replace the production default with a mock.
	GetToolCallStore = sync.OnceValue(func() resources.ToolCallStore {
		if config.Get().Local {
			return localresources.NewToolCallStore(context.TODO() /*shutdownCtx*/)
		}
		if cfg := config.Get(); cfg.AzuriteAccount != "" && cfg.AzuriteKey != "" {
			fmt.Println("Using Azurite for tool call storage")
			cred := must(azblob.NewSharedKeyCredential(cfg.AzuriteAccount, cfg.AzuriteKey))
			return azresources.NewToolCallStore(must(azblob.NewClientWithSharedKeyCredential(cfg.AzureBlobURL, cred, nil)))
		}
		cred := must(azidentity.NewDefaultAzureCredential(nil))
		serviceURL := must(url.Parse(config.Get().AzureBlobURL)).String()
		client := must(azblob.NewClient(serviceURL, cred, nil))
		return azresources.NewToolCallStore(client)
	})

	GetPhaseManager = sync.OnceValue(func() resources.PhaseMgr {
		ops := GetOps()
		return must(azresources.NewPhaseMgr(context.TODO() /*shutdownCtx*/, config.Get().AzureQueueURL, azresources.PhaseMgrConfig{
			Logger:                     slog.Default(),
			ToolNameToProcessPhaseFunc: ops.toolNameToProcessPhaseFunc,
		}, ops.ToolCallStore))
	})
)

func buildToolInfosMap() map[string]*ToolInfo {
	toolInfos := map[string]*ToolInfo{}
	ops := GetOps()
	for _, tc := range []ToolCaller{
		&addToolCaller{ops: ops},
		&countToolCaller{ops: ops},
		&piiToolCaller{ops: ops},
	} {
		if t := tc.Tool(); t != nil {
			toolInfos[t.Name] = &ToolInfo{Tool: t, Caller: tc}
		}
	}
	return toolInfos
}

// httpOperations wraps the version-agnostic resources (ToolCalls) with this specific api-version's HTTP operations: behavior wrapping state
type httpOperations struct{ resources.ToolCallStore }

// etag returns the ETag for this version's HTTP operations
func (ops *httpOperations) etag() *svrcore.ETag { return svrcore.Ptr(svrcore.ETag("v20250808")) }

// lookupToolCall retrieves the ToolInfo and ToolCall from the given request URL (and authentication for tenant).
// Writes an HTTP error response and returns a *ServerError if the tool name or tool call ID is missing or invalid.
func (*httpOperations) lookupToolCall(r *svrcore.ReqRes) (*ToolInfo, *toolcalls.ToolCall, error) {
	tenant := "sometenant"
	toolName, toolCallId := r.R.PathValue("toolName"), r.R.PathValue("toolCallId")
	if toolName == "" {
		return nil, nil, r.WriteError(http.StatusBadRequest, nil, nil, "BadRequest", "Tool name required")
	}
	if toolCallId == "" {
		return nil, nil, r.WriteError(http.StatusBadRequest, nil, nil, "BadRequest", "Tool call ID required")
	}
	infos := GetToolInfos()
	ti, ok := infos[toolName]
	if !ok {
		return nil, nil, r.WriteError(http.StatusBadRequest, nil, nil, "BadRequest", "Tool '%s' not found", toolName)
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
	return ti.Caller.ProcessPhase, nil
}

// putToolCallResource creates a new tool call resource (idempotently if a retry occurs).
// Writes an HTTP error response and returns a *ServerError if the tool name or tool call ID is missing or invalid.
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
	if tc.IdempotencyKey != nil { // PUT on an existing tool call ID
		if (tc.IdempotencyKey != nil) && (*tc.IdempotencyKey != *r.H.IdempotencyKey) { // Not a retry
			return r.WriteError(http.StatusConflict, nil, nil, "Conflict", "Tool call ID already exists")
		}
	}
	tc.IdempotencyKey = r.H.IdempotencyKey
	return ti.Caller.Create(ctx, tc, r)
}

// preambleToolCallResource retrieves the ToolInfo and ToolCall from the given request URL (and authentication for tenant),
// then retrieves the ToolCall resource from storage and validates preconditions.
// Writes an HTTP error response and returns a *ServerError if the tool name or tool call ID is missing or invalid,
// the ToolCall resource is not found, or preconditions are not met.
// This method is used is called by GET & POST (not PUT) because it assumes the resource must already exist.
func (ops *httpOperations) preambleToolCallResource(ctx context.Context, r *svrcore.ReqRes) (*ToolInfo, *toolcalls.ToolCall, error) {
	ti, tc, err := ops.lookupToolCall(r)
	if err != nil {
		return nil, nil, err
	}
	err = ops.Get(ctx, tc, svrcore.AccessConditions{IfMatch: r.H.IfMatch, IfNoneMatch: r.H.IfNoneMatch})
	if err != nil {
		return nil, nil, r.WriteError(http.StatusNotFound, nil, nil, "NotFound", "Tool call not found")
	}
	if err = r.ValidatePreconditions(svrcore.ResourceValues{AllowedConditionals: svrcore.AllowedConditionalsMatch, ETag: tc.ETag}); err != nil {
		return nil, nil, err
	}
	return ti, tc, nil
}

// getToolCallResource retrieves the ToolCall resource from the request.
func (ops *httpOperations) getToolCallResource(ctx context.Context, r *svrcore.ReqRes) error {
	ti, tc, err := ops.preambleToolCallResource(ctx, r)
	if err != nil {
		return err
	}
	return ti.Caller.Get(ctx, tc, r)
}

// postToolCallAdvance advances the state of a tool call using r's body (CreateMessageResult or ElicitResult)
func (ops *httpOperations) postToolCallResourceAdvance(ctx context.Context, r *svrcore.ReqRes) error {
	ti, tc, err := ops.preambleToolCallResource(ctx, r)
	if err != nil {
		return err
	}
	return ti.Caller.Advance(ctx, tc, r)
}

// postToolCallCancelResource cancels a tool call.
func (ops *httpOperations) postToolCallCancelResource(ctx context.Context, r *svrcore.ReqRes) error {
	ti, tc, err := ops.preambleToolCallResource(ctx, r)
	if err != nil {
		return err
	}
	return ti.Caller.Cancel(ctx, tc, r)
}

////////////////////////////////////////////////////////////////////////////////////////////////////////

// getToolList retrieves the list of tools.
func (ops *httpOperations) getToolList(ctx context.Context, r *svrcore.ReqRes) error {
	if err := r.ValidatePreconditions(svrcore.ResourceValues{AllowedConditionals: svrcore.AllowedConditionalsMatch, ETag: svrcore.Ptr(svrcore.ETag(v20250808))}); err != nil {
		return err
	}
	info := GetToolInfos()
	result := mcp.ListToolsResult{Tools: make([]mcp.Tool, 0, len(info))}
	for _, ti := range info {
		result.Tools = append(result.Tools, *ti.Tool)
	}
	return r.WriteSuccess(http.StatusOK, &svrcore.ResponseHeader{ETag: svrcore.Ptr(svrcore.ETag(v20250808))}, nil, result)
}

// listToolCalls retrieves the list of tool calls.
func (ops *httpOperations) listToolCalls(ctx context.Context, r *svrcore.ReqRes) error {
	body := any(nil)
	return r.WriteSuccess(http.StatusOK, &svrcore.ResponseHeader{ETag: ops.etag()}, nil, body)
}

// getResources retrieves the list of resources.
func (ops *httpOperations) getResources(ctx context.Context, r *svrcore.ReqRes) error {
	return r.WriteSuccess(http.StatusNoContent, &svrcore.ResponseHeader{}, nil, nil)
}

// getResourcesTemplates retrieves the list of resource templates.
func (ops *httpOperations) getResourcesTemplates(ctx context.Context, r *svrcore.ReqRes) error {
	return r.WriteSuccess(http.StatusNoContent, &svrcore.ResponseHeader{}, nil, nil)
}

// getResource retrieves a specific resource by name.
func (ops *httpOperations) getResource(ctx context.Context, r *svrcore.ReqRes) error {
	resourceName := r.R.PathValue("name")
	if resourceName == "" {
		return r.WriteError(http.StatusBadRequest, nil, nil, "BadRequest", "Resource name is required")
	}
	return r.WriteSuccess(http.StatusNoContent, &svrcore.ResponseHeader{}, nil, nil)
}

// getPrompts retrieves the list of prompts.
func (ops *httpOperations) getPrompts(ctx context.Context, r *svrcore.ReqRes) error {
	return r.WriteSuccess(http.StatusNoContent, &svrcore.ResponseHeader{}, nil, nil)
}

// getPrompt retrieves a specific prompt by name.
func (ops *httpOperations) getPrompt(ctx context.Context, r *svrcore.ReqRes) error {
	return r.WriteSuccess(http.StatusNoContent, &svrcore.ResponseHeader{}, nil, nil)
}

// putRoots updates the list of root resources.
func (ops *httpOperations) putRoots(ctx context.Context, r *svrcore.ReqRes) error {
	return r.WriteSuccess(http.StatusNoContent, &svrcore.ResponseHeader{}, nil, nil)
}

// postCompletion returns a text completion.
func (ops *httpOperations) postCompletion(ctx context.Context, r *svrcore.ReqRes) error {
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

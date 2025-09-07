package resources

import (
	"context"

	"github.com/JeffreyRichter/mcpsvr/mcp/toolcalls"
	"github.com/JeffreyRichter/svrcore"
)

// Resource type & operations pattern:
// 1. Define Resource Type struct & define api-agnostic resource type operations on this type
// 2. Construct global singleton instance/variable of the Resource Type used to call #1 methods
// 3. Define api-version Resource Type Operations struct with field of #1 & define api-specific HTTP operations on this type
// 4. Construct global singleton instance/variable of #3 wrapping #2 & set api-version routes to these var/methods

// ToolCallStore manages persistent storage of ToolCalls
type ToolCallStore interface {
	Get(ctx context.Context, tc *toolcalls.ToolCall, ac svrcore.AccessConditions) error
	Put(ctx context.Context, tc *toolcalls.ToolCall, ac svrcore.AccessConditions) error
	Delete(ctx context.Context, tc *toolcalls.ToolCall, ac svrcore.AccessConditions) error
}

type PhaseMgr interface {
	// StartPhaseProcessing: enqueues a new tool call phase with tool name & tool call id.
	StartPhaseProcessing(ctx context.Context, tc *toolcalls.ToolCall) error
}

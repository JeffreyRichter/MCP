package toolcall

import (
	"context"
	"encoding/json"
	"encoding/json/jsontext"
	"time"

	"github.com/JeffreyRichter/internal/aids"
	"github.com/JeffreyRichter/mcp"
	"github.com/JeffreyRichter/svrcore"
)

type (
	// Identity is the identity of a ToolCall whcih includes Tenant, ToolName, and ToolCallId
	Identity struct {
		Tenant   *string `json:"tenant"`
		ToolName *string `json:"toolname"`
		ID       *string `json:"id"` // Scoped within tenant & tool name
	}

	// Resource is the data model for the version-agnostic tool call resource type.
	Resource struct {
		Identity           `json:",inline"`
		Expiration         *time.Time              `json:"expiration,omitempty"`
		IdempotencyKey     *string                 `json:"idempotencyKey,omitempty"` // Used for retried PUTs to determine if PUT of same Request should be considered OK
		ETag               *svrcore.ETag           `json:"etag"`
		Phase              *string                 `json:"phase,omitempty"`
		Status             *mcp.Status             `json:"status,omitempty" enum:"running,awaitingSamplingResponse,awaitingElicitationResponse,success,failed,canceled"`
		Request            jsontext.Value          `json:"request,omitempty"`
		SamplingRequest    *mcp.SamplingRequest    `json:"samplingRequest,omitempty"`
		ElicitationRequest *mcp.ElicitationRequest `json:"elicitationRequest,omitempty"`
		ServerState        *string                 `json:"serverState,omitempty"` // Opaque ToolCall-specific state for round-tripping; allows some servers to avoid a durable state store
		Progress           jsontext.Value          `json:"progress,omitempty"`
		Result             jsontext.Value          `json:"result,omitempty"`
		Error              jsontext.Value          `json:"error,omitempty"`
		Internal           jsontext.Value          `json:"internal,omitempty"` // Tool-specific internal data, never returned to clients
	}

	// Store manages persistent storage of ToolCalls
	Store interface {
		// Put creates or updates the specified tool call in storage from the passed-in ToolCall struct.
		// On success, the ToolCall.ETag field is updated from the response ETag. Returns a
		// [svrcore.ServerError] if an error occurs.
		Put(ctx context.Context, tc *Resource, ac svrcore.AccessConditions) *svrcore.ServerError

		// Get retrieves the specified tool call from storage into the passed-in ToolCall struct or a
		// [svrcore.ServerError] if an error occurs.
		Get(ctx context.Context, tc *Resource, ac svrcore.AccessConditions) *svrcore.ServerError

		// Delete deletes the specified tool call from storage or returns a [svrcore.ServerError] if an error occurs.
		Delete(ctx context.Context, tc *Resource, ac svrcore.AccessConditions) *svrcore.ServerError
	}
)

// ToMCP convert the ToolCallResource to a public-facing MCP ToolCall returned to clients.
// It omits internal fields: Tenant, IdempotencyKey, Phase, Internal
func (tc *Resource) ToMCP() mcp.ToolCall {
	return mcp.ToolCall{
		ToolName:           tc.ToolName,
		ID:                 tc.ID,
		Expiration:         tc.Expiration,
		ETag:               (*string)(tc.ETag),
		Status:             tc.Status,
		Request:            tc.Request,
		SamplingRequest:    tc.SamplingRequest,
		ElicitationRequest: tc.ElicitationRequest,
		ServerState:        tc.ServerState,
		Progress:           tc.Progress,
		Result:             tc.Result,
		Error:              tc.Error,
	}
}

// New creates a new ToolCall with the specified tenant, tool name, and tool call ID.
func New(tenant, toolName, toolCallID string) *Resource {
	return &Resource{
		Identity:   Identity{Tenant: aids.New(tenant), ToolName: aids.New(toolName), ID: aids.New(toolCallID)},
		Expiration: aids.New(time.Now().Add(24 * time.Hour)), // Default maximum time a tool call lives
		Status:     aids.New(mcp.StatusSubmitted),
	}
}

// Copy returns a deep copy of tc
func (tc *Resource) Copy() Resource {
	aids.Assert(tc != nil, "ToolCall.Copy: tc is nil")
	b := aids.Must(json.Marshal(tc))
	cp := Resource{}
	aids.Must0(json.Unmarshal(b, &cp))
	return cp
}

type (
	// PhaseMgr manages the processing of tool call phases.
	PhaseMgr interface {
		// StartPhaseProcessing: enqueues a new tool call phase with tool name & tool call id.
		// It must succeed or panic due to internal server error.
		StartPhase(ctx context.Context, tc *Resource) *svrcore.ServerError
	}

	// PhaseProcessor processes the current phase of a tool call to its next phase.
	PhaseProcessor interface {
		// ExtendTime extends the allowed execution time for the current phase.
		// It must succeed or panic due to internal server error.
		ExtendTime(ctx context.Context, phaseExecutionTime time.Duration)
	}

	// ProcessPhaseFunc is the function signature for processing a tool call's current phase to its next phase.
	// It panics if phase processing fails.
	ProcessPhaseFunc func(context.Context, PhaseProcessor, *Resource)

	// ToolNameToProcessPhaseFunc maps a tool name to its ProcessPhaseFunc.
	// It must succeed or panic due to unrecognized tool name.
	ToolNameToProcessPhaseFunc func(toolName string) ProcessPhaseFunc
)

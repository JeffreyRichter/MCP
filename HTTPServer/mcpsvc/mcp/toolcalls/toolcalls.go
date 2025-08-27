package toolcalls

import (
	"context"
	"encoding/json/jsontext"
	"time"

	"github.com/JeffreyRichter/mcpsvc/mcp"
	si "github.com/JeffreyRichter/serviceinfra"
)

// ToolCall is the data model for the version-agnostic tool calls resource type.
type ToolCall struct {
	Tenant             *string             `json:"tenant"`
	ToolName           *string             `json:"name,omitempty" minlen:"3" maxlen:"64" regx:"^[a-zA-Z0-9_]+$"`
	ToolCallId         *string             `json:"toolCallId,omitempty" minlen:"3" maxlen:"64" regx:"^[a-zA-Z0-9_]+$"`
	Expiration         *time.Time          `json:"expiration,omitempty"`
	IdempotencyKey     *[]byte             `json:"idempotencyKey,omitempty"` // Used for retried PUTs to determine if PUT of same Request should be considered OK
	ETag               *si.ETag            `json:"etag"`
	Phase              *string             `json:"phase,omitempty"`
	AdvanceQueue       *string             `json:"advanceQueue,omitempty"` // Name of the queue to use for advancing the tool call's phase
	Attempt            *int32              `json:"attempt,omitempty"`      // Number of times this tool call has been attempted
	Created            *time.Time          `json:"created,omitempty"`
	Modified           *time.Time          `json:"modified,omitempty"`
	Status             *ToolCallStatus     `json:"status,omitempty" enum:"running,awaitingSamplingResponse,awaitingElicitationResponse,success,failed,canceled"`
	Request            jsontext.Value      `json:"request,omitempty"`
	SamplingRequest    *SamplingRequest    `json:"samplingRequest,omitempty"`
	ElicitationRequest *ElicitationRequest `json:"elicitationRequest,omitempty"`
	Progress           jsontext.Value      `json:"progress,omitempty"`
	Result             jsontext.Value      `json:"result,omitempty"`
	Error              jsontext.Value      `json:"error,omitempty"`
}

// TODO: Maybe use this for ToolCall's Result field & remove its Error field?
/*type ToolCallResult struct { // https://github.com/modelcontextprotocol/modelcontextprotocol/blob/main/schema/2025-06-18/schema.ts#L778
	Content           []mcp.ContentBlock `json:"content"`
	StructuredContent *map[string]any    `json:"structuredContent,omitempty"`
	IsError           *bool              `json:"isError,omitempty"`
}*/

func NewToolCall(tenant, toolName, toolCallId string) *ToolCall {
	return &ToolCall{
		Tenant:     si.Ptr(tenant),
		ToolName:   si.Ptr(toolName),
		ToolCallId: si.Ptr(toolCallId),
		Expiration: si.Ptr(time.Now().Add(24 * time.Hour)), // Default maximum time a tool call lives
		Status:     si.Ptr(ToolCallStatusSubmitted),
	}
}

type ToolCallStatus string

const (
	ToolCallStatusSubmitted                 ToolCallStatus = "submitted"
	ToolCallStatusRunning                   ToolCallStatus = "running"
	ToolCallStatusAwaitingSamplingResult    ToolCallStatus = "awaitingSamplingResult"
	ToolCallStatusAwaitingElicitationResult ToolCallStatus = "awaitingElicitationResult"
	ToolCallStatusSuccess                   ToolCallStatus = "success"
	ToolCallStatusFailed                    ToolCallStatus = "failed"
	ToolCallStatusCanceled                  ToolCallStatus = "canceled"
)

type AccessConditions struct {
	IfMatch     *si.ETag
	IfNoneMatch *si.ETag
}

type SamplingRequest struct { // https://github.com/modelcontextprotocol/modelcontextprotocol/blob/main/schema/2025-06-18/schema.ts#L986
	Messages []mcp.SamplingMessage `json:"messages"`

	// The server's preferences for which model to select. The client MAY ignore these preferences.
	ModelPreferences *mcp.ModelPreferences `json:"modelPreferences,omitempty"`

	// An optional system prompt the server wants to use for sampling. The client MAY modify or omit this prompt.
	SystemPrompt *string `json:"systemPrompt,omitempty"`

	// A request to include context from one or more MCP servers (including the caller), to be attached to the prompt. The client MAY ignore this request.
	IncludeContext *string `json:"includeContext,omitempty"` // "none" | "thisServer" | "allServers"

	// @TJS-type number
	Temperature *float64 `json:"temperature,omitempty"`

	// The maximum number of tokens to sample, as requested by the server. The client MAY choose to sample fewer tokens than requested.
	MaxTokens     *int64   `json:"maxTokens,omitempty"`
	StopSequences []string `json:"stopSequences,omitempty"`

	// Optional metadata to pass through to the LLM provider. The format of this metadata is provider-specific.
	Metadata map[string]any `json:"metadata,omitempty"`
}

type SamplingResult struct { // https://github.com/modelcontextprotocol/modelcontextprotocol/blob/main/schema/2025-06-18/schema.ts#L1023
	SamplingMessage mcp.SamplingMessage `json:"samplingMessage"`
	Model           string              `json:"model"`
	StopReason      *string             `json:"stopReason,omitempty"` // "endTurn" | "stopSequence" | "maxTokens" | string
}

type ElicitationRequest struct {
	Message         string `json:"message"`
	RequestedSchema struct {
		Type       string                                   `json:"type"`
		Properties map[string]mcp.PrimitiveSchemaDefinition `json:"properties"`
		Required   []string                                 `json:"required,omitempty"`
	} `json:"requestedSchema"`
}

type ElicitationResult struct {
	Action  string          `json:"action"` // "accept" | "decline" | "cancel"
	Content *map[string]any `json:"content,omitempty"`
}

/********************* Types for Phase Processing ***************/

type ProcessPhaseFunc func(context.Context, *ToolCall, PhaseProcessor) (*ToolCall, error)

type ToolNameToProcessPhaseFunc func(toolName string) (ProcessPhaseFunc, error)

type PhaseProcessor interface {
	ExtendProcessingTime(ctx context.Context, phaseExecutionTime time.Duration) error
}

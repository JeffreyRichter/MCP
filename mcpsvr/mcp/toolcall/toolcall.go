package toolcall

import (
	"context"
	"encoding/json"
	"encoding/json/jsontext"
	"errors"
	"fmt"
	"time"

	"github.com/JeffreyRichter/mcpsvr/mcp"
	"github.com/JeffreyRichter/svrcore"
)

// Identity is the identity of a ToolCall whcih includes Tenant, ToolName, and ToolCallId
type Identity struct {
	Tenant   *string `json:"tenant"`
	ToolName *string `json:"toolname"`
	ID       *string `json:"id"` // Scoped within tenant & tool name
}

// ToolCall is the data model for the version-agnostic tool call resource type.
type ToolCall struct {
	Identity           `json:",inline"`
	Expiration         *time.Time          `json:"expiration,omitempty"`
	IdempotencyKey     *string             `json:"idempotencyKey,omitempty"` // Used for retried PUTs to determine if PUT of same Request should be considered OK
	ETag               *svrcore.ETag       `json:"etag"`
	Phase              *string             `json:"phase,omitempty"`
	Status             *Status             `json:"status,omitempty" enum:"running,awaitingSamplingResponse,awaitingElicitationResponse,success,failed,canceled"`
	Request            jsontext.Value      `json:"request,omitempty"`
	SamplingRequest    *SamplingRequest    `json:"samplingRequest,omitempty"`
	ElicitationRequest *ElicitationRequest `json:"elicitationRequest,omitempty"`
	Progress           jsontext.Value      `json:"progress,omitempty"`
	Result             jsontext.Value      `json:"result,omitempty"`
	Error              jsontext.Value      `json:"error,omitempty"`
}

// New creates a new ToolCall with the specified tenant, tool name, and tool call ID.
func New(tenant, toolName, toolCallID string) *ToolCall {
	return &ToolCall{
		Identity:   Identity{Tenant: svrcore.Ptr(tenant), ToolName: svrcore.Ptr(toolName), ID: svrcore.Ptr(toolCallID)},
		Expiration: svrcore.Ptr(time.Now().Add(24 * time.Hour)), // Default maximum time a tool call lives
		Status:     svrcore.Ptr(StatusSubmitted),
	}
}

// Copy returns a deep copy of tc
func (tc *ToolCall) Copy() ToolCall {
	if tc == nil {
		panic("tc can't be nil")
	}
	cp := ToolCall{}
	buffer := must(json.Marshal(tc))
	must(0, json.Unmarshal(buffer, &cp))
	return cp
}

type Status string

const (
	StatusSubmitted                 Status = "submitted"
	StatusRunning                   Status = "running"
	StatusAwaitingSamplingResult    Status = "awaitingSamplingResult"
	StatusAwaitingElicitationResult Status = "awaitingElicitationResult"
	StatusSuccess                   Status = "success"
	StatusFailed                    Status = "failed"
	StatusCanceled                  Status = "canceled"
)

// Store manages persistent storage of ToolCalls
type Store interface {
	Put(ctx context.Context, tc *ToolCall, ac svrcore.AccessConditions) error
	Get(ctx context.Context, tc *ToolCall, ac svrcore.AccessConditions) error
	Delete(ctx context.Context, tc *ToolCall, ac svrcore.AccessConditions) error
}

/********************* Types for Phase Processing ***************/
type PhaseMgr interface {
	// StartPhaseProcessing: enqueues a new tool call phase with tool name & tool call id.
	StartPhase(ctx context.Context, tc *ToolCall) error
}

type PhaseProcessor interface {
	ExtendTime(ctx context.Context, phaseExecutionTime time.Duration) error
}

type ProcessPhaseFunc func(context.Context, *ToolCall, PhaseProcessor) error

type ToolNameToProcessPhaseFunc func(toolName string) (ProcessPhaseFunc, error)

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

func (er *ElicitationRequest) UnmarshalJSON(data []byte) error {
	// unmarshal into a temporary struct to handle the nested structure
	var temp struct {
		Message         string `json:"message"`
		RequestedSchema struct {
			Type       string         `json:"type"`
			Properties map[string]any `json:"properties"`
			Required   []string       `json:"required,omitempty"`
		} `json:"requestedSchema"`
	}

	if err := json.Unmarshal(data, &temp); err != nil {
		return err
	}

	er.Message = temp.Message
	er.RequestedSchema.Type = temp.RequestedSchema.Type
	er.RequestedSchema.Required = temp.RequestedSchema.Required
	er.RequestedSchema.Properties = make(map[string]mcp.PrimitiveSchemaDefinition)
	for propName, propValue := range temp.RequestedSchema.Properties {
		propMap, ok := propValue.(map[string]any)
		if !ok {
			return errors.New("invalid property format for " + propName)
		}
		typeValue, ok := propMap["type"].(string)
		if !ok {
			return errors.New("missing type for property " + propName)
		}
		if enumValue, hasEnum := propMap["enum"]; hasEnum {
			var schema mcp.EnumSchema
			schema.Type = typeValue
			if title, ok := propMap["title"].(string); ok {
				schema.Title = &title
			}
			if desc, ok := propMap["description"].(string); ok {
				schema.Description = &desc
			}
			if enumSlice, ok := enumValue.([]any); ok {
				schema.Enum = make([]string, len(enumSlice))
				for i, v := range enumSlice {
					if str, ok := v.(string); ok {
						schema.Enum[i] = str
					} else {
						return fmt.Errorf("invalid enum value for property %s", propName)
					}
				}
			}
			if enumNames, ok := propMap["enumNames"].([]any); ok {
				schema.EnumNames = make([]string, len(enumNames))
				for i, v := range enumNames {
					if str, ok := v.(string); ok {
						schema.EnumNames[i] = str
					}
				}
			}
			er.RequestedSchema.Properties[propName] = schema
			continue
		}
		switch typeValue {
		case "string":
			var schema mcp.StringSchema
			schema.Type = typeValue
			if title, ok := propMap["title"].(string); ok {
				schema.Title = &title
			}
			if desc, ok := propMap["description"].(string); ok {
				schema.Description = &desc
			}
			if minLen, ok := propMap["minLength"].(float64); ok {
				intVal := int(minLen)
				schema.MinLength = &intVal
			}
			if maxLen, ok := propMap["maxLength"].(float64); ok {
				intVal := int(maxLen)
				schema.MaxLength = &intVal
			}
			if format, ok := propMap["format"].(string); ok {
				schema.Format = &format
			}
			er.RequestedSchema.Properties[propName] = schema

		case "number", "integer":
			var schema mcp.NumberSchema
			schema.Type = typeValue
			if title, ok := propMap["title"].(string); ok {
				schema.Title = &title
			}
			if desc, ok := propMap["description"].(string); ok {
				schema.Description = &desc
			}
			if min, ok := propMap["minimum"].(float64); ok {
				schema.Minimum = &min
			}
			if max, ok := propMap["maximum"].(float64); ok {
				schema.Maximum = &max
			}
			er.RequestedSchema.Properties[propName] = schema

		case "boolean":
			var schema mcp.BooleanSchema
			schema.Type = typeValue
			if title, ok := propMap["title"].(string); ok {
				schema.Title = &title
			}
			if desc, ok := propMap["description"].(string); ok {
				schema.Description = &desc
			}
			if def, ok := propMap["default"].(bool); ok {
				schema.Default = &def
			}
			er.RequestedSchema.Properties[propName] = schema

		default:
			return fmt.Errorf("unknown primitive schema type: %s for property %s", typeValue, propName)
		}
	}

	return nil
}

func must[T any](val T, err error) T {
	if err != nil {
		panic(err)
	}
	return val
}

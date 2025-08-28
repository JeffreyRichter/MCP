package toolcalls

import (
	"context"
	"encoding/json"
	"encoding/json/jsontext"
	"errors"
	"fmt"
	"time"

	"github.com/JeffreyRichter/mcpsvc/mcp"
	"github.com/JeffreyRichter/svrcore"
)

type ToolCallIdentity struct {
	Tenant     *string `json:"tenant"`
	ToolName   *string `json:"name"`
	ToolCallId *string `json:"toolCallId"`
}

// ToolCall is the data model for the version-agnostic tool calls resource type.
type ToolCall struct {
	ToolCallIdentity `json:",inline"`
	Tenant           *string       `json:"tenant"`
	ToolName         *string       `json:"name,omitempty" minlen:"3" maxlen:"64" regx:"^[a-zA-Z0-9_]+$"`
	ToolCallId       *string       `json:"toolCallId,omitempty" minlen:"3" maxlen:"64" regx:"^[a-zA-Z0-9_]+$"`
	Expiration       *time.Time    `json:"expiration,omitempty"`
	IdempotencyKey   *[]byte       `json:"idempotencyKey,omitempty"` // Used for retried PUTs to determine if PUT of same Request should be considered OK
	ETag             *svrcore.ETag `json:"etag"`
	// Phase is for internal LRO processing; never send it to clients. It applies only during status "running" (it's a tool-specific subdivision of
	// Status: running because Status isn't granular enough for LROs). A phase transition implies persisting the ToolCall.
	Phase              *string             `json:"phase,omitempty"`
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
		ToolCallIdentity: ToolCallIdentity{Tenant: svrcore.Ptr(tenant), ToolName: svrcore.Ptr(toolName), ToolCallId: svrcore.Ptr(toolCallId)},
		Expiration:       svrcore.Ptr(time.Now().Add(24 * time.Hour)), // Default maximum time a tool call lives
		Status:           svrcore.Ptr(ToolCallStatusSubmitted),
	}
}

// Copy (deeply) the ToolCall
func (tc *ToolCall) Copy() ToolCall {
	if tc == nil {
		panic("tc can't be nil")
	}
	cp := ToolCall{}
	buffer := must(json.Marshal(tc))
	must(0, json.Unmarshal(buffer, &cp))
	return cp
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

/********************* Types for Phase Processing ***************/

type ProcessPhaseFunc func(context.Context, *ToolCall, PhaseProcessor) error

type ToolNameToProcessPhaseFunc func(toolName string) (ProcessPhaseFunc, error)

type PhaseProcessor interface {
	ExtendProcessingTime(ctx context.Context, phaseExecutionTime time.Duration) error
}

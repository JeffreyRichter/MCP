// https://raw.githubusercontent.com/modelcontextprotocol/modelcontextprotocol/refs/heads/main/schema/2025-06-18/schema.ts
// https://github.com/modelcontextprotocol/modelcontextprotocol/issues/1319
package mcp

import (
	"encoding/json"
	"errors"
)

// Constants
const (
	LatestProtocolVersion = "2025-06-18"
	JSONRPCVersion        = "2.0"
)

// Standard JSON-RPC error codes
const (
	ParseError     = -32700
	InvalidRequest = -32600
	MethodNotFound = -32601
	InvalidParams  = -32602
	InternalError  = -32603
)

// Basic types
type ProgressToken interface{} // string | number
type Cursor string
type RequestID interface{} // string | number
type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

// LoggingLevel represents the severity of a log message
type LoggingLevel string

const (
	LoggingDebug     LoggingLevel = "debug"
	LoggingInfo      LoggingLevel = "info"
	LoggingNotice    LoggingLevel = "notice"
	LoggingWarning   LoggingLevel = "warning"
	LoggingError     LoggingLevel = "error"
	LoggingCritical  LoggingLevel = "critical"
	LoggingAlert     LoggingLevel = "alert"
	LoggingEmergency LoggingLevel = "emergency"
)

// Core JSON-RPC types
type JSONRPCMessage interface {
	isJSONRPCMessage()
}

type JSONRPCRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      RequestID   `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

func (r JSONRPCRequest) isJSONRPCMessage() {}

type JSONRPCNotification struct {
	JSONRPC string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

func (n JSONRPCNotification) isJSONRPCMessage() {}

type JSONRPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      RequestID   `json:"id"`
	Result  interface{} `json:"result"`
}

func (r JSONRPCResponse) isJSONRPCMessage() {}

type JSONRPCError struct {
	JSONRPC string    `json:"jsonrpc"`
	ID      RequestID `json:"id"`
	Error   struct {
		Code    int         `json:"code"`
		Message string      `json:"message"`
		Data    interface{} `json:"data,omitempty"`
	} `json:"error"`
}

func (e JSONRPCError) isJSONRPCMessage() {}

// Meta field for requests and results
type Meta map[string]interface{}

// Request base structure
type RequestParams struct {
	Meta *RequestMeta `json:"_meta,omitempty"`
}

type RequestMeta struct {
	ProgressToken *ProgressToken `json:"progressToken,omitempty"`
	Extra         map[string]interface{}
}

/*func (m *RequestMeta) UnmarshalJSON(data []byte) error {
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); aids.IsError(err){
		return err
	}

	if token, ok := raw["progressToken"]; ok {
		m.ProgressToken = &token
		delete(raw, "progressToken")
	}

	m.Extra = raw
	return nil
}*/

func (m RequestMeta) MarshalJSON() ([]byte, error) {
	result := make(map[string]interface{})
	for k, v := range m.Extra {
		result[k] = v
	}
	if m.ProgressToken != nil {
		result["progressToken"] = *m.ProgressToken
	}
	return json.Marshal(result)
}

type Result struct {
	Meta Meta `json:"_meta,omitempty"`
}

type EmptyResult = Result

// Base metadata interface
type BaseMetadata struct {
	Name  string  `json:"name"`
	Title *string `json:"title,omitempty"`
}

// Implementation info
type Implementation struct {
	BaseMetadata
	Version string `json:"version"`
}

// Cancellation
type CancelledNotificationParams struct {
	RequestID RequestID `json:"requestId"`
	Reason    *string   `json:"reason,omitempty"`
}

// Initialization
type ClientCapabilities struct {
	Experimental *map[string]interface{} `json:"experimental,omitempty"`
	Roots        *struct {
		ListChanged *bool `json:"listChanged,omitempty"`
	} `json:"roots,omitempty"`
	Sampling    *interface{} `json:"sampling,omitempty"`
	Elicitation *interface{} `json:"elicitation,omitempty"`
}

type ServerCapabilities struct {
	Experimental *map[string]interface{} `json:"experimental,omitempty"`
	Logging      *interface{}            `json:"logging,omitempty"`
	Completions  *interface{}            `json:"completions,omitempty"`
	Prompts      *struct {
		ListChanged *bool `json:"listChanged,omitempty"`
	} `json:"prompts,omitempty"`
	Resources *struct {
		Subscribe   *bool `json:"subscribe,omitempty"`
		ListChanged *bool `json:"listChanged,omitempty"`
	} `json:"resources,omitempty"`
	Tools *struct {
		ListChanged *bool `json:"listChanged,omitempty"`
	} `json:"tools,omitempty"`
}

type InitializeRequestParams struct {
	RequestParams
	ProtocolVersion string             `json:"protocolVersion"`
	Capabilities    ClientCapabilities `json:"capabilities"`
	ClientInfo      Implementation     `json:"clientInfo"`
}

type InitializeResult struct {
	Result
	ProtocolVersion string             `json:"protocolVersion"`
	Capabilities    ServerCapabilities `json:"capabilities"`
	ServerInfo      Implementation     `json:"serverInfo"`
	Instructions    *string            `json:"instructions,omitempty"`
}

// Progress notifications
type ProgressNotificationParams struct {
	ProgressToken ProgressToken `json:"progressToken"`
	Progress      float64       `json:"progress"`
	Total         *float64      `json:"total,omitempty"`
	Message       *string       `json:"message,omitempty"`
}

// Pagination
type PaginatedRequestParams struct {
	RequestParams
	Cursor *Cursor `json:"cursor,omitempty"`
}

type PaginatedResult struct {
	Result
	NextCursor *Cursor `json:"nextCursor,omitempty"`
}

// Annotations
type Annotations struct {
	Audience     []Role   `json:"audience,omitempty"`
	Priority     *float64 `json:"priority,omitempty"`
	LastModified *string  `json:"lastModified,omitempty"`
}

// Content types
type ContentBlock interface {
	isContentBlock()
}

type TextContent struct {
	Type        string       `json:"type"`
	Text        string       `json:"text"`
	Annotations *Annotations `json:"annotations,omitempty"`
	Meta        Meta         `json:"_meta,omitempty"`
}

func (t TextContent) isContentBlock() {}

type ImageContent struct {
	Type        string       `json:"type"`
	Data        string       `json:"data"`
	MimeType    string       `json:"mimeType"`
	Annotations *Annotations `json:"annotations,omitempty"`
	Meta        Meta         `json:"_meta,omitempty"`
}

func (i ImageContent) isContentBlock() {}

type AudioContent struct {
	Type        string       `json:"type"`
	Data        string       `json:"data"`
	MimeType    string       `json:"mimeType"`
	Annotations *Annotations `json:"annotations,omitempty"`
	Meta        Meta         `json:"_meta,omitempty"`
}

func (a AudioContent) isContentBlock() {}

// Resources
type Resource struct {
	BaseMetadata
	URI         string       `json:"uri"`
	Description *string      `json:"description,omitempty"`
	MimeType    *string      `json:"mimeType,omitempty"`
	Annotations *Annotations `json:"annotations,omitempty"`
	Size        *int64       `json:"size,omitempty"`
	Meta        Meta         `json:"_meta,omitempty"`
}

type ResourceTemplate struct {
	BaseMetadata
	URITemplate string       `json:"uriTemplate"`
	Description *string      `json:"description,omitempty"`
	MimeType    *string      `json:"mimeType,omitempty"`
	Annotations *Annotations `json:"annotations,omitempty"`
	Meta        Meta         `json:"_meta,omitempty"`
}

type ResourceContents struct {
	URI      string  `json:"uri"`
	MimeType *string `json:"mimeType,omitempty"`
	Meta     Meta    `json:"_meta,omitempty"`
}

type TextResourceContents struct {
	ResourceContents
	Text string `json:"text"`
}

type BlobResourceContents struct {
	ResourceContents
	Blob string `json:"blob"`
}

type ResourceLink struct {
	Resource
	Type string `json:"type"`
}

func (r ResourceLink) isContentBlock() {}

type EmbeddedResource struct {
	Type        string       `json:"type"`
	Resource    interface{}  `json:"resource"` // TextResourceContents | BlobResourceContents
	Annotations *Annotations `json:"annotations,omitempty"`
	Meta        Meta         `json:"_meta,omitempty"`
}

func (e EmbeddedResource) isContentBlock() {}

// Resource requests/responses
type ListResourcesResult struct {
	PaginatedResult
	Resources []Resource `json:"resources"`
}

type ListResourceTemplatesResult struct {
	PaginatedResult
	ResourceTemplates []ResourceTemplate `json:"resourceTemplates"`
}

type ReadResourceRequestParams struct {
	RequestParams
	URI string `json:"uri"`
}

type ReadResourceResult struct {
	Result
	Contents []interface{} `json:"contents"` // []TextResourceContents | BlobResourceContents
}

type SubscribeRequestParams struct {
	RequestParams
	URI string `json:"uri"`
}

type UnsubscribeRequestParams struct {
	RequestParams
	URI string `json:"uri"`
}

type ResourceUpdatedNotificationParams struct {
	URI string `json:"uri"`
}

// Prompts
type Prompt struct {
	BaseMetadata
	Description *string          `json:"description,omitempty"`
	Arguments   []PromptArgument `json:"arguments,omitempty"`
	Meta        Meta             `json:"_meta,omitempty"`
}

type PromptArgument struct {
	BaseMetadata
	Description *string `json:"description,omitempty"`
	Required    *bool   `json:"required,omitempty"`
}

type PromptMessage struct {
	Role    Role         `json:"role"`
	Content ContentBlock `json:"content"`
}

type ListPromptsResult struct {
	PaginatedResult
	Prompts []Prompt `json:"prompts"`
}

type GetPromptRequestParams struct {
	RequestParams
	Name      string             `json:"name"`
	Arguments *map[string]string `json:"arguments,omitempty"`
}

type GetPromptResult struct {
	Result
	Description *string         `json:"description,omitempty"`
	Messages    []PromptMessage `json:"messages"`
}

// Tools
type ToolAnnotations struct {
	Title           *string `json:"title,omitempty"`
	ReadOnlyHint    *bool   `json:"readOnlyHint,omitempty"`
	DestructiveHint *bool   `json:"destructiveHint,omitempty"`
	IdempotentHint  *bool   `json:"idempotentHint,omitempty"`
	OpenWorldHint   *bool   `json:"openWorldHint,omitempty"`
}

type JSONSchema struct {
	Type       string                  `json:"type"`
	Properties *map[string]interface{} `json:"properties,omitempty"`
	Required   []string                `json:"required,omitempty"`
}

type Tool struct {
	BaseMetadata
	Description  *string          `json:"description,omitempty"`
	InputSchema  JSONSchema       `json:"inputSchema"`
	OutputSchema *JSONSchema      `json:"outputSchema,omitempty"`
	Annotations  *ToolAnnotations `json:"annotations,omitempty"`
	Meta         Meta             `json:"_meta,omitempty"`
}

type ListToolsResult struct {
	PaginatedResult
	Tools []Tool `json:"tools"`
}

type CallToolRequestParams struct {
	RequestParams
	Name      string                  `json:"name"`
	Arguments *map[string]interface{} `json:"arguments,omitempty"`
}

type CallToolResult struct {
	Result
	Content           []ContentBlock          `json:"content"`
	StructuredContent *map[string]interface{} `json:"structuredContent,omitempty"`
	IsError           *bool                   `json:"aids.IsError,omitempty"`
}

// Logging
type SetLevelRequestParams struct {
	RequestParams
	Level LoggingLevel `json:"level"`
}

type LoggingMessageNotificationParams struct {
	Level  LoggingLevel `json:"level"`
	Logger *string      `json:"logger,omitempty"`
	Data   interface{}  `json:"data"`
}

// Sampling
type SamplingMessage struct {
	Role    Role         `json:"role"`
	Content ContentBlock `json:"content"` // TextContent | ImageContent | AudioContent
}

type ModelHint struct {
	Name *string `json:"name,omitempty"`
}

type ModelPreferences struct {
	Hints                []ModelHint `json:"hints,omitempty"`
	CostPriority         *float64    `json:"costPriority,omitempty"`
	SpeedPriority        *float64    `json:"speedPriority,omitempty"`
	IntelligencePriority *float64    `json:"intelligencePriority,omitempty"`
}

type CreateMessageRequestParams struct {
	RequestParams
	Messages         []SamplingMessage `json:"messages"`
	ModelPreferences *ModelPreferences `json:"modelPreferences,omitempty"`
	SystemPrompt     *string           `json:"systemPrompt,omitempty"`
	IncludeContext   *string           `json:"includeContext,omitempty"` // "none" | "thisServer" | "allServers"
	Temperature      *float64          `json:"temperature,omitempty"`
	MaxTokens        int               `json:"maxTokens"`
	StopSequences    []string          `json:"stopSequences,omitempty"`
	Metadata         *interface{}      `json:"metadata,omitempty"`
}

type CreateMessageResult struct {
	Result
	SamplingMessage
	Model      string  `json:"model"`
	StopReason *string `json:"stopReason,omitempty"` // "endTurn" | "stopSequence" | "maxTokens" | string
}

// Autocompletion
type PromptReference struct {
	BaseMetadata
	Type string `json:"type"`
}

type ResourceTemplateReference struct {
	Type string `json:"type"`
	URI  string `json:"uri"`
}

type CompleteRequestParams struct {
	RequestParams
	Ref      interface{} `json:"ref"` // PromptReference | ResourceTemplateReference
	Argument struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	} `json:"argument"`
	Context *struct {
		Arguments *map[string]string `json:"arguments,omitempty"`
	} `json:"context,omitempty"`
}

type CompleteResult struct {
	Result
	Completion struct {
		Values  []string `json:"values"`
		Total   *int     `json:"total,omitempty"`
		HasMore *bool    `json:"hasMore,omitempty"`
	} `json:"completion"`
}

// Roots
type Root struct {
	URI  string  `json:"uri"`
	Name *string `json:"name,omitempty"`
	Meta Meta    `json:"_meta,omitempty"`
}

type ListRootsResult struct {
	Result
	Roots []Root `json:"roots"`
}

// Elicitation
type StringSchema struct {
	Type        string  `json:"type"`
	Title       *string `json:"title,omitempty"`
	Description *string `json:"description,omitempty"`
	MinLength   *int    `json:"minLength,omitempty"`
	MaxLength   *int    `json:"maxLength,omitempty"`
	Format      *string `json:"format,omitempty"` // "email" | "uri" | "date" | "date-time"
}

type NumberSchema struct {
	Type        string   `json:"type"` // "number" | "integer"
	Title       *string  `json:"title,omitempty"`
	Description *string  `json:"description,omitempty"`
	Minimum     *float64 `json:"minimum,omitempty"`
	Maximum     *float64 `json:"maximum,omitempty"`
}

type BooleanSchema struct {
	Type        string  `json:"type"`
	Title       *string `json:"title,omitempty"`
	Description *string `json:"description,omitempty"`
	Default     *bool   `json:"default,omitempty"`
}

type EnumSchema struct {
	Type        string   `json:"type"`
	Title       *string  `json:"title,omitempty"`
	Description *string  `json:"description,omitempty"`
	Enum        []string `json:"enum"`
	EnumNames   []string `json:"enumNames,omitempty"`
}

type PrimitiveSchemaDefinition interface {
	isPrimitiveSchema()
}

func (s StringSchema) isPrimitiveSchema()  {}
func (s NumberSchema) isPrimitiveSchema()  {}
func (s BooleanSchema) isPrimitiveSchema() {}
func (s EnumSchema) isPrimitiveSchema()    {}

// UnmarshalPrimitiveSchemaDefinition unmarshals JSON into the appropriate concrete type
// based on the "type" field in the JSON object
func UnmarshalPrimitiveSchemaDefinition(data []byte) (PrimitiveSchemaDefinition, error) {
	var typeCheck struct {
		Type string `json:"type"`
		Enum []any  `json:"enum,omitempty"`
	}

	if err := json.Unmarshal(data, &typeCheck); err != nil {
		return nil, err
	}

	// If enum field exists, it's an EnumSchema regardless of type
	if typeCheck.Enum != nil {
		var schema EnumSchema
		if err := json.Unmarshal(data, &schema); err != nil {
			return nil, err
		}
		return schema, nil
	}

	switch typeCheck.Type {
	case "string":
		var schema StringSchema
		if err := json.Unmarshal(data, &schema); err != nil {
			return nil, err
		}
		return schema, nil
	case "number", "integer":
		var schema NumberSchema
		if err := json.Unmarshal(data, &schema); err != nil {
			return nil, err
		}
		return schema, nil
	case "boolean":
		var schema BooleanSchema
		if err := json.Unmarshal(data, &schema); err != nil {
			return nil, err
		}
		return schema, nil
	default:
		return nil, errors.New("unknown primitive schema type: " + typeCheck.Type)
	}
}

type ElicitRequestParams struct { // https://github.com/modelcontextprotocol/modelcontextprotocol/blob/main/schema/2025-06-18/schema.ts#L1390
	RequestParams
	Message         string `json:"message"`
	RequestedSchema struct {
		Type       string                               `json:"type"`
		Properties map[string]PrimitiveSchemaDefinition `json:"properties"`
		Required   []string                             `json:"required,omitempty"`
	} `json:"requestedSchema"`
}

type ElicitResult struct { // https://github.com/modelcontextprotocol/modelcontextprotocol/blob/main/schema/2025-06-18/schema.ts#L1458
	Action  string          `json:"action"` // "accept" | "decline" | "cancel"
	Content *map[string]any `json:"content,omitempty"`
}

// Helper functions for creating content blocks
func NewTextContent(text string) TextContent {
	return TextContent{
		Type: "text",
		Text: text,
	}
}

func NewImageContent(data, mimeType string) ImageContent {
	return ImageContent{
		Type:     "image",
		Data:     data,
		MimeType: mimeType,
	}
}

func NewAudioContent(data, mimeType string) AudioContent {
	return AudioContent{
		Type:     "audio",
		Data:     data,
		MimeType: mimeType,
	}
}

func NewResourceLink(resource Resource) ResourceLink {
	return ResourceLink{
		Type:     "resource_link",
		Resource: resource,
	}
}

func NewEmbeddedResource(resource interface{}) EmbeddedResource {
	return EmbeddedResource{
		Type:     "resource",
		Resource: resource,
	}
}

// Helper functions for creating requests
func NewJSONRPCRequest(id RequestID, method string, params interface{}) JSONRPCRequest {
	return JSONRPCRequest{
		JSONRPC: JSONRPCVersion,
		ID:      id,
		Method:  method,
		Params:  params,
	}
}

func NewJSONRPCNotification(method string, params interface{}) JSONRPCNotification {
	return JSONRPCNotification{
		JSONRPC: JSONRPCVersion,
		Method:  method,
		Params:  params,
	}
}

func NewJSONRPCResponse(id RequestID, result interface{}) JSONRPCResponse {
	return JSONRPCResponse{
		JSONRPC: JSONRPCVersion,
		ID:      id,
		Result:  result,
	}
}

func NewJSONRPCError(id RequestID, code int, message string, data interface{}) JSONRPCError {
	return JSONRPCError{
		JSONRPC: JSONRPCVersion,
		ID:      id,
		Error: struct {
			Code    int         `json:"code"`
			Message string      `json:"message"`
			Data    interface{} `json:"data,omitempty"`
		}{
			Code:    code,
			Message: message,
			Data:    data,
		},
	}
}

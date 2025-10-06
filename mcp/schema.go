package mcp

import (
	"encoding/json/jsontext"
	"time"
)

type ( // Shared types
	// Base metadata interface
	BaseMetadata struct {
		Name  string  `json:"name"`
		Title *string `json:"title,omitempty"`
	}

	JSONSchema struct {
		Type       string          `json:"type"`
		Properties *map[string]any `json:"properties,omitempty"`
		Required   []string        `json:"required,omitempty"`
	}

	// Annotations
	Annotations struct {
		Audience     []Role   `json:"audience,omitempty"`
		Priority     *float64 `json:"priority,omitempty"`
		LastModified *string  `json:"lastModified,omitempty"`
	}

	Role string

	// Meta field for requests and results
	Meta map[string]any
)

type ( // PUT /mcp/roots
	// RootsList is passed in the body of PUT /mcp/roots
	RootsList struct {
		Roots []Root `json:"roots"`
	}

	// Root is an entry in the list passed to PUT GET /mcp/roots
	Root struct {
		URI  string  `json:"uri"`
		Name *string `json:"name,omitempty"`
		Meta Meta    `json:"_meta,omitempty"`
	}
)

type ( // POST /mcp/complete
)

type ( // GET /mcp/prompts
	PromptList struct {
		Prompts []Prompt `json:"prompts"`
	}

	// Prompt
	Prompt struct {
		BaseMetadata
		Description *string          `json:"description,omitempty"`
		Arguments   []PromptArgument `json:"arguments,omitempty"`
	}
)

type ( // POST /mcp/prompts/{name}
	PromptRequest struct {
		Name      string             `json:"name"`
		Arguments *map[string]string `json:"arguments,omitempty"`
	}
	PromptArgument struct {
		BaseMetadata
		Description *string `json:"description,omitempty"`
		Required    *bool   `json:"required,omitempty"`
	}

	PromptResponse struct {
		Description *string         `json:"description,omitempty"`
		Messages    []PromptMessage `json:"messages"`
	}

	PromptMessage struct {
		Role    Role         `json:"role"`
		Content ContentBlock `json:"content"`
	}
)

type ( // GET /mcp/resources
	// Resource requests/responses
	ListResources struct {
		Resources []Resource `json:"resources"`
	}

	// Resources
	Resource struct {
		BaseMetadata
		URI         string       `json:"uri"`
		Description *string      `json:"description,omitempty"`
		MimeType    *string      `json:"mimeType,omitempty"`
		Annotations *Annotations `json:"annotations,omitempty"`
		Size        *int64       `json:"size,omitempty"`
		Meta        Meta         `json:"_meta,omitempty"`
	}
)

type ( // GET /mcp/resources-templates
	ListResourceTemplates struct {
		ResourceTemplates []ResourceTemplate `json:"resourceTemplates"`
	}

	ResourceTemplate struct {
		BaseMetadata
		URITemplate string       `json:"uriTemplate"`
		Description *string      `json:"description,omitempty"`
		MimeType    *string      `json:"mimeType,omitempty"`
		Annotations *Annotations `json:"annotations,omitempty"`
		Meta        Meta         `json:"_meta,omitempty"`
	}
)

type ( // GET /mcp/resources/{name}
	ResourceContents struct {
		URI      string  `json:"uri"`
		MimeType *string `json:"mimeType,omitempty"`
		Meta     Meta    `json:"_meta,omitempty"`
	}
	TextResourceContents struct {
		ResourceContents
		Text string `json:"text"`
	}

	BlobResourceContents struct {
		ResourceContents
		Blob string `json:"blob"`
	}

	ResourceLink struct {
		Resource
		Type string `json:"type"`
	}

	EmbeddedResource struct {
		Type        string       `json:"type"`
		Resource    interface{}  `json:"resource"` // TextResourceContents | BlobResourceContents
		Annotations *Annotations `json:"annotations,omitempty"`
		Meta        Meta         `json:"_meta,omitempty"`
	}
)

// ListToolsResult is returned from GET /mcp/tools
type (
	ListToolsResult struct {
		Tools []Tool `json:"tools"`
	}

	Tool struct {
		BaseMetadata
		Description  *string          `json:"description,omitempty"`
		InputSchema  JSONSchema       `json:"inputSchema"`
		OutputSchema *JSONSchema      `json:"outputSchema,omitempty"`
		Annotations  *ToolAnnotations `json:"annotations,omitempty"`
		Meta         Meta             `json:"_meta,omitempty"`
	}

	ToolAnnotations struct {
		Title           *string `json:"title,omitempty"`
		ReadOnlyHint    *bool   `json:"readOnlyHint,omitempty"`
		DestructiveHint *bool   `json:"destructiveHint,omitempty"`
		IdempotentHint  *bool   `json:"idempotentHint,omitempty"`
		OpenWorldHint   *bool   `json:"openWorldHint,omitempty"`
	}
)

////////////////////////////////////////// /mcp/tools/{toolName}/calls/{toolCallID} //////////////////////////////////////////

type (
	ToolCall struct {
		ToolName           *string             `json:"toolname"`
		ID                 *string             `json:"id"` // Scoped within tenant & tool name
		Expiration         *time.Time          `json:"expiration,omitempty"`
		ETag               *string             `json:"etag"`
		Status             *Status             `json:"status,omitempty" enum:"running,awaitingSamplingResponse,awaitingElicitationResponse,success,failed,canceled"`
		Request            jsontext.Value      `json:"request,omitempty"`
		SamplingRequest    *SamplingRequest    `json:"samplingRequest,omitempty"`
		ElicitationRequest *ElicitationRequest `json:"elicitationRequest,omitempty"`
		ServerData         *string             `json:"serverData,omitempty"` // Opaque ToolCall-specific state for round-tripping; allows some servers to avoid a durable state store
		Progress           jsontext.Value      `json:"progress,omitempty"`
		Result             jsontext.Value      `json:"result,omitempty"`
		Error              jsontext.Value      `json:"error,omitempty"`
	}

	Status string
)

type ( // Sampling
	SamplingRequest struct { // https://github.com/modelcontextprotocol/modelcontextprotocol/blob/main/schema/2025-06-18/schema.ts#L986
		Messages []SamplingMessage `json:"messages"`

		// The server's preferences for which model to select. The client MAY ignore these preferences.
		ModelPreferences *ModelPreferences `json:"modelPreferences,omitempty"`

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

	SamplingMessage struct {
		Role    Role         `json:"role"`
		Content ContentBlock `json:"content"` // TextContent | ImageContent | AudioContent
	}

	ModelPreferences struct {
		Hints                []ModelHint `json:"hints,omitempty"`
		CostPriority         *float64    `json:"costPriority,omitempty"`
		SpeedPriority        *float64    `json:"speedPriority,omitempty"`
		IntelligencePriority *float64    `json:"intelligencePriority,omitempty"`
	}

	ModelHint struct {
		Name *string `json:"name,omitempty"`
	}

	SamplingResult struct { // https://github.com/modelcontextprotocol/modelcontextprotocol/blob/main/schema/2025-06-18/schema.ts#L1023
		SamplingMessage SamplingMessage `json:"samplingMessage"`
		Model           string          `json:"model"`
		StopReason      *string         `json:"stopReason,omitempty"` // "endTurn" | "stopSequence" | "maxTokens" | string
		ServerData      *string         `json:"serverData,omitempty"`
	}

	// Content types
	ContentBlock interface {
		isContentBlock()
	}

	TextContent struct {
		Type        string       `json:"type"`
		Text        string       `json:"text"`
		Annotations *Annotations `json:"annotations,omitempty"`
		Meta        Meta         `json:"_meta,omitempty"`
	}

	ImageContent struct {
		Type        string       `json:"type"`
		Data        string       `json:"data"`
		MimeType    string       `json:"mimeType"`
		Annotations *Annotations `json:"annotations,omitempty"`
		Meta        Meta         `json:"_meta,omitempty"`
	}

	AudioContent struct {
		Type        string       `json:"type"`
		Data        string       `json:"data"`
		MimeType    string       `json:"mimeType"`
		Annotations *Annotations `json:"annotations,omitempty"`
		Meta        Meta         `json:"_meta,omitempty"`
	}
)

type ( // Elicitation
	ElicitationRequest struct {
		Message         string `json:"message"`
		RequestedSchema struct {
			Type       string                               `json:"type"`
			Properties map[string]PrimitiveSchemaDefinition `json:"properties"`
			Required   []string                             `json:"required,omitempty"`
		} `json:"requestedSchema"`
	}

	PrimitiveSchemaDefinition interface {
		isPrimitiveSchema()
	}

	BooleanSchema struct {
		Type        string  `json:"type"`
		Title       *string `json:"title,omitempty"`
		Description *string `json:"description,omitempty"`
		Default     *bool   `json:"default,omitempty"`
	}

	NumberSchema struct {
		Type        string   `json:"type"` // "number" | "integer"
		Title       *string  `json:"title,omitempty"`
		Description *string  `json:"description,omitempty"`
		Minimum     *float64 `json:"minimum,omitempty"`
		Maximum     *float64 `json:"maximum,omitempty"`
	}

	StringSchema struct {
		Type        string  `json:"type"`
		Title       *string `json:"title,omitempty"`
		Description *string `json:"description,omitempty"`
		MinLength   *int    `json:"minLength,omitempty"`
		MaxLength   *int    `json:"maxLength,omitempty"`
		Format      *string `json:"format,omitempty"` // "email" | "uri" | "date" | "date-time"
	}

	EnumSchema struct {
		Type        string   `json:"type"`
		Title       *string  `json:"title,omitempty"`
		Description *string  `json:"description,omitempty"`
		Enum        []string `json:"enum"`
		EnumNames   []string `json:"enumNames,omitempty"`
	}

	ElicitationResult struct {
		Action     string          `json:"action"` // "accept" | "decline" | "cancel"
		Content    *map[string]any `json:"content,omitempty"`
		ServerData *string         `json:"serverData,omitempty"`
	}
)

import "@typespec/http";

using TypeSpec.Http;

@service(#{ title: "MCP HTTP-REST Transport" })
@route("mcp")
namespace MCP;

// Shared types
model BaseMetadata {
  /**
   * Intended for programmatic or logical use, but used as a display name in past specs or fallback (if title isn't present).
   */
  name: string;

  /**
   * Intended for UI and end-user contexts — optimized to be human-readable and easily understood,
   * even by those unfamiliar with domain-specific terminology.
   *
   * If not provided, the name should be used for display (except for Tool,
   * where `annotations.title` should be given precedence over using `name`,
   * if present).
   */
  title?: string;
}

model JSONSchema {
  type: string;
  properties?: Record<unknown>;
  required?: string[];
}

model Annotations {
  audience?: Role[];
  priority?: float64;
  lastModified?: string;
}

enum Role {
  user: "user",
  assistant: "assistant",
  // system: "system",
}

alias Meta = Record<unknown>;

// Root management
model RootsList {
  roots: Root[];
}

model Root {
  uri: string;
  name?: string;
  _meta?: Meta;
}

// Prompts
model PromptList {
  prompts: Prompt[];
}

model Prompt {
  ...BaseMetadata;
  description?: string;
  arguments?: PromptArgument[];
}

model PromptRequest {
  /**
   * The name of the prompt or prompt template.
   */
  name: string;

  /**
   * Arguments to use for templating the prompt.
   */
  arguments?: Record<string>;
}

model PromptArgument {
  ...BaseMetadata;
  description?: string;
  required?: boolean;
}

model PromptResponse {
  description?: string;
  messages: PromptMessage[];
}

model PromptMessage {
  role: Role;
  content: ContentBlock;
}

// Resources
model ResourcesList {
  resources: Resource[];
}

model Resource {
  ...BaseMetadata;

  /**
   * The URI of this resource.
   *
   * @format uri
   */
  uri: string;

  /**
   * A description of what this resource represents.
   *
   * This can be used by clients to improve the LLM's understanding of available resources. It can be thought of like a "hint" to the model.
   */
  description?: string;

  /**
   * The MIME type of this resource, if known.
   */
  mimeType?: string;

  /**
   * Optional annotations for the client.
   */
  annotations?: Annotations;

  /**
   * The size of the raw resource content, in bytes (i.e., before base64 encoding or any tokenization), if known.
   *
   * This can be used by Hosts to display file sizes and estimate context window usage.
   */
  size?: int64;

  /**
   * See [General fields: `_meta`](/specification/2025-06-18/basic/index#meta) for notes on `_meta` usage.
   */
  _meta?: Meta;
}

model ResourceTemplatesList {
  resourceTemplates: ResourceTemplate[];
}

model ResourceTemplate {
  ...BaseMetadata;

  /**
   * A URI template (according to RFC 6570) that can be used to construct resource URIs.
   *
   * @format uri-template
   */
  uriTemplate: string;

  /**
   * A description of what this resource represents.
   *
   * This can be used by clients to improve the LLM's understanding of available resources. It can be thought of like a "hint" to the model.
   */
  description?: string;

  /**
   * The MIME type of this resource, if known.
   */
  mimeType?: string;

  /**
   * Optional annotations for the client.
   */
  annotations?: Annotations;

  /**
   * See [General fields: `_meta`](/specification/2025-06-18/basic/index#meta) for notes on `_meta` usage.
   */
  _meta?: Meta;
}

model ResourceContents {
  /**
   * The URI of this resource.
   *
   * @format uri
   */
  uri: string;

  /**
   * The MIME type of this resource, if known.
   */
  mimeType?: string;

  /**
   * See [General fields: `_meta`](/specification/2025-06-18/basic/index#meta) for notes on `_meta` usage.
   */
  _meta?: Record<unknown>;
}

model TextResourceContents {
  ...ResourceContents;

  /**
   * The text of the item. This must only be set if the item can actually be represented as text (not binary data).
   */
  text: string;
}

model BlobResourceContents {
  ...ResourceContents;

  /**
   * A base64-encoded string representing the binary data of the item.
   *
   * @format byte
   */
  blob: string;
}

model ResourceLink {
  ...Resource;
  type: string;
}

model EmbeddedResource {
  type: string;
  resource: TextResourceContents | BlobResourceContents;
  annotations?: Annotations;
  _meta?: Meta;
}

// Tools
model ToolsList {
  tools: Tool[];
}

model Tool {
  ...BaseMetadata;
  description?: string;
  inputSchema: JSONSchema;
  outputSchema?: JSONSchema;
  annotations?: ToolAnnotations;
  _meta?: Meta;
}

model ToolAnnotations {
  title?: string;
  readOnlyHint?: boolean;
  destructiveHint?: boolean;
  idempotentHint?: boolean;
  openWorldHint?: boolean;
}

enum ToolCallStatus {
  submitted;
  running;
  awaitingSamplingResult;
  awaitingElicitationResult;
  success;
  failed;
  canceled;
}

@discriminated(#{envelope: "none", discriminatorPropertyName: "status" })
union ToolCallResponse {
  submitted: SubmittedToolCall;
  running: RunningToolCall;
  awaitingSamplingResult: AwaitingSamplingResultToolCall;
  awaitingElicitationResult: AwaitingElicitationResultToolCall;
}

model SubmittedToolCall extends ToolCall {
  status: ToolCallStatus.submitted;
}

model RunningToolCall extends ToolCall {
  status: ToolCallStatus.running;
}

model AwaitingSamplingResultToolCall extends ToolCall {
  status: ToolCallStatus.awaitingSamplingResult;
  samplingRequest: SamplingRequest;
}

model AwaitingElicitationResultToolCall extends ToolCall {
  status: ToolCallStatus.awaitingElicitationResult;
  elicitationRequest: ElicitationRequest;
}


// Tool Calls
model ToolCall {
  toolname?: string;
  id?: string; // Scoped within tenant & tool name
  expiration?: utcDateTime;
  etag?: string;
  status?: ToolCallStatus;
  request?: unknown;
  serverData?: string; // Opaque ToolCall-specific state for round-tripping
  progress?: Progress;
  result?: unknown;
  error?: unknown;
}

model Progress {
  // The progress thus far. This should increase every time progress is made, even if the total is unknown.
  progress: float32;

  // Total number of items to process (or total progress required), if known.
  total?: float32;

  // An optional message describing the current progress.
  message?: string;
}

enum Status {
  running: "running",
  awaitingSamplingResponse: "awaitingSamplingResponse",
  awaitingElicitationResponse: "awaitingElicitationResponse",
  success: "success",
  failed: "failed",
  canceled: "canceled",
}

model ToolCallsList {
  values: ToolCallResponse[];
}

// Sampling
model SamplingRequest {
  messages: SamplingMessage[];
  modelPreferences?: ModelPreferences;
  systemPrompt?: string;
  includeContext?: "none" | "thisServer" | "allServers";
  temperature?: float64;
  maxTokens?: int64;
  stopSequences?: string[];
  metadata?: Record<unknown>;
}

model SamplingMessage {
  role: Role;
  content: ContentBlock;
}

model ModelPreferences {
  hints?: ModelHint[];
  costPriority?: float64;
  speedPriority?: float64;
  intelligencePriority?: float64;
}

model ModelHint {
  name?: string;
}

model SamplingResult {
  samplingMessage: SamplingMessage;
  `model`: string;
  stopReason?: "endTurn" | "stopSequence" | "maxTokens" | string;
  serverData?: string;
}

// Content types
union ContentBlock {
  TextContent,
  ImageContent,
  AudioContent,
}

model TextContent {
  type: "text";
  text: string;
  annotations?: Annotations;
  _meta?: Meta;
}

model ImageContent {
  type: "image";
  data: string;
  mimeType: string;
  annotations?: Annotations;
  _meta?: Meta;
}

model AudioContent {
  type: "audio";
  data: string;
  mimeType: string;
  annotations?: Annotations;
  _meta?: Meta;
}

// Elicitation
model ElicitationRequest {
  // The message to present to the user.
  message: string;

  // A restricted subset of JSON Schema.
  // * Only top-level properties are allowed, without nesting.
  requestedSchema: {
    type: string;
    properties: Record<PrimitiveSchemaDefinition>;
    required?: string[];
  };
}

union PrimitiveSchemaDefinition {
  BooleanSchema,
  NumberSchema,
  StringSchema,
  EnumSchema,
}

model BooleanSchema {
  type: "boolean";
  title?: string;
  description?: string;
  default?: boolean;
}

model NumberSchema {
  type: "number" | "integer";
  title?: string;
  description?: string;
  minimum?: float64;
  maximum?: float64;
}

model StringSchema {
  type: "string";
  title?: string;
  description?: string;
  minLength?: int32;
  maxLength?: int32;
  format?: "email" | "uri" | "date" | "date-time";
}

model EnumSchema {
  type: "string";
  title?: string;
  description?: string;
  `enum`: string[];
  enumNames?: string[]; // Display names for enum values
}

model ElicitationResult {
  /**
   * The user action in response to the elicitation.
   * - "accept": User submitted the form/confirmed the action
   * - "decline": User explicitly declined the action
   * - "cancel": User dismissed without making an explicit choice
   */
  action: "accept" | "decline" | "cancel";

  /**
   * The submitted form data, only present when action is "accept".
   * Contains values matching the requested schema.
   */
  content?: Record<unknown>;

  // Opaque ToolCall-specific state sent from client to server
  serverData?: string;
}

@error
model Error {
  statusCode: int32;
  errorCode?: string;
  message?: string;
}

/**
 * Identifies a prompt.
 */
model PromptReference extends BaseMetadata {
  type: "ref/prompt";
}

/**
 * A reference to a resource or resource template definition.
 */
model ResourceTemplateReference {
  type: "ref/resource";

  /**
   * The URI or URI template of the resource.
   *
   * @format uri-template
   */
  uri: string;
}

model CompleteRequest {
  ref: PromptReference | ResourceTemplateReference;

  /**
   * The argument's information
   */
  argument: {
    /**
     * The name of the argument
     */
    name: string;

    /**
     * The value of the argument to use for completion matching.
     */
    value: string;
  };

  /**
   * Additional, optional context for completions
   */
  context?: {
    /**
     * Previously-resolved variables in a URI template or prompt.
     */
    arguments?: Record<string>;
  };
}

/**
 * The server's response to a completion/complete request
 *
 * @category completion/complete
 */
model CompleteResponse {
  /**
   * An array of completion values. Must not exceed 100 items.
   */
  values: string[];

  /**
   * The total number of completion options available. This can exceed the number of values actually sent in the response.
   */
  total?: int32;

  /**
   * Indicates whether there are additional completion options beyond those provided in the current response, even if the exact total is unknown.
   */
  hasMore?: boolean;
}

model ConditionalHeaders {
  @header ifMatch?: string;
  @header ifNonMatch?: string;
}

model BodyHeaders {
  @header contentType: "application/json";
}

model ResponseHeaders {
  @header accept: "application/json";
  @header etag?: string;
}

model ToolCallAdvanceRequest {
  serverData?: string;
  result: ElicitationResult | SamplingResult;
}

@route("roots")
interface Roots {
  @put updateRoots(@body body: RootsList, ...BodyHeaders): void;
}

@route("complete")
interface Complete {
  @post complete(
    @body body: CompleteRequest,
    ...BodyHeaders,
    ...ResponseHeaders,
  ): CompleteResponse;
}

@route("prompts")
interface Prompts {
  @get list(...ConditionalHeaders, ...ResponseHeaders): PromptList;

  @post get(
    @path name: string,
    @body body: PromptRequest,
    ...BodyHeaders,
    ...ResponseHeaders,
  ): PromptResponse;
}

@route("resources-templates")
interface ResourceTemplates {
  @get list(...ConditionalHeaders, ...ResponseHeaders): ResourceTemplatesList;
}

@route("resources")
interface Resources {
  @get list(...ConditionalHeaders, ...ResponseHeaders): ResourcesList;

  @get
  get(
    @path name: string,
    ...ResponseHeaders,
    ...ConditionalHeaders,
  ): TextResourceContents | BlobResourceContents;
}

@route("tools")
namespace Tools {
  @get op list(...ConditionalHeaders, ...ResponseHeaders): ToolsList;

  @route("{toolName}/calls")
  namespace ToolCalls {
    @get op list(@path toolName: string, ...ResponseHeaders): ToolCallsList;
    @put op create(
      @path toolName: string,
      @path toolCallID: string,
      @body toolCall: ToolCall,
      @header idempotencyKey: string,
      ...BodyHeaders,
      ...ResponseHeaders,
    ): ToolCall;

    @get op get(
      @path toolName: string,
      @path toolCallID: string,
      ...ConditionalHeaders,
      ...ResponseHeaders,
    ): ToolCallResponse;

    @route("advance")
    @post
    op advance(
      @path toolName: string,
      @path toolCallID: string,
      @body body: ToolCallAdvanceRequest,
      ...ConditionalHeaders,
      ...BodyHeaders,
      ...ResponseHeaders,
    ): ToolCallResponse;

    @route("cancel")
    @post
    op cancel(
      @path toolName: string,
      @path toolCallID: string,
      ...ConditionalHeaders,
      ...ResponseHeaders,
    ): ToolCallResponse;
  }
}

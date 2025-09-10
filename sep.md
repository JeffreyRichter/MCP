# SEP-xxxx: Pure HTTP Transport for Model Context Protocol (MCP)

<!-- markdownlint-disable MD024 -->

**Status:** draft
**Type:** Standards Track
**Created:** 2025-09-10
**Authors:** Jeffrey Richter, Mike Kistler

## Motivation

This SEP proposes a Pure HTTP transport layer for the Model Context Protocol (MCP). 

The motivation for this proposal comes from attempting to implement enterprise-scale MCP clients/servers for Microsoft Azure and Office (with ~350M monthly users).
- Our cloud architecture consists of a cluster of nodes all running MCP Server code.
- Each client network request enter through a load-balancer which randomly directs each request to a node.
   - NOTE: Azure's load-balancer forcibly terminates network connections every [4 minutes (by default)](https://learn.microsoft.com/en-us/azure/load-balancer/load-balancer-tcp-idle-timeout?tabs=tcp-reset-idle-portal).
- The number of nodes in the cluster dynamically scales up/down based on client load
- Nodes temporarily go down when new versions of the MCP code is deployed or if the code crashes.
- Office has a cluster of MCP client website nodes where each browser requests also goes through a load balancer and these nodes adhere to the same archtecture as the MCP Server nodes above.

The transport discussed in the proposal addresses the above cloud architecture and enables many benefits:
- Enterprise cloud-based MCP clients/servers to scale efficiently due to no long-lived or sticky network connections.
- Clients/servers can easily be made fault-tolerant and resilient to network failures, timeouts, and client/server crashes using retries and idempotency.
- Clients/servers can change their features/capabilities dynamically as new client/server code is deployed without any downtime or re-establishing of connections/sessions.
- HTTP has been an industry standard since 1997 with very significant cross-language support easing learning curve and adoption of MCP.
- The HTTP API is simpler in that it is resource-focused (with CRUDL operations) as opposed to JSON-RPC method focused.
- MCP SDKs are more for convenience and are not necessary for people to adopt the MCP protocol enabling MCP to be adopted by more languages quicker.
- Improved performance by reducing client/server messages using HTTP's standard caching & optimistic concurrency patterns (etags).
- The existing MCP Auth solutions continue to work as-is since they already follow HTTP industry standards.
- This same HTTP transport can be used securely for local MCP Servers HTTP enabling one transport for use by both local and remote MCP clients/servers.  ***** TODO: Add section on this.
- Improved performance since HTTP allows multiple requests from a single or multiple tenants to be processed in parallel.
- Existing HTTP services that can't parse the HTTP body can be used such as API Gateways, CDNs, SSL/TLS termination,  frontdoors, metrics/logging collection, and distributed tracing
- ***** TODO: Say something about multi-tenancy? (done via Auth?
- ***** TODO: Say something about not having to learn/understand/process JSON-RPC at all?

The list of HTTP routes necessary to implement the MCP protocol is shown here:

| MCP operation | HTTP route | Notes |
|---------------|---------------|- |
| roots/list | PUT /mcp/roots | |
| completion/complete | POST /mcp/complete | |
| resources/list | GET /mcp/resources | |
| resource/templates/list | GET /mcp/resources-templates | |
| resources/read  | POST /mcp/resources/{name} | TODO: Should really be a GET |
| prompts/list | GET /mcp/prompts | |
| prompts/get | POST /mcp/prompts/{name} | |
| tools/list | GET /mcp/tools | |
| tools/call | PUT /mcp/tools/{toolName}/calls/{toolCallID} | Returns ToolResult or ElicitRequest/SamplingRequest request |
| * new * | POST /mcp/tools/{toolName}/calls/{toolCallID}/advance | Client sends ElicitResult/SamplingResult |
| notification/cancelled | POST /mcp/tools/{toolName}/calls/{toolCallID}/cancel | | 
| * new * | GET /mcp/tools/{toolName}/calls | Fault-tolerance: enables client to get list of in-flight tool calls |
| * new * | GET /mcp/tools/{toolName}/calls/{toolCallID} | Fault-tolerance: enables client to get an inflight-tool call | 

- NOTE: List change notifications are not needed because client polls using industry standard HTTP GET with if-none-match: etag header. Cilents just push updated roots to server with HTTP PUT.

## Specification

The complete technical specification for this SEP will be provided in a forthcoming PR. Here we provide an overview of the key design elements and decisions.

The Pure HTTP transport for MCP utilizes standard HTTP methods (GET, POST, PUT, DELETE) to perform operations defined in the MCP protocol. Each MCP operation is mapped to a specific HTTP endpoint, allowing clients to interact with the MCP server using standard HTTP requests.

The transport defines a set of HTTP headers to convey metadata and control information necessary for MCP operations, such as authentication tokens, request identifiers, and content types.

### Schema changes

The Pure HTTP transport only flows the "payload" portion of the MCP messages over HTTP. This requires that message payload schemas be defined independently of the JSON-RPC message that is used in the STDIO and Streamable HTTP transports.
These schema changes have already been proposed in [SEP-1319] and are simply included here by reference.

[SEP-1319]: https://github.com/modelcontextprotocol/modelcontextprotocol/issues/1319

### Error Responses

The Pure HTTP transport uses standard HTTP status codes for error conditions that occur during MCP operations.

| MCP Error Condition               | HTTP Status Code |
|-----------------------------------|------------------|
| Invalid Request                    | 400 Bad Request   |
| Unauthorized Access                | 401 Unauthorized  |
| Resource Not Found                 | 404 Not Found     |
| Method Not Allowed                 | 405 Method Not Allowed |
| Internal Server Error              | 500 Internal Server Error |

The response body for error conditions should contain the `error` field of the `JSONRPCError` schema,
which includes the `code` and `message` properties as defined in the [JSON-RPC error codes] specification.

[JSON-RPC error codes]: https://json-rpc.dev/docs/reference/error-codes

#### Example Error Response

```http
HTTP/1.1 400 Bad Request
Content-Type: application/json

{
  "code": -32602,  ***** TODO: Mike, I don't think we'd keep the JSON-RPC error code numbers, do you?
  "message": "Unknown tool: invalid_tool_name"
}
```

### Mapping MCP Operations to HTTP Endpoints

Each MCP operation (at the application layer) is mapped to a specific HTTP endpoint and method. The following sections provide the details of this mapping, but here we describe the general pattern for mapping MCP operations to HTTP requests.

MCP operations that retrieve data (e.g., `tools/list`, `resources/get`) use the HTTP GET method, while operations that create or modify data (e.g., `tools/call`, `resources/create`) use the HTTP POST or PUT methods as appropriate.

Parameters to MCP operations mapped to HTTP GET requests are passed as URL query parameters, while parameters for POST and PUT requests are included in the JSON request body.

Note that MCP allows any request to contain a "_meta" property with arbitrary metadata for the request. For operations mapped to HTTP PUT or POST requests, the "_meta" property is included in the request body along with the other parameters. For operations mapped to HTTP GET requests, the "_meta" property values are passed in headers, with one header per property in the "_meta" object. These headers use a naming convention of "MCP-Meta-{property-name}" to allow the MCP Server to reconstruct the "_meta" object from the headers.

As in the Streamable HTTP transport, the Pure HTTP transport uses HTTP headers to convey certain protocol metadata, including:

| Header Name               | Description                                      |
|---------------------------|--------------------------------------------------|
| MCP-Protocol-Version      | Indicates the version of the MCP protocol being used in the request. |
| MCP-Session-ID            | Identifies the session associated with the request. |

### Initialization

***** TODO: Mike, we should discuss - this introduces statefulness and hurts the ability for clients/servers to dynamically change their features/capabilities at runtime (a bullet I have above under benefits)

The Pure HTTP transport supports an initialization step that allows the MCP Client to establish a session with the MCP Server. The "initialize" MCP operation is mapped to an HTTP POST request to the "/initialize" endpoint. The request body contains a JSON object representing the `InitializeRequest` schema, and the response body contains a JSON object representing the `InitializeResult` schema.

### Tools

#### tools/list

A "tools/list" MCP request is implemented as an HTTP GET request to the "/tools" endpoint. The `cursor` property of `ListToolsRequest.Params` is passed as a query parameter named `cursor`. The response body contains a JSON object representing the `ListToolsResult`.

##### Example Request

```http
GET /tools?cursor=abc123 HTTP/1.1
Host: mcp.example.com
Accept: application/json
```

##### Example Response

```http
HTTP/1.1 200 OK
Content-Type: application/json
{
  "_meta": {
    "requestID": "xyz789",
    "timestamp": "2025-09-08T12:34:56Z"
  },
  "tools": [
    {
      "name": "get_weather",
      "title": "Weather Information Provider",
      "description": "Get current weather information for a location",
      "inputSchema": { ... },
    }
  ],
  "nextCursor": "def456"
}
```

#### tools/call

A "tools/call" MCP request is implemented as an HTTP PUT request to the "/tools/{toolName}/calls/{toolCallID}" endpoint. The body of the HTTP request will contain the JSON object representing the `params` field of the `ToolCallRequest`, without the `name` field since this is already specified in the URL path. The `toolCallID` will correspond to the `id` field of the JSON-RPC request. The response body will contain a JSON object representing the `ToolCallResult`.

##### Example Request

```http
PUT /tools/get_weather/calls/42 HTTP/1.1
Host: mcp.example.com
Accept: application/json
Content-Type: application/json

{
  "location": "Seattle, WA"
}
```

##### Example Response

```http
HTTP/1.1 200 OK
Content-Type: application/json

{
  "_meta": {
    "requestID": "xyz789",
    "timestamp": "2025-09-08T12:34:56Z"
  },
  "content": [
    {
      "type": "text",
      "text": "Current weather in New York:\nTemperature: 72°F\nConditions: Partly cloudy"
    }
  ]
}
```

#### tools/list changed notification

The response of the `tools/list` request will include an etag header. The MCP Client can later poll the MCP Server by issuing another GET request where if-none-match: etag; this returns 304-NotModified without the list if the list hasn’t changed, or returns 200-OK with the new list if the list has changed.

### Resources

#### resources/list

#### resources/get

GETting a binary resources can return raw bytes; base-64 encoding/decoding is no longer necessary simplifying code and reducing bandwidth. A GET on a large resource could also support the range request header allowing partial/resumable and concurrent GETs.

### Prompts

## Rationale

This section provides the rationale for design choices in the Pure HTTP Transport for MCP SEP that might be questioned. While the Pure HTTP transport specification follows the Streamable HTTP transport patterns where possible, there are intentional deviations to provide the scalability and reliability benefits of pure HTTP.

Decision: Use pure HTTP rather than JSON-RPC over HTTP

Rationale: Using pure HTTP simplifies the transport layer and avoids the complexities introduced by JSON-RPC. This decision aligns with the goal of maintaining a lightweight and efficient communication protocol.

## Backward Compatibility

Because this is an additional transport layer, there are no backward compatibility concerns. Existing stdio and Streamable HTTP transports remain unchanged and fully functional.

## Reference Implementation

An initial reference implementation has been developed in Go. It is currently in a private repository and will be made publicly available once the SEP is finalized.

## Future Considerations

### Compatibility with Future MCP Versions

This transport specification is designed to stay at the transport layer and should be compatible with future MCP protocol versions.

### Security Implications

This SEP has no additional security implications.

## References

# SEP-xxxx: Pure HTTP Transport for Model Context Protocol (MCP)

<!-- markdownlint-disable MD024 -->

**Status:** draft
**Type:** Standards Track
**Created:** 2025-09-08
**Authors:** Jeffrey Richter, Mike Kistler

## Abstract

This SEP proposes a Pure HTTP transport layer for the Model Context Protocol (MCP). The proposed transport will enable
scalable and efficient communication between MCP clients and servers using the mature and proven HTTP protocol, without the complexities of JSON-RPC over HTTP.

## Motivation

The HTTP protocol is widely adopted and supported across various platforms and programming languages, making it an ideal choice for a transport layer in the MCP ecosystem. HTTP was designed to be scalable, reliable, and efficient, which aligns well with the needs of enterprise MCP servers.

Unlike local MCP Servers, remote MCP Servers are multi-tenant (accessed by multiple clients simultaneously). Remote MCP Servers must be scalable and fault tolerant and this is accomplished by creating a cluster of nodes running the MCP Server code with a load balancer that distributes incoming requests across the cluster. The Pure HTTP transport is designed to facilitate this architecture.

## Specification

The complete technical specification for this SEP will be provided in a forthcoming PR. Here we provide an overview of the key design elements and decisions.

The Pure HTTP transport for MCP will utilize standard HTTP methods (GET, POST, PUT, DELETE) to perform operations defined in the MCP protocol. Each MCP operation will be mapped to a specific HTTP endpoint, allowing clients to interact with the MCP server using standard HTTP requests.

The transport will also define a set of HTTP headers to convey metadata and control information necessary for MCP operations, such as authentication tokens, request identifiers, and content types.

### Schema changes

The Pure HTTP transport will only flow the "payload" portion of the MCP messages over HTTP. This will require that message payload schemas be defined independently of the JSON-RPC message that is used in the STDIO and Streamable HTTP transports.
These schema changes have already been proposed in [SEP-1319] and are simply included here by reference.

[SEP-1319]: https://github.com/modelcontextprotocol/modelcontextprotocol/issues/1319

### Error Responses

The Pure HTTP transport will use standard HTTP status codes for error conditions that occur during MCP operations.

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
  "code": -32602,
  "message": "Unknown tool: invalid_tool_name"
}
```

### Mapping MCP Operations to HTTP Endpoints

Each MCP operation (at the application layer) is mapped to a specific HTTP endpoint and method. The following sections provide the details of this mapping, but here we describe the general pattern for mapping MCP operations to HTTP requests.

MCP operations that retrieve data (e.g., `tools/list`, `resources/get`) will typically use the HTTP GET method, while operations that create or modify data (e.g., `tools/call`, `resources/create`) will use the HTTP POST or PUT methods as appropriate.

Parameters to MCP operations mapped to HTTP GET requests will be passed as query parameters in the URL, while parameters for POST and PUT requests will be included in the request body as JSON.

Note that MCP allows any request to contain a "_meta" property with arbitrary metadata for the request. For operations that are mapped to HTTP PUT or POST requests, the "_meta" property will be included in the request body along with the other parameters. For operations mapped to HTTP GET requests, the "_meta" property will be passed in headers, with one header per property in the "_meta" object. These headers will use a naming convention of "MCP-Meta-{property-name}" to allow the MCP Server to reconstruct the "_meta" object from the headers.

As in the Streamable HTTP transport, the Pure HTTP transport will use HTTP headers to convey certain protocol metadata, including:

| Header Name               | Description                                      |
|---------------------------|--------------------------------------------------|
| MCP-Protocol-Version      | Indicates the version of the MCP protocol being used in the request.                        |
| MCP-Session-ID            | Identifies the session associated with the request. |

### Initialization

The Pure HTTP transport will support an initialization step that allows the MCP Client to establish a session with the MCP Server. The "initialize" MCP operation is mapped to an HTTP POST request to the "/initialize" endpoint. The request body will contain a JSON object representing the `InitializeRequest` schema, and the response body will contain a JSON object representing the `InitializeResult` schema.

### Tools

#### tools/list

A "tools/list" MCP request will be implemented as an HTTP GET request to the "/tools" endpoint. The `cursor` property of `ListToolsRequest.Params` will be passed as a query parameter named `cursor`. The response body will contain a JSON object representing the `ListToolsResult`.

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

A "tools/call" MCP request will be implemented as an HTTP PUT request to the "/tools/{toolName}/calls/{toolCallID}" endpoint. The body of the HTTP request will contain the JSON object representing the `params` field of the `ToolCallRequest`, without the `name` field since this is already specified in the URL path. The `toolCallID` will correspond to the `id` field of the JSON-RPC request. The response body will contain a JSON object representing the `ToolCallResult`.

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

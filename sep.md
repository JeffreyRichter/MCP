# SEP-xxxx: Pure HTTP Transport for Model Context Protocol (MCP)

Status: draft
Type: Standards Track
Created: 2025-09-08
Authors: Jeffrey Richter, Mike Kistler

## Abstract

This SEP proposes a Pure HTTP transport layer for the Model Context Protocol (MCP). The proposed transport will enable
scalable and efficient communication between MCP clients and servers using the mature and proven HTTP protocol, without the complexities of JSON-RPC over HTTP.

## Motivation

The current MCP specification defines stdio and Streamable HTTP transport layers, both of which . Based on community feedback and real-world implementation experience, the Pure HTTP transport addresses several critical pain points.

## Specification

The complete technical specification for this SEP will be provided in a forthcoming PR. Here we provide an overview of the key design elements and decisions.

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

package main

/*
GET /mcp/tools
GET /mcp/tools/{toolName}/calls
PUT/GET /mcp/tools/{toolName}/calls/{toolCallID}
POST /mcp/tools/{toolName}/calls/{toolCallID}/advance
POST /mcp/tools/{toolName}/calls/{toolCallID}/cancel

GET /mcp/resources
GET /mcp/resources-templates
GET /mcp/resources/{name}

GET /mcp/prompts
GET /mcp/prompts/{name}

PUT /mcp/roots
POST /mcp/complete
*/

import (
	"github.com/JeffreyRichter/svrcore"
)

func (ops *httpOps) Routes20250808(baseRoutes svrcore.ApiVersionRoutes) svrcore.ApiVersionRoutes {
	// If no base api-version, baseRoutes == nil; build routes from scratch

	// Use the patterns below to MODIFY the base's routes (or ignore baseRoutes to build routes from scratch):
	// To existing URL, add/overwrite HTTP method: baseRoutes["<ExistinUrl>"]["<ExistingOrNewHttpMethod>"] = postFoo
	// To existing URL, remove HTTP method:        delete(baseRoutes["<ExistingUrl>"], "<ExisitngHttpMethod>")
	// Remove existing URL entirely:               delete(baseRoutes, "<ExistingUrl>")
	return svrcore.ApiVersionRoutes{
		// ***** TOOLS *****
		"/mcp/tools": map[string]*svrcore.MethodInfo{
			"GET": {Policy: ops.getToolList},
		},
		"/mcp/tools/{toolName}/calls": map[string]*svrcore.MethodInfo{
			"GET": {Policy: ops.listToolCalls},
		},
		"/mcp/tools/{toolName}/calls/{toolCallID}": map[string]*svrcore.MethodInfo{
			"PUT": {
				Policy: ops.putToolCallResource,
				ValidHeader: &svrcore.ValidHeader{
					ContentTypes:     []string{"application/json"},
					MaxContentLength: int64(1024),
				},
			},
			"GET": {Policy: ops.getToolCallResource},
		},

		"/mcp/tools/{toolName}/calls/{toolCallID}/advance": map[string]*svrcore.MethodInfo{
			"POST": {
				Policy: ops.postToolCallResourceAdvance,
				ValidHeader: &svrcore.ValidHeader{
					ContentTypes:     []string{"application/json"},
					MaxContentLength: int64(1024),
				},
			},
		},

		"/mcp/tools/{toolName}/calls/{toolCallID}/cancel": map[string]*svrcore.MethodInfo{
			"POST": {
				Policy: ops.postToolCallCancelResource,
				ValidHeader: &svrcore.ValidHeader{
					MaxContentLength: int64(0), // No content expected for cancel
				},
			},
		},

		// ***** RESOURCES *****
		"/mcp/resources": map[string]*svrcore.MethodInfo{
			"GET": {Policy: ops.getResources},
		},
		"/mcp/resources-templates": map[string]*svrcore.MethodInfo{
			"GET": {Policy: ops.getResourcesTemplates},
		},
		"/mcp/resources/{name}": map[string]*svrcore.MethodInfo{
			"GET": {Policy: ops.getResource},
		},

		// ***** PROMPTS *****
		"/mcp/prompts": map[string]*svrcore.MethodInfo{
			"GET": {Policy: ops.getPrompts},
		},
		"/mcp/prompts/{name}": map[string]*svrcore.MethodInfo{
			"GET": {Policy: ops.getPrompt},
		},

		// ***** ROOTS & COMPLETIONS *****
		"/mcp/roots": map[string]*svrcore.MethodInfo{
			"PUT": {Policy: ops.putRoots},
		},
		"/mcp/complete": map[string]*svrcore.MethodInfo{
			"POST": {Policy: ops.postCompletion},
		},
	}
}

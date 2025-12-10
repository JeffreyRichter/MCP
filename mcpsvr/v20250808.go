package main

/*
GET /mcp/tools
GET /mcp/tools/{toolName}/calls
PUT/GET /mcp/tools/{toolName}/calls/{toolCallID}
POST /mcp/tools/{toolName}/calls/{toolCallID}/advance
POST /mcp/tools/{toolName}/calls/{toolCallID}/cancel

GET /mcp/resources
GET /mcp/resources-templates
POST /mcp/resources/{name}

GET /mcp/prompts
POST /mcp/prompts/{name}

PUT /mcp/roots
POST /mcp/complete
*/

import (
	"github.com/JeffreyRichter/svrcore"
)

func (p *mcpStages) Routes20250808(baseRoutes svrcore.ApiVersionRoutes) svrcore.ApiVersionRoutes {
	// If no base api-version, baseRoutes == nil; build routes from scratch

	// Use the patterns below to MODIFY the base's routes (or ignore baseRoutes to build routes from scratch):
	// To existing URL, add/overwrite HTTP method: baseRoutes["<ExistinUrl>"]["<ExistingOrNewHttpMethod>"] = postFoo
	// To existing URL, remove HTTP method:        delete(baseRoutes["<ExistingUrl>"], "<ExisitngHttpMethod>")
	// Remove existing URL entirely:               delete(baseRoutes, "<ExistingUrl>")
	return svrcore.ApiVersionRoutes{
		// ***** TOOLS *****
		"/mcp/tools": map[string]*svrcore.MethodInfo{
			"GET": {Stage: p.getToolList},
		},
		"/mcp/tools/{toolName}/calls": map[string]*svrcore.MethodInfo{
			"GET": {Stage: p.listToolCalls},
		},
		"/mcp/tools/{toolName}/calls/{toolCallID}": map[string]*svrcore.MethodInfo{
			"PUT": {
				Stage: p.putToolCallResource,
				ValidHeader: &svrcore.ValidHeader{
					ContentTypes:     []string{"application/json"},
					MaxContentLength: int64(1024),
				},
			},
			"GET": {Stage: p.getToolCallResource},
		},

		"/mcp/tools/{toolName}/calls/{toolCallID}/advance": map[string]*svrcore.MethodInfo{
			"POST": {
				Stage: p.postToolCallResourceAdvance,
				ValidHeader: &svrcore.ValidHeader{
					ContentTypes:     []string{"application/json"},
					MaxContentLength: int64(1024),
				},
			},
		},

		"/mcp/tools/{toolName}/calls/{toolCallID}/cancel": map[string]*svrcore.MethodInfo{
			"POST": {
				Stage: p.postToolCallCancelResource,
				ValidHeader: &svrcore.ValidHeader{
					MaxContentLength: int64(0), // No content expected for cancel
				},
			},
		},

		// ***** RESOURCES *****
		"/mcp/resources": map[string]*svrcore.MethodInfo{
			"GET": {Stage: p.getResources},
		},
		"/mcp/resources-templates": map[string]*svrcore.MethodInfo{
			"GET": {Stage: p.getResourcesTemplates},
		},
		"/mcp/resources/{name}": map[string]*svrcore.MethodInfo{
			"POST": {Stage: p.getResource},
		},

		// ***** PROMPTS *****
		"/mcp/prompts": map[string]*svrcore.MethodInfo{
			"GET": {Stage: p.getPrompts},
		},
		"/mcp/prompts/{name}": map[string]*svrcore.MethodInfo{
			"POST": {Stage: p.getPrompt},
		},

		// ***** ROOTS & COMPLETIONS *****
		"/mcp/roots": map[string]*svrcore.MethodInfo{
			"PUT": {Stage: p.putRoots},
		},
		"/mcp/complete": map[string]*svrcore.MethodInfo{
			"POST": {Stage: p.postCompletion},
		},
	}
}

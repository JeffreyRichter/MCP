package v20250808

import (
	si "github.com/JeffreyRichter/serviceinfra"
)

/*
GET /mcp/tools
GET /mcp/tools/{toolName}/calls
PUT/GET /mcp/tools/{toolName}/calls/{toolCallId}
POST /mcp/tools/{toolName}/calls/{toolCallId}/advance
POST /mcp/tools/{toolName}/calls/{toolCallId}/cancel

GET /mcp/resources
GET //mcp/resources-templates
GET /mcp/resources/{name}

GET /mcp/prompts
GET /mcp/prompts/{name}

PUT /mcp/roots
POST /mcp/complete
*/

func Routes(baseRoutes si.ApiVersionRoutes) si.ApiVersionRoutes {
	// If no base api-version, baseRoutes == nil; build routes from scratch

	// Use the patterns below to MODIFY the base's routes (or ignore baseRoutes to build routes from scratch):
	// To existing URL, add/overwrite HTTP method: baseRoutes["<ExistinUrl>"]["<ExistingOrNewHttpMethod>"] = postFoo
	// To existing URL, remove HTTP method:        delete(baseRoutes["<ExistingUrl>"], "<ExisitngHttpMethod>")
	// Remove existing URL entirely:               delete(baseRoutes, "<ExistingUrl>")
	return si.ApiVersionRoutes{
		// ***** TOOLS *****
		"/mcp/tools": map[string]*si.MethodInfo{
			"GET": {Policy: ops.getToolList},
		},
		"/mcp/tools/{toolName}/calls": map[string]*si.MethodInfo{
			"GET": {Policy: ops.listToolCalls},
		},
		"/mcp/tools/{toolName}/calls/{toolCallId}": map[string]*si.MethodInfo{
			"PUT": {Policy: ops.putToolCallResource},
			"GET": {Policy: ops.getToolCallResource},
		},

		"/mcp/tools/{toolName}/calls/{toolCallId}/advance": map[string]*si.MethodInfo{
			"POST": {Policy: ops.postToolCallAdvance},
		},

		"/mcp/tools/{toolName}/calls/{toolCallId}/cancel": map[string]*si.MethodInfo{
			"POST": {Policy: ops.postToolCallCancelResource},
		},

		// ***** RESOURCES *****
		"/mcp/resources": map[string]*si.MethodInfo{
			"GET": {Policy: ops.getResources},
		},
		"/mcp/resources-templates": map[string]*si.MethodInfo{
			"GET": {Policy: ops.getResourcesTemplates},
		},
		"/mcp/resources/{name}": map[string]*si.MethodInfo{
			"GET": {Policy: ops.getResource},
		},

		// ***** PROMPTS *****
		"/mcp/prompts": map[string]*si.MethodInfo{
			"GET": {Policy: ops.getPrompts},
		},
		"/mcp/prompts/{name}": map[string]*si.MethodInfo{
			"GET": {Policy: ops.getPrompt},
		},

		// ***** ROOTS & COMPLETIONS *****
		"/mcp/roots": map[string]*si.MethodInfo{
			"PUT": {Policy: ops.putRoots},
		},
		"/mcp/complete": map[string]*si.MethodInfo{
			"POST": {Policy: ops.postCompletion},
		},
	}
}

func must[T any](value T, err error) T {
	if err != nil {
		panic(err)
	}
	return value
}

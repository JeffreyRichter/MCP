package v20250808

/*
GET /mcp/tools
GET /mcp/tools/{toolName}/calls
PUT/GET /mcp/tools/{toolName}/calls/{toolCallId}
POST /mcp/tools/{toolName}/calls/{toolCallId}/advance
POST /mcp/tools/{toolName}/calls/{toolCallId}/cancel

GET /mcp/resources
GET /mcp/resources-templates
GET /mcp/resources/{name}

GET /mcp/prompts
GET /mcp/prompts/{name}

PUT /mcp/roots
POST /mcp/complete
*/

import (
	"github.com/JeffreyRichter/serviceinfra"
)

func Routes(baseRoutes serviceinfra.ApiVersionRoutes) serviceinfra.ApiVersionRoutes {
	// If no base api-version, baseRoutes == nil; build routes from scratch

	// Use the patterns below to MODIFY the base's routes (or ignore baseRoutes to build routes from scratch):
	// To existing URL, add/overwrite HTTP method: baseRoutes["<ExistinUrl>"]["<ExistingOrNewHttpMethod>"] = postFoo
	// To existing URL, remove HTTP method:        delete(baseRoutes["<ExistingUrl>"], "<ExisitngHttpMethod>")
	// Remove existing URL entirely:               delete(baseRoutes, "<ExistingUrl>")
	ops := GetOps()
	return serviceinfra.ApiVersionRoutes{
		// ***** TOOLS *****
		"/mcp/tools": map[string]*serviceinfra.MethodInfo{
			"GET": {Policy: ops.getToolList},
		},
		"/mcp/tools/{toolName}/calls": map[string]*serviceinfra.MethodInfo{
			"GET": {Policy: ops.listToolCalls},
		},
		"/mcp/tools/{toolName}/calls/{toolCallId}": map[string]*serviceinfra.MethodInfo{
			"PUT": {
				Policy: ops.putToolCallResource,
				ValidHeader: &serviceinfra.ValidHeader{
					ContentTypes:     []string{"application/json"},
					MaxContentLength: int64(1024),
				},
			},
			"GET": {Policy: ops.getToolCallResource},
		},

		"/mcp/tools/{toolName}/calls/{toolCallId}/advance": map[string]*serviceinfra.MethodInfo{
			"POST": {
				Policy: ops.postToolCallAdvance,
				ValidHeader: &serviceinfra.ValidHeader{
					ContentTypes:     []string{"application/json"},
					MaxContentLength: int64(1024),
				},
			},
		},

		"/mcp/tools/{toolName}/calls/{toolCallId}/cancel": map[string]*serviceinfra.MethodInfo{
			"POST": {
				Policy: ops.postToolCallCancelResource,
				ValidHeader: &serviceinfra.ValidHeader{
					MaxContentLength: int64(0), // No content expected for cancel
				},
			},
		},

		// ***** RESOURCES *****
		"/mcp/resources": map[string]*serviceinfra.MethodInfo{
			"GET": {Policy: ops.getResources},
		},
		"/mcp/resources-templates": map[string]*serviceinfra.MethodInfo{
			"GET": {Policy: ops.getResourcesTemplates},
		},
		"/mcp/resources/{name}": map[string]*serviceinfra.MethodInfo{
			"GET": {Policy: ops.getResource},
		},

		// ***** PROMPTS *****
		"/mcp/prompts": map[string]*serviceinfra.MethodInfo{
			"GET": {Policy: ops.getPrompts},
		},
		"/mcp/prompts/{name}": map[string]*serviceinfra.MethodInfo{
			"GET": {Policy: ops.getPrompt},
		},

		// ***** ROOTS & COMPLETIONS *****
		"/mcp/roots": map[string]*serviceinfra.MethodInfo{
			"PUT": {Policy: ops.putRoots},
		},
		"/mcp/complete": map[string]*serviceinfra.MethodInfo{
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

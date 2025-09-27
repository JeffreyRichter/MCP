package main

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/JeffreyRichter/internal/aids"
)

// createAddCall creates a hardcoded tool call for the 'add' tool
func createAddCall() (string, map[string]any) {
	callID := fmt.Sprintf("%d", time.Now().UnixMicro())
	params := map[string]any{
		"x": 1,
		"y": 2,
	}
	return callID, params
}

// createPIICall creates a hardcoded tool call for the 'pii' tool
func createPIICall() (string, map[string]any) {
	callID := fmt.Sprintf("%d", time.Now().UnixMicro())
	params := map[string]any{
		"key": "test-data",
	}
	return callID, params
}

// parseLastCall parses a response to check if elicitation is required
func parseLastCall(responseBody string) (bool, ElicitationData) {
	var response map[string]any
	if err := json.Unmarshal([]byte(responseBody), &response); aids.IsError(err) {
		return false, ElicitationData{}
	}

	status, ok := response["status"].(string)
	if !ok || status != "awaitingElicitationResult" {
		return false, ElicitationData{}
	}

	callID, _ := response["id"].(string)
	toolName, _ := response["toolname"].(string)

	elicitation := ElicitationData{
		CallID:   callID,
		ToolName: toolName,
		Message:  "This tool requires approval to access sensitive data.",
	}

	return true, elicitation
}

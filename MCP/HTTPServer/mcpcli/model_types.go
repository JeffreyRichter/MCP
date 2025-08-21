package main

import "time"

// AppState represents the current application state
type AppState int

const (
	StateLoading AppState = iota
	StateToolList
	StateExecuting
	StateShowingResult
	StateElicitation
	StateError
)

// PanelType represents which panel is currently active
type PanelType int

const (
	PanelTools PanelType = iota
	PanelRequest
	PanelResponse
)

// ToolInfo represents information about an available tool
type ToolInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// HTTPTransaction represents a complete HTTP request/response cycle
type HTTPTransaction struct {
	Method       string
	URL          string
	RequestBody  string
	StatusCode   int
	ResponseBody string
	Headers      map[string]string
	Timestamp    time.Time
	Duration     time.Duration
	Error        error
}

// ElicitationData represents data needed for PII approval modal
type ElicitationData struct {
	CallID   string
	Message  string
	ToolName string
}

// Messages for tea.Cmd communication
type toolsLoadedMsg struct {
	tools []ToolInfo
	err   error
}
type httpResponseMsg struct {
	transaction *HTTPTransaction
	err         error
}
type elicitationMsg struct{ data ElicitationData }

// modalDecisionMsg emitted by modal submodel when user acts.
type modalDecisionMsg struct{ approved *bool }
type errorMsg struct{ error error }

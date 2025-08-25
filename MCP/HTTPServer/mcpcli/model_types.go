package main

import (
	"net/http"
	"os/exec"
	"time"
)

// AppState represents the current application state
type AppState int

const (
	StateLoading AppState = iota
	StateToolList
	StateExecuting
	StateShowingResult
	StateElicitation
	StatePathInput
	StateError
)

// PanelType represents which panel is currently active
type PanelType int

const (
	PanelTools PanelType = iota
	PanelRequest
	PanelResponse
	PanelServer
)

// ToolInfo represents information about an available tool
type ToolInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// HTTPTransaction represents a complete HTTP request/response cycle
type HTTPTransaction struct {
	Method          string
	URL             string
	RequestBody     string
	StatusCode      int
	ResponseBody    string
	RequestHeaders  http.Header
	ResponseHeaders http.Header
	Timestamp       time.Time
	Duration        time.Duration
	Error           error
}

// ElicitationData represents data needed for PII approval modal
type ElicitationData struct {
	CallID   string
	Message  string
	ToolName string
}

// Messages for tea.Cmd communication
type toolsLoadedMsg struct {
	tools       []ToolInfo
	transaction *HTTPTransaction
	err         error
}
type httpResponseMsg struct {
	transaction *HTTPTransaction
	err         error
}
type elicitationMsg struct{ data ElicitationData }

// modalDecisionMsg emitted by modal submodel when user acts.
type modalDecisionMsg struct{ approved *bool }
type errorMsg struct{ error error }

// pathInputSubmittedMsg emitted when user submits a path in path input modal.
type pathInputSubmittedMsg struct {
	path       string
	storageURL string
}

// pathExecResultMsg carries first line (if any) or an error from executing the submitted path.
type pathExecResultMsg struct {
	firstLine string
	err       error
	pid       int
	cmd       *exec.Cmd
}

// serverURLRefreshMsg triggers loading tools after a delay when switching servers.
type serverURLRefreshMsg struct{}

package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/term"
)

func main() {
	debugViewFlag := flag.String("debug-view", "", "Render a specific component with test data and exit. Available components: main, request, response, elicitation, server")
	serverFlag := flag.String("server", "http://localhost:8080", "MCP server URL")
	flag.Parse()

	model := Model{
		state: StateLoading,
		client: &httpClient{
			Client:     http.DefaultClient,
			serverURL:  *serverFlag,
			apiVersion: "v20250808",
		},
		activePanel:  PanelTools,
		pathInput:    textinput.New(),
		windowWidth:  80, // Default minimum width
		windowHeight: 12, // Default minimum height
		theme:        NewTheme(),
		keys:         defaultKeyMap(),
	}
	model.pathInput.Placeholder = "/path/to/file"
	model.modal.SetKeys(&model.keys)

	if v := *debugViewFlag; v != "" {
		model = setupDebugModel(model, v)
		// Get terminal size and apply the same logic as tea.WindowSizeMsg handling
		if width, height, err := term.GetSize(int(os.Stdout.Fd())); !isError(err) {
			model.setWindowSize(width, height)
		}
		fmt.Print(model.View())
		return
	}

	program := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := program.Run(); isError(err) {
		fmt.Printf("Error running program: %v", err)
		os.Exit(1)
	}
}

// setupDebugModel configures the model for debug/view mode with test data
func setupDebugModel(model Model, viewMode string) Model {
	switch viewMode {
	case "request":
		model.activePanel = PanelRequest
		model.lastRequest = &HTTPTransaction{
			Method:      "POST",
			URL:         "http://localhost:8080/v20250808/tool_calls",
			RequestBody: `{"tool_name":"add","call_id":"test-123","parameters":{"a":42,"b":8}}`,
			Timestamp:   time.Now().Add(-5 * time.Second),
		}
		model.formattedRequest = model.formatJSON(model.lastRequest.RequestBody)
		model.initOrResizeRequestViewport()
		model.syncRequestViewportContent()
	case "response":
		model.activePanel = PanelResponse
		model.lastResponse = &HTTPTransaction{
			Method:       "POST",
			URL:          "http://localhost:8080/v20250808/tool_calls",
			StatusCode:   200,
			ResponseBody: `{"result":50,"call_id":"test-123","status":"completed"}`,
			ResponseHeaders: http.Header{
				"Content-Type": []string{"application/json"},
				"Etag":         []string{`W/"abc123"`},
			},
			Timestamp: time.Now().Add(-5 * time.Second),
			Duration:  150 * time.Millisecond,
		}
		model.formattedResponse = model.formatJSON(model.lastResponse.ResponseBody)
		model.initOrResizeResponseViewport()
		model.syncResponseViewportContent()
	case "elicitation":
		model.activePanel = PanelTools
		model.state = StateToolList // Keep background in tool list state
		model.modal.Show(ElicitationData{
			CallID:   "pii-456",
			ToolName: "pii",
			Message:  "This operation will process sensitive personal information including names and email addresses. The data will be temporarily stored for processing purposes.",
		})
		model.state = StateElicitation
	case "main":
		model.activePanel = PanelTools
		model.state = StateToolList
	case "server":
		model.activePanel = PanelServer
		model.state = StateToolList
	default:
		fmt.Printf("Unknown view mode: %s\n", viewMode)
		fmt.Println("Available views: main, request, response, elicitation, server")
		os.Exit(1)
	}
	model.tools = []ToolInfo{ // Add sample tools for debug display
		{Name: "add", Description: "Simple mathematical addition"},
		{Name: "pii", Description: "Handle sensitive data with approval"},
	}
	return model
}

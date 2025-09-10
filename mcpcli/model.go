package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

// Model represents the main application state
type Model struct {
	// State management
	state AppState
	err   error

	// Tool management
	tools        []ToolInfo
	selectedTool int
	toolList     list.Model

	// HTTP transaction display
	lastRequest       *HTTPTransaction
	lastResponse      *HTTPTransaction
	formattedRequest  string
	formattedResponse string

	// Viewports
	requestViewport  viewport.Model
	responseViewport viewport.Model

	// Modal submodel
	modal ModalModel

	// Path input state
	pathInput                    textinput.Model
	serverLastPath               string
	pathInputActive              bool
	localServerConnectionDetails string
	localServerPID               int
	localServerCmd               *exec.Cmd
	origServerURL                string
	origAuthKey                  string
	usingLocalServer             bool

	keys KeyMap

	// UI state
	activePanel PanelType
	client      *httpClient
	theme       *Theme

	// Window dimensions
	windowWidth  int
	windowHeight int
}

// (Moved type and message definitions to model_types.go)

// Init initializes the model
func (m Model) Init() tea.Cmd { return m.loadTools() }

// setWindowSize enforces minimums and updates model dimensions.
func (m *Model) setWindowSize(w, h int) {
	if w < minWindowWidth {
		w = minWindowWidth
	}
	if h < minWindowHeight {
		h = minWindowHeight
	}
	m.windowWidth, m.windowHeight = w, h
}

// (Moved tool list & viewport helpers to tools_model.go and views.go)

// Update handles messages and updates the model
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.setWindowSize(msg.Width, msg.Height)
		m.resizeToolList()
		m.initOrResizeResponseViewport()
		m.initOrResizeRequestViewport()
	case tea.KeyMsg:
		if m.modal.Visible() { // delegate to modal
			var cmd tea.Cmd
			m.modal, cmd = m.modal.Update(msg)
			if cmd != nil {
				return m, cmd
			}
			// Path input handled separately when state is StatePathInput and modal not reused
			return m, nil
		}
		if m.state == StatePathInput {
			// handle path input keystrokes directly
			if msg.String() == "tab" { // toggle focus between path and storage inputs
				if m.pathInput.Focused() {
					m.pathInput.Blur()
				} else {
					m.pathInput.Focus()
				}
				return m, nil
			}
			if msg.Type == tea.KeyEnter { // submit
				p := m.pathInput.Value()
				if p == "" { // ignore empty
					return m, nil
				}
				return m, func() tea.Msg { return pathInputSubmittedMsg{path: p} }
			}
			if msg.Type == tea.KeyEsc {
				m.state = StateToolList
				m.pathInputActive = false
				return m, nil
			}
			var cmd tea.Cmd
			if m.pathInput.Focused() {
				m.pathInput, cmd = m.pathInput.Update(msg)
			}
			return m, cmd
		}
		return m.updateNormal(msg)
	case toolsLoadedMsg:
		if isError(msg.err) {
			m.err = msg.err
			m.state = StateError
		} else {
			if msg.transaction != nil {
				m.lastRequest = &HTTPTransaction{
					Method:         msg.transaction.Method,
					URL:            msg.transaction.URL,
					RequestBody:    msg.transaction.RequestBody,
					RequestHeaders: msg.transaction.RequestHeaders,
					Timestamp:      msg.transaction.Timestamp,
				}
				m.formattedRequest = m.formatJSON(m.lastRequest.RequestBody)
				m.initOrResizeRequestViewport()
				m.syncRequestViewportContent()
				m.lastResponse = msg.transaction
				m.formattedResponse = m.formatJSON(m.lastResponse.ResponseBody)
				m.initOrResizeResponseViewport()
				m.syncResponseViewportContent()
			}
			if m.toolList.Width() == 0 {
				m.initToolList()
			}
			m.tools = msg.tools
			m.syncToolItems()
			m.state = StateToolList
			m.err = nil
		}
	case httpResponseMsg:
		return m.handleHTTPResponse(msg)
	case elicitationMsg:
		return m.handleElicitation(msg)
	case modalDecisionMsg:
		if msg.approved != nil {
			// hide modal first
			m.modal.Hide()
			m.state = StateExecuting
			return m, m.advanceElicitation(*msg.approved)
		}
		// nil indicates cancel
		m.modal.Hide()
		m.state = StateToolList
		return m, nil
	case errorMsg:
		m.err = msg.error
		m.state = StateError
	case pathInputSubmittedMsg:
		m.serverLastPath = msg.path
		m.state = StateToolList
		m.pathInputActive = false
		// simple status flash can be done by setting err=nil & state revert
		return m, m.startPathExec(msg.path)
	case pathExecResultMsg:
		if isError(msg.err) {
			m.err = msg.err
			m.state = StateError
		} else {
			// expecting a single line of JSON like {"key":"...","port":<number>}
			var parsed struct {
				Key  string `json:"key"`
				Port int    `json:"port"`
			}
			if err := json.Unmarshal([]byte(msg.firstLine), &parsed); isError(err) || parsed.Key == "" || parsed.Port == 0 {
				// invalid; kill process if running
				if msg.cmd != nil && msg.cmd.Process != nil {
					_ = msg.cmd.Process.Kill()
				}
				m.err = fmt.Errorf("couldn't parse %s", msg.firstLine)
				m.state = StateError
				m.localServerPID = 0
				if m.usingLocalServer { // revert to originals and reload
					old := m.client.serverURL
					m.client.serverURL = m.origServerURL
					m.client.authKey = m.origAuthKey
					m.usingLocalServer = false
					if old != m.client.serverURL {
						return m, m.serverURLChanged()
					}
				}
				return m, nil
			}
			if !m.usingLocalServer { // capture originals only once
				m.origServerURL = m.client.serverURL
				m.origAuthKey = m.client.authKey
			}
			m.localServerConnectionDetails = msg.firstLine
			m.localServerPID = msg.pid
			if msg.cmd != nil {
				m.localServerCmd = msg.cmd
			}
			old := m.client.serverURL
			m.client.serverURL = fmt.Sprintf("http://localhost:%d", parsed.Port)
			m.client.authKey = parsed.Key
			m.usingLocalServer = true
			if old != m.client.serverURL {
				cmd := m.serverURLChanged() // schedule delayed refresh
				return m, cmd
			}
		}
	case serverURLRefreshMsg:
		// Only load if we're still in loading state from a prior URL change
		if m.state == StateLoading {
			return m, m.loadTools()
		}
	}
	return m, nil
}

// loadTools creates a command to load available tools
func (m Model) loadTools() tea.Cmd {
	return func() tea.Msg {
		tools, txn, err := m.client.getTools()
		return toolsLoadedMsg{tools: tools, transaction: txn, err: err}
	}
}

// updateNormal handles key presses in normal (non-modal) mode
func (m Model) updateNormal(msg tea.KeyMsg) (Model, tea.Cmd) {
	if key.Matches(msg, m.keys.Quit) {
		return m, tea.Quit
	}
	if key.Matches(msg, m.keys.NextPanel) {
		return m.switchPanel(), nil
	}
	if key.Matches(msg, m.keys.PrevPanel) {
		// cycle backwards
		switch m.activePanel {
		case PanelTools:
			m.activePanel = PanelServer
		case PanelRequest:
			m.activePanel = PanelTools
		case PanelResponse:
			m.activePanel = PanelRequest
		case PanelServer:
			m.activePanel = PanelResponse
		}
		return m, nil
	}
	if key.Matches(msg, m.keys.Refresh) {
		m.state = StateLoading
		return m, m.loadTools()
	}

	if m.activePanel == PanelServer && key.Matches(msg, m.keys.PathInput) && (m.state == StateToolList || m.state == StateShowingResult || m.state == StateError) {
		if !m.pathInputActive {
			m.pathInput = textinput.New()
			m.pathInput.Placeholder = "/path/to/file"
			m.pathInput.SetValue("")
			m.pathInput.Focus()
			m.pathInputActive = true
		}
		m.state = StatePathInput
		return m, nil
	}

	if m.activePanel == PanelServer && key.Matches(msg, m.keys.KillProc) {
		if m.localServerCmd != nil && m.localServerCmd.Process != nil {
			_ = m.localServerCmd.Process.Kill()
			m.localServerCmd = nil
			m.localServerPID = 0
			m.localServerConnectionDetails = ""
			if m.usingLocalServer {
				old := m.client.serverURL
				m.client.serverURL = m.origServerURL
				m.client.authKey = m.origAuthKey
				m.usingLocalServer = false
				if old != m.client.serverURL {
					cmd := m.serverURLChanged()
					return m, cmd
				}
			}
		}
		return m, nil
	}

	switch m.state {
	case StateToolList, StateShowingResult:
		if m.activePanel == PanelTools {
			return m.handleToolListKeys(msg)
		}
		if m.activePanel == PanelResponse {
			prev := m.responseViewport.YOffset
			var cmd tea.Cmd
			m.responseViewport, cmd = m.responseViewport.Update(msg)
			_ = prev
			return m, cmd
		}
		if m.activePanel == PanelRequest {
			prev := m.requestViewport.YOffset
			var cmd tea.Cmd
			m.requestViewport, cmd = m.requestViewport.Update(msg)
			_ = prev
			return m, cmd
		}
		return m, nil
	case StateError:
		// In error state, still allow scrolling existing request/response viewports
		if m.activePanel == PanelResponse {
			prev := m.responseViewport.YOffset
			var cmd tea.Cmd
			m.responseViewport, cmd = m.responseViewport.Update(msg)
			_ = prev
			return m, cmd
		}
		if m.activePanel == PanelRequest {
			prev := m.requestViewport.YOffset
			var cmd tea.Cmd
			m.requestViewport, cmd = m.requestViewport.Update(msg)
			_ = prev
			return m, cmd
		}
		return m, nil
	default:
		return m, nil
	}
}

// startPathExec launches the executable at path and captures first line of its combined output.
func (m Model) startPathExec(path string) tea.Cmd {
	return func() tea.Msg {
		cmd := exec.Command(path)
		stdout, err := cmd.StdoutPipe()
		if isError(err) {
			return pathExecResultMsg{err: err}
		}
		cmd.Stderr = cmd.Stdout
		cmd.Env = []string{"MCPSVR_LOCAL=true"}
		if err := cmd.Start(); isError(err) {
			return pathExecResultMsg{err: err}
		}
		// store command pointer in model via follow-up message (pid also captured here)
		pid := cmd.Process.Pid
		// We can't mutate model here directly besides returning message; embed cmd pointer in closure-scope and assign through global variable not allowed. Instead, we will attach it to a package-level map keyed by pid or extend message (simpler: extend message) but we just need to set Model.pathExecCmd. We'll piggyback on pathExecResultMsg handling? For simplicity, we'll set a package-level variable. (NOTE: This is a deviation; better approach is to return a tea.Cmd that sets it.)
		// Use a context with timeout for reading first line
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		reader := bufio.NewReader(stdout)
		lineCh := make(chan string, 1)
		go func() {
			line, _ := reader.ReadString('\n')
			// Trim newline characters
			for len(line) > 0 && (line[len(line)-1] == '\n' || line[len(line)-1] == '\r') {
				line = line[:len(line)-1]
			}
			lineCh <- line
		}()
		select {
		case line := <-lineCh:
			return pathExecResultMsg{firstLine: line, pid: pid, cmd: cmd}
		case <-ctx.Done():
			return pathExecResultMsg{firstLine: "(no output within 2s)", pid: pid, cmd: cmd}
		}
	}
}

// handleToolListKeys handles key presses when in tool list state
// switchPanel cycles through the available panels
func (m Model) switchPanel() Model {
	switch m.activePanel {
	case PanelTools:
		m.activePanel = PanelRequest
	case PanelRequest:
		m.activePanel = PanelResponse
	case PanelResponse:
		m.activePanel = PanelServer
	case PanelServer:
		m.activePanel = PanelTools
	}
	return m
}

// serverURLChanged resets UI state for Tools/Request/Response when the server URL changes.
// It clears loaded tools and any prior request/response so fresh data reflects the new server.
func (m *Model) serverURLChanged() tea.Cmd {
	m.tools = nil
	if m.toolList.Width() != 0 {
		m.toolList.SetItems([]list.Item{})
	}
	m.lastRequest = nil
	m.lastResponse = nil
	m.formattedRequest = ""
	m.formattedResponse = ""
	if m.requestViewport.Width > 0 {
		m.requestViewport.SetContent("")
	}
	if m.responseViewport.Width > 0 {
		m.responseViewport.SetContent("")
	}
	m.state = StateLoading
	// allow some time for a local server to start up
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return serverURLRefreshMsg{} })
}

// (Moved tool execution functions to tools_model.go)

// (Moved HTTP handlers & JSON helper to http_handlers.go / views.go)

package main

import (
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
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
	lastRequest           *HTTPTransaction
	lastResponse          *HTTPTransaction
	formattedRequestJSON  string
	formattedResponseJSON string

	// Viewports
	requestViewport  viewport.Model
	responseViewport viewport.Model

	// Modal submodel
	modal ModalModel

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
			return m, nil
		}
		return m.updateNormal(msg)
	case toolsLoadedMsg:
		if msg.err != nil {
			m.err = msg.err
			m.state = StateError
		} else {
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
	}
	return m, nil
}

// loadTools creates a command to load available tools
func (m Model) loadTools() tea.Cmd {
	return func() tea.Msg {
		tools, err := m.client.getTools()
		return toolsLoadedMsg{tools: tools, err: err}
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
			m.activePanel = PanelResponse
		case PanelRequest:
			m.activePanel = PanelTools
		case PanelResponse:
			m.activePanel = PanelRequest
		}
		return m, nil
	}
	if key.Matches(msg, m.keys.Refresh) {
		m.state = StateLoading
		return m, m.loadTools()
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
	default:
		return m, nil
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
		m.activePanel = PanelTools
	}
	return m
}

// (Moved tool execution functions to tools_model.go)

// (Moved HTTP handlers & JSON helper to http_handlers.go / views.go)

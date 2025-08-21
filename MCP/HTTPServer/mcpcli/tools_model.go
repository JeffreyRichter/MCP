package main

import (
	"fmt"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

func (m *Model) initToolList() {
	delegate := newToolItemDelegate(m.theme)
	m.toolList = list.New(nil, delegate, m.listWidth(), m.listHeight())
	m.toolList.Title = "Available Tools"
	m.toolList.SetShowHelp(false)
	m.toolList.SetShowStatusBar(false)
	m.toolList.SetShowPagination(false)
	m.toolList.SetFilteringEnabled(false)
	m.toolList.DisableQuitKeybindings()
}

func (m *Model) syncToolItems() {
	if len(m.toolList.Items()) == 0 && len(m.tools) == 0 {
		return
	}
	items := make([]list.Item, 0, len(m.tools))
	for _, t := range m.tools {
		items = append(items, toolItem{ToolInfo: t})
	}
	m.toolList.SetItems(items)
	if m.selectedTool < len(items) {
		m.toolList.Select(m.selectedTool)
	}
}

func (m *Model) resizeToolList() {
	if m.toolList.Width() > 0 {
		m.toolList.SetSize(m.listWidth(), m.listHeight())
	}
}

func (m *Model) listWidth() int  { return max(10, m.windowWidth-2) }
func (m *Model) listHeight() int { return max(3, m.bodyHeight()-4) }

// Tool execution
func (m Model) executeSelectedTool() tea.Cmd {
	if m.selectedTool >= len(m.tools) {
		return func() tea.Msg { return errorMsg{error: fmt.Errorf("no tool selected")} }
	}
	tool := m.tools[m.selectedTool]
	var callID string
	var params map[string]any
	switch tool.Name {
	case "add":
		callID, params = createAddCall()
	case "pii":
		callID, params = createPIICall()
	default:
		return func() tea.Msg { return errorMsg{error: fmt.Errorf("unsupported tool: %s", tool.Name)} }
	}
	return m.makeToolCall(tool.Name, callID, params)
}

func (m Model) makeToolCall(toolName, callID string, params map[string]any) tea.Cmd {
	return func() tea.Msg {
		transaction, err := m.client.createToolCall(toolName, callID, params)
		return httpResponseMsg{transaction: transaction, err: err}
	}
}

// Key handling scoped to tool list.
func (m Model) handleToolListKeys(msg tea.KeyMsg) (Model, tea.Cmd) {
	if key.Matches(msg, m.keys.Execute) {
		if len(m.tools) > 0 {
			m.selectedTool = m.toolList.Index()
			m.state = StateExecuting
			return m, m.executeSelectedTool()
		}
		return m, nil
	}
	if m.toolList.Width() == 0 {
		return m, nil
	}
	prev := m.toolList.Index()
	var cmd tea.Cmd
	m.toolList, cmd = m.toolList.Update(msg)
	if prev != m.toolList.Index() {
		m.selectedTool = m.toolList.Index()
		m.state = StateToolList
	}
	return m, cmd
}

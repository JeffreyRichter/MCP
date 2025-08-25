package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func truncateWidth(s string, w int) string {
	if lipgloss.Width(s) <= w {
		return s
	}
	// naive truncation; not splitting ANSI since we removed custom ANSI gen here
	// iterate rune-wise until width fits
	out := make([]rune, 0, len(s))
	cur := 0
	for _, r := range s {
		rw := lipgloss.Width(string(r))
		if cur+rw > w {
			break
		}
		out = append(out, r)
		cur += rw
	}
	return string(out)
}

func (m Model) View() string {
	base := m.renderHeader() + "\n" + m.renderActivePanel() + "\n" + m.renderStatusLine()
	if m.modal.Visible() {
		return m.renderModalOverlay(base)
	}
	if m.state == StatePathInput {
		return m.renderPathInputOverlay(base)
	}
	return base
}

func (m Model) renderModalOverlay(base string) string {
	modal := m.renderModal()
	mw := min(m.windowWidth-4, 60)
	if mw < 20 {
		mw = m.windowWidth - 2
	}
	box := lipgloss.NewStyle().Width(mw).Render(modal)
	placed := lipgloss.Place(m.windowWidth, m.windowHeight, lipgloss.Center, lipgloss.Center, box)
	baseLines := strings.Split(base, "\n")
	modalLines := strings.Split(placed, "\n")
	for i := range baseLines {
		if i < len(modalLines) && strings.TrimSpace(modalLines[i]) != "" {
			baseLines[i] = modalLines[i]
		}
	}
	return strings.Join(baseLines, "\n")
}

func (m Model) renderHeader() string {
	label := func(name string, active bool) string {
		txt := "[" + name + "]"
		if active {
			return m.theme.HeaderActive.Render(txt)
		}
		return m.theme.HeaderBase.Render(txt)
	}
	tabs := []string{
		label("Tools", m.activePanel == PanelTools), label("Request", m.activePanel == PanelRequest),
		label("Response", m.activePanel == PanelResponse),
		label("Server", m.activePanel == PanelServer),
	}
	left := m.theme.HeaderBase.Render("HTTP MCP Client ") + strings.Join(tabs, " ")
	right := "PID: " + fmt.Sprint(os.Getpid())
	pad := m.windowWidth - lipgloss.Width(left) - lipgloss.Width(right)
	if pad < 1 {
		pad = 1
	}
	line := left + strings.Repeat(" ", pad) + right
	sep := strings.Repeat("─", m.windowWidth)
	return line + "\n" + sep
}

func (m Model) renderActivePanel() string {
	var content string
	switch m.activePanel {
	case PanelTools:
		content = m.renderToolsContent()
	case PanelRequest:
		if m.requestViewport.Width > 0 {
			content = m.requestViewport.View()
		} else {
			content = m.renderRequestContent()
		}
	case PanelResponse:
		if m.responseViewport.Width > 0 {
			content = m.responseViewport.View()
		} else {
			content = m.renderResponseContent()
		}
	case PanelServer:
		content = m.renderServerContent()
	default:
		content = "Unknown panel"
	}
	lines := strings.Split(content, "\n")
	target := max(1, m.windowHeight-(2+2))
	if len(lines) > target {
		lines = lines[:target]
	}
	for len(lines) < target {
		lines = append(lines, "")
	}
	for i, ln := range lines {
		if lipgloss.Width(ln) > m.windowWidth {
			lines[i] = truncateWidth(ln, m.windowWidth)
		}
	}
	return strings.Join(lines, "\n")
}

func (m Model) renderToolsContent() string {
	if m.state == StateLoading {
		return "Loading tools..."
	}
	if len(m.tools) == 0 {
		return `Press "r" to refresh.`
	}
	if m.toolList.Width() == 0 {
		return "(initializing list)"
	}
	return m.toolList.View()
}

func (m Model) renderRequestContent() string {
	if m.lastRequest == nil {
		return "No request yet. Select a tool and press Enter."
	}
	s := fmt.Sprintf("%s %s\n", m.lastRequest.Method, m.lastRequest.URL)
	if authz := m.lastRequest.RequestHeaders.Get("Authorization"); authz != "" {
		s += "Authorization: " + authz + "\n"
	}
	s += m.formattedRequest + "\n\n"
	s += "Sent: " + m.lastRequest.Timestamp.Format("2006-01-02 15:04:05")
	return s
}

func (m Model) renderResponseContent() string {
	if m.lastResponse == nil {
		return "No response yet. Execute a tool call."
	}
	header := ""
	if code := m.lastResponse.StatusCode; code != 0 {
		// we got a response from the server
		header = fmt.Sprintf("HTTP/1.1 %d", m.lastResponse.StatusCode)
		if len(m.lastResponse.ResponseHeaders) > 0 {
			header += "\n"
			for name, values := range m.lastResponse.ResponseHeaders {
				for _, v := range values {
					header += fmt.Sprintf("%s: %s\n", name, v)
				}
			}
		}
	}

	receivedTime := m.lastResponse.Timestamp.Add(m.lastResponse.Duration)
	s := header + m.formattedResponse + "\n\n" + receivedTime.Format("2006-01-02 15:04:05") + " (" + m.lastResponse.Duration.String() + ")"
	return s
}

func (m Model) renderServerContent() string {
	if m.client == nil {
		return "Server client not initialized"
	}
	if m.localServerPID == 0 {
		return fmt.Sprintf("Server URL: %s\nAPI Version: %s\n\nPress p to enter a file path", m.client.serverURL, m.client.apiVersion)
	}
	line := m.localServerConnectionDetails
	if line == "" {
		line = "(waiting for connection details...)"
	}
	return fmt.Sprintf("Server URL: %s\nAPI Version: %s\n\nFile: %s\nPID: %d\nConnection details: %s\nPress k to kill process", m.client.serverURL, m.client.apiVersion, m.serverLastPath, m.localServerPID, line)
}

func (m Model) renderPathInputOverlay(base string) string {
	title := "Launch Local MCP Server"
	pathLabel := "Executable Path:"
	storLabel := "Storage URL:"
	pathView := m.pathInput.View()
	storView := m.storageInput.View()
	help := "Tab=Switch  Enter=Submit  Esc=Cancel"
	content := title + "\n\n" + pathLabel + "\n" + pathView + "\n\n" + storLabel + "\n" + storView + "\n\n" + help
	if m.theme != nil {
		content = m.theme.ModalBorder.Render(content)
	}
	mw := min(m.windowWidth-4, 70)
	if mw < 30 {
		mw = m.windowWidth - 2
	}
	box := lipgloss.NewStyle().Width(mw).Render(content)
	placed := lipgloss.Place(m.windowWidth, m.windowHeight, lipgloss.Center, lipgloss.Center, box)
	baseLines := strings.Split(base, "\n")
	modalLines := strings.Split(placed, "\n")
	for i := range baseLines {
		if i < len(modalLines) && strings.TrimSpace(modalLines[i]) != "" {
			baseLines[i] = modalLines[i]
		}
	}
	return strings.Join(baseLines, "\n")
}

func (m Model) renderModal() string {
	if !m.modal.Visible() {
		return ""
	}
	data := m.modal.Data()
	s := fmt.Sprintf("Elicitation: Approval Required\nTool: %s (call %s)\n", data.ToolName, data.CallID)
	if data.Message != "" {
		s += data.Message + "\n"
	}
	s += "Approve this request?\n[a] Approve   [d] Disapprove   [esc] Cancel"
	if m.theme != nil {
		return m.theme.ModalBorder.Render(s)
	}
	return s
}

func (m Model) renderStatusLine() string {
	st := "Ready"
	switch m.state {
	case StateLoading:
		st = "Loading..."
	case StateExecuting:
		st = "Executing..."
	case StateElicitation:
		st = "Waiting for approval..."
	case StatePathInput:
		st = "Entering file path..."
	case StateError:
		if m.err != nil {
			st = "Error: " + m.err.Error()
		}
	}
	if m.theme != nil && m.err != nil {
		st = m.theme.StatusError.Render(st)
	}
	line := "Status: " + st
	lineWidth := lipgloss.Width(line)
	if lineWidth < m.windowWidth {
		line += strings.Repeat(" ", m.windowWidth-lineWidth)
	} else if lineWidth > m.windowWidth {
		line = truncateWidth(line, m.windowWidth)
	}
	sep := strings.Repeat("─", m.windowWidth)
	return sep + "\n" + line
}

func (m Model) formatJSON(jsonStr string) string {
	if jsonStr == "" {
		return ""
	}
	var data any
	if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
		lines := strings.Split(jsonStr, "\n")
		for i, l := range lines {
			lines[i] = "    " + l
		}
		return strings.Join(lines, "\n")
	}
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return jsonStr
	}
	lines := strings.Split(string(b), "\n")
	for i, l := range lines {
		lines[i] = "    " + l
	}
	return strings.Join(lines, "\n")
}

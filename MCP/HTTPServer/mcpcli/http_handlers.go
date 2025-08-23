package main

import (
	tea "github.com/charmbracelet/bubbletea"
)

// HTTP / elicitation handlers separated from root Update.
func (m Model) handleHTTPResponse(msg httpResponseMsg) (Model, tea.Cmd) {
	if msg.err != nil {
		m.err = msg.err
		m.state = StateError
		return m, nil
	}
	m.lastRequest = &HTTPTransaction{
		Method:         msg.transaction.Method,
		URL:            msg.transaction.URL,
		RequestBody:    msg.transaction.RequestBody,
		RequestHeaders: msg.transaction.RequestHeaders,
		Timestamp:      msg.transaction.Timestamp,
	}
	m.formattedRequestJSON = m.formatAndIndentJSON(m.lastRequest.RequestBody)
	m.initOrResizeRequestViewport()
	m.syncRequestViewportContent()
	m.lastResponse = msg.transaction
	m.formattedResponseJSON = m.formatAndIndentJSON(m.lastResponse.ResponseBody)
	m.initOrResizeResponseViewport()
	m.syncResponseViewportContent()
	if needsElicitation, data := parseLastCall(msg.transaction.ResponseBody); needsElicitation {
		m.modal.Show(data)
		m.state = StateElicitation
	} else {
		m.state = StateShowingResult
	}
	return m, nil
}

func (m Model) handleElicitation(msg elicitationMsg) (Model, tea.Cmd) {
	m.modal.Show(msg.data)
	m.state = StateElicitation
	return m, nil
}

func (m Model) advanceElicitation(approved bool) tea.Cmd {
	return func() tea.Msg {
		transaction, err := m.client.advanceElicitation(m.modal.Data().CallID, approved)
		return httpResponseMsg{transaction: transaction, err: err}
	}
}

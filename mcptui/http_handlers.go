package main

import (
	"github.com/JeffreyRichter/internal/aids"
	tea "github.com/charmbracelet/bubbletea"
)

// HTTP / elicitation handlers separated from root Update.
func (m Model) handleHTTPResponse(msg httpResponseMsg) (Model, tea.Cmd) {
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
	}

	if aids.IsError(msg.err) {
		m.err = msg.err
		// If we have a transaction, synthesize an error response for the Response panel.
		// TODO: simplify this and clarify responsibility for response formatting
		if msg.transaction != nil {
			plain := "Error: " + msg.err.Error()
			m.lastResponse = &HTTPTransaction{
				Method:          msg.transaction.Method,
				URL:             msg.transaction.URL,
				StatusCode:      msg.transaction.StatusCode, // likely 0
				ResponseBody:    plain,
				RequestBody:     msg.transaction.RequestBody,
				RequestHeaders:  msg.transaction.RequestHeaders,
				ResponseHeaders: msg.transaction.ResponseHeaders,
				Timestamp:       msg.transaction.Timestamp,
				Duration:        msg.transaction.Duration,
				Error:           msg.err,
			}
			// Use raw plain text (no indentation) for error case.
			m.formattedResponse = plain
			m.initOrResizeResponseViewport()
			m.syncResponseViewportContent()
		}
		m.state = StateError
		return m, nil
	}

	m.lastResponse = msg.transaction
	m.formattedResponse = m.formatJSON(m.lastResponse.ResponseBody)
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

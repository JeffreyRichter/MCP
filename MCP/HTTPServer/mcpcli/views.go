package main

import (
	"github.com/charmbracelet/bubbles/viewport"
)

// Viewport sizing helpers
func (m *Model) responseViewportWidth() int  { return max(10, m.windowWidth-2) }
func (m *Model) responseViewportHeight() int { return max(1, m.bodyHeight()) }
func (m *Model) requestViewportWidth() int   { return max(10, m.windowWidth-2) }
func (m *Model) requestViewportHeight() int  { return max(1, m.bodyHeight()) }

func (m *Model) initOrResizeResponseViewport() {
	if m.responseViewport.Width == 0 {
		m.responseViewport = viewport.New(m.responseViewportWidth(), m.responseViewportHeight())
		m.syncResponseViewportContent()
		return
	}
	m.responseViewport.Width = m.responseViewportWidth()
	m.responseViewport.Height = m.responseViewportHeight()
}

func (m *Model) initOrResizeRequestViewport() {
	if m.requestViewport.Width == 0 {
		m.requestViewport = viewport.New(m.requestViewportWidth(), m.requestViewportHeight())
		m.syncRequestViewportContent()
		return
	}
	m.requestViewport.Width = m.requestViewportWidth()
	m.requestViewport.Height = m.requestViewportHeight()
}

func (m *Model) syncResponseViewportContent() {
	if m.lastResponse == nil {
		m.responseViewport.SetContent("  No response yet.\n  Execute a tool call to see the response.")
		return
	}
	m.responseViewport.SetContent(m.renderResponseContent())
}

func (m *Model) syncRequestViewportContent() {
	if m.lastRequest == nil {
		m.requestViewport.SetContent("  No request yet.\n  Select a tool and press Enter to create a request.")
		return
	}
	m.requestViewport.SetContent(m.renderRequestContent())
}

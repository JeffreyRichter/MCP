package main

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

// ModalModel encapsulates elicitation modal state and key handling.
type ModalModel struct {
	visible bool
	data    ElicitationData
	keys    *KeyMap
}

func (m ModalModel) Visible() bool              { return m.visible }
func (m *ModalModel) Show(data ElicitationData) { m.data = data; m.visible = true }
func (m *ModalModel) Hide()                     { m.visible = false }
func (m ModalModel) Data() ElicitationData      { return m.data }

// Update handles key messages when visible. Returns modalDecisionMsg
// containing approved=true/false, or approved=nil for cancel.
func (m ModalModel) Update(msg tea.Msg) (ModalModel, tea.Cmd) {
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	if m.keys == nil {
		return m, nil
	}
	switch {
	case key.Matches(km, m.keys.Approve):
		if m.visible {
			approved := true
			return m, func() tea.Msg { return modalDecisionMsg{approved: &approved} }
		}
	case key.Matches(km, m.keys.Decline):
		if m.visible {
			approved := false
			return m, func() tea.Msg { return modalDecisionMsg{approved: &approved} }
		}
	case key.Matches(km, m.keys.Cancel):
		if m.visible {
			return m, func() tea.Msg { return modalDecisionMsg{approved: nil} }
		}
	}
	return m, nil
}

func (m *ModalModel) SetKeys(k *KeyMap) { m.keys = k }

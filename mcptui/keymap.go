package main

import (
	"github.com/charmbracelet/bubbles/key"
)

// KeyMap centralizes all key bindings.
type KeyMap struct {
	Quit      key.Binding
	NextPanel key.Binding
	PrevPanel key.Binding
	Refresh   key.Binding
	Execute   key.Binding
	Approve   key.Binding
	Decline   key.Binding
	Cancel    key.Binding
	PathInput key.Binding
	KillProc  key.Binding
}

func defaultKeyMap() KeyMap {
	return KeyMap{
		Quit:      key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
		NextPanel: key.NewBinding(key.WithKeys("tab", "right"), key.WithHelp("tab", "next panel")),
		PrevPanel: key.NewBinding(key.WithKeys("shift+tab", "left"), key.WithHelp("shift+tab", "prev panel")),
		Refresh:   key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh tools")),
		Execute:   key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "execute tool")),
		Approve:   key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "approve")),
		Decline:   key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "decline")),
		Cancel:    key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel")),
		PathInput: key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "path input")),
		KillProc:  key.NewBinding(key.WithKeys("k"), key.WithHelp("k", "kill process")),
	}
}

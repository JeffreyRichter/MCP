package main

import (
	"io"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

type toolItem struct{ ToolInfo }

func (t toolItem) Title() string       { return t.ToolInfo.Name }
func (t toolItem) Description() string { return t.ToolInfo.Description }
func (t toolItem) FilterValue() string { return t.ToolInfo.Name }

type toolItemDelegate struct{ theme *Theme }

func newToolItemDelegate(theme *Theme) toolItemDelegate              { return toolItemDelegate{theme: theme} }
func (d toolItemDelegate) Height() int                               { return 1 }
func (d toolItemDelegate) Spacing() int                              { return 0 }
func (d toolItemDelegate) Update(msg tea.Msg, m *list.Model) tea.Cmd { return nil }
func (d toolItemDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	it, ok := listItem.(toolItem)
	if !ok {
		return
	}
	line := it.ToolInfo.Name + " - " + it.ToolInfo.Description
	if index == m.Index() {
		if d.theme != nil {
			line = d.theme.HeaderActive.Render("> " + line)
		} else {
			line = "> " + line
		}
	} else {
		line = "  " + line
	}
	_, _ = w.Write([]byte(line))
}

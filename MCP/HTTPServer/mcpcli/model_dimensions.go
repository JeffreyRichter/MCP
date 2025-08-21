package main

// Sizing helpers for layout chrome/body.
func (m *Model) headerHeight() int { return 2 }
func (m *Model) statusHeight() int { return 2 }
func (m *Model) bodyHeight() int   { return max(1, m.windowHeight-m.headerHeight()-m.statusHeight()) }

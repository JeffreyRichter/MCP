package main

// ElicitationHandler manages the elicitation modal state and interactions
type ElicitationHandler struct {
	visible bool
	data    ElicitationData
}

// Show displays the elicitation modal with the given data
func (h *ElicitationHandler) Show(data ElicitationData) {
	h.data = data
	h.visible = true
}

// Hide dismisses the elicitation modal
func (h *ElicitationHandler) Hide() {
	h.visible = false
	h.data = ElicitationData{}
}

// IsVisible returns whether the modal is currently visible
func (h *ElicitationHandler) IsVisible() bool {
	return h.visible
}

// GetData returns the current elicitation data
func (h *ElicitationHandler) GetData() ElicitationData {
	return h.data
}

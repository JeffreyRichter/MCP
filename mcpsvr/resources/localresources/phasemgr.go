package localresources

import (
	"context"
	"log/slog"
	"time"

	"github.com/JeffreyRichter/mcpsvr/mcp/toolcall"
)

type PhaseMgrConfig struct {
	// Logger for logging error events
	ErrorLogger *slog.Logger

	// ToolNameToProcessPhaseFunc converts a Tool Name to a function that processes its phases.
	toolcall.ToolNameToProcessPhaseFunc
}

type phaseMgr struct {
	config PhaseMgrConfig
}

// NewPhaseMgr creates a new Mgr.
// queueUrl must look like: https://myaccount.queue.core.windows.net/<queuename>
func NewPhaseMgr(ctx context.Context, o PhaseMgrConfig) toolcall.PhaseMgr {
	return &phaseMgr{config: o}
}

// StartPhaseProcessing: enqueues a new tool call phase with tool name & tool call id.
func (pm *phaseMgr) StartPhase(ctx context.Context, tc *toolcall.ToolCall) error {
	go func() { // Run each toolcall in its own goroutine to parallelize the work
		// Lookup PhaseProcessor for this ToolName
		tnpp, _ := pm.config.ToolNameToProcessPhaseFunc(*tc.ToolName) // Error can't happen here because tool call was validated earlier
		for (*tc.Status).Processing() {                               // Loop while tool call is server processing
			err := tnpp(ctx, pm, tc) // Transition tool call from current phase to next phase
			_ = err                  // TODO: Log?
		}
	}()
	return nil
}

// ExtendTime extends the time a phase is allowed to run. This method allows *phaseMgr to implement toolcall.PhaseProcessor
func (*phaseMgr) ExtendTime(_ context.Context, _ time.Duration) error { return nil }

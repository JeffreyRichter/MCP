package localresources

import (
	"context"
	"log/slog"
	"time"

	"github.com/JeffreyRichter/mcpsvr/mcp/toolcalls"
	"github.com/JeffreyRichter/mcpsvr/resources"
)

type PhaseMgrConfig struct {
	// Logger for logging events
	*slog.Logger

	// ToolNameToProcessPhaseFunc converts a Tool Name to a function that processes its phases.
	toolcalls.ToolNameToProcessPhaseFunc
}

type PhaseMgr struct {
	config PhaseMgrConfig
	tcs    resources.ToolCallStore
}

// NewPhaseMgr creates a new Mgr.
// queueUrl must look like: https://myaccount.queue.core.windows.net/<queuename>
func NewPhaseMgr(ctx context.Context, o PhaseMgrConfig, tcs resources.ToolCallStore) (resources.PhaseMgr, error) {
	return &PhaseMgr{config: o, tcs: tcs}, nil
}

// StartPhaseProcessing: enqueues a new tool call phase with tool name & tool call id.
func (pm *PhaseMgr) StartPhaseProcessing(ctx context.Context, tc *toolcalls.ToolCall) error {
	pp := &phaseProcessor{} // This doesn't do anything for local PhaseMgr
	go func() {             // Run each toolcall in its own goroutine to parallelize the work
		// Lookup PhaseProcessor for this ToolName
		tnpp, _ := pm.config.ToolNameToProcessPhaseFunc(*tc.ToolName) // Error can't happen here because tool call was validated earlier
		for *tc.Status == toolcalls.ToolCallStatusRunning {           // Loop while tool call is running
			tnpp(ctx, tc, pp) // Transition tool call from current phase to next phase
		}
	}()
	return nil
}

type phaseProcessor struct{}

func (pp *phaseProcessor) ExtendProcessingTime(_ context.Context, _ time.Duration) error { return nil }

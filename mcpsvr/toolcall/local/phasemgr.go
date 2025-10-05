package local

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/JeffreyRichter/internal/aids"
	"github.com/JeffreyRichter/mcpsvr/toolcall"
	"github.com/JeffreyRichter/svrcore"
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
func (pm *phaseMgr) StartPhase(ctx context.Context, tc *toolcall.Resource) *svrcore.ServerError {
	go func() { // Run each toolcall in its own goroutine to parallelize the work
		defer func() {
			if r := recover(); r != nil {
				if v := recover(); v != nil { // Panic: Capture error & stack trace
					stack := &strings.Builder{}
					stack.WriteString(fmt.Sprintf("Error: %v\n", v))
					aids.WriteStack(stack, aids.ParseStack(2))
					fmt.Fprint(os.Stderr, stack.String()) // Also write stack to stdout so it shows up in container logs
					pm.config.ErrorLogger.LogAttrs(ctx, slog.LevelError, "StartPhase error", slog.String("stack", stack.String()))
				}
			}
		}()
		// Lookup PhaseProcessor for this ToolName
		tnpp := pm.config.ToolNameToProcessPhaseFunc(*tc.ToolName) // Error can't happen here because tool call was validated earlier
		for (*tc.Status).Processing() {                            // Loop while tool call is server processing
			tnpp(ctx, pm, tc) // Transition tool call from current phase to next phase
		}
	}()
	return nil
}

// ExtendTime extends the time a phase is allowed to run. This method allows *phaseMgr to implement toolcall.PhaseProcessor
func (*phaseMgr) ExtendTime(_ context.Context, _ time.Duration) {}

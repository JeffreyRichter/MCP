package azresources

/*
Most advancements of a tool call are client-induced: initiating the tool call (status-submitted),
sending elicitation/sampling result.

Some tools require server-induced advancements. For example, a tool polling for some condition:
a specific day/time, the completion of a task from another service, a stock price to reach a certain value, etc.
*/
import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azqueue"
	"github.com/JeffreyRichter/internal/aids"
	"github.com/JeffreyRichter/mcpsvr/mcp/toolcall"
	"github.com/JeffreyRichter/svrcore"
)

type PhaseMgrConfig struct {
	// Logger for logging error events
	ErrorLogger *slog.Logger

	// ToolNameToProcessPhaseFunc converts a Tool Name to a function that processes its phases.
	toolcall.ToolNameToProcessPhaseFunc

	// PhaseExecutionTime is the initial duration for which a phase is allowed to run.
	PhaseExecutionTime time.Duration
}

type PhaseMgr struct {
	queueClient *azqueue.QueueClient
	tcs         toolcall.Store
	config      PhaseMgrConfig
}

// NewPhaseMgr creates a new Mgr.
// queueUrl must look like: https://myaccount.queue.core.windows.net/<queuename>
func NewPhaseMgr(ctx context.Context, queueClient *azqueue.QueueClient, tcs toolcall.Store, o PhaseMgrConfig) (toolcall.PhaseMgr, *svrcore.ServerError) {
	if _, err := queueClient.Create(ctx, nil); aids.IsError(err) { // Make sure the queue exists
		return nil, svrcore.NewServerError(http.StatusInternalServerError, "", "Failed to create phase manager queue")
	}
	pm := &PhaseMgr{queueClient: queueClient, tcs: tcs, config: o}
	go func() {
		for { // If the goroutine dies, create a new one
			if ctx.Err() != nil {
				return // Context cancelled; exit goroutine
			}
			defer func() {
				if v := recover(); v != nil { // Panic: Capture error & stack trace
					stack := &strings.Builder{}
					stack.WriteString(fmt.Sprintf("Error: %v\n", v))
					aids.WriteStack(stack, aids.ParseStack(2))
					fmt.Fprint(os.Stderr, stack.String()) // Also write stack to stdout so it shows up in container logs
					pm.config.ErrorLogger.LogAttrs(ctx, slog.LevelError, "PhaseMgr error", slog.String("stack", stack.String()))
				}
			}()
			pm.processor(ctx)
		}
	}()
	return pm, nil
}

// DeleteQueue delete the queue. This is most useful for debugging/testing.
func (pm *PhaseMgr) DeleteQueue(ctx context.Context) error {
	_, err := pm.queueClient.Delete(ctx, nil)
	return err
}

// Processor loops forever dequeuing/processing ToolCall Phases.
// Use ctx to cancel Processor & all ToolCall Phases in flight.
// Poison messages & other failures are logged.
func (pm *PhaseMgr) processor(ctx context.Context) {
	o := &azqueue.DequeueMessagesOptions{
		NumberOfMessages:  aids.New(int32(10)),
		VisibilityTimeout: aids.New(int32(pm.config.PhaseExecutionTime.Seconds())),
	}
	for ctx.Err() == nil {
		time.Sleep(time.Millisecond * 200)
		// TODO: If CPU Usage > 90%, continue
		resp, err := pm.queueClient.DequeueMessages(ctx, o)
		aids.Assert(!aids.IsError(err), "DequeueMessages: "+err.Error()) // Maybe exponential delay for time.Sleep if service is down?

		for _, m := range resp.Messages {
			if *m.DequeueCount > 3 { // Poison Message
				pm.config.ErrorLogger.Error("PoisonMessage", slog.String("messageID", *m.MessageID))
				pm.queueClient.DeleteMessage(ctx, *m.MessageID, *m.PopReceipt, nil) // Ignore any failure
				continue
			}
			go func() { // Each tool call runs in a separate goroutine for parallelism
				// TODO: Add defer & recover here
				var tc toolcall.ToolCall
				if err := json.Unmarshal(([]byte)(*m.MessageText), &tc); aids.IsError(err) {
					pm.config.ErrorLogger.Error("UnexpectedMessageFormat", slog.String("messageID", *m.MessageID), slog.String("error", err.Error()))
					return
				}
				se := pm.tcs.Get(ctx, &tc, svrcore.AccessConditions{})
				if se != nil { // ToolCallID not expired/not found
					// No more phases to execute; delete the queue message (or let it become a poison message)
					// continuePhaseProcessing will delete the message if Status != toolcall.ToolCallStatusRunning
					// TODO: log; maybe not
					return
				}
				pp := pm.newPhaseProcessor(*m.MessageID, *m.PopReceipt)
				pm.continuePhaseProcessing(ctx, pp, &tc)
			}()
		}
	}
}

// StartPhaseProcessing: enqueues a new tool call phase with tool name & tool call id.
// It must succeed or panic due to internal server error
func (pm *PhaseMgr) StartPhase(ctx context.Context, tc *toolcall.ToolCall) *svrcore.ServerError {
	data := aids.MustMarshal(tc.Identity)
	_, err := pm.queueClient.EnqueueMessage(ctx, string(data), nil)
	if aids.IsError(err) {
		return svrcore.NewServerError(http.StatusInternalServerError, "", "Failed to enqueue tool call phase")
	}
	return nil
}

func (pm *PhaseMgr) continuePhaseProcessing(ctx context.Context, pp *phaseProcessor, tc *toolcall.ToolCall) {
	// Lookup PhaseProcessor for this ToolName
	tnpp := pm.config.ToolNameToProcessPhaseFunc(*tc.ToolName) // panics if tool name unrecgnized
	for (*tc.Status).Processing() {                            // Loop while tool call is running
		tnpp(ctx, pp, tc) // Transition tool call from current phase to next phase
		// Persists new state of tool call resource (etag must match)
		aids.Must0(pm.tcs.Put(ctx, tc, svrcore.AccessConditions{IfMatch: tc.ETag}))
	}

	// When no longer "running", phase processing is complete, so delete the queue message
	pm.queueClient.DeleteMessage(ctx, pp.messageID, pp.popReceipt, nil) // Ignore any failure
}

func (pm *PhaseMgr) newPhaseProcessor(messageID, popReceipt string) *phaseProcessor {
	return &phaseProcessor{mgr: pm, messageID: messageID, popReceipt: popReceipt}
}

type phaseProcessor struct {
	mgr        *PhaseMgr
	messageID  string
	popReceipt string
}

func (pp *phaseProcessor) ExtendTime(ctx context.Context, phaseExecutionTime time.Duration) {
	resp, err := pp.mgr.queueClient.UpdateMessage(ctx, pp.messageID, pp.popReceipt, "",
		&azqueue.UpdateMessageOptions{VisibilityTimeout: aids.New(int32(phaseExecutionTime.Seconds()))})
	aids.Assert(!aids.IsError(err), err)
	pp.popReceipt = *resp.PopReceipt
}

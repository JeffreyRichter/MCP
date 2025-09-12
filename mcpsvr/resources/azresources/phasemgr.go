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
	"log/slog"
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
func NewPhaseMgr(ctx context.Context, queueClient *azqueue.QueueClient, tcs toolcall.Store, o PhaseMgrConfig) (toolcall.PhaseMgr, error) {
	if _, err := queueClient.Create(ctx, nil); aids.IsError(err) { // Make sure the queue exists
		return nil, err
	}
	mgr := &PhaseMgr{queueClient: queueClient, tcs: tcs, config: o}
	go mgr.processor(ctx)
	return mgr, nil
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
		NumberOfMessages:  svrcore.Ptr(int32(10)),
		VisibilityTimeout: svrcore.Ptr(int32(pm.config.PhaseExecutionTime.Seconds())),
	}
	for {
		time.Sleep(time.Millisecond * 200)
		// TODO: If CPU Usage > 90%, continue
		resp, err := pm.queueClient.DequeueMessages(ctx, o)
		if aids.IsError(err) {
			pm.config.ErrorLogger.Error("DequeueMessages", slog.String("error", err.Error()))
			continue // Maybe exponential delay for time.Sleep if service is down?
		}
		for _, m := range resp.Messages {
			if *m.DequeueCount > 3 { // Poison Message
				pm.config.ErrorLogger.Error("PoisonMessage", slog.String("messageID", *m.MessageID))
				continue
			}
			go func() { // Each tool call runs in a separate goroutine for parallelism
				var tc toolcall.ToolCall
				if err := json.Unmarshal(([]byte)(*m.MessageText), &tc); aids.IsError(err) {
					pm.config.ErrorLogger.Error("UnexpectedMessageFormat", slog.String("messageID", *m.MessageID), slog.String("error", err.Error()))
					return
				}
				// TODO: use the ToolCallStore singleton (it's a sync.OnceValue)
				err := pm.tcs.Get(ctx, &tc, svrcore.AccessConditions{})
				if aids.IsError(err) { // ToolCallID not expired/not found
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
func (pm *PhaseMgr) StartPhase(ctx context.Context, tc *toolcall.ToolCall) error {
	data, err := json.Marshal(tc.Identity)
	if aids.IsError(err) {
		return err
	}
	_, err = pm.queueClient.EnqueueMessage(ctx, string(data), nil)
	return err
}

func (pm *PhaseMgr) continuePhaseProcessing(ctx context.Context, pp *phaseProcessor, tc *toolcall.ToolCall) error {
	// Lookup PhaseProcessor for this ToolName
	tnpp, err := pm.config.ToolNameToProcessPhaseFunc(*tc.ToolName)
	if aids.IsError(err) {
		return err // unrecognized tool name; log?
	}
	for *tc.Status == toolcall.StatusRunning { // Loop while tool call is running
		err := tnpp(ctx, pp, tc) // Transition tool call from current phase to next phase
		if aids.IsError(err) {
			return err
		}
		// Persists new state of tool call resource (etag must match)
		err = pm.tcs.Put(ctx, tc, svrcore.AccessConditions{IfMatch: tc.ETag})
		if aids.IsError(err) {
			return err // log?
		}
	}

	// When no longer "running", phase processing is complete, so delete the queue message
	pm.queueClient.DeleteMessage(ctx, pp.messageID, pp.popReceipt, nil) // Ignore any failure
	return nil
}

func (pm *PhaseMgr) newPhaseProcessor(messageID, popReceipt string) *phaseProcessor {
	return &phaseProcessor{mgr: pm, messageID: messageID, popReceipt: popReceipt}
}

type phaseProcessor struct {
	mgr        *PhaseMgr
	messageID  string
	popReceipt string
}

func (pp *phaseProcessor) ExtendTime(ctx context.Context, phaseExecutionTime time.Duration) error {
	resp, err := pp.mgr.queueClient.UpdateMessage(ctx, pp.messageID, pp.popReceipt, "",
		&azqueue.UpdateMessageOptions{VisibilityTimeout: svrcore.Ptr(int32(phaseExecutionTime.Seconds()))})
	if aids.IsError(err) {
		return nil
	}
	pp.popReceipt = *resp.PopReceipt
	return nil
}

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
	"github.com/JeffreyRichter/mcpsvr/mcp/toolcalls"
	"github.com/JeffreyRichter/mcpsvr/resources"
	"github.com/JeffreyRichter/svrcore"
)

type PhaseMgrConfig struct {
	// Logger for logging events
	*slog.Logger

	// ToolNameToProcessPhaseFunc converts a Tool Name to a function that processes its phases.
	toolcalls.ToolNameToProcessPhaseFunc

	// PhaseExecutionTime is the initial duration for which a phase is allowed to run.
	PhaseExecutionTime time.Duration
}

type PhaseMgr struct {
	queueClient *azqueue.QueueClient
	config      PhaseMgrConfig
	tcs         resources.ToolCallStore
}

// NewPhaseMgr creates a new Mgr.
// queueUrl must look like: https://myaccount.queue.core.windows.net/<queuename>
func NewPhaseMgr(ctx context.Context, queueUrl string, o PhaseMgrConfig, tcs resources.ToolCallStore) (resources.PhaseMgr, error) {
	queueClient, err := azqueue.NewQueueClient(queueUrl, nil /*cred azcore.TokenCredential*/, nil)
	if err != nil {
		return nil, err
	}
	if _, err = queueClient.Create(ctx, nil); err != nil { // Make sure the queue exists
		return nil, err
	}
	mgr := &PhaseMgr{queueClient: queueClient, config: o}
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
		if err != nil {
			pm.config.Logger.Error("DequeueMessages", slog.String("error", err.Error()))
			continue // Maybe exponential delay for time.Sleep if service is down?
		}
		for _, m := range resp.Messages {
			if *m.DequeueCount > 3 { // Poison Message
				pm.config.Logger.Error("PoisonMessage", slog.String("messageID", *m.MessageID))
				continue
			}
			go func() { // Each tool call runs in a separate goroutine for parallelism
				var tc toolcalls.ToolCall
				if err := json.Unmarshal(([]byte)(*m.MessageText), &tc); err != nil {
					pm.config.Logger.Error("UnexpectedMessageFormat", slog.String("messageID", *m.MessageID), slog.String("error", err.Error()))
					return
				}
				// TODO: use the ToolCallStore singleton (it's a sync.OnceValue)
				err := pm.tcs.Get(ctx, &tc, svrcore.AccessConditions{})
				if err != nil { // ToolCallID not expired/not found
					// No more phases to execute; delete the queue message (or let it become a poison message)
					// continuePhaseProcessing will delete the message if Status != toolcalls.ToolCallStatusRunning
					// TODO: log; maybe not
					return
				}
				pp := pm.newPhaseProcessor(*m.MessageID, *m.PopReceipt)
				pm.continuePhaseProcessing(ctx, &tc, pp)
			}()
		}
	}
}

// StartPhaseProcessing: enqueues a new tool call phase with tool name & tool call id.
func (pm *PhaseMgr) StartPhaseProcessing(ctx context.Context, tc *toolcalls.ToolCall) error {
	data, err := json.Marshal(tc.ToolCallIdentity)
	if err != nil {
		return err
	}
	_, err = pm.queueClient.EnqueueMessage(ctx, string(data), nil)
	return err
}

func (pm *PhaseMgr) continuePhaseProcessing(ctx context.Context, tc *toolcalls.ToolCall, pp *phaseProcessor) error {
	// Lookup PhaseProcessor for this ToolName
	tnpp, err := pm.config.ToolNameToProcessPhaseFunc(*tc.ToolName)
	if err != nil {
		return err // unrecognized tool name; log?
	}
	for *tc.Status == toolcalls.ToolCallStatusRunning { // Loop while tool call is running
		err := tnpp(ctx, tc, pp) // Transition tool call from current phase to next phase
		if err != nil {
			return err
		}
		// Persists new state of tool call resource (etag must match)
		err = pm.tcs.Put(ctx, tc, svrcore.AccessConditions{IfMatch: tc.ETag})
		if err != nil {
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

func (pp *phaseProcessor) ExtendProcessingTime(ctx context.Context, phaseExecutionTime time.Duration) error {
	resp, err := pp.mgr.queueClient.UpdateMessage(ctx, pp.messageID, pp.popReceipt, "",
		&azqueue.UpdateMessageOptions{VisibilityTimeout: svrcore.Ptr(int32(phaseExecutionTime.Seconds()))})
	if err != nil {
		return nil
	}
	pp.popReceipt = *resp.PopReceipt
	return nil
}

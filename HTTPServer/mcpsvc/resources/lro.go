package resources

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
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azqueue"
	"github.com/JeffreyRichter/mcpsvc/mcp/toolcalls"
	si "github.com/JeffreyRichter/serviceinfra"
)

type azureQueueToolCallPhaseMgr struct {
	queueClient            *azqueue.QueueClient
	toolNameToProcessPhase toolcalls.ToolNameToProcessPhaseFunc
}

// NewAzureQueueToolCallPhaseMgr creates a new azureQueueToolCallPhaseMgr.
// queueUrl must look like: https://myaccount.queue.core.windows.net/<queuename>
func NewAzureQueueToolCallPhaseMgr(ctx context.Context, queueUrl string, tn2pp toolcalls.ToolNameToProcessPhaseFunc) (*azureQueueToolCallPhaseMgr, error) {
	queueClient, err := azqueue.NewQueueClient(queueUrl, nil /*cred azcore.TokenCredential*/, nil)
	if err != nil {
		return nil, err
	}
	if _, err = queueClient.Create(ctx, nil); err != nil { // Make sure the queue exists
		return nil, err
	}
	return &azureQueueToolCallPhaseMgr{queueClient: queueClient, toolNameToProcessPhase: tn2pp}, nil
}

// DeleteQueue delete the queue. This is most useful for debugging/testing.
func (pm *azureQueueToolCallPhaseMgr) DeleteQueue(ctx context.Context) error {
	_, err := pm.queueClient.Delete(ctx, nil)
	return err
}

// Processor forever loops dequeuing/processing ToolCall Phases.
// Use ctx to cancel Processor & all ToolCall Phases in flight.
// Poison messages & other failures are logged.
func (pm *azureQueueToolCallPhaseMgr) Processor(ctx context.Context, phaseExecutionTime time.Duration, loggerTODO bool) {
	o := &azqueue.DequeueMessagesOptions{
		NumberOfMessages:  si.Ptr(int32(10)),
		VisibilityTimeout: si.Ptr(int32(phaseExecutionTime.Seconds())),
	}
	for {
		time.Sleep(time.Millisecond * 200)
		resp, err := pm.queueClient.DequeueMessages(ctx, o)
		if err != nil {
			// TODO: log
			continue // Maybe exponential delay for time.Sleep if service is down?
		}
		// TODO: For each message, GET its TC resource and then call ContinueToolCallPhaseProcessing
		for _, m := range resp.Messages {
			if *m.DequeueCount > 3 { // Poison Message
				// TODO: log
				continue
			}
			var aqm azureQueueMessage
			if err := json.Unmarshal(([]byte)(*m.MessageText), &aqm); err != nil {
				// TODO: Log unexpected message format
			}
			tc := toolcalls.NewToolCall(aqm.Parse())
			// TODO: use the ToolCallStore singleton (it's a sync.OnceValue)
			tc, err := ToolCallOps.Get(ctx, tc, nil)
			if err != nil { // ToolCallID not expired/not found
				// No more phases to execute; delete the queue message (or let it become a poison message)
				// ContinuePhaseProcessing will delete the message if Status != toolcalls.ToolCallStatusRunning
				// TODO: log; maybe not
				continue
			}
			pp := pm.NewPhaseProcessor(*m.MessageID, *m.PopReceipt)
			pm.ContinuePhaseProcessing(ctx, tc, pp)
		}
	}
}

type azureQueueMessage struct {
	ToolCallIDUrl string `json:"toolCallIDUrl"`
}

func (m *azureQueueMessage) Parse() (tenant, toolName, toolCallID string) {
	return ToolCallOps.fromBlobUrl(m.ToolCallIDUrl)
}

// StartPhaseProcessing: enqueues a new tool call phase with tool name & tool call id.
// Calls continuePhaseProcessing (passing time extender in PhaseProcessor interface) while status is in progress. Updates tc resource after continue returns. Deletes queue message when status is not in progress.
func (pm *azureQueueToolCallPhaseMgr) StartPhaseProcessing(ctx context.Context, tc *toolcalls.ToolCall, phaseExecutionTime time.Duration) error {
	containerName, blobName := ToolCallOps.toBlobInfo(tc)
	data := must(json.Marshal(azureQueueMessage{
		ToolCallIDUrl: fmt.Sprintf("https://%s.blob.core.windows.net/%s", containerName, blobName),
	}))
	resp, err := pm.queueClient.EnqueueMessage(ctx, string(data),
		&azqueue.EnqueueMessageOptions{VisibilityTimeout: si.Ptr(int32(phaseExecutionTime.Seconds()))})
	if err != nil {
		return nil
	}
	pp := pm.NewPhaseProcessor(*resp.Messages[0].MessageID, *resp.Messages[0].PopReceipt)
	return pm.ContinuePhaseProcessing(ctx, tc, pp)
}

func (pm *azureQueueToolCallPhaseMgr) ContinuePhaseProcessing(ctx context.Context, tc *toolcalls.ToolCall, pp *azureQueuePhaseProcessor) error {
	// Lookup PhaseProcessor for this ToolName
	tnpp, err := pm.toolNameToProcessPhase(*tc.ToolName)
	if err != nil {
		return err // unrecognized tool name; log?
	}
	for *tc.Status == toolcalls.ToolCallStatusRunning { // Loop while tool call is running
		tc, err := tnpp(ctx, tc, pp) // Transition tool call from current phase to next phase
		if err != nil {
			return err
		}
		// Persists new state of tool call resource (etag must match)
		_, err = ToolCallOps.Put(ctx, tc, &toolcalls.AccessConditions{IfMatch: tc.ETag})
		if err != nil {
			return err // log?
		}
	}

	// When no longer "running", phase processing is complete, so delete the queue message
	_, err = pm.queueClient.DeleteMessage(ctx, pp.messageID, pp.popReceipt, nil)
	return err // Ignore any failure
}

func (pm *azureQueueToolCallPhaseMgr) NewPhaseProcessor(messageID, popReceipt string) *azureQueuePhaseProcessor {
	return &azureQueuePhaseProcessor{mgr: pm, messageID: messageID, popReceipt: popReceipt}
}

type azureQueuePhaseProcessor struct {
	mgr        *azureQueueToolCallPhaseMgr
	messageID  string
	popReceipt string
}

func (pp *azureQueuePhaseProcessor) ExtendProcessingTime(ctx context.Context, phaseExecutionTime time.Duration) error {
	resp, err := pp.mgr.queueClient.UpdateMessage(ctx, pp.messageID, pp.popReceipt, "",
		&azqueue.UpdateMessageOptions{VisibilityTimeout: si.Ptr(int32(phaseExecutionTime.Seconds()))})
	if err != nil {
		return nil
	}
	pp.popReceipt = *resp.PopReceipt
	return nil
}

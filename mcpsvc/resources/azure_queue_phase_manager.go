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
	"path"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azqueue"
	"github.com/JeffreyRichter/mcpsvc/config"
	"github.com/JeffreyRichter/mcpsvc/mcp/toolcalls"
	si "github.com/JeffreyRichter/serviceinfra"
)

type azureQueueToolCallPhaseMgr struct {
	queueClient            *azqueue.QueueClient
	toolCallStore          ToolCallStore
	toolNameToProcessPhase toolcalls.ToolNameToProcessPhaseFunc
}

// NewAzureQueueToolCallPhaseMgr creates a new azureQueueToolCallPhaseMgr.
// queueUrl must look like: https://myaccount.queue.core.windows.net/<queuename>
// TODO: call this at startup (singleton)
// TODO: why does this take a Context? It spawns a goroutine that runs forever
func NewAzureQueueToolCallPhaseMgr(ctx context.Context, store ToolCallStore, queueUrl string, tn2pp toolcalls.ToolNameToProcessPhaseFunc) (PhaseManager, error) {
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, err
	}
	queueClient, err := azqueue.NewQueueClient(queueUrl, cred, nil)
	if err != nil {
		return nil, err
	}
	// if the queue already exists, no error is returned
	if _, err = queueClient.Create(ctx, nil); err != nil {
		return nil, err
	}
	pm := &azureQueueToolCallPhaseMgr{queueClient: queueClient, toolCallStore: store, toolNameToProcessPhase: tn2pp}
	go pm.Processor(ctx, 30*time.Second, true) // TODO: logger
	return pm, nil
}

// DeleteQueue delete the queue. This is most useful for debugging/testing.
func (pm *azureQueueToolCallPhaseMgr) DeleteQueue(ctx context.Context) error {
	_, err := pm.queueClient.Delete(ctx, nil)
	return err
}

// Processor forever loops dequeuing/processing ToolCall Phases.
// Use ctx to cancel Processor & all ToolCall Phases in flight.
// Poison messages & other failures are logged.
// TODO: put logger on the struct
// TODO: launch a goroutine for this at startup; maybe do it in NewAzureQueueToolCallPhaseMgr and unexport this
func (pm *azureQueueToolCallPhaseMgr) Processor(ctx context.Context, phaseExecutionTime time.Duration, loggerTODO bool) {
	o := &azqueue.DequeueMessagesOptions{
		NumberOfMessages:  si.Ptr(int32(10)),
		VisibilityTimeout: si.Ptr(int32(phaseExecutionTime.Seconds())),
	}
	for {
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
			tc, err := GetToolCallStore().Get(ctx, tc, &toolcalls.AccessConditions{})
			if err != nil { // ToolCallID not expired/not found
				// No more phases to execute; delete the queue message (or let it become a poison message)
				// ContinuePhaseProcessing will delete the message if Status != toolcalls.ToolCallStatusRunning
				// TODO: log; maybe not
				continue
			}
			pp := pm.NewPhaseProcessor(*m.MessageID, *m.PopReceipt)
			pm.ContinuePhaseProcessing(ctx, tc, pp)
		}
		time.Sleep(time.Millisecond * 200)
	}
}

// TODO: this creates a dependency on a specific tool call storage implementation (Azure Blob). Could we queue
// the tool call JSON instead of a blob URL pointing to it? That would allow using the store interface's Get
// method, eliminating the implementation dependency
type azureQueueMessage struct {
	ToolCallIDUrl string `json:"toolCallIDUrl"`
}

func (m *azureQueueMessage) Parse() (tenant, toolName, toolCallID string) {
	return ToolCallOps.fromBlobUrl(m.ToolCallIDUrl)
}

// StartPhaseProcessing: enqueues a new tool call phase with tool name & tool call id.
// Calls continuePhaseProcessing (passing time extender in PhaseProcessor interface) while status is in progress. Updates tc resource after continue returns. Deletes queue message when status is not in progress.
// ToolCall calls this when it transitions to status "running" (from "submitted" or from a callback)
//
// TODO: reconsider calling pattern and signature. Processing should run asynchronously from HTTP handling and start when a tool call is created.
// This function doesn't work well on a separate goroutine because the caller is handling a client request and needs to return a response quickly.
// The caller can't pass the Context associated with a client request to this handler or create a new one it's responsible for canceling--caller wants
// to return ASAP. Raises bigger question: who should own the Context for a tool call's phase processing? Who would want to cancel it, and why?
func (pm *azureQueueToolCallPhaseMgr) StartPhaseProcessing(ctx context.Context, tc *toolcalls.ToolCall, phaseExecutionTime time.Duration) error {
	containerName, blobName := ToolCallOps.toBlobInfo(tc)
	data := must(json.Marshal(azureQueueMessage{
		// got "{\"toolCallIDUrl\":\"https://sometenant.blob.core.windows.net/count/test1\"}"
		// want https://samfzqhbrdlxlsm.blob.core.windows.net/sometenant/count/test1
		ToolCallIDUrl: path.Join(config.Get().AzureStorageBlobURL, containerName, blobName),
	}))
	resp, err := pm.queueClient.EnqueueMessage(ctx, string(data),
		&azqueue.EnqueueMessageOptions{VisibilityTimeout: si.Ptr(int32(phaseExecutionTime.Seconds()))})
	if err != nil {
		return nil
	}
	pp := pm.NewPhaseProcessor(*resp.Messages[0].MessageID, *resp.Messages[0].PopReceipt)
	return pm.ContinuePhaseProcessing(ctx, tc, pp)
}

// TODO: unexport
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
		_, err = GetToolCallStore().Put(ctx, tc, &toolcalls.AccessConditions{IfMatch: tc.ETag})
		if err != nil {
			return err // log?
		}
	}

	// When no longer "running", phase processing is complete, so delete the queue message
	if _, err = pm.queueClient.DeleteMessage(ctx, pp.messageID, pp.popReceipt, nil); err != nil {
		// TODO: compare popReceipt; it's like an etag for q messages
	}
	return nil // Ignore any failure
}

// TODO: unexport
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

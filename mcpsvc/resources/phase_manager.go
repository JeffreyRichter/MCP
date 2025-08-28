package resources

/*
Most advancements of a tool call are client-induced: initiating the tool call (status-submitted),
sending elicitation/sampling result.

Some tools require server-induced advancements. For example, a tool polling for some condition:
a specific day/time, the completion of a task from another service, a stock price to reach a certain value, etc.
*/
import (
	"context"
	"encoding/json/v2"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/JeffreyRichter/mcpsvc/config"
	"github.com/JeffreyRichter/mcpsvc/mcp/toolcalls"
	si "github.com/JeffreyRichter/serviceinfra"
)

type PhaseManager interface {
	StartPhaseProcessing(ctx context.Context, tc *toolcalls.ToolCall, phaseExecutionTime time.Duration) error
}

var GetPhaseManager = sync.OnceValue(func() PhaseManager {
	tn2pp := func(tool string) (toolcalls.ProcessPhaseFunc, error) {
		switch tool {
		case "count":
			return processPhaseToolCallCount, nil
		default:
			return nil, fmt.Errorf("unrecognized tool name %q", tool)
		}
	}
	cfg := config.Get()
	return must(NewAzureQueueToolCallPhaseMgr(context.Background(), GetToolCallStore(), cfg.AzureStorageQueueURL, tn2pp))
})

// TODO: this belongs on v20250808.httpOperations or at least in v20250808 but can't be there now because
// the phase manager needs to reference this function at construction; see GetPhaseManager above. Overall
// this arrangement is awkward because the phase manager is nominally version-independent but in practice
// needs to know about specific tool calls
func processPhaseToolCallCount(_ context.Context, tc *toolcalls.ToolCall, _ toolcalls.PhaseProcessor) (*toolcalls.ToolCall, error) {
	phase, err := strconv.Atoi(*tc.Phase)
	if err != nil {
		return nil, fmt.Errorf("invalid phase %q", *tc.Phase)
	}

	// simulate doing work
	time.Sleep(17 * time.Millisecond)

	phase--
	tc.Phase = si.Ptr(strconv.Itoa(phase))
	// TODO: if we needed data from the client request e.g. CountToolCallRequest here, we'd have to unmarshal it again
	if phase <= 0 {
		tc.Status = si.Ptr(toolcalls.ToolCallStatusSuccess)
		tc.Result = must(json.Marshal(struct{ N int }{N: 42}))
	}
	return tc, nil
}

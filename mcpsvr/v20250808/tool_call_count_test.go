package v20250808

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/JeffreyRichter/mcpsvr/mcp/toolcalls"
	"github.com/stretchr/testify/require"
)

func TestToolCallCount(t *testing.T) {
	t.Skip("finish the feature then write this test; need in-memory queue implementation")
	os.Setenv("MCPSVR_AZURE_QUEUE_URL", "https://samfzqhbrdlxlsm.queue.core.windows.net/mcpsvr")
	os.Setenv("MCPSVR_AZURE_BLOB_URL", "https://samfzqhbrdlxlsm.blob.core.windows.net/")
	client := newTestClient(t)
	resp := client.Put("/mcp/tools/count/calls/"+t.Name(), http.Header{}, strings.NewReader(`{"start":40,"increments":2}`))
	require.Equal(t, http.StatusOK, resp.StatusCode)

	tc := toolcalls.ToolCall{}
	for {
		resp = client.Get("/mcp/tools/count/calls/"+t.Name(), http.Header{})
		require.Equal(t, http.StatusOK, resp.StatusCode)

		b, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		err = json.Unmarshal(b, &tc)
		require.NoError(t, err)

		require.NotNil(t, tc.Status)
		if *tc.Status != toolcalls.ToolCallStatusRunning {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	require.Equal(t, toolcalls.ToolCallStatusSuccess, *tc.Status)
}

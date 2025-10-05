package main

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/JeffreyRichter/mcp"
	"github.com/JeffreyRichter/mcpsvr/toolcall"
	"github.com/stretchr/testify/require"
)

func TestToolCallCount(t *testing.T) {
	client := newTestClient(t)
	urlPath := "/mcp/tools/count/calls/" + t.Name()
	resp := client.Put(urlPath,
		http.Header{
			"Idempotency-Key": []string{time.Now().Format(time.RFC3339Nano)},
			"Content-Type":    []string{"application/json"},
			"Accept":          []string{"application/json"},
		},
		strings.NewReader(`{"countto":40}`))
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var tc toolcall.Resource
	for {
		jsonBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		err = json.Unmarshal(jsonBody, &tc)
		require.NoError(t, err)
		if *tc.Status == mcp.StatusSuccess {
			break
		}

		resp = client.Get(urlPath, http.Header{"Accept": []string{"application/json"}})
		require.Equal(t, http.StatusOK, resp.StatusCode)
		time.Sleep(50 * time.Millisecond)
	}
	require.Equal(t, mcp.StatusSuccess, *tc.Status)
}

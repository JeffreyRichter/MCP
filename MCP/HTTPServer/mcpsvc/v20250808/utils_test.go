package v20250808

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/JeffreyRichter/mcpsvc/mcp/toolcalls"
	"github.com/JeffreyRichter/mcpsvc/resources"
	si "github.com/JeffreyRichter/serviceinfra"
	"github.com/JeffreyRichter/serviceinfra/policies"
)

func testServer(t *testing.T) *httptest.Server {
	setupMockStore(t)

	policies := []si.Policy{
		policies.NewGracefulShutdownPolicy(),
		policies.NewLoggingPolicy(os.Stderr),
		policies.NewThrottlingPolicy(100),
		policies.NewAuthorizationPolicy(""),
		policies.NewMetricsPolicy(),
		policies.NewDistributedTracing(),
	}
	avis := []*si.ApiVersionInfo{{GetRoutes: Routes}}
	handler := si.BuildHandler(policies, avis, 5*time.Second)

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	return srv
}

type testClient struct {
	t   *testing.T
	url string
}

func newTestClient(t *testing.T) *testClient {
	srv := testServer(t)
	t.Cleanup(srv.Close)
	return &testClient{t: t, url: srv.URL}
}

func (c *testClient) Put(path string, headers http.Header, body io.Reader) *http.Response {
	return c.do(http.MethodPut, path, headers, body)
}

func (c *testClient) Post(path string, headers http.Header, body io.Reader) *http.Response {
	return c.do(http.MethodPost, path, headers, body)
}

func (c *testClient) Get(path string, headers http.Header) *http.Response {
	return c.do(http.MethodGet, path, headers, nil)
}

func (c *testClient) do(method, path string, headers http.Header, body io.Reader) *http.Response {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, method, c.url+path, body)
	if err != nil {
		c.t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		for _, val := range v {
			req.Header.Add(k, val)
		}
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	return resp
}

// setupMockStore replaces the default Azure Storage persistence implementation with an in-memory one for the duration of a test.
// This won't work for parallel tests because the store is a singleton.
func setupMockStore(t *testing.T) {
	mock := &inMemoryToolCallStore{
		calls: make(map[string]*toolcalls.ToolCall),
		mtx:   &sync.RWMutex{},
	}

	before := resources.GetToolCallStore
	resources.GetToolCallStore = sync.OnceValue(func() resources.ToolCallStore {
		return mock
	})

	beforeGetOps := GetOps
	GetOps = sync.OnceValue(func() *httpOperations {
		return &httpOperations{ToolCallStore: mock}
	})

	// Reset the GetToolInfos singleton so it picks up the new GetOps
	beforeGetToolInfos := GetToolInfos
	GetToolInfos = sync.OnceValue(buildToolInfosMap)

	t.Cleanup(func() {
		resources.GetToolCallStore = before
		GetOps = beforeGetOps
		GetToolInfos = beforeGetToolInfos
	})
}

type inMemoryToolCallStore struct {
	calls map[string]*toolcalls.ToolCall
	mtx   *sync.RWMutex
}

func (s *inMemoryToolCallStore) Get(_ context.Context, tenant string, toolCall *toolcalls.ToolCall, _ *toolcalls.AccessConditions) (*toolcalls.ToolCall, error) {
	s.mtx.RLock()
	defer s.mtx.RUnlock()

	key := tenant + "/" + *toolCall.ToolName + "/" + *toolCall.ToolCallId
	if stored, exists := s.calls[key]; exists {
		return stored, nil
	}
	return nil, &si.ServiceError{StatusCode: 404, ErrorCode: "NotFound", Message: "Tool call not found"}
}

func (s *inMemoryToolCallStore) Put(_ context.Context, tenant string, toolCall *toolcalls.ToolCall, _ *toolcalls.AccessConditions) (*toolcalls.ToolCall, error) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	stored := &toolcalls.ToolCall{
		ToolName:           toolCall.ToolName,
		ToolCallId:         toolCall.ToolCallId,
		Expiration:         toolCall.Expiration,
		AdvanceQueue:       toolCall.AdvanceQueue,
		ETag:               si.Ptr(si.ETag("mock-etag-123")),
		Status:             toolCall.Status,
		Request:            toolCall.Request,
		SamplingRequest:    toolCall.SamplingRequest,
		ElicitationRequest: toolCall.ElicitationRequest,
		Progress:           toolCall.Progress,
		Result:             toolCall.Result,
		Error:              toolCall.Error,
	}

	key := tenant + "/" + *toolCall.ToolName + "/" + *toolCall.ToolCallId
	s.calls[key] = stored
	return stored, nil
}

func (s *inMemoryToolCallStore) Delete(_ context.Context, tenant string, toolCall *toolcalls.ToolCall, _ *toolcalls.AccessConditions) error {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	key := tenant + "/" + *toolCall.ToolName + "/" + *toolCall.ToolCallId
	delete(s.calls, key)
	return nil
}

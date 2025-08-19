package v20250808

import (
	"context"
	"sync"
	"testing"

	"github.com/JeffreyRichter/mcpsvc/mcp/toolcalls"
	"github.com/JeffreyRichter/mcpsvc/resources"
	si "github.com/JeffreyRichter/serviceinfra"
)

// SetupMockStore replaces the default Azure Storage persistence implementation with an in-memory one for the duration of a test.
// This won't work for parallel tests because the store is a singleton.
func SetupMockStore(t *testing.T) {
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

package resources

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/JeffreyRichter/mcpsvc/mcp/toolcalls"
	"github.com/JeffreyRichter/svrcore"
)

// InMemoryToolCallStore is an in-memory [ToolCallStore] having the same semantics as [AzureBlobToolCallStore]
type InMemoryToolCallStore struct {
	data map[string]*toolcalls.ToolCall
	mu   *sync.RWMutex
}

// NewInMemoryToolCallStore creates a new InMemoryToolCallStore; ctx is used to cancel the reaper goroutine
func NewInMemoryToolCallStore(ctx context.Context) *InMemoryToolCallStore {
	s := &InMemoryToolCallStore{
		data: map[string]*toolcalls.ToolCall{},
		mu:   &sync.RWMutex{},
	}
	go s.reaper(ctx)
	return s
}

// reaper removes expired tool calls from the store
func (s *InMemoryToolCallStore) reaper(ctx context.Context) {
	for {
		select { // Return if canceled or process expired tool calls
		case <-ctx.Done():
			return
		default:
			time.Sleep(time.Minute)
			s.mu.Lock()
			for k, v := range s.data {
				if v.Expiration.Before(time.Now()) {
					delete(s.data, k)
				}
			}
			s.mu.Unlock()
		}
	}
}

func (s *InMemoryToolCallStore) Get(_ context.Context, tc *toolcalls.ToolCall, ac svrcore.AccessConditions) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key := key(*tc.Tenant, *tc.ToolName, *tc.ToolCallId)
	stored, ok := s.data[key]
	if !ok {
		return &svrcore.ServiceError{
			StatusCode: 404,
			ErrorCode:  "NotFound",
			Message:    "Tool call not found",
		}
	}
	*tc = stored.Copy() // copying prevents the caller mutating stored data
	err := svrcore.ValidatePreconditions(svrcore.ResourceValues{AllowedConditionals: svrcore.AllowedConditionalsMatch, ETag: stored.ETag}, http.MethodGet, ac)
	if err != nil {
		return err
	}
	return nil
}

func (s *InMemoryToolCallStore) Put(_ context.Context, tc *toolcalls.ToolCall, ac svrcore.AccessConditions) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := key(*tc.Tenant, *tc.ToolName, *tc.ToolCallId)
	if stored, ok := s.data[key]; ok {
		err := svrcore.ValidatePreconditions(svrcore.ResourceValues{AllowedConditionals: svrcore.AllowedConditionalsNone, ETag: stored.ETag}, http.MethodPut, ac)
		if err != nil {
			return err
		}
	}
	cp := tc.Copy() // storing a copy prevents mutating the caller's data
	cp.ETag = svrcore.Ptr(svrcore.ETag(time.Now().Format("20060102150405.000000")))
	s.data[key] = &cp
	*tc = cp // except we want the caller to have the actual ETag
	return nil
}

func (s *InMemoryToolCallStore) Delete(_ context.Context, tc *toolcalls.ToolCall, ac svrcore.AccessConditions) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := key(*tc.Tenant, *tc.ToolName, *tc.ToolCallId)
	if stored, ok := s.data[key]; ok {
		err := svrcore.ValidatePreconditions(svrcore.ResourceValues{AllowedConditionals: svrcore.AllowedConditionalsMatch, ETag: stored.ETag}, http.MethodDelete, ac)
		if err != nil {
			return err
		}
	}
	delete(s.data, key)
	return nil
}

func key(tenant, toolName, toolCallId string) string {
	return tenant + "/" + toolName + "/" + toolCallId
}

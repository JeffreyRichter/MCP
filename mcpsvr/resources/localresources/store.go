package localresources

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/JeffreyRichter/mcpsvr/mcp/toolcalls"
	"github.com/JeffreyRichter/mcpsvr/resources"
	"github.com/JeffreyRichter/svrcore"
)

// LocalToolCallStore is an in-memory [ToolCallStore] having the same semantics as [AzureBlobToolCallStore]
type localToolCallStore struct {
	data map[string]*toolcalls.ToolCall
	mu   *sync.RWMutex
}

// NewToolCallStore creates a [resources.ToolCallStore]; ctx is used to cancel the expiry goroutine
func NewToolCallStore(ctx context.Context) resources.ToolCallStore {
	s := &localToolCallStore{data: map[string]*toolcalls.ToolCall{}, mu: &sync.RWMutex{}}
	go s.expiry(ctx)
	return s
}

// reaper removes expired tool calls from the store
func (s *localToolCallStore) expiry(ctx context.Context) {
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

func (s *localToolCallStore) Get(_ context.Context, tc *toolcalls.ToolCall, ac svrcore.AccessConditions) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key := key(*tc.Tenant, *tc.ToolName, *tc.ToolCallId)
	stored, ok := s.data[key]
	if !ok {
		return &svrcore.ServerError{
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

func (s *localToolCallStore) Put(_ context.Context, tc *toolcalls.ToolCall, ac svrcore.AccessConditions) error {
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

func (s *localToolCallStore) Delete(_ context.Context, tc *toolcalls.ToolCall, ac svrcore.AccessConditions) error {
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

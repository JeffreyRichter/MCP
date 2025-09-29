package localresources

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/JeffreyRichter/mcpsvr/mcp/toolcall"
	"github.com/JeffreyRichter/svrcore"
)

// LocalToolCallStore is an in-memory [ToolCallStore] having the same semantics as [AzureBlobToolCallStore]
type localToolCallStore struct {
	data map[string]*toolcall.ToolCall
	mu   *sync.RWMutex
}

// NewToolCallStore creates a [toolcall.Store]; ctx is used to cancel the expiry goroutine
func NewToolCallStore(ctx context.Context) toolcall.Store {
	s := &localToolCallStore{data: map[string]*toolcall.ToolCall{}, mu: &sync.RWMutex{}}
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

func (s *localToolCallStore) Get(_ context.Context, tc *toolcall.ToolCall, ac svrcore.AccessConditions) *svrcore.ServerError {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key := s.key(*tc.Tenant, *tc.ToolName, *tc.ID)
	stored, ok := s.data[key]
	if !ok {
		return &svrcore.ServerError{
			StatusCode: 404,
			ErrorCode:  "NotFound",
			Message:    "Tool call not found",
		}
	}
	*tc = stored.Copy() // copying prevents the caller mutating stored data
	se := svrcore.CheckPreconditions(svrcore.ResourceValues{AllowedConditionals: svrcore.AllowedConditionalsMatch, ETag: stored.ETag}, http.MethodGet, ac)
	if se != nil {
		return se
	}
	return nil
}

func (s *localToolCallStore) Put(_ context.Context, tc *toolcall.ToolCall, ac svrcore.AccessConditions) *svrcore.ServerError {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := s.key(*tc.Tenant, *tc.ToolName, *tc.ID)
	if stored, ok := s.data[key]; ok {
		se := svrcore.CheckPreconditions(svrcore.ResourceValues{AllowedConditionals: svrcore.AllowedConditionalsMatch, ETag: stored.ETag}, http.MethodPut, ac)
		if se != nil {
			tc.ETag = stored.ETag // return the current ETag to the caller
			return se
		}
	}
	cp := tc.Copy() // storing a copy prevents mutating the caller's data
	cp.ETag = svrcore.Ptr(svrcore.ETag(time.Now().Format("20060102150405.000000")))
	s.data[key] = &cp
	*tc = cp // except we want the caller to have the actual ETag
	return nil
}

func (s *localToolCallStore) Delete(_ context.Context, tc *toolcall.ToolCall, ac svrcore.AccessConditions) *svrcore.ServerError {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := s.key(*tc.Tenant, *tc.ToolName, *tc.ID)
	if stored, ok := s.data[key]; ok {
		se := svrcore.CheckPreconditions(svrcore.ResourceValues{AllowedConditionals: svrcore.AllowedConditionalsMatch, ETag: stored.ETag}, http.MethodDelete, ac)
		if se != nil {
			return se
		}
	}
	delete(s.data, key)
	return nil
}

func (*localToolCallStore) key(tenant, toolName, toolCallID string) string {
	return tenant + "/" + toolName + "/" + toolCallID
}

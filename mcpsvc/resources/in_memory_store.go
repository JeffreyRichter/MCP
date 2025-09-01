package resources

import (
	"context"
	"sync"
	"time"

	"github.com/JeffreyRichter/mcpsvc/mcp/toolcalls"
	"github.com/JeffreyRichter/serviceinfra"
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

func (s *InMemoryToolCallStore) Get(_ context.Context, toolCall *toolcalls.ToolCall, accessConditions *toolcalls.AccessConditions) (*toolcalls.ToolCall, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key := key(*toolCall.Tenant, *toolCall.ToolName, *toolCall.ToolCallId)
	stored, ok := s.data[key]
	if !ok {
		return nil, &serviceinfra.ServiceError{
			StatusCode: 404,
			ErrorCode:  "NotFound",
			Message:    "Tool call not found",
		}
	}

	// TODO: clean up and consolidate AccessConditions handling
	if accessConditions != nil {
		if accessConditions.IfMatch != nil && stored.ETag != nil && !accessConditions.IfMatch.Equals(*stored.ETag) {
			return nil, &serviceinfra.ServiceError{
				StatusCode: 412,
				ErrorCode:  "PreconditionFailed",
				Message:    "The condition specified using HTTP conditional header(s) is not met",
			}
		}
		if accessConditions.IfNoneMatch != nil && stored.ETag != nil && accessConditions.IfNoneMatch.Equals(*stored.ETag) {
			return nil, &serviceinfra.ServiceError{
				StatusCode: 304,
				ErrorCode:  "NotModified",
				Message:    "The resource has not been modified",
			}
		}
	}

	// copying prevents the caller mutating stored data
	return stored.Copy(), nil
}

func (s *InMemoryToolCallStore) Put(_ context.Context, toolCall *toolcalls.ToolCall, accessConditions *toolcalls.AccessConditions) (*toolcalls.ToolCall, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := key(*toolCall.Tenant, *toolCall.ToolName, *toolCall.ToolCallId)
	if accessConditions != nil {
		if stored, ok := s.data[key]; ok {
			if accessConditions.IfMatch != nil && stored.ETag != nil && !accessConditions.IfMatch.Equals(*stored.ETag) {
				return nil, &serviceinfra.ServiceError{
					StatusCode: 412,
					ErrorCode:  "PreconditionFailed",
					Message:    "The condition specified using HTTP conditional header(s) is not met",
				}
			}
			if accessConditions.IfNoneMatch != nil {
				return nil, &serviceinfra.ServiceError{
					StatusCode: 412,
					ErrorCode:  "PreconditionFailed",
					Message:    "The condition specified using HTTP conditional header(s) is not met",
				}
			}
		} else if accessConditions.IfMatch != nil {
			return nil, &serviceinfra.ServiceError{
				StatusCode: 412,
				ErrorCode:  "PreconditionFailed",
				Message:    "The condition specified using HTTP conditional header(s) is not met",
			}
		}
	}

	// storing a copy prevents mutating the caller's data
	cp := toolCall.Copy()
	cp.ETag = serviceinfra.Ptr(serviceinfra.ETag(time.Now().Format("20060102150405.000000")))
	s.data[key] = cp

	// except we want the caller to have the actual ETag
	toolCall.ETag = cp.ETag

	return toolCall, nil
}

func (s *InMemoryToolCallStore) Delete(_ context.Context, toolCall *toolcalls.ToolCall, accessConditions *toolcalls.AccessConditions) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := key(*toolCall.Tenant, *toolCall.ToolName, *toolCall.ToolCallId)
	if accessConditions != nil {
		if stored, ok := s.data[key]; ok {
			if accessConditions.IfMatch != nil && stored.ETag != nil && !accessConditions.IfMatch.Equals(*stored.ETag) {
				return &serviceinfra.ServiceError{
					StatusCode: 412,
					ErrorCode:  "PreconditionFailed",
					Message:    "The condition specified using HTTP conditional header(s) is not met",
				}
			}
			if accessConditions.IfNoneMatch != nil && stored.ETag != nil && accessConditions.IfNoneMatch.Equals(*stored.ETag) {
				return &serviceinfra.ServiceError{
					StatusCode: 412,
					ErrorCode:  "PreconditionFailed",
					Message:    "The condition specified using HTTP conditional header(s) is not met",
				}
			}
		} else {
			return nil
		}
	}

	delete(s.data, key)
	return nil
}

func key(tenant, toolName, toolCallId string) string {
	return tenant + "/" + toolName + "/" + toolCallId
}

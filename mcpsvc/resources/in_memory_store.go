package resources

import (
	"context"
	"time"

	"github.com/JeffreyRichter/mcpsvc/mcp/toolcalls"
	si "github.com/JeffreyRichter/serviceinfra"
	"github.com/JeffreyRichter/serviceinfra/syncmap"
)

// InMemoryToolCallStore is an in-memory [ToolCallStore] having the same semantics as [AzureBlobToolCallStore]
type InMemoryToolCallStore struct {
	data syncmap.Map[string, *toolcalls.ToolCall]
}

func NewInMemoryToolCallStore() *InMemoryToolCallStore {
	return &InMemoryToolCallStore{}
}

func (s *InMemoryToolCallStore) Get(_ context.Context, toolCall *toolcalls.ToolCall, accessConditions *toolcalls.AccessConditions) (*toolcalls.ToolCall, error) {
	key := key(*toolCall.Tenant, *toolCall.ToolName, *toolCall.ToolCallId)

	stored, ok := s.data.Load(key)
	if !ok {
		return nil, &si.ServiceError{
			StatusCode: 404,
			ErrorCode:  "NotFound",
			Message:    "Tool call not found",
		}
	}

	// TODO: clean up and consolidate AccessConditions handling
	if accessConditions != nil {
		if accessConditions.IfMatch != nil && stored.ETag != nil && !accessConditions.IfMatch.Equals(*stored.ETag) {
			return nil, &si.ServiceError{
				StatusCode: 412,
				ErrorCode:  "PreconditionFailed",
				Message:    "The condition specified using HTTP conditional header(s) is not met",
			}
		}
		if accessConditions.IfNoneMatch != nil && stored.ETag != nil && accessConditions.IfNoneMatch.Equals(*stored.ETag) {
			return nil, &si.ServiceError{
				StatusCode: 304,
				ErrorCode:  "NotModified",
				Message:    "The resource has not been modified",
			}
		}
	}

	// copying prevents mutating shared data
	return stored.Copy(), nil
}

func (s *InMemoryToolCallStore) Put(_ context.Context, toolCall *toolcalls.ToolCall, accessConditions *toolcalls.AccessConditions) (*toolcalls.ToolCall, error) {
	key := key(*toolCall.Tenant, *toolCall.ToolName, *toolCall.ToolCallId)

	if accessConditions != nil {
		if stored, ok := s.data.Load(key); ok {
			if accessConditions.IfMatch != nil && stored.ETag != nil && !accessConditions.IfMatch.Equals(*stored.ETag) {
				return nil, &si.ServiceError{
					StatusCode: 412,
					ErrorCode:  "PreconditionFailed",
					Message:    "The condition specified using HTTP conditional header(s) is not met",
				}
			}
			if accessConditions.IfNoneMatch != nil {
				return nil, &si.ServiceError{
					StatusCode: 412,
					ErrorCode:  "PreconditionFailed",
					Message:    "The condition specified using HTTP conditional header(s) is not met",
				}
			}
		} else if accessConditions.IfMatch != nil {
			return nil, &si.ServiceError{
				StatusCode: 412,
				ErrorCode:  "PreconditionFailed",
				Message:    "The condition specified using HTTP conditional header(s) is not met",
			}
		}
	}

	// storing a copy prevents mutating the caller's data
	cp := toolCall.Copy()
	cp.ETag = si.Ptr(si.ETag(time.Now().Format("20060102150405.000000")))
	s.data.Store(key, cp)

	// except we want the caller to have the actual ETag
	toolCall.ETag = cp.ETag

	return toolCall, nil
}

func (s *InMemoryToolCallStore) Delete(_ context.Context, toolCall *toolcalls.ToolCall, accessConditions *toolcalls.AccessConditions) error {
	key := key(*toolCall.Tenant, *toolCall.ToolName, *toolCall.ToolCallId)
	if accessConditions != nil {
		if stored, ok := s.data.Load(key); ok {
			if accessConditions.IfMatch != nil && stored.ETag != nil && !accessConditions.IfMatch.Equals(*stored.ETag) {
				return &si.ServiceError{
					StatusCode: 412,
					ErrorCode:  "PreconditionFailed",
					Message:    "The condition specified using HTTP conditional header(s) is not met",
				}
			}
			if accessConditions.IfNoneMatch != nil && stored.ETag != nil && accessConditions.IfNoneMatch.Equals(*stored.ETag) {
				return &si.ServiceError{
					StatusCode: 412,
					ErrorCode:  "PreconditionFailed",
					Message:    "The condition specified using HTTP conditional header(s) is not met",
				}
			}
		} else {
			return nil
		}
	}

	s.data.Delete(key)
	return nil
}

func key(tenant, toolName, toolCallId string) string {
	return tenant + "/" + toolName + "/" + toolCallId
}

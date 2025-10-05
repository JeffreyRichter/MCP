package local

import (
	"context"
	"encoding/json/jsontext"
	"testing"
	"time"

	"github.com/JeffreyRichter/internal/aids"
	"github.com/JeffreyRichter/mcp"
	"github.com/JeffreyRichter/mcpsvr/toolcall"
	"github.com/JeffreyRichter/svrcore"
)

var ctx = context.Background()

func TestLocalToolCallStore_Get_NotFound(t *testing.T) {
	store := NewToolCallStore(ctx)
	tc := &toolcall.Resource{
		Identity: toolcall.Identity{
			Tenant:   aids.New("test-tenant"),
			ToolName: aids.New("test-tool"),
			ID:       aids.New("test-id"),
		},
	}
	se := store.Get(ctx, tc, svrcore.AccessConditions{})
	if se.StatusCode != 404 {
		t.Errorf("Expected status code 404, got %d", se.StatusCode)
	}
	if se.ErrorCode != "NotFound" {
		t.Errorf("Expected error code 'NotFound', got %s", se.ErrorCode)
	}
}

func TestLocalToolCallStore_Put_and_Get(t *testing.T) {
	store := NewToolCallStore(ctx)

	originalToolCall := &toolcall.Resource{
		Identity: toolcall.Identity{
			Tenant:   aids.New("test-tenant"),
			ToolName: aids.New("test-tool"),
			ID:       aids.New("test-id"),
		},
		Expiration: aids.New(time.Now().Add(24 * time.Hour)),
		Status:     aids.New(mcp.StatusRunning),
		Request:    jsontext.Value(`{"param":"value"}`),
	}

	putResult := originalToolCall.Copy()
	se := store.Put(ctx, &putResult, svrcore.AccessConditions{})
	if se != nil {
		t.Fatalf("Put failed: %v", se)
	}

	if putResult.ETag == nil {
		t.Fatal("Expected ETag to be set on put result")
	}

	if *putResult.ToolName != *originalToolCall.ToolName {
		t.Errorf("ToolName mismatch: expected %s, got %s", *originalToolCall.ToolName, *putResult.ToolName)
	}

	if *putResult.ID != *originalToolCall.ID {
		t.Errorf("ID mismatch: expected %s, got %s", *originalToolCall.ID, *putResult.ID)
	}

	getToolCall := &toolcall.Resource{
		Identity: toolcall.Identity{
			Tenant:   aids.New("test-tenant"),
			ToolName: aids.New("test-tool"),
			ID:       aids.New("test-id"),
		},
	}

	getResult := getToolCall.Copy()
	se = store.Get(ctx, &getResult, svrcore.AccessConditions{})
	if se != nil {
		t.Fatalf("Get failed: %v", se)
	}

	if *getResult.ToolName != *originalToolCall.ToolName {
		t.Errorf("ToolName mismatch: expected %s, got %s", *originalToolCall.ToolName, *getResult.ToolName)
	}

	if *getResult.ID != *originalToolCall.ID {
		t.Errorf("ID mismatch: expected %s, got %s", *originalToolCall.ID, *getResult.ID)
	}

	if getResult.ETag == nil {
		t.Error("Expected ETag to be set on get result")
	} else if *getResult.ETag != *putResult.ETag {
		t.Errorf("ETag mismatch: expected %s, got %s", *putResult.ETag, *getResult.ETag)
	}
}

func TestLocalToolCallStore_Put_AccessConditions_IfMatch(t *testing.T) {
	store := NewToolCallStore(ctx)
	ctx := context.Background()

	tc1 := &toolcall.Resource{
		Identity: toolcall.Identity{
			Tenant:   aids.New("test-tenant"),
			ToolName: aids.New("test-tool"),
			ID:       aids.New("test-id"),
		},
		Status: aids.New(mcp.StatusRunning),
	}

	se := store.Put(ctx, tc1, svrcore.AccessConditions{})
	if se != nil {
		t.Fatalf("First put failed: %v", se)
	}

	tc2 := &toolcall.Resource{
		Identity: toolcall.Identity{
			Tenant:   aids.New("test-tenant"),
			ToolName: aids.New("test-tool"),
			ID:       aids.New("test-id"),
		},
		Status: aids.New(mcp.StatusSuccess),
	}
	accessConditions := svrcore.AccessConditions{IfMatch: aids.New(svrcore.ETag("NoMatch"))}
	se = store.Put(ctx, tc2, accessConditions)
	if se == nil || se.StatusCode != 412 {
		t.Fatalf("Second put with if-match should give 412, got %d", se.StatusCode)
	}

	if *tc2.Status != mcp.StatusSuccess {
		t.Errorf("Expected status to be updated to success, got %s", *tc2.Status)
	}
}

func TestLocalToolCallStore_Get_AccessConditions_IfMatch(t *testing.T) {
	store := NewToolCallStore(ctx)
	ctx := context.Background()

	originalToolCall := &toolcall.Resource{
		Identity: toolcall.Identity{
			Tenant:   aids.New("test-tenant"),
			ToolName: aids.New("test-tool"),
			ID:       aids.New("test-id"),
		},
		Status: aids.New(mcp.StatusRunning),
	}
	putResult := originalToolCall.Copy()
	se := store.Put(ctx, &putResult, svrcore.AccessConditions{})
	if se != nil {
		t.Fatalf("Put failed: %v", se)
	}

	getToolCall := &toolcall.Resource{
		Identity: toolcall.Identity{
			Tenant:   aids.New("test-tenant"),
			ToolName: aids.New("test-tool"),
			ID:       aids.New("test-id"),
		},
	}

	accessConditions := svrcore.AccessConditions{IfMatch: putResult.ETag}

	getResult := getToolCall.Copy()
	se = store.Get(ctx, getToolCall, accessConditions)
	if se != nil {
		t.Fatalf("Get with correct ETag failed: %v", se)
	}

	if *getResult.ToolName != *originalToolCall.ToolName {
		t.Errorf("Expected tool call to be returned")
	}

	wrongETag := svrcore.ETag("wrong-etag")
	accessConditions.IfMatch = &wrongETag

	se = store.Get(ctx, getToolCall, accessConditions)
	if se.StatusCode != 412 {
		t.Errorf("Expected status code 412, got %d", se.StatusCode)
	}
}

func TestLocalToolCallStore_Get_AccessConditions_IfNoneMatch(t *testing.T) {
	store := NewToolCallStore(ctx)
	ctx := context.Background()

	originalToolCall := &toolcall.Resource{
		Identity: toolcall.Identity{
			Tenant:   aids.New("test-tenant"),
			ToolName: aids.New("test-tool"),
			ID:       aids.New("test-id"),
		},
		Status: aids.New(mcp.StatusRunning),
	}

	putResult := originalToolCall.Copy()
	se := store.Put(ctx, &putResult, svrcore.AccessConditions{})
	if se != nil {
		t.Fatalf("Put failed: %v", se)
	}

	getToolCall := &toolcall.Resource{
		Identity: toolcall.Identity{
			Tenant:   aids.New("test-tenant"),
			ToolName: aids.New("test-tool"),
			ID:       aids.New("test-id"),
		},
	}

	accessConditions := svrcore.AccessConditions{IfNoneMatch: putResult.ETag}

	se = store.Get(ctx, getToolCall, accessConditions)
	if se.StatusCode != 304 {
		t.Errorf("Expected status code 304, got %d", se.StatusCode)
	}
}

func TestLocalToolCallStore_Delete(t *testing.T) {
	store := NewToolCallStore(ctx)
	ctx := context.Background()

	originalToolCall := &toolcall.Resource{
		Identity: toolcall.Identity{
			Tenant:   aids.New("test-tenant"),
			ToolName: aids.New("test-tool"),
			ID:       aids.New("test-id"),
		},
		Status: aids.New(mcp.StatusRunning),
	}

	se := store.Put(ctx, originalToolCall, svrcore.AccessConditions{})
	if se != nil {
		t.Fatalf("Put failed: %v", se)
	}

	se = store.Delete(ctx, originalToolCall, svrcore.AccessConditions{})
	if se != nil {
		t.Fatalf("Delete failed: %v", se)
	}

	se = store.Get(ctx, originalToolCall, svrcore.AccessConditions{})
	if se.StatusCode != 404 {
		t.Errorf("Expected status code 404 after delete, got %d", se.StatusCode)
	}
}

func TestLocalToolCallStore_Delete_AccessConditions(t *testing.T) {
	store := NewToolCallStore(ctx)
	ctx := context.Background()

	originalToolCall := &toolcall.Resource{
		Identity: toolcall.Identity{
			Tenant:   aids.New("test-tenant"),
			ToolName: aids.New("test-tool"),
			ID:       aids.New("test-id"),
		},
		Status: aids.New(mcp.StatusRunning),
	}
	putResult := originalToolCall.Copy()
	se := store.Put(ctx, &putResult, svrcore.AccessConditions{})
	if se != nil {
		t.Fatalf("Put failed: %v", se)
	}

	wrongETag := svrcore.ETag("wrong-etag")
	accessConditions := svrcore.AccessConditions{IfMatch: &wrongETag}
	se = store.Delete(ctx, originalToolCall, accessConditions)
	if se.StatusCode != 412 {
		t.Errorf("Expected status code 412, got %d", se.StatusCode)
	}

	accessConditions.IfMatch = putResult.ETag
	se = store.Delete(ctx, originalToolCall, accessConditions)
	if se != nil {
		t.Fatalf("Delete with correct ETag failed: %v", se)
	}
}

func TestLocalToolCallStore_Delete_NonExistent(t *testing.T) {
	store := NewToolCallStore(ctx)
	ctx := context.Background()

	toolCall := &toolcall.Resource{
		Identity: toolcall.Identity{
			Tenant:   aids.New("test-tenant"),
			ToolName: aids.New("test-tool"),
			ID:       aids.New("test-id"),
		},
	}

	se := store.Delete(ctx, toolCall, svrcore.AccessConditions{})
	if se != nil {
		t.Fatalf("Delete of non-existent item should not fail, got: %v", se)
	}
}

func TestLocalToolCallStore_TenantIsolation(t *testing.T) {
	store := NewToolCallStore(ctx)
	ctx := context.Background()

	tenant1 := "test-tenant"
	tenant2 := "different-tenant"

	toolCall := &toolcall.Resource{
		Identity: toolcall.Identity{
			Tenant:   aids.New("test-tenant"),
			ToolName: aids.New("test-tool"),
			ID:       aids.New("test-id"),
		},
		Status: aids.New(mcp.StatusRunning),
	}

	se := store.Put(ctx, toolCall, svrcore.AccessConditions{})
	if se != nil {
		t.Fatalf("Put to tenant1 failed: %v", se)
	}

	toolCall.Tenant = &tenant2
	se = store.Get(ctx, toolCall, svrcore.AccessConditions{})
	if se.StatusCode != 404 {
		t.Errorf("Expected status code 404 for different tenant, got %d", se.StatusCode)
	}

	toolCall.Tenant = &tenant1
	se = store.Get(ctx, toolCall, svrcore.AccessConditions{})
	if se != nil {
		t.Fatalf("Get from tenant1 should still work: %v", se)
	}
}

func TestLocalToolCallStore_DataIsolation(t *testing.T) {
	store := NewToolCallStore(ctx)
	ctx := context.Background()

	originalToolCall := &toolcall.Resource{
		Identity: toolcall.Identity{
			Tenant:   aids.New("test-tenant"),
			ToolName: aids.New("test-tool"),
			ID:       aids.New("test-id"),
		},
		Status:  aids.New(mcp.StatusRunning),
		Request: jsontext.Value(`{"param":"original"}`),
	}
	putResult := originalToolCall.Copy()
	se := store.Put(ctx, &putResult, svrcore.AccessConditions{})
	if se != nil {
		t.Fatalf("Put failed: %v", se)
	}

	*originalToolCall.Status = mcp.StatusSuccess
	originalToolCall.Request = jsontext.Value(`{"param":"modified"}`)

	getToolCall := &toolcall.Resource{
		Identity: toolcall.Identity{
			Tenant:   aids.New("test-tenant"),
			ToolName: aids.New("test-tool"),
			ID:       aids.New("test-id"),
		},
	}

	getResult := getToolCall.Copy()
	se = store.Get(ctx, &getResult, svrcore.AccessConditions{})
	if se != nil {
		t.Fatalf("Get failed: %v", se)
	}

	if *getResult.Status != mcp.StatusRunning {
		t.Errorf("Expected status to remain 'running', got %s", *getResult.Status)
	}

	if string(getResult.Request) != `{"param":"original"}` {
		t.Errorf("Expected request to remain original, got %s", string(getResult.Request))
	}

	*getResult.Status = mcp.StatusFailed
	getResult.Request = jsontext.Value(`{"param":"get-modified"}`)

	getResult2 := getToolCall.Copy()
	se = store.Get(ctx, &getResult2, svrcore.AccessConditions{})
	if se != nil {
		t.Fatalf("Second get failed: %v", se)
	}

	if *getResult2.Status != mcp.StatusRunning {
		t.Errorf("Expected stored status to remain 'running', got %s", *getResult2.Status)
	}

	if *putResult.ETag != *getResult.ETag {
		t.Errorf("Expected same ETags for put and get results, got put: %s, get: %s", *putResult.ETag, *getResult.ETag)
	}
}

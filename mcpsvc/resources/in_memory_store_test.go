package resources

import (
	"context"
	"encoding/json/jsontext"
	"testing"
	"time"

	"github.com/JeffreyRichter/mcpsvc/mcp/toolcalls"
	"github.com/JeffreyRichter/serviceinfra"
)

var ctx = context.Background()

func TestInMemoryToolCallStore_Get_NotFound(t *testing.T) {
	store := NewInMemoryToolCallStore(ctx)

	toolCall := &toolcalls.ToolCall{
		Tenant:     serviceinfra.Ptr("test-tenant"),
		ToolName:   serviceinfra.Ptr("test-tool"),
		ToolCallId: serviceinfra.Ptr("test-id"),
	}

	result, err := store.Get(ctx, toolCall, nil)

	if result != nil {
		t.Errorf("Expected nil result, got %v", result)
	}

	serviceError, ok := err.(*serviceinfra.ServiceError)
	if !ok {
		t.Fatalf("Expected ServiceError, got %T", err)
	}

	if serviceError.StatusCode != 404 {
		t.Errorf("Expected status code 404, got %d", serviceError.StatusCode)
	}

	if serviceError.ErrorCode != "NotFound" {
		t.Errorf("Expected error code 'NotFound', got %s", serviceError.ErrorCode)
	}
}

func TestInMemoryToolCallStore_Put_and_Get(t *testing.T) {
	store := NewInMemoryToolCallStore(ctx)

	originalToolCall := &toolcalls.ToolCall{
		Tenant:     serviceinfra.Ptr("test-tenant"),
		ToolName:   serviceinfra.Ptr("test-tool"),
		ToolCallId: serviceinfra.Ptr("test-id"),
		Expiration: serviceinfra.Ptr(time.Now().Add(24 * time.Hour)),
		Status:     serviceinfra.Ptr(toolcalls.ToolCallStatusRunning),
		Request:    jsontext.Value(`{"param":"value"}`),
	}

	putResult, err := store.Put(ctx, originalToolCall, nil)
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	if putResult.ETag == nil {
		t.Fatal("Expected ETag to be set on put result")
	}

	if *putResult.ToolName != *originalToolCall.ToolName {
		t.Errorf("ToolName mismatch: expected %s, got %s", *originalToolCall.ToolName, *putResult.ToolName)
	}

	if *putResult.ToolCallId != *originalToolCall.ToolCallId {
		t.Errorf("ToolCallId mismatch: expected %s, got %s", *originalToolCall.ToolCallId, *putResult.ToolCallId)
	}

	getToolCall := &toolcalls.ToolCall{
		Tenant:     serviceinfra.Ptr("test-tenant"),
		ToolName:   serviceinfra.Ptr("test-tool"),
		ToolCallId: serviceinfra.Ptr("test-id"),
	}

	getResult, err := store.Get(ctx, getToolCall, nil)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if *getResult.ToolName != *originalToolCall.ToolName {
		t.Errorf("ToolName mismatch: expected %s, got %s", *originalToolCall.ToolName, *getResult.ToolName)
	}

	if *getResult.ToolCallId != *originalToolCall.ToolCallId {
		t.Errorf("ToolCallId mismatch: expected %s, got %s", *originalToolCall.ToolCallId, *getResult.ToolCallId)
	}

	if getResult.ETag == nil {
		t.Error("Expected ETag to be set on get result")
	} else if *getResult.ETag != *putResult.ETag {
		t.Errorf("ETag mismatch: expected %s, got %s", *putResult.ETag, *getResult.ETag)
	}
}

func TestInMemoryToolCallStore_Put_AccessConditions_IfMatch(t *testing.T) {
	store := NewInMemoryToolCallStore(ctx)
	ctx := context.Background()

	originalToolCall := &toolcalls.ToolCall{
		Tenant:     serviceinfra.Ptr("test-tenant"),
		ToolName:   serviceinfra.Ptr("test-tool"),
		ToolCallId: serviceinfra.Ptr("test-id"),
		Status:     serviceinfra.Ptr(toolcalls.ToolCallStatusRunning),
	}

	putResult1, err := store.Put(ctx, originalToolCall, nil)
	if err != nil {
		t.Fatalf("First put failed: %v", err)
	}

	updatedToolCall := &toolcalls.ToolCall{
		Tenant:     serviceinfra.Ptr("test-tenant"),
		ToolName:   serviceinfra.Ptr("test-tool"),
		ToolCallId: serviceinfra.Ptr("test-id"),
		Status:     serviceinfra.Ptr(toolcalls.ToolCallStatusSuccess),
	}

	accessConditions := &toolcalls.AccessConditions{
		IfMatch: putResult1.ETag,
	}

	putResult2, err := store.Put(ctx, updatedToolCall, accessConditions)
	if err != nil {
		t.Fatalf("Second put with correct ETag failed: %v", err)
	}

	if *putResult2.Status != toolcalls.ToolCallStatusSuccess {
		t.Errorf("Expected status to be updated to success, got %s", *putResult2.Status)
	}

	wrongETag := serviceinfra.ETag("wrong-etag")
	accessConditions.IfMatch = &wrongETag

	_, err = store.Put(ctx, updatedToolCall, accessConditions)
	serviceError, ok := err.(*serviceinfra.ServiceError)
	if !ok {
		t.Fatalf("Expected ServiceError, got %T", err)
	}

	if serviceError.StatusCode != 412 {
		t.Errorf("Expected status code 412, got %d", serviceError.StatusCode)
	}
}

func TestInMemoryToolCallStore_Put_AccessConditions_IfNoneMatch(t *testing.T) {
	store := NewInMemoryToolCallStore(ctx)
	ctx := context.Background()

	originalToolCall := &toolcalls.ToolCall{
		Tenant:     serviceinfra.Ptr("test-tenant"),
		ToolName:   serviceinfra.Ptr("test-tool"),
		ToolCallId: serviceinfra.Ptr("test-id"),
		Status:     serviceinfra.Ptr(toolcalls.ToolCallStatusRunning),
	}

	_, err := store.Put(ctx, originalToolCall, nil)
	if err != nil {
		t.Fatalf("First put failed: %v", err)
	}

	accessConditions := &toolcalls.AccessConditions{
		IfNoneMatch: serviceinfra.Ptr(serviceinfra.ETag("*")),
	}

	_, err = store.Put(ctx, originalToolCall, accessConditions)
	serviceError, ok := err.(*serviceinfra.ServiceError)
	if !ok {
		t.Fatalf("Expected ServiceError, got %T", err)
	}

	if serviceError.StatusCode != 412 {
		t.Errorf("Expected status code 412, got %d", serviceError.StatusCode)
	}
}

func TestInMemoryToolCallStore_Get_AccessConditions_IfMatch(t *testing.T) {
	store := NewInMemoryToolCallStore(ctx)
	ctx := context.Background()

	originalToolCall := &toolcalls.ToolCall{
		Tenant:     serviceinfra.Ptr("test-tenant"),
		ToolName:   serviceinfra.Ptr("test-tool"),
		ToolCallId: serviceinfra.Ptr("test-id"),
		Status:     serviceinfra.Ptr(toolcalls.ToolCallStatusRunning),
	}

	putResult, err := store.Put(ctx, originalToolCall, nil)
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	getToolCall := &toolcalls.ToolCall{
		Tenant:     serviceinfra.Ptr("test-tenant"),
		ToolName:   serviceinfra.Ptr("test-tool"),
		ToolCallId: serviceinfra.Ptr("test-id"),
	}

	accessConditions := &toolcalls.AccessConditions{
		IfMatch: putResult.ETag,
	}

	getResult, err := store.Get(ctx, getToolCall, accessConditions)
	if err != nil {
		t.Fatalf("Get with correct ETag failed: %v", err)
	}

	if *getResult.ToolName != *originalToolCall.ToolName {
		t.Errorf("Expected tool call to be returned")
	}

	wrongETag := serviceinfra.ETag("wrong-etag")
	accessConditions.IfMatch = &wrongETag

	_, err = store.Get(ctx, getToolCall, accessConditions)
	serviceError, ok := err.(*serviceinfra.ServiceError)
	if !ok {
		t.Fatalf("Expected ServiceError, got %T", err)
	}

	if serviceError.StatusCode != 412 {
		t.Errorf("Expected status code 412, got %d", serviceError.StatusCode)
	}
}

func TestInMemoryToolCallStore_Get_AccessConditions_IfNoneMatch(t *testing.T) {
	store := NewInMemoryToolCallStore(ctx)
	ctx := context.Background()

	originalToolCall := &toolcalls.ToolCall{
		Tenant:     serviceinfra.Ptr("test-tenant"),
		ToolName:   serviceinfra.Ptr("test-tool"),
		ToolCallId: serviceinfra.Ptr("test-id"),
		Status:     serviceinfra.Ptr(toolcalls.ToolCallStatusRunning),
	}

	putResult, err := store.Put(ctx, originalToolCall, nil)
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	getToolCall := &toolcalls.ToolCall{
		Tenant:     serviceinfra.Ptr("test-tenant"),
		ToolName:   serviceinfra.Ptr("test-tool"),
		ToolCallId: serviceinfra.Ptr("test-id"),
	}

	accessConditions := &toolcalls.AccessConditions{
		IfNoneMatch: putResult.ETag,
	}

	_, err = store.Get(ctx, getToolCall, accessConditions)
	serviceError, ok := err.(*serviceinfra.ServiceError)
	if !ok {
		t.Fatalf("Expected ServiceError, got %T", err)
	}

	if serviceError.StatusCode != 304 {
		t.Errorf("Expected status code 304, got %d", serviceError.StatusCode)
	}
}

func TestInMemoryToolCallStore_Delete(t *testing.T) {
	store := NewInMemoryToolCallStore(ctx)
	ctx := context.Background()

	originalToolCall := &toolcalls.ToolCall{
		Tenant:     serviceinfra.Ptr("test-tenant"),
		ToolName:   serviceinfra.Ptr("test-tool"),
		ToolCallId: serviceinfra.Ptr("test-id"),
		Status:     serviceinfra.Ptr(toolcalls.ToolCallStatusRunning),
	}

	_, err := store.Put(ctx, originalToolCall, nil)
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	err = store.Delete(ctx, originalToolCall, nil)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, err = store.Get(ctx, originalToolCall, nil)
	serviceError, ok := err.(*serviceinfra.ServiceError)
	if !ok {
		t.Fatalf("Expected ServiceError after delete, got %T", err)
	}

	if serviceError.StatusCode != 404 {
		t.Errorf("Expected status code 404 after delete, got %d", serviceError.StatusCode)
	}
}

func TestInMemoryToolCallStore_Delete_AccessConditions(t *testing.T) {
	store := NewInMemoryToolCallStore(ctx)
	ctx := context.Background()

	originalToolCall := &toolcalls.ToolCall{
		Tenant:     serviceinfra.Ptr("test-tenant"),
		ToolName:   serviceinfra.Ptr("test-tool"),
		ToolCallId: serviceinfra.Ptr("test-id"),
		Status:     serviceinfra.Ptr(toolcalls.ToolCallStatusRunning),
	}

	putResult, err := store.Put(ctx, originalToolCall, nil)
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	wrongETag := serviceinfra.ETag("wrong-etag")
	accessConditions := &toolcalls.AccessConditions{
		IfMatch: &wrongETag,
	}

	err = store.Delete(ctx, originalToolCall, accessConditions)
	serviceError, ok := err.(*serviceinfra.ServiceError)
	if !ok {
		t.Fatalf("Expected ServiceError, got %T", err)
	}

	if serviceError.StatusCode != 412 {
		t.Errorf("Expected status code 412, got %d", serviceError.StatusCode)
	}

	accessConditions.IfMatch = putResult.ETag
	err = store.Delete(ctx, originalToolCall, accessConditions)
	if err != nil {
		t.Fatalf("Delete with correct ETag failed: %v", err)
	}
}

func TestInMemoryToolCallStore_Delete_NonExistent(t *testing.T) {
	store := NewInMemoryToolCallStore(ctx)
	ctx := context.Background()

	toolCall := &toolcalls.ToolCall{
		Tenant:     serviceinfra.Ptr("test-tenant"),
		ToolName:   serviceinfra.Ptr("test-tool"),
		ToolCallId: serviceinfra.Ptr("test-id"),
	}

	err := store.Delete(ctx, toolCall, nil)
	if err != nil {
		t.Fatalf("Delete of non-existent item should not fail, got: %v", err)
	}
}

func TestInMemoryToolCallStore_TenantIsolation(t *testing.T) {
	store := NewInMemoryToolCallStore(ctx)
	ctx := context.Background()

	tenant1 := "test-tenant"
	tenant2 := "different-tenant"

	toolCall := &toolcalls.ToolCall{
		Tenant:     &tenant1,
		ToolName:   serviceinfra.Ptr("test-tool"),
		ToolCallId: serviceinfra.Ptr("test-id"),
		Status:     serviceinfra.Ptr(toolcalls.ToolCallStatusRunning),
	}

	_, err := store.Put(ctx, toolCall, nil)
	if err != nil {
		t.Fatalf("Put to tenant1 failed: %v", err)
	}

	toolCall.Tenant = &tenant2
	_, err = store.Get(ctx, toolCall, nil)
	serviceError, ok := err.(*serviceinfra.ServiceError)
	if !ok {
		t.Fatalf("Expected ServiceError, got %T", err)
	}

	if serviceError.StatusCode != 404 {
		t.Errorf("Expected status code 404 for different tenant, got %d", serviceError.StatusCode)
	}

	toolCall.Tenant = &tenant1
	_, err = store.Get(ctx, toolCall, nil)
	if err != nil {
		t.Fatalf("Get from tenant1 should still work: %v", err)
	}
}

func TestInMemoryToolCallStore_DataIsolation(t *testing.T) {
	store := NewInMemoryToolCallStore(ctx)
	ctx := context.Background()

	originalToolCall := &toolcalls.ToolCall{
		Tenant:     serviceinfra.Ptr("test-tenant"),
		ToolName:   serviceinfra.Ptr("test-tool"),
		ToolCallId: serviceinfra.Ptr("test-id"),
		Status:     serviceinfra.Ptr(toolcalls.ToolCallStatusRunning),
		Request:    jsontext.Value(`{"param":"original"}`),
	}

	putResult, err := store.Put(ctx, originalToolCall, nil)
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	*originalToolCall.Status = toolcalls.ToolCallStatusSuccess
	originalToolCall.Request = jsontext.Value(`{"param":"modified"}`)

	getToolCall := &toolcalls.ToolCall{
		Tenant:     serviceinfra.Ptr("test-tenant"),
		ToolName:   serviceinfra.Ptr("test-tool"),
		ToolCallId: serviceinfra.Ptr("test-id"),
	}

	getResult, err := store.Get(ctx, getToolCall, nil)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if *getResult.Status != toolcalls.ToolCallStatusRunning {
		t.Errorf("Expected status to remain 'running', got %s", *getResult.Status)
	}

	if string(getResult.Request) != `{"param":"original"}` {
		t.Errorf("Expected request to remain original, got %s", string(getResult.Request))
	}

	*getResult.Status = toolcalls.ToolCallStatusFailed
	getResult.Request = jsontext.Value(`{"param":"get-modified"}`)

	getResult2, err := store.Get(ctx, getToolCall, nil)
	if err != nil {
		t.Fatalf("Second get failed: %v", err)
	}

	if *getResult2.Status != toolcalls.ToolCallStatusRunning {
		t.Errorf("Expected stored status to remain 'running', got %s", *getResult2.Status)
	}

	if *putResult.ETag != *getResult.ETag {
		t.Errorf("Expected same ETags for put and get results, got put: %s, get: %s", *putResult.ETag, *getResult.ETag)
	}
}

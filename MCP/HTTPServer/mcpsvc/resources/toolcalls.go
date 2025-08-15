package resources

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"io"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/blob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/bloberror"
	"github.com/JeffreyRichter/mcpsvc/mcp/toolcalls"
	si "github.com/JeffreyRichter/serviceinfra"
)

// Resource type & operations pattern:
// 1. Define Resource Type struct & define api-agnostic resource type operations on this type
// 2. Construct global singleton instance/variable of the Resource Type used to call #1 methods
// 3. Define api-version Resource Type Operations struct with field of #1 & define api-specific HTTP operations on this type
// 4. Construct global singleton instance/variable of #3 wrapping #2 & set api-version routes to these var/methods

// ToolCalls maintains the state required to manage all operations for the users resource type.
type ToolCalls struct {
	client *azblob.Client // Client to access the Azure Blob Storage service
}

// ToolCaallOps is a singleton for the entire service
var ToolCallOps = &ToolCalls{
	client: func() *azblob.Client {
		cred := must(azidentity.NewDefaultAzureCredential(nil))
		serviceURL := fmt.Sprintf("https://%s.blob.core.windows.net/", "jeffreymcp") // Replace with your storage account name
		client := must(azblob.NewClient(serviceURL, cred, nil))
		return client
	}(),
}

func (tc *ToolCalls) blobName(toolName, toolCallId string) string { return toolName + "/" + toolCallId }

func (tc *ToolCalls) accessConditions(ac *toolcalls.AccessConditions) *azblob.AccessConditions {
	return &azblob.AccessConditions{
		ModifiedAccessConditions: &blob.ModifiedAccessConditions{IfMatch: (*azcore.ETag)(ac.IfMatch), IfNoneMatch: (*azcore.ETag)(ac.IfNoneMatch)},
	}
}

func (tc *ToolCalls) Get(ctx context.Context, tenant string, toolCall *toolcalls.ToolCall, accessConditions *toolcalls.AccessConditions) (*toolcalls.ToolCall, error) {
	// Get the tool call by tenant, tool name and tool call id
	response, err := tc.client.DownloadStream(ctx, tenant, tc.blobName(*toolCall.ToolName, *toolCall.ToolCallId),
		&azblob.DownloadStreamOptions{AccessConditions: tc.accessConditions(accessConditions)})
	if err != nil {
		return toolCall, err // Blob not found; return a brand new one
	}

	// Read the blob contents into a buffer and then deserialize it into a ToolCall struct
	defer response.Body.Close()
	const MaxToolCallResourceSizeInBytes = 4 * 1024 * 1024 // 4MB
	buffer, err := io.ReadAll(io.LimitReader(response.Body, MaxToolCallResourceSizeInBytes))
	if err != nil {
		return nil, err // panic?
	}
	if err := json.Unmarshal(buffer, &toolCall); err != nil {
		return nil, err // panic?
	}
	toolCall.ETag = (*si.ETag)(response.ETag) // Set the ETag from the response
	return toolCall, nil
}

func (tc *ToolCalls) Put(ctx context.Context, tenant string, toolCall *toolcalls.ToolCall, accessConditions *toolcalls.AccessConditions) (*toolcalls.ToolCall, error) {
	blobName := tc.blobName(*toolCall.ToolName, *toolCall.ToolCallId)
	buffer := must(json.Marshal(toolCall))
	for {
		// Attempt to upload the Tool Call blob
		response, err := tc.client.UploadBuffer(ctx, tenant, blobName, buffer, &azblob.UploadBufferOptions{AccessConditions: tc.accessConditions(accessConditions)})
		if err == nil { // Successfully uploaded the Tool Call blob
			toolCall.ETag = (*si.ETag)(response.ETag) // Update the passed-in ToolCall's ETag from the response ETag
			return toolCall, nil
		}

		// An error occured; if not related to missing container, return the error
		if !bloberror.HasCode(err, bloberror.ContainerNotFound) {
			return nil, err
		}
		if _, err := tc.client.CreateContainer(ctx, tenant, nil); err != nil { // Attempt to create the missing tenant container
			return nil, err // Failed to create container, return
		}
		// Successfully created the container, retry uploading the Tool Call blob
	}
}

func (tc *ToolCalls) Delete(ctx context.Context, tenant string, toolCall *toolcalls.ToolCall, accessConditions *toolcalls.AccessConditions) error {
	_, err := tc.client.DeleteBlob(ctx, tenant, tc.blobName(*toolCall.ToolName, *toolCall.ToolCallId), &azblob.DeleteBlobOptions{AccessConditions: tc.accessConditions(accessConditions)})
	return err // panic?
}

func must[T any](t T, err error) T {
	if err != nil {
		panic(err)
	}
	return t
}

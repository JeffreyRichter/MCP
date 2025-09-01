package resources

import (
	"context"
	"encoding/json/v2"
	"io"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/blob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/bloberror"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/blockblob"
	"github.com/JeffreyRichter/mcpsvc/mcp/toolcalls"
	"github.com/JeffreyRichter/serviceinfra"
)

// AzureBlobToolCallStore maintains the state required to manage all operations for the users resource type.
type AzureBlobToolCallStore struct {
	client *azblob.Client // Client to access the Azure Blob Storage service
}

func (*AzureBlobToolCallStore) blobName(toolName, toolCallId string) string {
	return toolName + "/" + toolCallId
}

func (*AzureBlobToolCallStore) accessConditions(ac *toolcalls.AccessConditions) *azblob.AccessConditions {
	return &azblob.AccessConditions{
		ModifiedAccessConditions: &blob.ModifiedAccessConditions{
			IfMatch:     (*azcore.ETag)(ac.IfMatch),
			IfNoneMatch: (*azcore.ETag)(ac.IfNoneMatch),
		},
	}
}

func (ab *AzureBlobToolCallStore) Get(ctx context.Context, toolCall *toolcalls.ToolCall, accessConditions *toolcalls.AccessConditions) (*toolcalls.ToolCall, error) {
	// Get the tool call by tenant, tool name and tool call id
	response, err := ab.client.DownloadStream(ctx, *toolCall.Tenant, ab.blobName(*toolCall.ToolName, *toolCall.ToolCallId),
		&azblob.DownloadStreamOptions{AccessConditions: ab.accessConditions(accessConditions)})
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
	toolCall.ETag = (*serviceinfra.ETag)(response.ETag) // Set the ETag from the response
	return toolCall, nil
}

func (ab *AzureBlobToolCallStore) Put(ctx context.Context, toolCall *toolcalls.ToolCall, accessConditions *toolcalls.AccessConditions) (*toolcalls.ToolCall, error) {
	blobName := ab.blobName(*toolCall.ToolName, *toolCall.ToolCallId)
	buffer := must(json.Marshal(toolCall))
	tenant := *toolCall.Tenant
	for {
		// Attempt to upload the Tool Call blob
		response, err := ab.client.UploadBuffer(ctx, tenant, blobName, buffer, &azblob.UploadBufferOptions{AccessConditions: ab.accessConditions(accessConditions)})
		if err == nil { // Successfully uploaded the Tool Call blob
			toolCall.ETag = (*serviceinfra.ETag)(response.ETag) // Update the passed-in ToolCall's ETag from the response ETag
			blockClient := ab.client.ServiceClient().NewContainerClient(tenant).NewBlockBlobClient(blobName)
			// TODO: this error should be logged but isn't cause for panic and shouldn't be sent to the client
			_, _ = blockClient.SetExpiry(ctx, blockblob.ExpiryTypeRelativeToNow(24*time.Hour), nil)
			return toolCall, nil
		}

		// An error occured; if not related to missing container, return the error
		if !bloberror.HasCode(err, bloberror.ContainerNotFound) {
			return nil, err
		}
		if _, err := ab.client.CreateContainer(ctx, tenant, nil); err != nil { // Attempt to create the missing tenant container
			return nil, err // Failed to create container, return
		}
		// Successfully created the container, retry uploading the Tool Call blob
	}
}

func (ab *AzureBlobToolCallStore) Delete(ctx context.Context, toolCall *toolcalls.ToolCall, accessConditions *toolcalls.AccessConditions) error {
	_, err := ab.client.DeleteBlob(ctx, *toolCall.Tenant, ab.blobName(*toolCall.ToolName, *toolCall.ToolCallId), &azblob.DeleteBlobOptions{AccessConditions: ab.accessConditions(accessConditions)})
	return err // panic?
}

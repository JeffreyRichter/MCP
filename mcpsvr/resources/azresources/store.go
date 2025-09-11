package azresources

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
	"github.com/JeffreyRichter/internal/aids"
	"github.com/JeffreyRichter/mcpsvr/mcp/toolcall"
	"github.com/JeffreyRichter/svrcore"
)

// Resource type & operations pattern:
// 1. Define Resource Type struct & define api-agnostic resource type operations on this type
// 2. Construct global singleton instance/variable of the Resource Type used to call #1 methods
// 3. Define api-version Resource Type Operations struct with field of #1 & define api-specific HTTP operations on this type
// 4. Construct global singleton instance/variable of #3 wrapping #2 & set api-version routes to these var/methods

// toolCallStore maintains the state required to manage all operations for the users resource type.
type toolCallStore struct {
	client *azblob.Client // Client to access the Azure Blob Storage service
}

// NewToolCallStore creates a new localToolCallStore; ctx is used to cancel the expiry goroutine
func NewToolCallStore(client *azblob.Client) toolcall.Store {
	return &toolCallStore{client: client}
}

func (tcs *toolCallStore) toBlobInfo(tc *toolcall.ToolCall) (containerName, blobName string) {
	return *tc.Tenant, *tc.ToolName + "/" + *tc.ID
}

/*func (tcs *toolCallStore) fromBlobUrl(blobUrl string) (tenant, toolName, toolCallID string) {
	parts, err := azblob.ParseURL(blobUrl)
	if aids.IsError(err){
		return "", "", ""
	}
	segments := strings.Split(parts.BlobName, "/")
	if segments == nil || len(segments) != 2 {
		return "", "", ""
	}
	return parts.ContainerName, segments[0], segments[1]
}*/

func (*toolCallStore) accessConditions(ac svrcore.AccessConditions) *azblob.AccessConditions {
	return &azblob.AccessConditions{
		ModifiedAccessConditions: &blob.ModifiedAccessConditions{
			IfMatch:     (*azcore.ETag)(ac.IfMatch),
			IfNoneMatch: (*azcore.ETag)(ac.IfNoneMatch)},
	}
}

func (tcs *toolCallStore) Get(ctx context.Context, tc *toolcall.ToolCall, ac svrcore.AccessConditions) error {
	// Get the tool call by tenant, tool name and tool call id
	containerName, blobName := tcs.toBlobInfo(tc)
	response, err := tcs.client.DownloadStream(ctx, containerName, blobName,
		&azblob.DownloadStreamOptions{AccessConditions: tcs.accessConditions(ac)})
	if aids.IsError(err) {
		return err // Blob not found or precondition failed
	}

	// Read the blob contents into a buffer and then deserialize it into a ToolCall struct
	defer response.Body.Close()
	const MaxToolCallResourceSizeInBytes = 4 * 1024 * 1024 // 4MB
	buffer, err := io.ReadAll(io.LimitReader(response.Body, MaxToolCallResourceSizeInBytes))
	if aids.IsError(err) {
		return err
	}
	if err := json.Unmarshal(buffer, tc); aids.IsError(err) {
		return err
	}
	tc.ETag = (*svrcore.ETag)(response.ETag) // Set the ETag from the response
	return nil
}

func (tcs *toolCallStore) Put(ctx context.Context, tc *toolcall.ToolCall, ac svrcore.AccessConditions) error {
	buffer, err := json.Marshal(tc)
	if aids.IsError(err) {
		return err
	}
	containerName, blobName := tcs.toBlobInfo(tc)
	for {
		// Attempt to upload the Tool Call blob
		response, err := tcs.client.UploadBuffer(ctx, containerName, blobName, buffer,
			&azblob.UploadBufferOptions{AccessConditions: tcs.accessConditions(ac)})
		if !aids.IsError(err) { // Successfully uploaded the Tool Call blob
			tc.ETag = (*svrcore.ETag)(response.ETag) // Update the passed-in ToolCall's ETag from the response ETag
			blockClient := tcs.client.ServiceClient().NewContainerClient(containerName).NewBlockBlobClient(blobName)
			// TODO: Log any error from SetExpiry
			_, _ = blockClient.SetExpiry(ctx, blockblob.ExpiryTypeRelativeToNow(24*time.Hour), nil)
			return nil
		}

		// An error occured; if not related to missing container, return the error
		if !bloberror.HasCode(err, bloberror.ContainerNotFound) {
			return err
		}
		if _, err := tcs.client.CreateContainer(ctx, containerName, nil); aids.IsError(err) { // Attempt to create the missing tenant container
			return err // Failed to create container, return
		}
		// Successfully created the container, retry uploading the Tool Call blob
	}
}

func (tcs *toolCallStore) Delete(ctx context.Context, tc *toolcall.ToolCall, ac svrcore.AccessConditions) error {
	containerName, blobName := tcs.toBlobInfo(tc)
	_, err := tcs.client.DeleteBlob(ctx, containerName, blobName, &azblob.DeleteBlobOptions{AccessConditions: tcs.accessConditions(ac)})
	return err
}

// Blobs are cheap, fast (below link), simple, and offer features we need (like expiry)
// https://learn.microsoft.com/en-us/azure/architecture/best-practices/data-partitioning-strategies

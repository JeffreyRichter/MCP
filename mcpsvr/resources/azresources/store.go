package azresources

import (
	"context"
	"errors"
	"io"
	"net/http"
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

// store maintains the state required to manage all operations for the users resource type.
type store struct {
	client *azblob.Client // Client to access the Azure Blob Storage service
}

// NewToolCallStore creates a new [toolcall.Store]
func NewToolCallStore(client *azblob.Client) toolcall.Store {
	return &store{client: client}
}

// toBlobInfo returns the container and blob name for the specified tool call
func (s *store) toBlobInfo(tc *toolcall.ToolCall) (containerName, blobName string) {
	return *tc.Tenant, *tc.ToolName + "/" + *tc.ID
}

// accessConditions converts svrcore.AccessConditions to azblob.AccessConditions
func (*store) accessConditions(ac svrcore.AccessConditions) *azblob.AccessConditions {
	return &azblob.AccessConditions{
		ModifiedAccessConditions: &blob.ModifiedAccessConditions{
			IfMatch:     (*azcore.ETag)(ac.IfMatch),
			IfNoneMatch: (*azcore.ETag)(ac.IfNoneMatch)},
	}
}

// Get retrieves the specified tool call from storage into the passed-in ToolCall struct or a
// [svrcore.ServerError] if an error occurs.
func (s *store) Get(ctx context.Context, tc *toolcall.ToolCall, ac svrcore.AccessConditions) *svrcore.ServerError {
	// Get the tool call by tenant, tool name and tool call id
	containerName, blobName := s.toBlobInfo(tc)
	response, err := s.client.DownloadStream(ctx, containerName, blobName,
		&azblob.DownloadStreamOptions{AccessConditions: s.accessConditions(ac)})
	if aids.IsError(err) {
		if rerr := (*azcore.ResponseError)(nil); errors.As(err, &rerr) { // Blob not found or precondition failed
			return svrcore.NewServerError(rerr.StatusCode, "", "Failed to get tool call")
		}
		return svrcore.NewServerError(http.StatusInternalServerError, "InternalServerError", "failed to get tool call")
	}

	// Read the blob contents into a buffer and then deserialize it into a ToolCall struct
	defer response.Body.Close()
	const MaxToolCallResourceSizeInBytes = 4 * 1024 * 1024 // 4MB
	buffer, err := io.ReadAll(io.LimitReader(response.Body, MaxToolCallResourceSizeInBytes))
	if aids.IsError(err) {
		return svrcore.NewServerError(http.StatusInternalServerError, "InternalServerError", "failed to read tool call")
	}
	aids.MustUnmarshal[toolcall.ToolCall](buffer)
	tc.ETag = (*svrcore.ETag)(response.ETag) // Set the ETag from the response
	return nil
}

// Put creates or updates the specified tool call in storage from the passed-in ToolCall struct.
// On success, the ToolCall.ETag field is updated from the response ETag. Returns a
// [svrcore.ServerError] if an error occurs.
func (s *store) Put(ctx context.Context, tc *toolcall.ToolCall, ac svrcore.AccessConditions) *svrcore.ServerError {
	buffer := aids.MustMarshal(tc)
	containerName, blobName := s.toBlobInfo(tc)
	for {
		// Attempt to upload the Tool Call blob
		response, err := s.client.UploadBuffer(ctx, containerName, blobName, buffer,
			&azblob.UploadBufferOptions{AccessConditions: s.accessConditions(ac)})
		if !aids.IsError(err) { // Successfully uploaded the Tool Call blob
			tc.ETag = (*svrcore.ETag)(response.ETag) // Update the passed-in ToolCall's ETag from the response ETag
			blockClient := s.client.ServiceClient().NewContainerClient(containerName).NewBlockBlobClient(blobName)
			// TODO: Log any error from SetExpiry
			_, _ = blockClient.SetExpiry(ctx, blockblob.ExpiryTypeRelativeToNow(24*time.Hour), nil)
			return nil
		}

		// An error occured; if not related to missing container, return the error
		if !bloberror.HasCode(err, bloberror.ContainerNotFound) {
			return svrcore.NewServerError(http.StatusInternalServerError, "", "failed to upload tool call")
		}
		if _, err := s.client.CreateContainer(ctx, containerName, nil); aids.IsError(err) { // Attempt to create the missing tenant container
			return svrcore.NewServerError(http.StatusInternalServerError, "", "failed to create container")
		}
		// Successfully created the container, retry uploading the Tool Call blob
	}
}

// Delete deletes the specified tool call from storage or returns a [svrcore.ServerError] if an error occurs.
func (s *store) Delete(ctx context.Context, tc *toolcall.ToolCall, ac svrcore.AccessConditions) *svrcore.ServerError {
	containerName, blobName := s.toBlobInfo(tc)
	_, err := s.client.DeleteBlob(ctx, containerName, blobName, &azblob.DeleteBlobOptions{AccessConditions: s.accessConditions(ac)})
	if aids.IsError(err) {
		return svrcore.NewServerError(http.StatusInternalServerError, "", "failed to delete tool call")
	}
	return nil
}

// Blobs are cheap, fast (below link), simple, and offer features we need (like expiry)
// https://learn.microsoft.com/en-us/azure/architecture/best-practices/data-partitioning-strategies

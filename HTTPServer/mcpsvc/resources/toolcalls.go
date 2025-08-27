package resources

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"io"
	"strings"

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

// ToolCallOperations maintains the state required to manage all operations for the users resource type.
type ToolCallOperations struct {
	client *azblob.Client // Client to access the Azure Blob Storage service
}

// ToolCallOps is a singleton for the entire service
var ToolCallOps = &ToolCallOperations{
	client: func() *azblob.Client {
		cred := must(azidentity.NewDefaultAzureCredential(nil))
		serviceURL := fmt.Sprintf("https://%s.blob.core.windows.net/", "jeffreymcp") // Replace with your storage account name
		client := must(azblob.NewClient(serviceURL, cred, nil))
		return client
	}(),
}

func (tco *ToolCallOperations) toBlobInfo(tc *toolcalls.ToolCall) (containerName, blobName string) {
	return *tc.Tenant, *tc.ToolName + "/" + *tc.ToolCallId
}

func (tco *ToolCallOperations) fromBlobUrl(blobUrl string) (tenant, toolName, toolCallID string) {
	parts, err := azblob.ParseURL(blobUrl)
	if err != nil {
		return "", "", ""
	}
	segments := strings.Split(parts.BlobName, "/")
	if segments == nil || len(segments) != 2 {
		return "", "", ""
	}
	return parts.ContainerName, segments[0], segments[1]
}

func (tco *ToolCallOperations) accessConditions(ac *toolcalls.AccessConditions) *azblob.AccessConditions {
	return &azblob.AccessConditions{
		ModifiedAccessConditions: &blob.ModifiedAccessConditions{IfMatch: (*azcore.ETag)(ac.IfMatch), IfNoneMatch: (*azcore.ETag)(ac.IfNoneMatch)},
	}
}

func (tco *ToolCallOperations) Get(ctx context.Context, tc *toolcalls.ToolCall, accessConditions *toolcalls.AccessConditions) (*toolcalls.ToolCall, error) {
	// Get the tool call by tenant, tool name and tool call id
	containerName, blobName := tco.toBlobInfo(tc)
	response, err := tco.client.DownloadStream(ctx, containerName, blobName,
		&azblob.DownloadStreamOptions{AccessConditions: tco.accessConditions(accessConditions)})
	if err != nil {
		return tc, err // Blob not found; return a brand new one
	}

	// Read the blob contents into a buffer and then deserialize it into a ToolCall struct
	defer response.Body.Close()
	const MaxToolCallResourceSizeInBytes = 4 * 1024 * 1024 // 4MB
	buffer, err := io.ReadAll(io.LimitReader(response.Body, MaxToolCallResourceSizeInBytes))
	if err != nil {
		return nil, err // panic?
	}
	if err := json.Unmarshal(buffer, tc); err != nil {
		return nil, err // panic?
	}
	tc.ETag = (*si.ETag)(response.ETag) // Set the ETag from the response
	return tc, nil
}

func (tco *ToolCallOperations) Put(ctx context.Context, tc *toolcalls.ToolCall, accessConditions *toolcalls.AccessConditions) (*toolcalls.ToolCall, error) {
	buffer := must(json.Marshal(tc))
	containerName, blobName := tco.toBlobInfo(tc)
	for {
		// Attempt to upload the Tool Call blob
		response, err := tco.client.UploadBuffer(ctx, containerName, blobName, buffer, &azblob.UploadBufferOptions{AccessConditions: tco.accessConditions(accessConditions)})
		if err == nil { // Successfully uploaded the Tool Call blob
			tc.ETag = (*si.ETag)(response.ETag) // Update the passed-in ToolCall's ETag from the response ETag
			return tc, nil
		}

		// An error occured; if not related to missing container, return the error
		if !bloberror.HasCode(err, bloberror.ContainerNotFound) {
			return nil, err
		}
		if _, err := tco.client.CreateContainer(ctx, containerName, nil); err != nil { // Attempt to create the missing tenant container
			return nil, err // Failed to create container, return
		}
		// Successfully created the container, retry uploading the Tool Call blob
	}
}

func (tco *ToolCallOperations) Delete(ctx context.Context, tc *toolcalls.ToolCall, accessConditions *toolcalls.AccessConditions) error {
	containerName, blobName := tco.toBlobInfo(tc)
	_, err := tco.client.DeleteBlob(ctx, containerName, blobName, &azblob.DeleteBlobOptions{AccessConditions: tco.accessConditions(accessConditions)})
	return err // panic?
}

func must[T any](t T, err error) T {
	if err != nil {
		panic(err)
	}
	return t
}

// Blobs are cheap, fast (below link), simple, and offer features we need (like expiry)
// https://learn.microsoft.com/en-us/azure/architecture/best-practices/data-partitioning-strategies

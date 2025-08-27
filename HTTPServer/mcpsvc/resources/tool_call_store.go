package resources

import (
	"context"
	"fmt"
	"net/url"
	"sync"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/JeffreyRichter/mcpsvc/config"
	"github.com/JeffreyRichter/mcpsvc/mcp/toolcalls"
)

// Resource type & operations pattern:
// 1. Define Resource Type struct & define api-agnostic resource type operations on this type
// 2. Construct global singleton instance/variable of the Resource Type used to call #1 methods
// 3. Define api-version Resource Type Operations struct with field of #1 & define api-specific HTTP operations on this type
// 4. Construct global singleton instance/variable of #3 wrapping #2 & set api-version routes to these var/methods

// ToolCallStore manages persistent storage of ToolCalls
type ToolCallStore interface {
	Get(ctx context.Context, toolCall *toolcalls.ToolCall, accessConditions *toolcalls.AccessConditions) (*toolcalls.ToolCall, error)
	Put(ctx context.Context, toolCall *toolcalls.ToolCall, accessConditions *toolcalls.AccessConditions) (*toolcalls.ToolCall, error)
	Delete(ctx context.Context, toolCall *toolcalls.ToolCall, accessConditions *toolcalls.AccessConditions) error
}

// GetToolCallStore returns a singleton ToolCallStore. It's an exported variable so offline tests can replace the production default with a mock.
var GetToolCallStore = sync.OnceValue(func() ToolCallStore {
	if config.Get().Local {
		return NewInMemoryToolCallStore()
	}
	return &AzureBlobToolCallStore{
		client: func() *azblob.Client {
			cfg := config.Get()
			if cfg.AzuriteAccount != "" && cfg.AzuriteKey != "" {
				fmt.Println("Using Azurite for tool call storage")
				cred := must(azblob.NewSharedKeyCredential(cfg.AzuriteAccount, cfg.AzuriteKey))
				return must(azblob.NewClientWithSharedKeyCredential(cfg.AzureStorageURL, cred, nil))
			}
			cred := must(azidentity.NewDefaultAzureCredential(nil))
			serviceURL := must(url.Parse(config.Get().AzureStorageURL)).String()
			client := must(azblob.NewClient(serviceURL, cred, nil))
			return client
		}(),
	}
})

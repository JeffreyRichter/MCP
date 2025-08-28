module github.com/JeffreyRichter/mcpsvc

go 1.25.0

replace github.com/JeffreyRichter/svrcore => ../svrcore

require (
	github.com/Azure/azure-sdk-for-go/sdk/azcore v1.18.2
	github.com/Azure/azure-sdk-for-go/sdk/azidentity v1.11.0
	github.com/Azure/azure-sdk-for-go/sdk/storage/azblob v1.6.2
	github.com/Azure/azure-sdk-for-go/sdk/storage/azqueue v1.0.1
	github.com/JeffreyRichter/svrcore v0.0.0-00010101000000-000000000000
	github.com/caarlos0/env/v11 v11.3.1
	github.com/stretchr/testify v1.11.1
)

require (
	github.com/Azure/azure-sdk-for-go/sdk/internal v1.11.2 // indirect
	github.com/AzureAD/microsoft-authentication-library-for-go v1.4.2 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/golang-jwt/jwt/v5 v5.3.0 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/kylelemons/godebug v1.1.0 // indirect
	github.com/pkg/browser v0.0.0-20240102092130-5ac0b6a4141c // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	golang.org/x/crypto v0.40.0 // indirect
	golang.org/x/net v0.42.0 // indirect
	golang.org/x/sys v0.35.0 // indirect
	golang.org/x/text v0.27.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

package config

import "testing"

func TestConfig_validate(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
	}{
		{
			name:    "zero",
			config:  Config{},
			wantErr: true,
		},
		{
			name: "URLs only",
			config: Config{
				AzureBlobURL:  "https://example.blob.core.windows.net",
				AzureQueueURL: "https://example.queue.core.windows.net",
			},
		},
		{
			name: "URLs and Azurite config",
			config: Config{
				AzureBlobURL:   "http://azurite:10000/devstoreaccount1",
				AzureQueueURL:  "http://example.queue.core.windows.net",
				AzuriteAccount: "devstoreaccount1",
				AzuriteKey:     "some-key",
			},
		},
		{
			name: "Azurite config without URLs",
			config: Config{
				AzuriteAccount: "devstoreaccount1",
				AzuriteKey:     "some-key",
			},
			wantErr: true,
		},
		{
			name: "Azurite account without key",
			config: Config{
				AzureBlobURL:   "http://azurite:10000/devstoreaccount1",
				AzuriteAccount: "devstoreaccount1",
			},
			wantErr: true,
		},
		{
			name: "Azurite key without account",
			config: Config{
				AzureBlobURL: "http://azurite:10000/devstoreaccount1",
				AzuriteKey:   "some-key",
			},
			wantErr: true,
		},
		{
			name: "empty strings considered unspecified",
			config: Config{
				AzureBlobURL:   "http://azurite:10000/devstoreaccount1",
				AzureQueueURL:  "http://example.queue.core.windows.net",
				AzuriteAccount: "",
				AzuriteKey:     "",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.validate()
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error but got none")
					return
				}
			} else if err != nil {
				t.Fatalf("unexpected error = %v", err)
			}
		})
	}
}

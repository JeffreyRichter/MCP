package main

import (
	"os"
	"strings"

	"github.com/JeffreyRichter/internal/aids"
)

type Configuration struct {
	AzureBlobURL   string `env:"AZURE_BLOB_URL"`
	AzureQueueURL  string `env:"AZURE_QUEUE_URL"`
	AzuriteAccount string `env:"AZURITE_ACCOUNT"`
	AzuriteKey     string `env:"AZURITE_KEY"`
	Local          bool   `env:"LOCAL"`
}

func (c *Configuration) Load() {
	b, err := os.ReadFile(".env")
	aids.AssertSuccess(err)

	// read lines froma buffer:
	for _, line := range strings.Split(string(b), "\r\n") {
		tokens := strings.Split(line, "=")
		switch tokens[0] {
		case "AZURE_BLOB_URL":
			c.AzureBlobURL = tokens[1]
		case "AZURE_QUEUE_URL":
			c.AzureQueueURL = tokens[1]
		case "AZURITE_ACCOUNT":
			c.AzuriteAccount = tokens[1]
		case "AZURITE_KEY":
			c.AzuriteKey = tokens[1]
		case "LOCAL":
			c.Local = tokens[1] == "true"
		default:
			panic("unknown env var: " + tokens[0])
		}
	}
}

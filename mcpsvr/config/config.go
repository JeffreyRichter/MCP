package config

import (
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/caarlos0/env/v11"
)

type Config struct {
	AzureBlobURL   string `env:"AZURE_BLOB_URL"`
	AzureQueueURL  string `env:"AZURE_QUEUE_URL"`
	AzuriteAccount string `env:"AZURITE_ACCOUNT"`
	AzuriteKey     string `env:"AZURITE_KEY"`
	Local          bool   `env:"LOCAL"`
}

func (c *Config) validate() error {
	if c.AzureBlobURL == "" && !c.Local {
		return errors.New("no Azure Blob URL specified")
	}
	if c.AzureQueueURL == "" && !c.Local {
		return errors.New("no Azure Queue URL specified")
	}
	if c.AzuriteAccount != "" {
		if c.AzuriteKey == "" {
			return errors.New("no key specified for Azurite account")
		}
	} else if c.AzuriteKey != "" {
		return errors.New("no account specified for Azurite key")
	}
	return nil
}

var Get = sync.OnceValue(func() *Config {
	cfg := &Config{}
	err := env.ParseWithOptions(cfg, env.Options{Prefix: "MCPSVR_"})
	if err == nil {
		err = cfg.validate()
	}
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	return cfg
})

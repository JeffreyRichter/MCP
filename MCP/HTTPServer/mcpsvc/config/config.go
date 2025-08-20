package config

import (
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/caarlos0/env/v11"
)

type Config struct {
	AzureStorageURL string `env:"AZURE_STORAGE_URL"`
	AzuriteAccount  string `env:"AZURITE_ACCOUNT"`
	AzuriteKey      string `env:"AZURITE_KEY"`
}

func (c *Config) validate() error {
	if c.AzureStorageURL == "" {
		return errors.New("no Azure Storage URL specified")
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
	err := env.ParseWithOptions(cfg, env.Options{Prefix: "MCPSVC_"})
	if err == nil {
		err = cfg.validate()
	}
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	return cfg
})

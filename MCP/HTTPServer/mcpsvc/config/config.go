package config

import (
	"fmt"
	"os"
	"sync"

	"github.com/caarlos0/env/v11"
)

type Config struct {
	AzureStorageURL string `env:"AZURE_STORAGE_URL,required"`
}

var Get = sync.OnceValue(func() *Config {
	cfg := &Config{}
	if err := env.ParseWithOptions(cfg, env.Options{Prefix: "MCPSVC_"}); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	return cfg
})

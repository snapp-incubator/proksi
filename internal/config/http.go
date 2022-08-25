package config

import (
	"github.com/knadh/koanf"
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/file"
	"go.uber.org/zap"

	"github.com/anvari1313/proksi/internal/logging"
)

var (
	// k is the global koanf instance. Use "." as the key path delimiter.
	k = koanf.New(".")

	// HTTP is the config for Proksi HTTP
	HTTP *httpConfig
)

// httpConfig represent config of the Proksi HTTP.
type httpConfig struct {
	Bind          string        `koanf:"bind"`
	Elasticsearch Elasticsearch `koanf:"elasticsearch"`
	Upstreams     struct {
		Main httpUpstream `koanf:"main"`
		Test httpUpstream `koanf:"test"`
	} `koanf:"upstreams"`
}

type httpUpstream struct {
	Address string `koanf:"address"`
}

// Load function will load the file located in path and return the parsed config. This function will panic on errors
func Load(path string) *httpConfig {
	// Load YAML config and merge into the previously loaded config.
	err := k.Load(file.Provider(path), yaml.Parser())
	if err != nil {
		logging.L.Fatal("error in loading the config file", zap.Error(err))
	}

	var c httpConfig
	err = k.Unmarshal("", &c)
	if err != nil {
		logging.L.Fatal("error in unmarshalling the config file", zap.Error(err))
	}

	HTTP = &c
	return &c
}

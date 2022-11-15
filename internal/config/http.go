package config

import (
	"github.com/knadh/koanf"
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/providers/structs"
	"go.uber.org/zap"

	"github.com/snapp-incubator/proksi/internal/logging"
)

var (
	// k is the global koanf instance. Use "." as the key path delimiter.
	k = koanf.New(".")

	// HTTP is the config for Proksi HTTP
	HTTP *HTTPConfig
)

var defaultHTTP = HTTPConfig{
	Bind: "0.0.0.0:9090",
	Metrics: metric{
		Enabled: true,
		Bind:    "0.0.0.0:9001",
	},
	Elasticsearch: Elasticsearch{
		Addresses:              []string{"::9200"},
		Username:               "",
		Password:               "",
		CloudID:                "",
		APIKey:                 "",
		ServiceToken:           "",
		CertificateFingerprint: "",
	},
	Upstreams: struct {
		Main httpUpstream `koanf:"main"`
		Test httpUpstream `koanf:"test"`
	}{
		Main: httpUpstream{Address: "127.0.0.1:8080"},
		Test: httpUpstream{Address: "127.0.0.1:8081"},
	},
	Worker: worker{
		Count:     50,
		QueueSize: 2048,
	},
	SkipJSONPaths:   []string{},
	TestProbability: 100,
}

// HTTPConfig represent config of the Proksi HTTP.
type HTTPConfig struct {
	Bind          string        `koanf:"bind"`
	Metrics       metric        `koanf:"metrics"`
	Elasticsearch Elasticsearch `koanf:"elasticsearch"`
	Upstreams     struct {
		Main httpUpstream `koanf:"main"`
		Test httpUpstream `koanf:"test"`
	} `koanf:"upstreams"`
	Worker          worker   `koanf:"worker"`
	SkipJSONPaths   []string `koanf:"skip_json_paths"`
	TestProbability uint64   `koanf:"test_probability"`
}

type httpUpstream struct {
	Address string `koanf:"address"`
}

type worker struct {
	Count     uint `koanf:"count"`
	QueueSize uint `koanf:"queue_size"`
}

// LoadHTTP function will load the file located in path and return the parsed config for ProksiHTTP. This function will panic on errors
func LoadHTTP(path string) *HTTPConfig {
	// LoadHTTP default config in the beginning
	err := k.Load(structs.Provider(defaultHTTP, "koanf"), nil)
	if err != nil {
		logging.L.Fatal("error in loading the default config", zap.Error(err))
	}

	// LoadHTTP YAML config and merge into the previously loaded config.
	err = k.Load(file.Provider(path), yaml.Parser())
	if err != nil {
		logging.L.Fatal("error in loading the config file", zap.Error(err))
	}

	var c HTTPConfig
	err = k.Unmarshal("", &c)
	if err != nil {
		logging.L.Fatal("error in unmarshalling the config file", zap.Error(err))
	}

	HTTP = &c
	return &c
}

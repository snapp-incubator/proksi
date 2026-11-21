package config

import (
	"errors"
	"fmt"
	"time"

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
	StorageType: "stdout",
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
		Main HTTPUpstream `koanf:"main"`
		Test HTTPUpstream `koanf:"test"`
	}{
		Main: defaultUpstream("127.0.0.1:8080"),
		Test: defaultUpstream("127.0.0.1:8081"),
	},
	Worker: worker{
		Count:     50,
		QueueSize: 2048,
	},
	SkipJSONPaths:      []string{},
	TestProbability:    100,
	LogResponsePayload: true,
	CompareHeaders:     true,
}

// HTTPConfig represent config of the Proksi HTTP.
type HTTPConfig struct {
	Bind          string        `koanf:"bind"`
	Metrics       metric        `koanf:"metrics"`
	StorageType   string        `koanf:"storage_type"` // Storage backend type: "elasticsearch" or "stdout"
	Elasticsearch Elasticsearch `koanf:"elasticsearch"`
	Upstreams     struct {
		Main HTTPUpstream `koanf:"main"`
		Test HTTPUpstream `koanf:"test"`
	} `koanf:"upstreams"`
	Worker             worker   `koanf:"worker"`
	SkipJSONPaths      []string `koanf:"skip_json_paths"`
	TestProbability    uint64   `koanf:"test_probability"`
	LogResponsePayload bool     `koanf:"log_response_payload"`
	CompareHeaders     bool     `koanf:"compare_headers"`
}

// HTTPUpstream is the config of an upstream. Zero timeouts disable the corresponding limit.
type HTTPUpstream struct {
	Address               string        `koanf:"address"`
	Timeout               time.Duration `koanf:"timeout"`                 // Total time limit of a request, including reading the body
	DialTimeout           time.Duration `koanf:"dial_timeout"`            // Time limit for establishing a TCP connection
	TLSHandshakeTimeout   time.Duration `koanf:"tls_handshake_timeout"`   // Time limit for the TLS handshake
	ResponseHeaderTimeout time.Duration `koanf:"response_header_timeout"` // Time limit for receiving response headers after sending the request
}

func defaultUpstream(address string) HTTPUpstream {
	return HTTPUpstream{
		Address:               address,
		Timeout:               30 * time.Second,
		DialTimeout:           5 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 20 * time.Second,
	}
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

	if err := c.Validate(); err != nil {
		logging.L.Fatal("invalid config", zap.Error(err))
	}

	HTTP = &c
	return &c
}

// Validate checks the config for invalid values.
func (c *HTTPConfig) Validate() error {
	var errs []error
	if c.TestProbability > 100 {
		errs = append(errs, fmt.Errorf("test_probability must be between 0 and 100, got %d", c.TestProbability))
	}
	if c.Worker.Count == 0 {
		errs = append(errs, errors.New("worker.count must be greater than 0"))
	}
	if c.Worker.QueueSize == 0 {
		errs = append(errs, errors.New("worker.queue_size must be greater than 0"))
	}
	if c.Upstreams.Main.Address == "" {
		errs = append(errs, errors.New("upstreams.main.address must not be empty"))
	}
	if c.Upstreams.Test.Address == "" {
		errs = append(errs, errors.New("upstreams.test.address must not be empty"))
	}
	switch c.StorageType {
	case "stdout", "elasticsearch":
	default:
		errs = append(errs, fmt.Errorf("storage_type must be \"stdout\" or \"elasticsearch\", got %q", c.StorageType))
	}
	return errors.Join(errs...)
}

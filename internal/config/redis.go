package config

import (
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/providers/structs"
	"go.uber.org/zap"

	"github.com/snapp-incubator/proksi/internal/logging"
)

var (
	// Redis is the config for Proksi Redis
	Redis *RedisConfig
)

var defaultRedis = RedisConfig{
	Bind: "0.0.0.0:6380",
	Metrics: metric{
		Enabled: true,
		Bind:    "0.0.0.0:9001",
	},
	Upstreams: struct {
		Main RedisUpstream `koanf:"main"`
		Test RedisUpstream `koanf:"test"`
	}{
		Main: RedisUpstream{Address: "127.0.0.1:6379"},
		Test: RedisUpstream{Address: "127.0.0.1:6380"},
	},
	Worker: worker{
		Count:     50,
		QueueSize: 2048,
	},
}

// RedisConfig represent config of the Proksi Redis.
type RedisConfig struct {
	Bind      string `koanf:"bind"`
	Metrics   metric `koanf:"metrics"`
	Upstreams struct {
		Main RedisUpstream `koanf:"main"`
		Test RedisUpstream `koanf:"test"`
	} `koanf:"upstreams"`
	Worker worker `koanf:"worker"`
}

type RedisUpstream struct {
	Address string `koanf:"address"`
	// Password, when non-empty, is sent as AUTH to the upstream on every new
	// connection (initial dial, pool growth, reconnects after failures).
	Password string               `koanf:"password"`
	Cluster  RedisClusterUpstream `koanf:"cluster"`
}

// RedisClusterUpstream configures a Redis Cluster upstream. When Enabled, the proxy
// routes each keyed command to the node that owns its slot, discovering the topology
// via CLUSTER SLOTS against Addresses (the seed nodes).
type RedisClusterUpstream struct {
	Enabled   bool     `koanf:"enabled"`
	Addresses []string `koanf:"addresses"`
}

// LoadRedis function will load the file located in path and return the parsed config for ProksiRedis. This function will panic on errors
func LoadRedis(path string) *RedisConfig {
	// Load default config in the beginning
	err := k.Load(structs.Provider(defaultRedis, "koanf"), nil)
	if err != nil {
		logging.L.Fatal("error in loading the default config", zap.Error(err))
	}

	// Load YAML config and merge into the previously loaded config.
	err = k.Load(file.Provider(path), yaml.Parser())
	if err != nil {
		logging.L.Fatal("error in loading the config file", zap.Error(err))
	}

	var c RedisConfig
	err = k.Unmarshal("", &c)
	if err != nil {
		logging.L.Fatal("error in unmarshalling the config file", zap.Error(err))
	}

	Redis = &c
	return &c
}
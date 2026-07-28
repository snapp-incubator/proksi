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
		Main redisUpstream `koanf:"main"`
		Test redisUpstream `koanf:"test"`
	}{
		Main: redisUpstream{Address: "127.0.0.1:6379"},
		Test: redisUpstream{Address: "127.0.0.1:6380"},
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
		Main redisUpstream `koanf:"main"`
		Test redisUpstream `koanf:"test"`
	} `koanf:"upstreams"`
	Worker worker `koanf:"worker"`
}

type redisUpstream struct {
	Address string `koanf:"address"`
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
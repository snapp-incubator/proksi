package config

import (
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/providers/structs"
	"go.uber.org/zap"

	"github.com/snapp-incubator/proksi/internal/logging"
)

var (
	// Redis is the config for ProksiRedis
	Redis *RedisConfig
)

var defaultRedis = RedisConfig{
	MainFrontend: Frontend{
		Bind: "127.0.0.1:6380",
	},
	TestFrontend: Frontend{
		Bind: "127.0.0.1:6381",
	},
	Backend: Backend{
		Address: "127.0.0.1:6379",
	},
}

// RedisConfig represent config of the ProksiRedis.
type RedisConfig struct {
	MainFrontend Frontend `koanf:"main_frontend"`
	TestFrontend Frontend `koanf:"test_frontend"`
	Backend      Backend  `koanf:"backend"`
}

type Frontend struct {
	Bind string `koanf:"bind"`
}

type Backend struct {
	Address string `koanf:"address"`
}

// LoadRedis function will load the file located in path and return the parsed config. This function will panic on errors
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

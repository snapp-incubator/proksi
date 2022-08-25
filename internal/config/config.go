package config

// Elasticsearch is the config of Elasticsearch
type Elasticsearch struct {
	Addresses []string `koanf:"addresses"`
	Username  string   `koanf:"username"`
	Password  string   `koanf:"password"`
}

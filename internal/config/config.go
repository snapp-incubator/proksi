package config

type Elasticsearch struct {
	Addresses []string `koanf:"addresses"`
	Username  string   `koanf:"username"`
	Password  string   `koanf:"password"`
}

package config

// Elasticsearch is the config of Elasticsearch
type Elasticsearch struct {
	Addresses []string `koanf:"addresses"` // A list of Elasticsearch nodes to use.
	Username  string   `koanf:"username"`  // Username for HTTP Basic Authentication.
	Password  string   `koanf:"password"`  // Password for HTTP Basic Authentication.

	CloudID                string `koanf:"cloud_id"`                // Endpoint for the Elastic Service (https://elastic.co/cloud).
	APIKey                 string `koanf:"api_key"`                 // Base64-encoded token for authorization; if set, overrides username/password and service token.
	ServiceToken           string `koanf:"service_token"`           // Service token for authorization; if set, overrides username/password.
	CertificateFingerprint string `koanf:"certificate_fingerprint"` // SHA256 hex fingerprint given by Elasticsearch on first launch.
}

type metric struct {
	Enabled bool   `koanf:"enabled"` // Enablement of the metric exposure
	Bind    string `koanf:"bind"`    // Address of the http server
}

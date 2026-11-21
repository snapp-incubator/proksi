package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadHTTP(t *testing.T) {
	tests := []struct {
		name  string
		yaml  string
		check func(t *testing.T, c *HTTPConfig)
	}{
		{
			name: "empty file keeps defaults",
			yaml: "",
			check: func(t *testing.T, c *HTTPConfig) {
				if c.Bind != "0.0.0.0:9090" || c.StorageType != "stdout" || c.TestProbability != 100 ||
					!c.CompareHeaders || !c.LogResponsePayload || c.Worker.Count != 50 || c.Worker.QueueSize != 2048 {
					t.Fatalf("defaults not applied: %+v", c)
				}
			},
		},
		{
			name: "overrides and partial merge",
			yaml: `
bind: "127.0.0.1:1234"
storage_type: elasticsearch
upstreams:
  main:
    address: "http://main:1"
  test:
    address: "http://test:2"
worker:
  count: 3
skip_json_paths: ["a.b", "ts"]
test_probability: 25
log_response_payload: false
compare_headers: false
metrics:
  enabled: false
elasticsearch:
  addresses: ["es:9200"]
  username: u
`,
			check: func(t *testing.T, c *HTTPConfig) {
				if c.Bind != "127.0.0.1:1234" || c.StorageType != "elasticsearch" ||
					c.Upstreams.Main.Address != "http://main:1" || c.Upstreams.Test.Address != "http://test:2" {
					t.Fatalf("basic overrides wrong: %+v", c)
				}
				if c.Worker.Count != 3 || c.Worker.QueueSize != 2048 {
					t.Fatalf("worker merge wrong: %+v", c.Worker)
				}
				if !reflect.DeepEqual(c.SkipJSONPaths, []string{"a.b", "ts"}) {
					t.Fatalf("skip paths: %v", c.SkipJSONPaths)
				}
				if c.TestProbability != 25 || c.LogResponsePayload || c.CompareHeaders || c.Metrics.Enabled {
					t.Fatalf("scalars wrong: %+v", c)
				}
				if c.Metrics.Bind != "0.0.0.0:9001" {
					t.Fatalf("metrics bind default lost: %q", c.Metrics.Bind)
				}
				if !reflect.DeepEqual(c.Elasticsearch.Addresses, []string{"es:9200"}) || c.Elasticsearch.Username != "u" {
					t.Fatalf("elasticsearch: %+v", c.Elasticsearch)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := LoadHTTP(writeConfig(t, tt.yaml))
			if HTTP != c {
				t.Fatal("global HTTP not set to returned config")
			}
			tt.check(t, c)
		})
	}
}

func TestLoadHTTPExampleConfig(t *testing.T) {
	c := LoadHTTP(filepath.Join("..", "..", "http", "config.example.yaml"))
	if c.Upstreams.Main.Address == "" || c.Upstreams.Test.Address == "" || c.TestProbability != 100 {
		t.Fatalf("example config parsed wrong: %+v", c)
	}
}

func TestValidate(t *testing.T) {
	valid := func() HTTPConfig { return defaultHTTP }
	if err := (&HTTPConfig{}).Validate(); err == nil {
		t.Fatal("empty config should be invalid")
	}
	d := valid()
	if err := d.Validate(); err != nil {
		t.Fatalf("default config should be valid: %v", err)
	}
	cases := map[string]func(*HTTPConfig){
		"probability>100": func(c *HTTPConfig) { c.TestProbability = 101 },
		"worker count":    func(c *HTTPConfig) { c.Worker.Count = 0 },
		"queue size":      func(c *HTTPConfig) { c.Worker.QueueSize = 0 },
		"main address":    func(c *HTTPConfig) { c.Upstreams.Main.Address = "" },
		"test address":    func(c *HTTPConfig) { c.Upstreams.Test.Address = "" },
		"storage type":    func(c *HTTPConfig) { c.StorageType = "foo" },
	}
	for name, mut := range cases {
		c := valid()
		mut(&c)
		if err := c.Validate(); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
	for _, p := range []uint64{0, 50, 100} {
		c := valid()
		c.TestProbability = p
		if err := c.Validate(); err != nil {
			t.Errorf("probability %d should be valid: %v", p, err)
		}
	}
}

func TestLoadHTTPTimeouts(t *testing.T) {
	p := filepath.Join(t.TempDir(), "c.yaml")
	if err := os.WriteFile(p, []byte("upstreams:\n  main:\n    address: http://m\n    timeout: 2s\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	c := LoadHTTP(p)
	if c.Upstreams.Main.Timeout != 2*time.Second {
		t.Errorf("main timeout = %v", c.Upstreams.Main.Timeout)
	}
	if c.Upstreams.Main.DialTimeout != 5*time.Second || c.Upstreams.Test.Timeout != 30*time.Second {
		t.Errorf("defaults not applied: %+v", c.Upstreams)
	}
}

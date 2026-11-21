package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

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

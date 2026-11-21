package config

import "testing"

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

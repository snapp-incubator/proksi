package main

import (
	"net/http"
	"testing"
)

func TestStripHopByHop(t *testing.T) {
	h := http.Header{}
	h.Add("Connection", "keep-alive, X-Custom")
	h.Set("X-Custom", "1")
	h.Set("Transfer-Encoding", "chunked")
	h.Add("Set-Cookie", "a=1")
	h.Add("Set-Cookie", "b=2")

	stripHopByHop(h)

	for _, k := range []string{"Connection", "X-Custom", "Transfer-Encoding"} {
		if h.Get(k) != "" {
			t.Errorf("%s should be stripped", k)
		}
	}
	if got := h.Values("Set-Cookie"); len(got) != 2 {
		t.Errorf("multi-value header not preserved: %v", got)
	}
}

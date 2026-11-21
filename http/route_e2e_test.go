package main

import (
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/snapp-incubator/proksi/internal/config"
)

func doGet(t *testing.T, url string) string {
	t.Helper()
	res, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	b, _ := io.ReadAll(res.Body)
	return string(b)
}

func TestRouteSkippedIsServedButNotMirrored(t *testing.T) {
	var testHits int64
	testH := func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&testHits, 1)
		w.WriteHeader(500)
	}
	mainH := func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("main")) }
	h := newHarness(t, mainH, testH, func(c *config.HTTPConfig) {
		c.SkipRoutes = []string{"GET:/health", "*:/internal/*"}
	})
	if got := doGet(t, h.proxy.URL+"/health"); got != "main" {
		t.Fatalf("skipped route must still be served by main, got %q", got)
	}
	if got := doGet(t, h.proxy.URL+"/internal/a/b"); got != "main" {
		t.Fatalf("got %q", got)
	}
	// /internalx must NOT be treated as skipped (segment boundary).
	doGet(t, h.proxy.URL+"/internalx")
	h.drain()
	if n := atomic.LoadInt64(&testHits); n != 1 {
		t.Fatalf("test upstream hit %d times, want 1 (only /internalx)", n)
	}
}

func TestRouteProbabilityOverride(t *testing.T) {
	var testHits int64
	testH := func(w http.ResponseWriter, r *http.Request) { atomic.AddInt64(&testHits, 1) }
	h := newHarness(t, func(w http.ResponseWriter, r *http.Request) {}, testH, func(c *config.HTTPConfig) {
		c.TestProbability = 0                                                      // mirror nothing globally...
		c.RouteConfigs["GET:/mirrored"] = config.RouteConfig{TestProbability: 100} // ...except this route
	})
	for i := 0; i < 20; i++ {
		doGet(t, h.proxy.URL+"/mirrored")
		doGet(t, h.proxy.URL+"/other")
	}
	h.drain()
	if n := atomic.LoadInt64(&testHits); n != 20 {
		t.Fatalf("test upstream hit %d times, want 20", n)
	}
}

func TestCompareBodyDisabled(t *testing.T) {
	js := "application/json"
	h := newHarness(t, respond(200, js, `{"a":1}`), respond(200, js, `{"a":2}`), func(c *config.HTTPConfig) {
		c.RouteConfigs["GET:/nobody"] = config.RouteConfig{CompareBody: "disable"}
	})
	doGet(t, h.proxy.URL+"/nobody")
	doGet(t, h.proxy.URL+"/body")
	h.drain()
	logs := h.store.Logs()
	if len(logs) != 1 || logs[0].Route != "GET:/body" {
		t.Fatalf("want exactly one mismatch for /body, got %+v", logs)
	}
}

func TestHeaderComparison(t *testing.T) {
	mainH := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Noise", "a")
		w.Header().Set("X-Real", "a")
		_, _ = w.Write([]byte("same"))
	}
	testH := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Noise", "b")
		w.Header().Set("X-Real", "a")
		_, _ = w.Write([]byte("same"))
	}
	t.Run("skipped header difference is ignored", func(t *testing.T) {
		h := newHarness(t, mainH, testH, func(c *config.HTTPConfig) {
			c.GlobalConfig.SkipHeaders = []string{"x-noise"}
		})
		doGet(t, h.proxy.URL+"/")
		h.drain()
		if logs := h.store.Logs(); len(logs) != 0 {
			t.Fatalf("want no mismatch, got %+v", logs)
		}
	})
	t.Run("header difference is stored with payloads", func(t *testing.T) {
		h := newHarness(t, mainH, testH, nil)
		doGet(t, h.proxy.URL+"/")
		h.drain()
		logs := h.store.Logs()
		if len(logs) != 1 || logs[0].ComparisonType != "header_diff" {
			t.Fatalf("want one header_diff, got %+v", logs)
		}
		l := logs[0]
		if len(l.DifferentHeaders) != 1 || l.DifferentHeaders[0] != "X-Noise" {
			t.Fatalf("different headers = %v", l.DifferentHeaders)
		}
		if l.MainUpstreamResponsePayload == nil || *l.MainUpstreamResponsePayload != "same" ||
			l.TestUpstreamResponsePayload == nil || *l.TestUpstreamResponsePayload != "same" {
			t.Fatalf("response payloads must be stored on header_diff, got %v / %v",
				l.MainUpstreamResponsePayload, l.TestUpstreamResponsePayload)
		}
	})
}

func TestStoreReqBody(t *testing.T) {
	h := newHarness(t, respond(200, "text/plain", "x"), respond(200, "text/plain", "y"), func(c *config.HTTPConfig) {
		c.RouteConfigs["POST:/store"] = config.RouteConfig{StoreReqBody: "enable"}
	})
	for _, path := range []string{"/store", "/nostore"} {
		res, err := http.Post(h.proxy.URL+path, "text/plain", strings.NewReader("payload"))
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, res.Body)
		_ = res.Body.Close()
	}
	h.drain()
	got := map[string]*string{}
	for _, l := range h.store.Logs() {
		got[l.Route] = l.RequestBody
	}
	if b := got["POST:/store"]; b == nil || *b != "payload" {
		t.Fatalf("/store request body = %v, want payload", b)
	}
	if got["POST:/nostore"] != nil {
		t.Fatal("/nostore must not store the request body")
	}
}

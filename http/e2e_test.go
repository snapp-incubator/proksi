package main

import (
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/snapp-incubator/proksi/internal/config"
)

func respond(status int, contentType, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		if contentType != "" {
			w.Header().Set("Content-Type", contentType)
		}
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}
}

func TestE2EMismatchDetection(t *testing.T) {
	const js = "application/json"
	tests := []struct {
		name         string
		mainH, testH http.HandlerFunc
		mutate       func(*config.HTTPConfig)
		wantStored   int
	}{
		{"equal json", respond(200, js, `{"a":1}`), respond(200, js, `{ "a": 1 }`), nil, 0},
		{"equal plain text", respond(200, "text/plain", "ok"), respond(200, "text/plain", "ok"), nil, 0},
		{"status mismatch", respond(200, js, `{}`), respond(500, js, `{}`), nil, 1},
		{"body mismatch json", respond(200, js, `{"a":1}`), respond(200, js, `{"a":2}`), nil, 1},
		{"body mismatch text", respond(200, "text/plain", "x"), respond(200, "text/plain", "y"), nil, 1},
		{"content-type mismatch", respond(200, js, `{}`), respond(200, "text/plain", `{}`), nil, 1},
		{
			"content-type mismatch ignored when compare_headers=false",
			respond(200, js, `{}`), respond(200, "text/plain", `{}`),
			func(c *config.HTTPConfig) { c.CompareHeaders = false }, 0,
		},
		{
			"skipped json path hides difference",
			respond(200, js, `{"a":1,"ts":111}`), respond(200, js, `{"a":1,"ts":222}`),
			func(c *config.HTTPConfig) { c.SkipJSONPaths = []string{"ts"} }, 0,
		},
		{
			"skipped nested path hides difference",
			respond(200, js, `{"a":{"id":"x"},"b":1}`), respond(200, js, `{"a":{"id":"y"},"b":1}`),
			func(c *config.HTTPConfig) { c.SkipJSONPaths = []string{"a.id"} }, 0,
		},
		{
			"non-skipped difference still reported",
			respond(200, js, `{"a":1,"ts":111}`), respond(200, js, `{"a":2,"ts":222}`),
			func(c *config.HTTPConfig) { c.SkipJSONPaths = []string{"ts"} }, 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, tt.mainH, tt.testH, tt.mutate)
			res, err := http.Get(h.proxy.URL + "/path?q=1")
			if err != nil {
				t.Fatal(err)
			}
			_, _ = io.Copy(io.Discard, res.Body)
			_ = res.Body.Close()
			h.drain()

			logs := h.store.Logs()
			if len(logs) != tt.wantStored {
				t.Fatalf("stored %d logs, want %d: %+v", len(logs), tt.wantStored, logs)
			}
			if tt.wantStored > 0 && logs[0].URL != "/path?q=1" {
				t.Fatalf("stored URL = %q", logs[0].URL)
			}
		})
	}
}

func TestE2EClientGetsMainResponse(t *testing.T) {
	h := newHarness(t,
		func(w http.ResponseWriter, r *http.Request) {
			b, _ := io.ReadAll(r.Body)
			w.Header().Set("Content-Type", "text/plain")
			w.Header().Set("X-Main", "1")
			w.WriteHeader(201)
			_, _ = fmt.Fprintf(w, "main:%s:%s", r.Method, b)
		},
		func(w http.ResponseWriter, r *http.Request) {
			b, _ := io.ReadAll(r.Body)
			if string(b) != "payload" {
				t.Errorf("test upstream got body %q, want replayed %q", b, "payload")
			}
			respond(418, "text/plain", "test")(w, r)
		}, nil)

	res, err := http.Post(h.proxy.URL+"/x", "text/plain", stringsReader("payload"))
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	h.drain()

	if res.StatusCode != 201 || string(b) != "main:POST:payload" || res.Header.Get("X-Main") != "1" {
		t.Fatalf("client saw %d %q %v", res.StatusCode, b, res.Header)
	}
	logs := h.store.Logs()
	if len(logs) != 1 || logs[0].MainUpstreamStatusCode != 201 || logs[0].TestUpstreamStatusCode != 418 {
		t.Fatalf("unexpected logs: %+v", logs)
	}
}

func TestE2ELogResponsePayload(t *testing.T) {
	for _, on := range []bool{true, false} {
		t.Run(fmt.Sprintf("log_response_payload=%v", on), func(t *testing.T) {
			h := newHarness(t, respond(200, "text/plain", "m"), respond(200, "text/plain", "t"),
				func(c *config.HTTPConfig) { c.LogResponsePayload = on })
			res, err := http.Get(h.proxy.URL + "/")
			if err != nil {
				t.Fatal(err)
			}
			_ = res.Body.Close()
			h.drain()
			logs := h.store.Logs()
			if len(logs) != 1 {
				t.Fatalf("got %d logs", len(logs))
			}
			l := logs[0]
			if on && (l.MainUpstreamResponsePayload == nil || *l.MainUpstreamResponsePayload != "m" ||
				l.TestUpstreamResponsePayload == nil || *l.TestUpstreamResponsePayload != "t") {
				t.Fatalf("payloads not stored: %+v", l)
			}
			if !on && (l.MainUpstreamResponsePayload != nil || l.TestUpstreamResponsePayload != nil) {
				t.Fatalf("payloads stored but disabled: %+v", l)
			}
		})
	}
}

func TestProbabilityGating(t *testing.T) {
	const n = 200
	run := func(t *testing.T, prob uint64) int64 {
		var testHits int64
		h := newHarness(t, respond(200, "text/plain", "x"),
			func(w http.ResponseWriter, r *http.Request) {
				atomic.AddInt64(&testHits, 1)
				respond(200, "text/plain", "x")(w, r)
			},
			func(c *config.HTTPConfig) { c.TestProbability = prob })
		for i := 0; i < n; i++ {
			res, err := http.Get(h.proxy.URL + "/")
			if err != nil {
				t.Fatal(err)
			}
			_, _ = io.Copy(io.Discard, res.Body)
			_ = res.Body.Close()
		}
		h.drain()
		return atomic.LoadInt64(&testHits)
	}

	t.Run("0 percent mirrors nothing", func(t *testing.T) {
		if got := run(t, 0); got != 0 {
			t.Fatalf("mirrored %d, want 0", got)
		}
	})
	t.Run("100 percent mirrors everything", func(t *testing.T) {
		got := run(t, 100)
		if got != n {
			t.Fatalf("mirrored %d of %d, want all", got, n)
		}
	})
	t.Run("50 percent mirrors roughly half", func(t *testing.T) {
		got := run(t, 50)
		if got < n*40/100 || got > n*60/100 {
			t.Fatalf("mirrored %d of %d, want about half", got, n)
		}
	})
}

// Runs under -race: concurrent requests must not race on the probability counter,
// and exactly half of 100 requests fall in a 50% bucket.
func TestE2EConcurrentProbabilityGate(t *testing.T) {
	var testHits atomic.Int64
	h := newHarness(t, respond(200, "text/plain", "ok"),
		func(w http.ResponseWriter, _ *http.Request) { testHits.Add(1); _, _ = io.WriteString(w, "ok") },
		func(c *config.HTTPConfig) { c.TestProbability = 50 })

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := http.Get(h.proxy.URL + "/")
			if err != nil {
				t.Error(err)
				return
			}
			_, _ = io.Copy(io.Discard, res.Body)
			_ = res.Body.Close()
		}()
	}
	wg.Wait()
	h.drain()
	if got := testHits.Load(); got != 50 {
		t.Fatalf("test upstream hits = %d, want 50", got)
	}
}

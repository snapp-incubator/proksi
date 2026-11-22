package main

import (
	"io"
	"net/http"
	"testing"

	"github.com/snapp-incubator/proksi/internal/config"
)

// TestSkipJSONPathsSetErrorKeepsMismatch makes sjson.Set fail ("a.b.c" on a body where
// "a" is an array: non-numeric key on an array element) and asserts the worker does not
// panic, the original bodies are kept (so the "ts" difference is NOT hidden by the first,
// valid skip path) and the mismatch is still stored.
func TestSkipJSONPathsSetErrorKeepsMismatch(t *testing.T) {
	const js = "application/json"
	h := newHarness(t,
		respond(200, js, `{"a":[1],"ts":111}`),
		respond(200, js, `{"a":[1],"ts":222}`),
		func(c *config.HTTPConfig) { c.SkipJSONPaths = []string{"ts", "a.b.c"} },
	)

	res, err := http.Get(h.proxy.URL + "/p")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if string(body) != `{"a":[1],"ts":111}` {
		t.Fatalf("client body = %q", body)
	}
	h.drain() // a worker panic would crash the test binary

	logs := h.store.Logs()
	if len(logs) != 1 {
		t.Fatalf("stored %d logs, want 1: %+v", len(logs), logs)
	}
	if logs[0].ComparisonType != "body_diff" {
		t.Fatalf("comparison type = %q", logs[0].ComparisonType)
	}
}

// TestSkipJSONPathsEmptyPathKeepsMismatch covers a second sjson failure mode (empty path).
func TestSkipJSONPathsEmptyPathKeepsMismatch(t *testing.T) {
	const js = "application/json"
	h := newHarness(t,
		respond(200, js, `{"ts":1}`), respond(200, js, `{"ts":2}`),
		func(c *config.HTTPConfig) { c.SkipJSONPaths = []string{""} },
	)
	res, err := http.Get(h.proxy.URL + "/p")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, res.Body)
	_ = res.Body.Close()
	h.drain()
	if n := len(h.store.Logs()); n != 1 {
		t.Fatalf("stored %d logs, want 1", n)
	}
}

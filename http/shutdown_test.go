package main

import (
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"
)

// TestGracefulShutdownDrainsQueuedJobs queues more mirror jobs than workers while the test
// upstream is slow, then shuts down and asserts every queued job was processed and stored.
func TestGracefulShutdownDrainsQueuedJobs(t *testing.T) {
	const n = 12 // harness runs 4 workers, so most jobs are still queued
	h := newHarness(t,
		respond(200, "text/plain", "main"),
		func(w http.ResponseWriter, _ *http.Request) {
			time.Sleep(100 * time.Millisecond)
			_, _ = io.WriteString(w, "different")
		},
		nil,
	)

	// Requests are sent sequentially: concurrent handlers trip the reqCounter data race
	// tracked in #73 under -race. Mirroring is async, so jobs still pile up in the queue.
	for i := 0; i < n; i++ {
		res, err := http.Get(fmt.Sprintf("%s/p%d", h.proxy.URL, i))
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, res.Body)
		_ = res.Body.Close()
	}

	if q := len(h.jobs); q == 0 {
		t.Log("warning: queue already empty before shutdown; drain not exercised")
	}

	gracefulShutdown(h.proxy.Config, h.jobs, &h.wg, 10*time.Second)

	if got := len(h.store.Logs()); got != n {
		t.Fatalf("stored %d logs after shutdown, want %d", got, n)
	}
	if _, err := http.Get(h.proxy.URL + "/after"); err == nil {
		t.Fatal("server still accepting requests after shutdown")
	}
}

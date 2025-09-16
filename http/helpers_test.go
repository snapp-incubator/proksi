package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/snapp-incubator/proksi/internal/config"
	"github.com/snapp-incubator/proksi/internal/storage"
)

// fakeStorage records every Log passed to Store.
type fakeStorage struct {
	mu   sync.Mutex
	logs []storage.Log
}

func (f *fakeStorage) Store(l storage.Log) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.logs = append(f.logs, l)
	return nil
}

func (f *fakeStorage) Logs() []storage.Log {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]storage.Log(nil), f.logs...)
}

// harness is an in-process proxy wired to two httptest upstreams and a fake storage.
type harness struct {
	proxy *httptest.Server
	store *fakeStorage
	jobs  chan Job
	wg    sync.WaitGroup
}

// newHarness starts the main/test upstreams and the proxy. It mutates the package
// globals (config.HTTP, strg), so tests using it must not run in parallel.
func newHarness(t *testing.T, mainH, testH http.HandlerFunc, mutate func(*config.HTTPConfig)) *harness {
	t.Helper()

	mainSrv := httptest.NewServer(mainH)
	testSrv := httptest.NewServer(testH)

	defaults := config.DefaultHTTP()
	cfg := &defaults
	cfg.Upstreams.Main.Address = mainSrv.URL
	cfg.Upstreams.Test.Address = testSrv.URL
	if mutate != nil {
		mutate(cfg)
	}
	cfg.Finalize()

	oldCfg, oldStrg, oldMain, oldTest := config.HTTP, strg, mainServiceClient, testServiceClient
	config.HTTP = cfg
	mainServiceClient = newUpstreamClient(cfg.Upstreams.Main)
	testServiceClient = newUpstreamClient(cfg.Upstreams.Test)
	h := &harness{store: &fakeStorage{}, jobs: make(chan Job, 1024)}
	strg = h.store

	for i := 0; i < 4; i++ {
		h.wg.Add(1)
		go func() {
			defer h.wg.Done()
			for j := range h.jobs {
				j.Do()
			}
		}()
	}

	s := &server{job: h.jobs}
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handle)
	h.proxy = httptest.NewServer(mux)

	t.Cleanup(func() {
		h.proxy.Close()
		mainSrv.Close()
		testSrv.Close()
		config.HTTP, strg, mainServiceClient, testServiceClient = oldCfg, oldStrg, oldMain, oldTest
	})
	return h
}

// drain waits until all queued mirror jobs finished. Call once per harness.
func (h *harness) drain() {
	close(h.jobs)
	h.wg.Wait()
}

func stringsReader(s string) io.Reader { return strings.NewReader(s) }

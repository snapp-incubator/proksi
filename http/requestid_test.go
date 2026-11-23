package main

import (
	"io"
	"net/http"
	"regexp"
	"sync"
	"testing"
)

type idRecorder struct {
	mu  sync.Mutex
	ids []string
}

func (r *idRecorder) handler(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		r.mu.Lock()
		r.ids = append(r.ids, req.Header.Get("X-Request-Id"))
		r.mu.Unlock()
		_, _ = io.WriteString(w, body)
	}
}

func (r *idRecorder) get() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.ids...)
}

func getWithID(t *testing.T, url, id string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	if id != "" {
		req.Header.Set("X-Request-Id", id)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, res.Body)
	_ = res.Body.Close()
	return res
}

func TestRequestIDSuppliedPropagates(t *testing.T) {
	var mainRec, testRec idRecorder
	h := newHarness(t, mainRec.handler("a"), testRec.handler("b"), nil)

	res := getWithID(t, h.proxy.URL+"/x", "my-id-123")
	h.drain()

	if got := res.Header.Get("X-Request-Id"); got != "my-id-123" {
		t.Fatalf("response id = %q", got)
	}
	if ids := mainRec.get(); len(ids) != 1 || ids[0] != "my-id-123" {
		t.Fatalf("main upstream ids = %v", ids)
	}
	if ids := testRec.get(); len(ids) != 1 || ids[0] != "my-id-123" {
		t.Fatalf("test upstream ids = %v", ids)
	}
	logs := h.store.Logs()
	if len(logs) != 1 || logs[0].RequestID != "my-id-123" {
		t.Fatalf("stored logs = %+v", logs)
	}
}

func TestRequestIDGeneratedWhenMissing(t *testing.T) {
	var mainRec, testRec idRecorder
	h := newHarness(t, mainRec.handler("a"), testRec.handler("b"), nil)

	res := getWithID(t, h.proxy.URL+"/x", "")
	h.drain()

	id := res.Header.Get("X-Request-Id")
	if !regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(id) {
		t.Fatalf("generated id = %q, want 32 hex chars", id)
	}
	if ids := mainRec.get(); len(ids) != 1 || ids[0] != id {
		t.Fatalf("main upstream ids = %v, want %q", ids, id)
	}
	if ids := testRec.get(); len(ids) != 1 || ids[0] != id {
		t.Fatalf("test upstream ids = %v, want %q", ids, id)
	}
	logs := h.store.Logs()
	if len(logs) != 1 || logs[0].RequestID != id {
		t.Fatalf("stored logs = %+v, want request_id %q", logs, id)
	}
}

func TestRequestIDGeneratedIsUnique(t *testing.T) {
	var mainRec, testRec idRecorder
	h := newHarness(t, mainRec.handler("a"), testRec.handler("a"), nil)
	a := getWithID(t, h.proxy.URL+"/x", "").Header.Get("X-Request-Id")
	b := getWithID(t, h.proxy.URL+"/x", "").Header.Get("X-Request-Id")
	h.drain()
	if a == b {
		t.Fatalf("generated ids not unique: %q", a)
	}
}

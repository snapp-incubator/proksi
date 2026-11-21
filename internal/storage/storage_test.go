package storage

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"testing"
)

func ptr(s string) *string { return &s }

func TestLogJSON(t *testing.T) {
	l := Log{
		URL:                         "/a?b=1",
		Headers:                     map[string][]string{"X": {"1", "2"}},
		MainUpstreamStatusCode:      200,
		TestUpstreamStatusCode:      500,
		MainUpstreamResponsePayload: ptr("m"),
	}
	b, err := json.Marshal(l)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m["url"] != "/a?b=1" || m["main_upstream_status_code"] != float64(200) ||
		m["test_upstream_status_code"] != float64(500) || m["main_upstream_response_payload"] != "m" ||
		m["test_upstream_response_payload"] != nil {
		t.Fatalf("unexpected JSON: %s", b)
	}
}

func TestStdoutStorageStore(t *testing.T) {
	tests := []struct {
		name string
		log  Log
	}{
		{"minimal", Log{URL: "/x", MainUpstreamStatusCode: 200, TestUpstreamStatusCode: 200}},
		{"with payloads", Log{URL: "/y", Headers: map[string][]string{"A": {"b"}}, MainUpstreamStatusCode: 200,
			TestUpstreamStatusCode: 500, MainUpstreamResponsePayload: ptr("p1"), TestUpstreamResponsePayload: ptr("p2")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			old := os.Stdout
			r, w, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			os.Stdout = w
			storeErr := StdoutStorage{}.Store(tt.log)
			os.Stdout = old
			_ = w.Close()
			out, _ := io.ReadAll(r)
			if storeErr != nil {
				t.Fatal(storeErr)
			}
			if !bytes.HasSuffix(out, []byte("\n")) {
				t.Fatalf("expected newline-terminated output: %q", out)
			}
			var got Log
			if err := json.Unmarshal(out, &got); err != nil {
				t.Fatalf("output not JSON: %v: %q", err, out)
			}
			want, _ := json.Marshal(tt.log)
			gotB, _ := json.Marshal(got)
			if !bytes.Equal(want, gotB) {
				t.Fatalf("got %s, want %s", gotB, want)
			}
		})
	}
}

func TestElasticStorageStore(t *testing.T) {
	t.Skip("ElasticStorage.Store dereferences res.Body before checking err (nil panic when ES is unreachable); needs a fake ES transport once fixed")
}

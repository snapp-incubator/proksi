package storage

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/elastic/go-elasticsearch/v8"
)

func TestElasticStoreUnreachable(t *testing.T) {
	es, err := elasticsearch.NewClient(elasticsearch.Config{
		Addresses:    []string{"http://127.0.0.1:1"},
		MaxRetries:   1,
		DisableRetry: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := (ElasticStorage{ES: es}).Store(Log{URL: "/x"}); err == nil {
		t.Fatal("expected error for unreachable Elasticsearch")
	}
}

func newTestES(t *testing.T, h http.HandlerFunc) ElasticStorage {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	es, err := elasticsearch.NewClient(elasticsearch.Config{
		Addresses:    []string{srv.URL},
		DisableRetry: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	return ElasticStorage{ES: es}
}

func TestElasticStoreErrorResponses(t *testing.T) {
	for _, code := range []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusTooManyRequests, http.StatusInternalServerError} {
		s := newTestES(t, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("X-Elastic-Product", "Elasticsearch")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(code)
			_, _ = w.Write([]byte(`{"error":"boom"} `))
		})
		err := s.Store(Log{URL: "/x"})
		if err == nil {
			t.Fatalf("status %d: expected error", code)
		}
		if !strings.Contains(err.Error(), http.StatusText(code)) || !strings.Contains(err.Error(), `{"error":"boom"}`) {
			t.Fatalf("status %d: error lacks status/body: %v", code, err)
		}
	}
}

func TestElasticStoreTruncatesBody(t *testing.T) {
	s := newTestES(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Elastic-Product", "Elasticsearch")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(strings.Repeat("a", 10000)))
	})
	err := s.Store(Log{URL: "/x"})
	if err == nil || len(err.Error()) > 1000 {
		t.Fatalf("expected trimmed error, got %v", err)
	}
}

func TestElasticStoreSuccess(t *testing.T) {
	s := newTestES(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Elastic-Product", "Elasticsearch")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"result":"created"}`))
	})
	if err := s.Store(Log{URL: "/x"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

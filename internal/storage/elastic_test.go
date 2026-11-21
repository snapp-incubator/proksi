package storage

import (
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

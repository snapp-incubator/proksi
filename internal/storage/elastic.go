package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/elastic/go-elasticsearch/v8"
	"github.com/elastic/go-elasticsearch/v8/esapi"
)

// ElasticStorage is the backend Storage interface that works with Elasticsearch
type ElasticStorage struct {
	ES *elasticsearch.Client
}

// Store is the action of storing
func (s ElasticStorage) Store(l Log) error {
	b, _ := json.Marshal(&l)
	now := time.Now()
	r := esapi.IndexRequest{
		Index: fmt.Sprintf("%d-%d-%d", now.Year(), now.Month(), now.Day()),
		Body:  bytes.NewReader(b),
	}

	res, err := r.Do(context.Background(), s.ES)
	defer func() { _ = res.Body.Close() }()

	return err
}

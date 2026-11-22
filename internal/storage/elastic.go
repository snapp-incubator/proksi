package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/elastic/go-elasticsearch/v8"
	"github.com/elastic/go-elasticsearch/v8/esapi"
)

// maxErrBody caps how much of an Elasticsearch error body is put in the error.
const maxErrBody = 512

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
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()

	if res.IsError() {
		body, _ := io.ReadAll(io.LimitReader(res.Body, maxErrBody))
		return fmt.Errorf("elasticsearch index request failed: status %s: %s",
			res.Status(), strings.TrimSpace(string(body)))
	}

	return nil
}

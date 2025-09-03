package storage

import (
	"encoding/json"
	"fmt"
	"os"
)

// StdoutStorage is a Storage implementation that outputs logs to stdout
type StdoutStorage struct{}

// Store outputs the log as JSON to stdout
func (s StdoutStorage) Store(l Log) error {
	b, err := json.Marshal(&l)
	if err != nil {
		return fmt.Errorf("failed to marshal log to JSON: %w", err)
	}

	_, err = fmt.Fprintln(os.Stdout, string(b))
	if err != nil {
		return fmt.Errorf("failed to write log to stdout: %w", err)
	}

	return nil
}

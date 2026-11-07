package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"ahmelia/production"
)

// backgroundResultStore is the durable queue of results that still need to be
// announced. It is separate from WorkStore: Work owns the attempt, this store
// owns the pending conversational delivery.
type backgroundResultStore struct {
	mu   sync.Mutex
	path string
}

type backgroundResultRecord struct {
	JobID       string                  `json:"job_id,omitempty"`
	WorkID      string                  `json:"work_id,omitempty"`
	Result      production.ActionResult `json:"result"`
	Error       string                  `json:"error,omitempty"`
	Recovered   bool                    `json:"recovered,omitempty"`
	Interrupted bool                    `json:"interrupted,omitempty"`
	Canceled    bool                    `json:"canceled,omitempty"`
}

func OpenBackgroundResultStore(path string) *backgroundResultStore {
	return &backgroundResultStore{path: path}
}

func backgroundResultKey(item backgroundResult) string {
	if item.JobID != "" {
		return item.JobID
	}
	if item.WorkID != "" {
		return "work:" + item.WorkID
	}
	return "result:anonymous"
}

func (s *backgroundResultStore) Load() ([]backgroundResult, error) {
	if s == nil || s.path == "" {
		return nil, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("background results: read: %w", err)
	}
	var records map[string]backgroundResultRecord
	if err := json.Unmarshal(data, &records); err != nil {
		return nil, fmt.Errorf("background results: decode: %w", err)
	}
	keys := make([]string, 0, len(records))
	for key := range records {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]backgroundResult, 0, len(keys))
	for _, key := range keys {
		record := records[key]
		item := backgroundResult{
			JobID: record.JobID, WorkID: record.WorkID, Result: record.Result,
			Recovered: record.Recovered, Interrupted: record.Interrupted, Canceled: record.Canceled,
		}
		if record.Error != "" {
			item.Err = errors.New(record.Error)
		}
		out = append(out, item)
	}
	return out, nil
}

func (s *backgroundResultStore) Save(items []backgroundResult) error {
	if s == nil || s.path == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	records := make(map[string]backgroundResultRecord, len(items))
	for _, item := range items {
		record := backgroundResultRecord{
			JobID: item.JobID, WorkID: item.WorkID, Result: item.Result,
			Recovered: item.Recovered, Interrupted: item.Interrupted, Canceled: item.Canceled,
		}
		if item.Err != nil {
			record.Error = item.Err.Error()
		}
		records[backgroundResultKey(item)] = record
	}
	data, err := json.Marshal(records)
	if err != nil {
		return fmt.Errorf("background results: encode: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("background results: mkdir: %w", err)
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("background results: write: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("background results: commit: %w", err)
	}
	return nil
}

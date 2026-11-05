package execution

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// TaskGraphCheckpoint is durable operational state for a graph execution.
// It records only serializable execution facts; planning.Task remains the
// authority for task identity and lifecycle.
type TaskGraphCheckpoint struct {
	WorkID           string    `json:"work_id"`
	GraphFingerprint string    `json:"graph_fingerprint,omitempty"`
	UpdatedAt        time.Time `json:"updated_at"`
	Completed        []string  `json:"completed,omitempty"`
	// Blocked records tasks whose uncertain effect was reconciled as explicitly
	// blocked. Unlike transient execution errors, this state must not dispatch
	// the same unchanged operation on a later invocation.
	Blocked  []string          `json:"blocked,omitempty"`
	Failed   map[string]string `json:"failed,omitempty"`
	Attempts map[string]int    `json:"attempts,omitempty"`
	Running  map[string]string `json:"running,omitempty"`
}

// TaskGraphCheckpointStore persists resumable graph state. A file-backed
// implementation is provided for process-restart continuity.
type TaskGraphCheckpointStore interface {
	Load(workID string) (TaskGraphCheckpoint, bool, error)
	Save(checkpoint TaskGraphCheckpoint) error
	Delete(workID string) error
}

type FileTaskGraphCheckpointStore struct {
	mu  sync.Mutex
	dir string
}

func NewFileTaskGraphCheckpointStore(dir string) (*FileTaskGraphCheckpointStore, error) {
	if dir == "" {
		return nil, fmt.Errorf("execution: checkpoint directory vacío")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("execution: crear checkpoint directory: %w", err)
	}
	return &FileTaskGraphCheckpointStore{dir: dir}, nil
}

func (s *FileTaskGraphCheckpointStore) path(workID string) string {
	return filepath.Join(s.dir, filepath.Base(workID)+".json")
}

func (s *FileTaskGraphCheckpointStore) Load(workID string) (TaskGraphCheckpoint, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	raw, err := os.ReadFile(s.path(workID))
	if os.IsNotExist(err) {
		return TaskGraphCheckpoint{}, false, nil
	}
	if err != nil {
		return TaskGraphCheckpoint{}, false, err
	}
	var cp TaskGraphCheckpoint
	if err := json.Unmarshal(raw, &cp); err != nil {
		return TaskGraphCheckpoint{}, false, fmt.Errorf("execution: decode checkpoint: %w", err)
	}
	return cp, true, nil
}

func (s *FileTaskGraphCheckpointStore) Save(cp TaskGraphCheckpoint) error {
	if cp.WorkID == "" {
		return fmt.Errorf("execution: checkpoint work id vacío")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cp.UpdatedAt = time.Now().UTC()
	raw, err := json.MarshalIndent(cp, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path(cp.WorkID) + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path(cp.WorkID))
}

func (s *FileTaskGraphCheckpointStore) Delete(workID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.Remove(s.path(workID)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

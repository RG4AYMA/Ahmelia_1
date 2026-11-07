// Package experience is the append-only memory of failed and successful
// attempts. It is the external learning surface: the model is frozen, but the
// harness records what failed, why, and what lesson the next attempt should
// receive. Nothing here changes authority; it only feeds the next proposal.
package experience

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// StorePath is the live append-only experience log.
const StorePath = "memory/experience.jsonl"

// Record is one observed attempt. Lesson is a short, reusable correction; Error
// is the raw diagnostic and Outcome is retry/success/failure.
type Record struct {
	At        time.Time `json:"at"`
	Domain    string    `json:"domain"`
	ProjectID string    `json:"project_id,omitempty"`
	Attempt   int       `json:"attempt"`
	Stage     string    `json:"stage"`
	Error     string    `json:"error,omitempty"`
	Outcome   string    `json:"outcome"`
	Lesson    string    `json:"lesson,omitempty"`
	Source    string    `json:"source,omitempty"`
}

// Store is an append-only JSONL log. Appends are serialized within the
// process; the file itself remains the durable history.
type Store struct {
	mu   sync.Mutex
	path string
}

// Open returns a store for path. It does not create the file until Append.
func Open(path string) (*Store, error) {
	return &Store{path: strings.TrimSpace(path)}, nil
}

// Append writes one record. Empty paths and nil stores are inert.
func (s *Store) Append(record Record) error {
	if s == nil || s.path == "" {
		return nil
	}
	if record.At.IsZero() {
		record.At = time.Now()
	}
	if strings.TrimSpace(record.Domain) == "" {
		return errors.New("experience: domain vacío")
	}
	data, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("experience: serializar: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("experience: crear directorio: %w", err)
	}
	file, err := os.OpenFile(s.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("experience: abrir: %w", err)
	}
	defer file.Close()
	if _, err := file.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("experience: escribir: %w", err)
	}
	return nil
}

// Records returns the records for a domain. Corrupt lines are skipped; a
// missing file is an empty history.
func (s *Store) Records(domain string) ([]Record, error) {
	if s == nil || s.path == "" {
		return nil, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	file, err := os.Open(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("experience: leer: %w", err)
	}
	defer file.Close()
	var out []Record
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64<<10), 1<<20)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var record Record
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			continue
		}
		if domain != "" && record.Domain != domain {
			continue
		}
		out = append(out, record)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("experience: escanear: %w", err)
	}
	return out, nil
}

// Lessons returns the most recent unique non-empty lessons for a domain,
// newest last, capped at limit.
func (s *Store) Lessons(domain string, limit int) []string {
	if limit <= 0 {
		limit = 5
	}
	records, err := s.Records(domain)
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	var lessons []string
	for i := len(records) - 1; i >= 0 && len(lessons) < limit; i-- {
		lesson := strings.TrimSpace(records[i].Lesson)
		if lesson == "" || seen[lesson] {
			continue
		}
		seen[lesson] = true
		lessons = append(lessons, lesson)
	}
	// Return oldest-first so the prompt reads as a progression.
	for i, j := 0, len(lessons)-1; i < j; i, j = i+1, j-1 {
		lessons[i], lessons[j] = lessons[j], lessons[i]
	}
	return lessons
}

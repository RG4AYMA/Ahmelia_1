package ownerstate

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// SchemaVersion is the on-disk format version.
const SchemaVersion = 1

// StorePath is the live durable file. The content is ephemeral state with a
// TTL; it is not the UserModel and not a medical record.
const StorePath = "memory/owner_state.json"

type fileFormat struct {
	SchemaVersion  int         `json:"schema_version"`
	State          *State      `json:"state,omitempty"`
	Receptivity    Receptivity `json:"receptivity"`
	QuestionsAsked int         `json:"questions_asked"`
	LastProposalAt time.Time   `json:"last_proposal_at,omitempty"`
}

// Store owns the owner's ephemeral readings. An empty path keeps it in memory.
type Store struct {
	mu             sync.Mutex
	path           string
	now            func() time.Time
	state          *State
	receptivity    Receptivity
	questionsAsked int
	lastProposalAt time.Time
	warnings       []string
	corrupt        bool
}

// Open loads the store. A missing file is empty; a corrupt file is empty plus
// a warning and is quarantined before the first overwrite.
func Open(path string) (*Store, error) {
	s := &Store{path: strings.TrimSpace(path), now: time.Now}
	if s.path == "" {
		return s, nil
	}
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, fmt.Errorf("ownerstate: leer %s: %w", s.path, err)
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return s, nil
	}
	var f fileFormat
	if err := json.Unmarshal(data, &f); err != nil {
		s.corrupt = true
		s.warnings = append(s.warnings, "archivo corrupto: se ignora y se conserva una copia antes de sobrescribir")
		return s, nil
	}
	if f.SchemaVersion != 0 && f.SchemaVersion != SchemaVersion {
		s.corrupt = true
		s.warnings = append(s.warnings, fmt.Sprintf("schema_version %d no soportada: se ignora y se conserva una copia", f.SchemaVersion))
		return s, nil
	}
	s.state = f.State
	s.receptivity = f.Receptivity
	s.questionsAsked = f.QuestionsAsked
	s.lastProposalAt = f.LastProposalAt
	return s, nil
}

// SetClock injects the scene clock. It must be called before writes that use
// the store's own clock.
func (s *Store) SetClock(now func() time.Time) {
	if s == nil || now == nil {
		return
	}
	s.mu.Lock()
	s.now = now
	s.mu.Unlock()
}

// Save writes the current state. An empty path is a no-op.
func (s *Store) Save() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveLocked()
}

func (s *Store) saveLocked() error {
	if s.path == "" {
		return nil
	}
	if s.corrupt {
		backup := fmt.Sprintf("%s.corrupt-%d", s.path, time.Now().UnixNano())
		if err := os.Rename(s.path, backup); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("ownerstate: conservar copia corrupta: %w", err)
		}
		s.corrupt = false
		s.warnings = append(s.warnings, "copia corrupta conservada en "+backup)
	}
	data, err := json.MarshalIndent(fileFormat{
		SchemaVersion:  SchemaVersion,
		State:          s.state,
		Receptivity:    s.receptivity,
		QuestionsAsked: s.questionsAsked,
		LastProposalAt: s.lastProposalAt,
	}, "", "  ")
	if err != nil {
		return fmt.Errorf("ownerstate: serializar: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("ownerstate: crear directorio: %w", err)
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("ownerstate: escribir temporal: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		if removeErr := os.Remove(s.path); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			_ = os.Remove(tmp)
			return fmt.Errorf("ownerstate: reemplazar archivo: %w", err)
		}
		if retryErr := os.Rename(tmp, s.path); retryErr != nil {
			_ = os.Remove(tmp)
			return fmt.Errorf("ownerstate: renombrar archivo: %w", retryErr)
		}
	}
	return nil
}

// SetState records one ephemeral reading. Health states are normalized to
// sensitive + ephemeral; a durable or public state is rejected.
func (s *Store) SetState(state State) error {
	if s == nil {
		return errors.New("ownerstate: store nil")
	}
	if state.Privacy == "" {
		state.Privacy = PrivacySensitive
	}
	if state.Persist == "" {
		state.Persist = PersistEphemeral
	}
	if err := state.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	previous := s.state
	stored := state
	s.state = &stored
	if err := s.saveLocked(); err != nil {
		s.state = previous
		return err
	}
	return nil
}

// ClearState drops the ephemeral state immediately.
func (s *Store) ClearState() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state = nil
	return s.saveLocked()
}

// ActiveState returns the state only while its TTL is valid.
func (s *Store) ActiveState(now time.Time) (State, bool) {
	if s == nil {
		return State{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state == nil || !s.state.Active(now) {
		return State{}, false
	}
	return *s.state, true
}

// SetReceptivity records a short-lived receptivity reading.
func (s *Store) SetReceptivity(level string, ttl time.Duration, evidence []string) error {
	if s == nil {
		return errors.New("ownerstate: store nil")
	}
	level = strings.ToLower(strings.TrimSpace(level))
	switch level {
	case "low", "neutral", "high":
	default:
		level = "neutral"
	}
	if ttl <= 0 {
		ttl = time.Hour
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	previous := s.receptivity
	s.receptivity = Receptivity{
		Level:      level,
		Evidence:   append([]string(nil), evidence...),
		ObservedAt: now,
		ExpiresAt:  now.Add(ttl),
	}
	if err := s.saveLocked(); err != nil {
		s.receptivity = previous
		return err
	}
	return nil
}

// Receptivity returns the active reading, or neutral when stale/absent.
func (s *Store) Receptivity(now time.Time) Receptivity {
	if s == nil {
		return Receptivity{Level: "neutral"}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.receptivity.Active(now) {
		return Receptivity{Level: "neutral"}
	}
	return s.receptivity
}

// AskQuestion allows at most one question about the reason for rest.
func (s *Store) AskQuestion() bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.questionsAsked >= 1 {
		return false
	}
	s.questionsAsked++
	_ = s.saveLocked()
	return true
}

// QuestionsAsked returns the persisted counter.
func (s *Store) QuestionsAsked() int {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.questionsAsked
}

// ProposalWindowOpen reports whether an optional recreation proposal is allowed.
func (s *Store) ProposalWindowOpen(now time.Time, window time.Duration) bool {
	if s == nil {
		return false
	}
	if window <= 0 {
		window = 2 * time.Hour
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastProposalAt.IsZero() || now.Sub(s.lastProposalAt) >= window
}

// MarkProposal records the instant of an accepted recreation proposal.
func (s *Store) MarkProposal(now time.Time) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastProposalAt = now
	return s.saveLocked()
}

// Sweep clears expired readings and persists the transition when changed.
func (s *Store) Sweep(now time.Time) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	changed := false
	if s.state != nil && !s.state.Active(now) {
		s.state = nil
		changed = true
	}
	if !s.receptivity.Active(now) && !s.receptivity.ObservedAt.IsZero() {
		s.receptivity = Receptivity{}
		changed = true
	}
	if !changed {
		return nil
	}
	return s.saveLocked()
}

// Warnings returns load/quarantine notices.
func (s *Store) Warnings() []string {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.warnings...)
}

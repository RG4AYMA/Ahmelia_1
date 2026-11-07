package restriction

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

// StorePath is the live durable state. It is runtime state, not code.
const StorePath = "memory/owner_restrictions.json"

type fileFormat struct {
	SchemaVersion int                `json:"schema_version"`
	Restrictions  []OwnerRestriction `json:"restrictions"`
}

// Store owns the durable list. An empty path keeps it in memory, which is the
// inert default for callers that have not enabled owner restrictions.
type Store struct {
	mu       sync.Mutex
	path     string
	items    []OwnerRestriction
	warnings []string
	corrupt  bool
	seq      int
}

// Open loads the store. A missing file is an empty store. A corrupt file is an
// empty store plus a warning: the harness never invents a restriction, and the
// corrupt bytes are quarantined before the first overwrite.
func Open(path string) (*Store, error) {
	s := &Store{path: strings.TrimSpace(path)}
	if s.path == "" {
		return s, nil
	}
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, fmt.Errorf("restriction: leer %s: %w", s.path, err)
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
	s.items = append([]OwnerRestriction(nil), f.Restrictions...)
	s.seq = len(s.items)
	return s, nil
}

// Save writes the current list. An empty path is a no-op.
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
			return fmt.Errorf("restriction: conservar copia corrupta: %w", err)
		}
		s.corrupt = false
		s.warnings = append(s.warnings, "copia corrupta conservada en "+backup)
	}
	data, err := json.MarshalIndent(fileFormat{
		SchemaVersion: SchemaVersion,
		Restrictions:  s.items,
	}, "", "  ")
	if err != nil {
		return fmt.Errorf("restriction: serializar: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("restriction: crear directorio: %w", err)
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("restriction: escribir temporal: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		// Windows can refuse to replace an existing file. Remove the target and
		// retry once; the temporary file is the only copy at risk.
		if removeErr := os.Remove(s.path); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			_ = os.Remove(tmp)
			return fmt.Errorf("restriction: reemplazar archivo: %w", err)
		}
		if retryErr := os.Rename(tmp, s.path); retryErr != nil {
			_ = os.Remove(tmp)
			return fmt.Errorf("restriction: renombrar archivo: %w", retryErr)
		}
	}
	return nil
}

// Add validates and persists one restriction.
func (s *Store) Add(r OwnerRestriction) error {
	if s == nil {
		return ErrInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := r.Validate(); err != nil {
		return err
	}
	for _, existing := range s.items {
		if existing.ID == r.ID {
			return fmt.Errorf("%w: id duplicado %q", ErrInvalid, r.ID)
		}
	}
	s.items = append(s.items, r)
	if err := s.saveLocked(); err != nil {
		s.items = s.items[:len(s.items)-1]
		return err
	}
	return nil
}

// AdmitWorkInitiative creates the bounded restriction used by scene 1: it only
// subtracts Ahmelia's own work initiative and expires at the end of the local
// day. It does not touch the owner's authorized actions.
func (s *Store) AdmitWorkInitiative(statement string, source Source, turn string, now time.Time, loc *time.Location, evidence []string) (OwnerRestriction, error) {
	if s == nil {
		return OwnerRestriction{}, ErrInvalid
	}
	if strings.TrimSpace(string(source)) == "" {
		source = SourceExplicitOwner
	}
	expires := EndOfDay(now, loc)
	r := OwnerRestriction{
		ID:        s.nextID(now),
		Scope:     ScopeWorkInitiative,
		Statement: strings.TrimSpace(statement),
		Source:    source,
		Turn:      turn,
		Since:     now,
		ExpiresAt: &expires,
		Status:    StatusActive,
		Evidence:  append([]string(nil), evidence...),
	}
	if err := s.Add(r); err != nil {
		return OwnerRestriction{}, err
	}
	return r, nil
}

func (s *Store) nextID(now time.Time) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seq++
	return fmt.Sprintf("or_%d_%d", now.UnixNano(), s.seq)
}

// Lift ends a restriction explicitly and keeps the record in history.
func (s *Store) Lift(id, by string, at time.Time) (OwnerRestriction, error) {
	if s == nil {
		return OwnerRestriction{}, ErrNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	idx := -1
	for i := range s.items {
		if s.items[i].ID == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		return OwnerRestriction{}, ErrNotFound
	}
	r := s.items[idx]
	if r.IsExpired(at) {
		r.Status = StatusExpired
		expiredAt := at
		if r.ExpiresAt != nil {
			expiredAt = *r.ExpiresAt
		}
		r.LiftedAt = &expiredAt
		r.LiftedBy = "expiry"
		s.items[idx] = r
		_ = s.saveLocked()
		return OwnerRestriction{}, ErrNotActive
	}
	if r.Status != StatusActive && r.Status != StatusPending {
		return OwnerRestriction{}, ErrNotActive
	}
	if strings.TrimSpace(by) == "" {
		by = "owner"
	}
	original := r
	r.Status = StatusLifted
	r.LiftedAt = &at
	r.LiftedBy = by
	s.items[idx] = r
	if err := s.saveLocked(); err != nil {
		s.items[idx] = original
		return OwnerRestriction{}, err
	}
	return r, nil
}

// Sweep marks expired records and persists the transition. Callers can also
// rely on IsActive/Active, which already ignore expired records at read time.
func (s *Store) Sweep(now time.Time) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	changed := false
	for i := range s.items {
		r := &s.items[i]
		if (r.Status == StatusActive || r.Status == StatusPending) && r.IsExpired(now) {
			at := now
			if r.ExpiresAt != nil {
				at = *r.ExpiresAt
			}
			r.Status = StatusExpired
			r.LiftedAt = &at
			r.LiftedBy = "expiry"
			changed = true
		}
	}
	if !changed {
		return nil
	}
	return s.saveLocked()
}

// Active returns the restrictions that govern behavior at now.
func (s *Store) Active(now time.Time) []OwnerRestriction {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]OwnerRestriction, 0, len(s.items))
	for _, r := range s.items {
		if r.IsActive(now) {
			out = append(out, r)
		}
	}
	return out
}

// Pending returns records that need owner confirmation before governing.
func (s *Store) Pending() []OwnerRestriction {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]OwnerRestriction, 0)
	for _, r := range s.items {
		if r.Status == StatusPending {
			out = append(out, r)
		}
	}
	return out
}

// History returns every record, including lifted and expired ones.
func (s *Store) History() []OwnerRestriction {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]OwnerRestriction(nil), s.items...)
}

// ByID returns one record.
func (s *Store) ByID(id string) (OwnerRestriction, bool) {
	if s == nil {
		return OwnerRestriction{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range s.items {
		if r.ID == id {
			return r, true
		}
	}
	return OwnerRestriction{}, false
}

// WorkInitiativeActive is the scene-1 gate: true means Ahmelia's work
// initiative is closed until the restriction expires or the owner lifts it.
func (s *Store) WorkInitiativeActive(now time.Time) bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range s.items {
		if r.Scope == ScopeWorkInitiative && r.IsActive(now) {
			return true
		}
	}
	return false
}

// ActionBanned reports an exact normalized match against an active action_ban.
// Structural target matching belongs to the BF2 consumer, not to this store.
func (s *Store) ActionBanned(target string, now time.Time) bool {
	if s == nil {
		return false
	}
	want := strings.ToLower(strings.TrimSpace(target))
	if want == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range s.items {
		if r.Scope != ScopeActionBan || !r.IsActive(now) {
			continue
		}
		if strings.ToLower(strings.TrimSpace(r.Target)) == want {
			return true
		}
	}
	return false
}

// Warnings returns the load/quarantine notices accumulated by this store.
func (s *Store) Warnings() []string {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.warnings...)
}

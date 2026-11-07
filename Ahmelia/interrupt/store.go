package interrupt

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"
)

// StoreSchemaVersion is the on-disk format version.
const StoreSchemaVersion = 1

// StorePath is the live durable state. It is runtime state, not code.
const StorePath = "memory/interrupt_state.json"

// Blocked is a critical event that could not be delivered yet. It stays in the
// store with its reason and attempt count; it is never silently downgraded.
type Blocked struct {
	Event     Event     `json:"event"`
	Reason    string    `json:"reason"`
	BlockedAt time.Time `json:"blocked_at"`
	Attempts  int       `json:"attempts"`
}

// Expired is the recorded fate of a queued event whose TTL ended. Expiry is a
// recorded outcome, not a deletion.
type Expired struct {
	EventID   string    `json:"event_id"`
	Reason    string    `json:"reason"`
	ExpiredAt time.Time `json:"expired_at"`
}

type interruptFile struct {
	SchemaVersion int                  `json:"schema_version"`
	Notices       map[string]Notice    `json:"notices"`
	Blocked       map[string]Blocked   `json:"blocked"`
	Digest        []Event              `json:"digest"`
	Escalated     []Event              `json:"escalated"`
	Cooldowns     map[string]time.Time `json:"cooldowns"`
	Reminders     map[string]time.Time `json:"reminders"`
	Expired       []Expired            `json:"expired"`
}

// Store owns the durable notice state. An empty path keeps it in memory, which
// is the inert default for callers that have not enabled the scene.
type Store struct {
	mu        sync.Mutex
	path      string
	notices   map[string]Notice
	blocked   map[string]Blocked
	digest    []Event
	escalated []Event
	cooldowns map[string]time.Time
	reminders map[string]time.Time
	expired   []Expired
	warnings  []string
	corrupt   bool
}

// Open loads the store. A missing file is an empty store. A corrupt file is an
// empty store plus a warning and is quarantined before the first overwrite.
func Open(path string) (*Store, error) {
	s := &Store{
		path:      strings.TrimSpace(path),
		notices:   map[string]Notice{},
		blocked:   map[string]Blocked{},
		cooldowns: map[string]time.Time{},
		reminders: map[string]time.Time{},
	}
	if s.path == "" {
		return s, nil
	}
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, fmt.Errorf("interrupt: leer %s: %w", s.path, err)
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return s, nil
	}
	var f interruptFile
	if err := json.Unmarshal(data, &f); err != nil {
		s.corrupt = true
		s.warnings = append(s.warnings, "archivo corrupto: se ignora y se conserva una copia antes de sobrescribir")
		return s, nil
	}
	if f.SchemaVersion != 0 && f.SchemaVersion != StoreSchemaVersion {
		s.corrupt = true
		s.warnings = append(s.warnings, fmt.Sprintf("schema_version %d no soportada: se ignora y se conserva una copia", f.SchemaVersion))
		return s, nil
	}
	if f.Notices != nil {
		s.notices = f.Notices
	}
	if f.Blocked != nil {
		s.blocked = f.Blocked
	}
	s.digest = append([]Event(nil), f.Digest...)
	s.escalated = append([]Event(nil), f.Escalated...)
	if f.Cooldowns != nil {
		s.cooldowns = f.Cooldowns
	}
	if f.Reminders != nil {
		s.reminders = f.Reminders
	}
	s.expired = append([]Expired(nil), f.Expired...)
	return s, nil
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
			return fmt.Errorf("interrupt: conservar copia corrupta: %w", err)
		}
		s.corrupt = false
		s.warnings = append(s.warnings, "copia corrupta conservada en "+backup)
	}
	data, err := json.MarshalIndent(interruptFile{
		SchemaVersion: StoreSchemaVersion,
		Notices:       s.notices,
		Blocked:       s.blocked,
		Digest:        s.digest,
		Escalated:     s.escalated,
		Cooldowns:     s.cooldowns,
		Reminders:     s.reminders,
		Expired:       s.expired,
	}, "", "  ")
	if err != nil {
		return fmt.Errorf("interrupt: serializar: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("interrupt: crear directorio: %w", err)
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("interrupt: escribir temporal: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		if removeErr := os.Remove(s.path); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			_ = os.Remove(tmp)
			return fmt.Errorf("interrupt: reemplazar archivo: %w", err)
		}
		if retryErr := os.Rename(tmp, s.path); retryErr != nil {
			_ = os.Remove(tmp)
			return fmt.Errorf("interrupt: renombrar archivo: %w", retryErr)
		}
	}
	return nil
}

// Delivered returns the notice already delivered for an event. Duplicate
// events are answered from this map instead of being delivered twice.
func (s *Store) Delivered(eventID string) (Notice, bool) {
	if s == nil {
		return Notice{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	n, ok := s.notices[strings.TrimSpace(eventID)]
	return n, ok
}

// RecordDelivery stores the notice, clears any blocked entry for the same
// event and starts the channel cooldown.
func (s *Store) RecordDelivery(n Notice, now time.Time) error {
	if s == nil {
		return ErrInvalid
	}
	if strings.TrimSpace(n.EventID) == "" || strings.TrimSpace(n.ID) == "" {
		return fmt.Errorf("%w: aviso sin event_id o id", ErrInvalid)
	}
	if strings.TrimSpace(n.Channel) == "" {
		return fmt.Errorf("%w: aviso sin canal", ErrInvalid)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	n.DeliveredAt = now
	if n.DeviceID == "" {
		n.DeviceID = n.Channel
	}
	s.notices[n.EventID] = n
	delete(s.blocked, n.EventID)
	delete(s.reminders, n.EventID)
	s.cooldowns[n.Channel] = now
	return s.saveLocked()
}

// RecordRewritten updates the prose of an already delivered notice without
// changing its facts. It refuses to invent a new event.
func (s *Store) RecordRewritten(n Notice) error {
	if s == nil {
		return ErrInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	previous, ok := s.notices[n.EventID]
	if !ok {
		return fmt.Errorf("%w: no hay aviso previo para %s", ErrInvalid, n.EventID)
	}
	if previous.ID != n.ID {
		return fmt.Errorf("%w: el id del aviso no puede cambiar", ErrInvalid)
	}
	if !reflect.DeepEqual(n.AlreadyDone, previous.AlreadyDone) ||
		!reflect.DeepEqual(n.Options, previous.Options) ||
		n.DecisionNeeded != previous.DecisionNeeded {
		return fmt.Errorf("%w: una reescritura no puede cambiar acciones, opciones ni decisión", ErrInvalid)
	}
	n.DeviceID = previous.DeviceID
	n.DeliveredAt = previous.DeliveredAt
	n.Fingerprint = previous.Fingerprint
	s.notices[n.EventID] = n
	return s.saveLocked()
}

// Block records a critical event that could not be delivered. Repeated blocks
// increment the attempt count instead of creating a second entry.
func (s *Store) Block(e Event, reason string, at time.Time) error {
	if s == nil {
		return ErrInvalid
	}
	if err := e.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(reason) == "" {
		return fmt.Errorf("%w: motivo de bloqueo vacío", ErrInvalid)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.blocked[e.ID]
	if !ok {
		b = Blocked{Event: e, BlockedAt: at}
	}
	b.Reason = strings.TrimSpace(reason)
	b.Attempts++
	s.blocked[e.ID] = b
	return s.saveLocked()
}

// Pending returns the blocked criticals that have not expired at now.
func (s *Store) Pending(now time.Time) []Blocked {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Blocked, 0, len(s.blocked))
	for _, b := range s.blocked {
		if b.Event.ExpiresAt.IsZero() || now.Before(b.Event.ExpiresAt) {
			out = append(out, b)
		}
	}
	return out
}

// PendingOrdered returns the blocked criticals that have not expired at now,
// ordered by the same consequence/urgency/severity priority used by the score.
func (s *Store) PendingOrdered(now time.Time) []Blocked {
	pending := s.Pending(now)
	sort.SliceStable(pending, func(i, j int) bool {
		si, _ := Assess(pending[i].Event)
		sj, _ := Assess(pending[j].Event)
		if si.Priority() != sj.Priority() {
			return si.Priority() > sj.Priority()
		}
		if pending[i].Event.Urgency != pending[j].Event.Urgency {
			return pending[i].Event.Urgency < pending[j].Event.Urgency
		}
		return pending[i].BlockedAt.Before(pending[j].BlockedAt)
	})
	return pending
}

// Expire moves blocked events whose TTL ended into the recorded history and
// returns the newly expired entries. Nothing is deleted without a record.
func (s *Store) Expire(now time.Time) ([]Expired, error) {
	if s == nil {
		return nil, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var newly []Expired
	for id, b := range s.blocked {
		if b.Event.ExpiresAt.IsZero() || now.Before(b.Event.ExpiresAt) {
			continue
		}
		record := Expired{EventID: id, Reason: b.Reason, ExpiredAt: b.Event.ExpiresAt}
		s.expired = append(s.expired, record)
		newly = append(newly, record)
		delete(s.blocked, id)
	}
	if len(newly) == 0 {
		return nil, nil
	}
	if err := s.saveLocked(); err != nil {
		return nil, err
	}
	return newly, nil
}

// CooldownActive reports whether a channel delivered inside the window.
func (s *Store) CooldownActive(channel string, now time.Time, window time.Duration) bool {
	if s == nil || window <= 0 {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	last, ok := s.cooldowns[strings.TrimSpace(channel)]
	return ok && !now.Before(last) && now.Sub(last) < window
}

// MarkReminder records the single critical reminder for an event.
func (s *Store) MarkReminder(eventID string, at time.Time) error {
	if s == nil {
		return ErrInvalid
	}
	id := strings.TrimSpace(eventID)
	if id == "" {
		return fmt.Errorf("%w: event_id vacío", ErrInvalid)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.reminders == nil {
		s.reminders = map[string]time.Time{}
	}
	s.reminders[id] = at
	return s.saveLocked()
}

// ReminderSent reports whether the single reminder for an event was delivered.
func (s *Store) ReminderSent(eventID string) bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.reminders[strings.TrimSpace(eventID)]
	return ok
}

// ReminderCount returns how many critical reminders were delivered.
func (s *Store) ReminderCount() int {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.reminders)
}

// AddDigest appends a non-critical event to the deferred digest. Duplicates are
// ignored by event ID.
func (s *Store) AddDigest(e Event) error {
	if s == nil {
		return ErrInvalid
	}
	if err := e.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.digest {
		if existing.ID == e.ID {
			return nil
		}
	}
	s.digest = append(s.digest, e)
	return s.saveLocked()
}

// Digest returns a copy of the deferred events.
func (s *Store) Digest() []Event {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Event(nil), s.digest...)
}

// AddEscalated records an irreversible event below the interrupt threshold.
// It is never silently discarded.
func (s *Store) AddEscalated(e Event) error {
	if s == nil {
		return ErrInvalid
	}
	if err := e.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.escalated {
		if existing.ID == e.ID {
			return nil
		}
	}
	s.escalated = append(s.escalated, e)
	return s.saveLocked()
}

// Escalated returns a copy of the escalated events.
func (s *Store) Escalated() []Event {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Event(nil), s.escalated...)
}

// Notices returns a copy of every delivered notice.
func (s *Store) Notices() map[string]Notice {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]Notice, len(s.notices))
	for k, v := range s.notices {
		out[k] = v
	}
	return out
}

// BlockedList returns a copy of the blocked entries.
func (s *Store) BlockedList() []Blocked {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Blocked, 0, len(s.blocked))
	for _, b := range s.blocked {
		out = append(out, b)
	}
	return out
}

// History returns the recorded expirations.
func (s *Store) History() []Expired {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Expired(nil), s.expired...)
}

// Warnings returns load/quarantine notices accumulated by this store.
func (s *Store) Warnings() []string {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.warnings...)
}

package interaction

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// PersonalMemory is the durable experiential layer behind UserProfile.
//
// UserProfile is the current model ("what Ahmelia currently believes").
// PersonalMemory is the evidence history ("what happened and why the model
// believes it"). Keeping both separate lets later layers revise beliefs
// without destroying the conversational history that produced them.
type PersonalMemory struct {
	mu      sync.RWMutex
	Version int                   `json:"version"`
	Entries []PersonalMemoryEntry `json:"entries"`
}

type PersonalMemoryEntry struct {
	ID         uint64    `json:"id"`
	At         time.Time `json:"at"`
	Category   string    `json:"category"`
	Key        string    `json:"key"`
	Value      string    `json:"value"`
	Source     string    `json:"source"`
	Confidence float64   `json:"confidence"`
	SessionID  string    `json:"session_id,omitempty"`
	TurnID     uint64    `json:"turn_id,omitempty"`
}

const PersonalMemoryPath = "memory/personal_memory.json"

func NewPersonalMemory() *PersonalMemory {
	return &PersonalMemory{Version: 1, Entries: make([]PersonalMemoryEntry, 0, 128)}
}

func LoadPersonalMemory(path string) *PersonalMemory {
	data, err := os.ReadFile(path)
	if err != nil {
		return NewPersonalMemory()
	}
	var m PersonalMemory
	if json.Unmarshal(data, &m) != nil {
		return NewPersonalMemory()
	}
	if m.Version == 0 {
		m.Version = 1
	}
	if m.Entries == nil {
		m.Entries = make([]PersonalMemoryEntry, 0, 128)
	}
	return &m
}

func (m *PersonalMemory) Record(category, key, value, source, sessionID string, turnID uint64, confidence float64) PersonalMemoryEntry {
	if m == nil || strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" {
		return PersonalMemoryEntry{}
	}
	if confidence < 0 {
		confidence = 0
	}
	if confidence > 1 {
		confidence = 1
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	var next uint64
	if len(m.Entries) > 0 {
		next = m.Entries[len(m.Entries)-1].ID + 1
	}
	entry := PersonalMemoryEntry{
		ID: next, At: time.Now(), Category: strings.TrimSpace(category),
		Key: strings.TrimSpace(key), Value: strings.TrimSpace(value),
		Source: strings.TrimSpace(source), Confidence: confidence,
		SessionID: strings.TrimSpace(sessionID), TurnID: turnID,
	}
	m.Entries = append(m.Entries, entry)

	// Keep the evidence history bounded. The UserProfile remains the compact
	// current model, while this layer retains a useful recent experiential log.
	if len(m.Entries) > 2048 {
		m.Entries = append([]PersonalMemoryEntry(nil), m.Entries[len(m.Entries)-2048:]...)
	}
	return entry
}

func (m *PersonalMemory) Recent(limit int) []PersonalMemoryEntry {
	if m == nil || limit <= 0 {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if limit > len(m.Entries) {
		limit = len(m.Entries)
	}
	out := make([]PersonalMemoryEntry, limit)
	copy(out, m.Entries[len(m.Entries)-limit:])
	return out
}

func (m *PersonalMemory) Find(category, key string) []PersonalMemoryEntry {
	if m == nil {
		return nil
	}
	cat := strings.ToLower(strings.TrimSpace(category))
	k := strings.ToLower(strings.TrimSpace(key))
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]PersonalMemoryEntry, 0)
	for _, e := range m.Entries {
		if (cat == "" || strings.EqualFold(e.Category, cat)) &&
			(k == "" || strings.EqualFold(e.Key, k)) {
			out = append(out, e)
		}
	}
	return out
}

func (m *PersonalMemory) Save(path string) error {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	data, err := json.MarshalIndent(m, "", "  ")
	m.mu.RUnlock()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

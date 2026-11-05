package deliberation

// shadow_log.go — REGISTRO DE DELIBERACIÓN SOMBRA.
//
// El registro es evidencia local, no estado operativo. Se mantiene un anillo
// acotado en memoria para lectura inmediata y, si hay ruta configurada, una
// línea JSON por turno en disco. Un fallo de escritura nunca interrumpe el
// turno: la sombra jamás tiene autoridad sobre la conversación.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

const (
	// DefaultShadowCapacity acota la lectura en memoria.
	DefaultShadowCapacity = 256
	// ShadowLogPath es el registro vivo del proceso.
	ShadowLogPath = "memory/deliberation_shadow.jsonl"
)

type ShadowLog struct {
	mu       sync.Mutex
	path     string
	capacity int
	records  []Record
	lastErr  error
}

func NewShadowLog(path string) *ShadowLog {
	return &ShadowLog{path: path, capacity: DefaultShadowCapacity}
}

// SetPath redirects the persistent log. The in-memory ring is preserved.
func (l *ShadowLog) SetPath(path string) {
	if l == nil {
		return
	}
	l.mu.Lock()
	l.path = path
	l.mu.Unlock()
}

// Record appends the record to the in-memory ring and, when configured, to
// the JSONL file. The in-memory copy always succeeds unless the log is nil.
func (l *ShadowLog) Record(r Record) error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	l.records = append(l.records, r)
	if len(l.records) > l.capacity {
		l.records = l.records[len(l.records)-l.capacity:]
	}

	if l.path == "" {
		return nil
	}
	if dir := filepath.Dir(l.path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			l.lastErr = err
			return err
		}
	}
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		l.lastErr = err
		return err
	}
	defer f.Close()
	data, err := json.Marshal(r)
	if err != nil {
		l.lastErr = err
		return err
	}
	if _, err := f.Write(append(data, '\n')); err != nil {
		l.lastErr = err
		return err
	}
	l.lastErr = nil
	return nil
}

// Recent returns up to n records, newest last.
func (l *ShadowLog) Recent(n int) []Record {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if n <= 0 || n > len(l.records) {
		n = len(l.records)
	}
	out := make([]Record, n)
	copy(out, l.records[len(l.records)-n:])
	return out
}

// Counts summarizes outcomes since process start. Useful for the question
// "¿el modelo está de acuerdo con el arnés o lo está corrigiendo?".
func (l *ShadowLog) Counts() map[Outcome]int {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make(map[Outcome]int, 7)
	for _, r := range l.records {
		out[r.Outcome]++
	}
	return out
}

// LastError reports the last file write failure, if any.
func (l *ShadowLog) LastError() error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.lastErr
}

// Describe returns a one-line human summary for the console. It never
// includes hidden chain-of-thought: only the typed outcome and evidence count.
func (r Record) Describe() string {
	if r.Error != "" {
		return fmt.Sprintf("%s turn=%s error=%s", r.Outcome, r.TurnID, r.Error)
	}
	return fmt.Sprintf("%s turn=%s strategy=%q evidence=%d confidence=%.2f",
		r.Outcome, r.TurnID, r.Proposal.Strategy, len(r.Proposal.EvidenceIDs), r.Proposal.Confidence)
}

package expectation

// ledger.go — LIBRO DURABLE DE EXPECTATIVAS (PASO 4.1).
//
// Formato: JSONL append-only, una línea por declaración, veredicto o
// hipótesis. El libro tiene un solo escritor (el Engine vivo). No se
// reescribe una línea anterior: una corrección futura será otro registro.

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// LedgerPath is the live durable act. It is runtime state, not code.
const LedgerPath = "memory/expectations.jsonl"

var (
	// ErrDisabled means the ledger has no durable location.
	ErrDisabled = errors.New("expectation: libro no configurado")
	// ErrInvalid means a record cannot be appended under the contract.
	ErrInvalid = errors.New("expectation: registro inválido")
)

var idCounter uint64

// NewID returns a process-unique, sortable record identity.
func NewID(prefix string) string {
	if strings.TrimSpace(prefix) == "" {
		prefix = "exp"
	}
	return fmt.Sprintf("%s-%s-%06d", prefix, time.Now().UTC().Format("20060102T150405.000000000"), atomic.AddUint64(&idCounter, 1)%1000000)
}

// Ledger is a single-writer append-only JSONL store.
type Ledger struct {
	mu   sync.Mutex
	path string
}

func NewLedger(path string) *Ledger {
	return &Ledger{path: strings.TrimSpace(path)}
}

func (l *Ledger) Path() string {
	if l == nil {
		return ""
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.path
}

// SetPath changes the durable location. It never copies records between
// locations; the caller owns the decision to point at a different act.
func (l *Ledger) SetPath(path string) {
	if l == nil {
		return
	}
	l.mu.Lock()
	l.path = strings.TrimSpace(path)
	l.mu.Unlock()
}

// Ensure creates the parent directory and the file when missing. It never
// truncates an existing act.
func (l *Ledger) Ensure() error {
	if l == nil {
		return ErrDisabled
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.ensureLocked()
}

func (l *Ledger) ensureLocked() error {
	path := strings.TrimSpace(l.path)
	if path == "" {
		return ErrDisabled
	}
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	return f.Close()
}

// Declare appends one expectation and returns the stored copy with identity
// and timestamp filled. The declaration must be durable before execution: if
// this method fails, the caller must not publish a confirmed verdict.
func (l *Ledger) Declare(exp Expectation) (Expectation, error) {
	if l == nil {
		return exp, ErrDisabled
	}
	exp.Action = strings.TrimSpace(exp.Action)
	exp.Target = strings.TrimSpace(exp.Target)
	exp.WorkID = strings.TrimSpace(exp.WorkID)
	exp.Source = strings.TrimSpace(exp.Source)
	if exp.Source == "" {
		exp.Source = "harness"
	}
	if exp.Action == "" {
		return exp, fmt.Errorf("%w: falta la acción", ErrInvalid)
	}
	if len(exp.Facts) == 0 {
		return exp, fmt.Errorf("%w: falta al menos un hecho esperado", ErrInvalid)
	}
	for i := range exp.Facts {
		exp.Facts[i].Subject = strings.TrimSpace(exp.Facts[i].Subject)
		exp.Facts[i].Predicate = strings.TrimSpace(exp.Facts[i].Predicate)
		exp.Facts[i].Expected = strings.TrimSpace(exp.Facts[i].Expected)
		exp.Facts[i].Observed = ""
		if exp.Facts[i].Subject == "" || exp.Facts[i].Predicate == "" {
			return exp, fmt.Errorf("%w: hecho %d sin sujeto o predicado", ErrInvalid, i)
		}
	}
	if strings.TrimSpace(exp.ID) == "" {
		exp.ID = NewID("exp")
	}
	if exp.DeclaredAt.IsZero() {
		exp.DeclaredAt = time.Now().UTC()
	}
	rec := Record{Kind: RecordExpectation, Expectation: &exp}
	if err := l.append(rec); err != nil {
		return exp, err
	}
	return exp, nil
}

// Conclude appends one terminal verdict. A declaration status is never a
// verdict; the ledger refuses it instead of manufacturing closure.
func (l *Ledger) Conclude(verdict Verdict) error {
	if l == nil {
		return ErrDisabled
	}
	verdict.ExpectationID = strings.TrimSpace(verdict.ExpectationID)
	verdict.Source = strings.TrimSpace(verdict.Source)
	if verdict.ExpectationID == "" {
		return fmt.Errorf("%w: falta expectation_id", ErrInvalid)
	}
	if !verdict.Status.Terminal() {
		return fmt.Errorf("%w: %q no es un veredicto terminal", ErrInvalid, verdict.Status)
	}
	if verdict.ObservedAt.IsZero() {
		verdict.ObservedAt = time.Now().UTC()
	}
	rec := Record{Kind: RecordVerdict, Verdict: &verdict}
	return l.append(rec)
}

// Hypothesize appends a model proposal. It is always checkable=false and
// source=model: the model cannot turn prose into a harness criterion.
func (l *Ledger) Hypothesize(hyp Hypothesis) (Hypothesis, error) {
	if l == nil {
		return hyp, ErrDisabled
	}
	hyp.Text = strings.TrimSpace(hyp.Text)
	if hyp.Text == "" {
		return hyp, fmt.Errorf("%w: hipótesis vacía", ErrInvalid)
	}
	hyp.Source = "model"
	hyp.Checkable = false
	if strings.TrimSpace(hyp.ID) == "" {
		hyp.ID = NewID("hyp")
	}
	if hyp.RecordedAt.IsZero() {
		hyp.RecordedAt = time.Now().UTC()
	}
	rec := Record{Kind: RecordHypothesis, Hypothesis: &hyp}
	if err := l.append(rec); err != nil {
		return hyp, err
	}
	return hyp, nil
}

func (l *Ledger) append(rec Record) error {
	if l == nil {
		return ErrDisabled
	}
	data, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	data = append(data, '\n')

	l.mu.Lock()
	defer l.mu.Unlock()
	path := strings.TrimSpace(l.path)
	if path == "" {
		return ErrDisabled
	}
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// Entries reads the whole act in append order. Malformed lines are reported,
// but valid records before and after them are preserved: corruption is
// evidence and must not silently erase history.
func (l *Ledger) Entries() ([]Record, error) {
	if l == nil {
		return nil, ErrDisabled
	}
	l.mu.Lock()
	path := strings.TrimSpace(l.path)
	l.mu.Unlock()
	if path == "" {
		return nil, ErrDisabled
	}
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var records []Record
	var firstErr error
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	line := 0
	for scanner.Scan() {
		line++
		raw := strings.TrimSpace(scanner.Text())
		if raw == "" {
			continue
		}
		var rec Record
		if err := json.Unmarshal([]byte(raw), &rec); err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("expectation: línea %d inválida: %w", line, err)
			}
			continue
		}
		switch rec.Kind {
		case RecordExpectation:
			if rec.Expectation == nil {
				if firstErr == nil {
					firstErr = fmt.Errorf("expectation: línea %d sin expectativa", line)
				}
				continue
			}
		case RecordVerdict:
			if rec.Verdict == nil {
				if firstErr == nil {
					firstErr = fmt.Errorf("expectation: línea %d sin veredicto", line)
				}
				continue
			}
		case RecordHypothesis:
			if rec.Hypothesis == nil {
				if firstErr == nil {
					firstErr = fmt.Errorf("expectation: línea %d sin hipótesis", line)
				}
				continue
			}
		default:
			if firstErr == nil {
				firstErr = fmt.Errorf("expectation: línea %d con tipo desconocido %q", line, rec.Kind)
			}
			continue
		}
		records = append(records, rec)
	}
	if err := scanner.Err(); err != nil && firstErr == nil {
		firstErr = err
	}
	return records, firstErr
}

// LatestVerdict returns the most recent terminal verdict, if any.
func (l *Ledger) LatestVerdict() (Verdict, bool, error) {
	records, err := l.Entries()
	var latest Verdict
	found := false
	for i := len(records) - 1; i >= 0; i-- {
		if records[i].Kind == RecordVerdict && records[i].Verdict != nil {
			latest = *records[i].Verdict
			found = true
			break
		}
	}
	return latest, found, err
}

// Counts summarizes terminal verdicts. Declarations and hypotheses never
// count as outcomes.
func (l *Ledger) Counts() (map[Status]int, error) {
	records, err := l.Entries()
	counts := map[Status]int{
		StatusConfirmed:    0,
		StatusContradicted: 0,
		StatusInsufficient: 0,
	}
	for _, rec := range records {
		if rec.Kind != RecordVerdict || rec.Verdict == nil {
			continue
		}
		counts[rec.Verdict.Status]++
	}
	return counts, err
}

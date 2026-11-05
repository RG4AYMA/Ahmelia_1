package taste

// taste.go — GUSTOS PROPIOS CON EVIDENCIA (PASO 3.3).
//
// Un gusto propio no se declara porque el modelo lo diga: se forma con
// evidencia trazable. Este paquete posee las dos mitades del registro:
//
//   - candidatos: una experiencia de trabajo VERIFICADA. El arnés registra la
//     actividad y su referencia (work id, goal id), pero no la convierte en
//     gusto. Queda pendiente de confirmación del propietario.
//   - efectos: confirmaciones y correcciones del propietario. Solo estos
//     alimentan el posterior Beta (personality.PreferenceModel).
//
// Reglas de autoridad:
//
//   - El modelo no escribe gustos. Solo el propietario confirma o corrige.
//   - Un candidato sin referencia trazable no se registra.
//   - La confirmación es un acto explícito con ID exacto.
//   - La corrección borra la evidencia anterior de esa actividad y abre un
//     posterior negativo: la corrección pesa más que la inferencia previa.
//   - Sin MinN=5 el posterior no expresa gusto (StanceUnformed).
//
// El registro es JSONL append-only. Un evento se escribe antes de existir en
// memoria; un fallo de escritura falla cerrado.

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"ahmelia/personality"
)

// LedgerPath es el registro durable de experiencias y decisiones.
const LedgerPath = "memory/self_taste.jsonl"

// MaxPending acota las experiencias que esperan decisión para que el
// propietario no reciba una cola infinita.
const MaxPending = 8

type Status string

const (
	CandidatePending   Status = "pending"
	CandidateConfirmed Status = "confirmed"
	CandidateRejected  Status = "rejected"
)

var (
	ErrNotFound    = errors.New("taste: experiencia no encontrada")
	ErrNotPending  = errors.New("taste: la experiencia ya fue decidida")
	ErrPendingFull = errors.New("taste: demasiadas experiencias esperando confirmación")
	ErrNoEvidence  = errors.New("taste: falta actividad o referencia trazable")
)

// Candidate is one verified activity waiting for the owner's confirmation.
// It is evidence that the activity happened and was verified, never evidence
// that Ahmelia liked it.
type Candidate struct {
	ID         string    `json:"id"`
	Key        string    `json:"key"`
	Ref        string    `json:"ref"`
	RecordedAt time.Time `json:"recorded_at"`
	Status     Status    `json:"status"`
	DecidedAt  time.Time `json:"decided_at,omitempty"`
	Seq        int64     `json:"seq"`
}

// Correction is the owner's explicit correction of a formed taste. It is a
// full negative observation that replaces the previous evidence for the key.
type Correction struct {
	ID   string    `json:"id"`
	Key  string    `json:"key"`
	Ref  string    `json:"ref"`
	At   time.Time `json:"at"`
	Note string    `json:"note,omitempty"`
	Seq  int64     `json:"seq"`
}

type event struct {
	Kind       string      `json:"kind"` // candidate | correction
	Candidate  *Candidate  `json:"candidate,omitempty"`
	Correction *Correction `json:"correction,omitempty"`
}

var (
	idSeq        uint64
	candidateSeq int64
)

func nextID(prefix string) string {
	return fmt.Sprintf("%s-%d-%d", prefix, time.Now().UnixNano(), atomic.AddUint64(&idSeq, 1))
}

// NormalizeKey derives the stable activity key. It is deliberately blunt:
// lowercase, single spaces, surrounding punctuation removed, bounded length.
func NormalizeKey(value string) string {
	v := strings.ToLower(strings.TrimSpace(strings.Join(strings.Fields(value), " ")))
	v = strings.Trim(v, " .,;:¡!¿?\"'")
	if runes := []rune(v); len(runes) > 80 {
		v = string(runes[:80])
	}
	return v
}

type Ledger struct {
	mu          sync.Mutex
	path        string
	now         func() time.Time
	records     []Candidate
	index       map[string]int
	corrections []Correction
	seen        map[string]bool // key + "\x00" + ref
	lastErr     error
}

func NewLedger(path string) *Ledger {
	return &Ledger{path: path, now: time.Now, index: map[string]int{}, seen: map[string]bool{}}
}

func (l *Ledger) SetPath(path string) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.path = path
}

func (l *Ledger) SetClock(fn func() time.Time) {
	if l == nil || fn == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.now = fn
}

func (l *Ledger) Path() string {
	if l == nil {
		return ""
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.path
}

func (l *Ledger) LastError() error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.lastErr
}

func (l *Ledger) Load() error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if strings.TrimSpace(l.path) == "" {
		return nil
	}
	f, err := os.Open(l.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		l.lastErr = err
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var ev event
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			l.lastErr = fmt.Errorf("taste: línea inválida en %s: %w", l.path, err)
			continue
		}
		l.applyLocked(ev)
	}
	if err := scanner.Err(); err != nil {
		l.lastErr = err
		return err
	}
	return nil
}

func (l *Ledger) applyLocked(ev event) {
	switch ev.Kind {
	case "candidate":
		if ev.Candidate == nil || strings.TrimSpace(ev.Candidate.ID) == "" {
			return
		}
		rec := *ev.Candidate
		if idx, ok := l.index[rec.ID]; ok {
			l.records[idx] = rec
		} else {
			l.index[rec.ID] = len(l.records)
			l.records = append(l.records, rec)
		}
		l.seen[rec.Key+"\x00"+rec.Ref] = true
	case "correction":
		if ev.Correction == nil || strings.TrimSpace(ev.Correction.ID) == "" {
			return
		}
		l.corrections = append(l.corrections, *ev.Correction)
	}
}

// RecordVerified registers one verified activity as a pending candidate. The
// same activity+reference is recorded once; replay is idempotent.
func (l *Ledger) RecordVerified(key, ref string) (Candidate, bool, error) {
	if l == nil {
		return Candidate{}, false, errors.New("taste: registro no configurado")
	}
	key, ref = NormalizeKey(key), strings.TrimSpace(ref)
	if key == "" || ref == "" {
		return Candidate{}, false, ErrNoEvidence
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.seen[key+"\x00"+ref] {
		return Candidate{}, false, nil
	}
	pending := 0
	for _, c := range l.records {
		if c.Status == CandidatePending {
			pending++
		}
	}
	if pending >= MaxPending {
		return Candidate{}, false, ErrPendingFull
	}

	now := l.now()
	rec := Candidate{
		ID:         nextID("tg"),
		Key:        key,
		Ref:        ref,
		RecordedAt: now,
		Status:     CandidatePending,
		Seq:        atomic.AddInt64(&candidateSeq, 1),
	}
	ev := event{Kind: "candidate", Candidate: &rec}
	if err := l.appendLocked(ev); err != nil {
		return Candidate{}, false, err
	}
	l.applyLocked(ev)
	return rec, true, nil
}

func (l *Ledger) decide(id string, status Status) (Candidate, error) {
	if l == nil {
		return Candidate{}, errors.New("taste: registro no configurado")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return Candidate{}, ErrNotFound
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	idx, ok := l.index[id]
	if !ok {
		return Candidate{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	rec := l.records[idx]
	if rec.Status != CandidatePending {
		return Candidate{}, fmt.Errorf("%w: %s está en %s", ErrNotPending, id, rec.Status)
	}
	rec.Status = status
	rec.DecidedAt = l.now()
	ev := event{Kind: "candidate", Candidate: &rec}
	if err := l.appendLocked(ev); err != nil {
		return Candidate{}, err
	}
	l.applyLocked(ev)
	return rec, nil
}

func (l *Ledger) Confirm(id string) (Candidate, error) {
	return l.decide(id, CandidateConfirmed)
}

func (l *Ledger) Reject(id string) (Candidate, error) {
	return l.decide(id, CandidateRejected)
}

// Correct records the owner's correction. Ref must point to the owner's turn:
// a correction without traceable origin is not accepted.
func (l *Ledger) Correct(key, ref, note string) (Correction, error) {
	if l == nil {
		return Correction{}, errors.New("taste: registro no configurado")
	}
	key, ref = NormalizeKey(key), strings.TrimSpace(ref)
	if key == "" || ref == "" {
		return Correction{}, ErrNoEvidence
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	rec := Correction{
		ID:   nextID("tc"),
		Key:  key,
		Ref:  ref,
		At:   now,
		Note: strings.TrimSpace(note),
		Seq:  atomic.AddInt64(&candidateSeq, 1),
	}
	ev := event{Kind: "correction", Correction: &rec}
	if err := l.appendLocked(ev); err != nil {
		return Correction{}, err
	}
	l.applyLocked(ev)
	return rec, nil
}

func (l *Ledger) Get(id string) (Candidate, bool) {
	if l == nil {
		return Candidate{}, false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	idx, ok := l.index[strings.TrimSpace(id)]
	if !ok {
		return Candidate{}, false
	}
	return l.records[idx], true
}

// Pending returns candidates waiting for the owner, newest first.
func (l *Ledger) Pending() []Candidate {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]Candidate, 0, len(l.records))
	for _, c := range l.records {
		if c.Status == CandidatePending {
			out = append(out, c)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].RecordedAt.After(out[j].RecordedAt) })
	return out
}

// Effect is one ordered piece of evidence for the posterior. Outcome +1 is a
// confirmed experience; -1 is an owner correction.
type Effect struct {
	At      time.Time
	Key     string
	Ref     string
	Outcome float64
	Seq     int64
}

func (l *Ledger) Effects() []Effect {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]Effect, 0, len(l.records)+len(l.corrections))
	for _, c := range l.records {
		if c.Status != CandidateConfirmed || c.DecidedAt.IsZero() {
			continue
		}
		out = append(out, Effect{At: c.DecidedAt, Key: c.Key, Ref: c.Ref, Outcome: 1, Seq: c.Seq})
	}
	for _, c := range l.corrections {
		out = append(out, Effect{At: c.At, Key: c.Key, Ref: c.Ref, Outcome: -1, Seq: c.Seq})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].At.Equal(out[j].At) {
			return out[i].Seq < out[j].Seq
		}
		return out[i].At.Before(out[j].At)
	})
	return out
}

func (l *Ledger) appendLocked(ev event) error {
	if strings.TrimSpace(l.path) == "" {
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
	data, err := json.Marshal(ev)
	if err != nil {
		_ = f.Close()
		l.lastErr = err
		return err
	}
	_, werr := f.Write(append(data, '\n'))
	cerr := f.Close()
	if werr != nil {
		l.lastErr = werr
		return werr
	}
	if cerr != nil {
		l.lastErr = cerr
		return cerr
	}
	l.lastErr = nil
	return nil
}

// Model owns the posterior derived from the ledger. It never stores the
// posterior separately: Rebuild() replays the ledger, so evidence and stance
// cannot diverge.
type Model struct {
	mu     sync.Mutex
	ledger *Ledger
	pref   *personality.PreferenceModel
}

func NewModel(ledger *Ledger) *Model {
	m := &Model{ledger: ledger, pref: personality.NewPreferenceModel()}
	m.Rebuild()
	return m
}

func (m *Model) Ledger() *Ledger { return m.ledger }

func (m *Model) Rebuild() {
	if m == nil {
		return
	}
	pref := personality.NewPreferenceModel()
	if m.ledger != nil {
		for _, fx := range m.ledger.Effects() {
			if fx.Outcome > 0 {
				_ = pref.Observe(personality.Experience{
					Key: fx.Key, Outcome: 1, Weight: 1, Ref: fx.Ref, At: fx.At,
				})
				continue
			}
			_ = pref.Correct(fx.Key, fx.Ref, fx.At)
		}
	}
	m.mu.Lock()
	m.pref = pref
	m.mu.Unlock()
}

func (m *Model) RecordVerified(key, ref string) (Candidate, bool, error) {
	if m == nil || m.ledger == nil {
		return Candidate{}, false, errors.New("taste: modelo no configurado")
	}
	return m.ledger.RecordVerified(key, ref)
}

func (m *Model) Confirm(id string) (Candidate, error) {
	if m == nil || m.ledger == nil {
		return Candidate{}, errors.New("taste: modelo no configurado")
	}
	rec, err := m.ledger.Confirm(id)
	if err != nil {
		return Candidate{}, err
	}
	m.Rebuild()
	return rec, nil
}

func (m *Model) Reject(id string) (Candidate, error) {
	if m == nil || m.ledger == nil {
		return Candidate{}, errors.New("taste: modelo no configurado")
	}
	return m.ledger.Reject(id)
}

func (m *Model) Correct(key, ref, note string) (Correction, error) {
	if m == nil || m.ledger == nil {
		return Correction{}, errors.New("taste: modelo no configurado")
	}
	rec, err := m.ledger.Correct(key, ref, note)
	if err != nil {
		return Correction{}, err
	}
	m.Rebuild()
	return rec, nil
}

func (m *Model) Pending() []Candidate {
	if m == nil || m.ledger == nil {
		return nil
	}
	return m.ledger.Pending()
}

// Stance is the formed taste for one activity, with its traceable evidence.
type Stance struct {
	Key      string
	Stance   personality.Stance
	Mean     float64
	EffN     float64
	Evidence []string
	At       time.Time
}

// Stances returns only formed tastes (not StanceUnformed). Mixed is included:
// it is honest ambivalence, not a missing value.
func (m *Model) Stances() []Stance {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	pref := m.pref
	m.mu.Unlock()
	if pref == nil {
		return nil
	}
	var out []Stance
	for _, key := range pref.Keys() {
		v := pref.View(key)
		if v.Stance == personality.StanceUnformed {
			continue
		}
		out = append(out, Stance{
			Key: key, Stance: v.Stance, Mean: v.Mean, EffN: v.EffN,
			Evidence: append([]string(nil), v.Evidence...), At: v.At,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].EffN == out[j].EffN {
			return out[i].Key < out[j].Key
		}
		return out[i].EffN > out[j].EffN
	})
	return out
}

func (m *Model) View(key string) personality.View {
	if m == nil {
		return personality.View{Stance: personality.StanceUnformed}
	}
	m.mu.Lock()
	pref := m.pref
	m.mu.Unlock()
	if pref == nil {
		return personality.View{Stance: personality.StanceUnformed}
	}
	return pref.View(NormalizeKey(key))
}

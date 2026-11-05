package agency

// ledger.go — LIBRO DE METAS PROPIAS.
//
// El libro es evidencia y estado de propuesta, no una cola de ejecución. Una
// meta propia nace en estado "proposed" y solo cambia de estado por una
// decisión explícita del propietario (o por vencimiento). Autorizarla NO la
// registra en el planificador ni la ejecuta: el paso 3.1 termina en la puerta
// de autorización, no en la acción.
//
// El archivo es JSONL append-only. Cada línea es un evento; al cargar se
// reproduce el último estado por ID. Un fallo de escritura no deja el evento
// en memoria: el libro falla cerrado.

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
	"time"
)

// LedgerPath is the live proposal book. It is runtime evidence, not code.
const LedgerPath = "memory/agent_goals.jsonl"

const (
	defaultMaxOpen   = 1
	defaultMaxPerDay = 3
	defaultTTL       = 72 * time.Hour
	duplicateWindow  = 7 * 24 * time.Hour
)

// GoalStatus is the lifecycle of one self-proposed goal.
type GoalStatus string

const (
	GoalProposed   GoalStatus = "proposed"
	GoalAuthorized GoalStatus = "authorized"
	GoalRegistered GoalStatus = "registered"
	GoalRejected   GoalStatus = "rejected"
	GoalExpired    GoalStatus = "expired"
)

var (
	ErrQuota             = errors.New("agency: cupo de metas propias agotado")
	ErrDuplicate         = errors.New("agency: meta propia duplicada")
	ErrNotFound          = errors.New("agency: meta propia no encontrada")
	ErrNotProposed       = errors.New("agency: la meta ya no está en estado propuesto")
	ErrNotAuthorized     = errors.New("agency: la meta no está autorizada")
	ErrAlreadyRegistered = errors.New("agency: la meta ya está registrada en el planificador")
)

// GoalRecord is one proposal and its owner decision.
type GoalRecord struct {
	ID           string       `json:"id"`
	TurnID       string       `json:"turn_id,omitempty"`
	Contract     string       `json:"contract"`
	CreatedAt    time.Time    `json:"created_at"`
	DecidedAt    time.Time    `json:"decided_at,omitempty"`
	RegisteredAt time.Time    `json:"registered_at,omitempty"`
	Status       GoalStatus   `json:"status"`
	Reason       string       `json:"reason,omitempty"`
	Impulse      Impulse      `json:"impulse"`
	Goal         GoalProposal `json:"goal"`
	// PlannerGoalID links the record to the planning.Goal created after the
	// owner authorized it. Empty until the registration succeeds.
	PlannerGoalID string `json:"planner_goal_id,omitempty"`
	// PolicyVersion records which harness ceiling produced the record.
	PolicyVersion string `json:"policy_version,omitempty"`
}

type ledgerEvent struct {
	Event  string     `json:"event"`
	At     time.Time  `json:"at"`
	Record GoalRecord `json:"record"`
}

// Ledger keeps the bounded set of self-proposed goals. It is safe for
// concurrent use.
type Ledger struct {
	mu        sync.Mutex
	path      string
	records   []GoalRecord
	index     map[string]int
	maxOpen   int
	maxPerDay int
	ttl       time.Duration
	now       func() time.Time
	lastErr   error
}

// NewLedger creates an in-memory book. Call Load to replay an existing file.
func NewLedger(path string) *Ledger {
	return &Ledger{
		path:      path,
		index:     map[string]int{},
		maxOpen:   defaultMaxOpen,
		maxPerDay: defaultMaxPerDay,
		ttl:       defaultTTL,
		now:       time.Now,
	}
}

// SetPolicy changes the harness ceiling. Zero or negative values keep the
// current value. It exists for tests and for an owner who wants a stricter
// ceiling; it never widens authority by itself.
func (l *Ledger) SetPolicy(maxOpen, maxPerDay int, ttl time.Duration) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if maxOpen > 0 {
		l.maxOpen = maxOpen
	}
	if maxPerDay > 0 {
		l.maxPerDay = maxPerDay
	}
	if ttl > 0 {
		l.ttl = ttl
	}
}

// SetClock replaces the time source. Tests use it; live code does not.
func (l *Ledger) SetClock(now func() time.Time) {
	if l == nil || now == nil {
		return
	}
	l.mu.Lock()
	l.now = now
	l.mu.Unlock()
}

// SetPath redirects the append-only file. The in-memory state is preserved.
func (l *Ledger) SetPath(path string) {
	if l == nil {
		return
	}
	l.mu.Lock()
	l.path = path
	l.mu.Unlock()
}

// Path reports the current file, for diagnostics.
func (l *Ledger) Path() string {
	if l == nil {
		return ""
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.path
}

// Load replays the JSONL file. A missing file is an empty book, not an error.
func (l *Ledger) Load() error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.loadLocked()
}

func (l *Ledger) loadLocked() error {
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
		var ev ledgerEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			// A corrupt line is preserved on disk and skipped in memory. The
			// error is visible through LastError; analysis never stops.
			l.lastErr = fmt.Errorf("agency: línea inválida en %s: %w", l.path, err)
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

func (l *Ledger) applyLocked(ev ledgerEvent) {
	rec := ev.Record
	if strings.TrimSpace(rec.ID) == "" {
		return
	}
	if idx, ok := l.index[rec.ID]; ok {
		l.records[idx] = rec
		return
	}
	l.index[rec.ID] = len(l.records)
	l.records = append(l.records, rec)
}

// LastError reports the last persistence or replay error.
func (l *Ledger) LastError() error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.lastErr
}

// Propose records one grounded self-goal proposal. It applies the harness
// ceiling: one open proposal, a daily cap, a duplicate window and a TTL.
func (l *Ledger) Propose(turnID string, impulse Impulse, goal GoalProposal) (GoalRecord, error) {
	if l == nil {
		return GoalRecord{}, errors.New("agency: libro no configurado")
	}
	if err := goal.Validate(); err != nil {
		return GoalRecord{}, err
	}
	if len(goal.EvidenceIDs) == 0 {
		return GoalRecord{}, errors.New("agency: la meta propia no tiene evidencia válida")
	}
	if strings.TrimSpace(impulse.Reason) == "" {
		return GoalRecord{}, errors.New("agency: el impulso no tiene razón")
	}
	if impulse.Strength < 0 {
		impulse.Strength = 0
	}
	if impulse.Strength > 1 {
		impulse.Strength = 1
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	if _, err := l.expireLocked(now); err != nil {
		return GoalRecord{}, err
	}
	if open := l.openCountLocked(); open >= l.maxOpen {
		return GoalRecord{}, fmt.Errorf("%w: ya hay %d propuesta abierta", ErrQuota, open)
	}
	if created := l.createdTodayLocked(now); created >= l.maxPerDay {
		return GoalRecord{}, fmt.Errorf("%w: %d propuestas creadas hoy", ErrQuota, created)
	}
	if l.duplicateLocked(goal.Title, now) {
		return GoalRecord{}, fmt.Errorf("%w: %q", ErrDuplicate, goal.Title)
	}

	rec := GoalRecord{
		ID:            l.nextIDLocked(now),
		TurnID:        strings.TrimSpace(turnID),
		Contract:      "B96.41.1-agency",
		CreatedAt:     now,
		Status:        GoalProposed,
		Impulse:       impulse,
		Goal:          goal,
		PolicyVersion: fmt.Sprintf("open=%d/day=%d/ttl=%s", l.maxOpen, l.maxPerDay, l.ttl),
	}
	ev := ledgerEvent{Event: string(GoalProposed), At: now, Record: rec}
	if err := l.appendLocked(ev); err != nil {
		return GoalRecord{}, err
	}
	l.applyLocked(ev)
	return cloneRecord(rec), nil
}

// Authorize marks a proposal as accepted by the owner. It does not register
// the goal in the planner and does not execute anything.
func (l *Ledger) Authorize(id, reason string) (GoalRecord, error) {
	return l.decide(id, GoalAuthorized, reason)
}

// Reject marks a proposal as declined by the owner.
func (l *Ledger) Reject(id, reason string) (GoalRecord, error) {
	return l.decide(id, GoalRejected, reason)
}

// MarkRegistered links an authorized proposal to the planner goal created at
// the owner's request. It is a second explicit act: authorizing the proposal
// never registers anything by itself.
func (l *Ledger) MarkRegistered(id, plannerGoalID string) (GoalRecord, error) {
	if l == nil {
		return GoalRecord{}, errors.New("agency: libro no configurado")
	}
	plannerGoalID = strings.TrimSpace(plannerGoalID)
	if plannerGoalID == "" {
		return GoalRecord{}, errors.New("agency: planner goal id vacío")
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	if _, err := l.expireLocked(now); err != nil {
		return GoalRecord{}, err
	}
	idx, ok := l.index[strings.TrimSpace(id)]
	if !ok {
		return GoalRecord{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	rec := l.records[idx]
	if rec.PlannerGoalID != "" {
		return GoalRecord{}, fmt.Errorf("%w: %s -> %s", ErrAlreadyRegistered, id, rec.PlannerGoalID)
	}
	if rec.Status != GoalAuthorized {
		return GoalRecord{}, fmt.Errorf("%w: %s está en %s", ErrNotAuthorized, id, rec.Status)
	}
	rec.Status = GoalRegistered
	rec.PlannerGoalID = plannerGoalID
	rec.RegisteredAt = now
	ev := ledgerEvent{Event: string(GoalRegistered), At: now, Record: rec}
	if err := l.appendLocked(ev); err != nil {
		return GoalRecord{}, err
	}
	l.applyLocked(ev)
	return cloneRecord(rec), nil
}

func (l *Ledger) decide(id string, status GoalStatus, reason string) (GoalRecord, error) {
	if l == nil {
		return GoalRecord{}, errors.New("agency: libro no configurado")
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	if _, err := l.expireLocked(now); err != nil {
		return GoalRecord{}, err
	}
	idx, ok := l.index[strings.TrimSpace(id)]
	if !ok {
		return GoalRecord{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	rec := l.records[idx]
	if rec.Status != GoalProposed {
		return GoalRecord{}, fmt.Errorf("%w: %s está en %s", ErrNotProposed, id, rec.Status)
	}
	rec.Status = status
	rec.DecidedAt = now
	rec.Reason = strings.TrimSpace(reason)
	ev := ledgerEvent{Event: string(status), At: now, Record: rec}
	if err := l.appendLocked(ev); err != nil {
		return GoalRecord{}, err
	}
	l.records[idx] = rec
	return cloneRecord(rec), nil
}

// Expire closes proposals older than the TTL. It returns how many changed.
func (l *Ledger) Expire(now time.Time) (int, error) {
	if l == nil {
		return 0, nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if now.IsZero() {
		now = l.now()
	}
	return l.expireLocked(now)
}

func (l *Ledger) expireLocked(now time.Time) (int, error) {
	changed := 0
	for idx := range l.records {
		rec := l.records[idx]
		if rec.Status != GoalProposed || now.Sub(rec.CreatedAt) < l.ttl {
			continue
		}
		rec.Status = GoalExpired
		rec.DecidedAt = now
		if rec.Reason == "" {
			rec.Reason = "venció sin decisión del propietario"
		}
		ev := ledgerEvent{Event: string(GoalExpired), At: now, Record: rec}
		if err := l.appendLocked(ev); err != nil {
			return changed, err
		}
		l.records[idx] = rec
		changed++
	}
	return changed, nil
}

// Open returns proposals still waiting for the owner, newest first.
func (l *Ledger) Open() []GoalRecord {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	_, _ = l.expireLocked(l.now())
	var out []GoalRecord
	for _, rec := range l.records {
		if rec.Status == GoalProposed {
			out = append(out, cloneRecord(rec))
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

// All returns every record, oldest first.
func (l *Ledger) All() []GoalRecord {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]GoalRecord, len(l.records))
	for i, rec := range l.records {
		out[i] = cloneRecord(rec)
	}
	return out
}

// Get returns one record by ID.
func (l *Ledger) Get(id string) (GoalRecord, bool) {
	if l == nil {
		return GoalRecord{}, false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	idx, ok := l.index[strings.TrimSpace(id)]
	if !ok {
		return GoalRecord{}, false
	}
	return cloneRecord(l.records[idx]), true
}

// Counts summarizes the book by status.
func (l *Ledger) Counts() map[GoalStatus]int {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make(map[GoalStatus]int, 4)
	for _, rec := range l.records {
		out[rec.Status]++
	}
	return out
}

func (l *Ledger) openCountLocked() int {
	n := 0
	for _, rec := range l.records {
		if rec.Status == GoalProposed {
			n++
		}
	}
	return n
}

func (l *Ledger) createdTodayLocked(now time.Time) int {
	n := 0
	for _, rec := range l.records {
		if sameDay(rec.CreatedAt, now) {
			n++
		}
	}
	return n
}

func (l *Ledger) duplicateLocked(title string, now time.Time) bool {
	key := normalizeTitle(title)
	if key == "" {
		return false
	}
	for _, rec := range l.records {
		if rec.Status == GoalRejected {
			continue
		}
		if now.Sub(rec.CreatedAt) > duplicateWindow {
			continue
		}
		if normalizeTitle(rec.Goal.Title) == key {
			return true
		}
	}
	return false
}

func (l *Ledger) nextIDLocked(now time.Time) string {
	base := fmt.Sprintf("ag-%d", now.UnixNano())
	id := base
	for n := 1; ; n++ {
		if _, exists := l.index[id]; !exists {
			return id
		}
		id = fmt.Sprintf("%s-%d", base, n)
	}
}

func (l *Ledger) appendLocked(ev ledgerEvent) error {
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

func cloneRecord(rec GoalRecord) GoalRecord {
	out := rec
	out.Impulse.Evidence = append([]string(nil), rec.Impulse.Evidence...)
	out.Goal.EvidenceIDs = append([]string(nil), rec.Goal.EvidenceIDs...)
	out.Goal.RejectedEvidenceIDs = append([]string(nil), rec.Goal.RejectedEvidenceIDs...)
	return out
}

func normalizeTitle(title string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(title))), " ")
}

func sameDay(a, b time.Time) bool {
	y1, m1, d1 := a.In(b.Location()).Date()
	y2, m2, d2 := b.Date()
	return y1 == y2 && m1 == m2 && d1 == d2
}

package persistentmode

// mode.go — MODO PERSISTENTE ELEGIDO POR EL PROPIETARIO (PASO 3.4).
//
// Hasta ahora los modos de conversación se calculaban por turno. Este paquete
// posee otra cosa: una postura durable que el propietario elige y que
// sobrevive a los turnos y a los arranques.
//
// Reglas de autoridad:
//
//   - el propietario es el único que cambia el modo;
//   - el modelo puede verlo y sugerirlo, nunca aplicarlo;
//   - el arnés lo aplica de forma acotada: sesgo de expresión, impulsos,
//     iniciativa y cupo de metas;
//   - un archivo inválido no se repara ni se adivina: el llamador deshabilita
//     el mecanismo (fail closed).
//
// El paquete no importa core, cognition ni deliberation. Solo posee el estado
// y el perfil, para que no haya dos dueños del mismo vocabulario.

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"ahmelia/agency"
	"ahmelia/shared"
)

// StorePath is the live durable state. It is runtime state, not code.
const StorePath = "memory/persistent_mode.json"

// Mode is the closed vocabulary owned by agency. Alias keeps call sites short
// without creating a second enum.
type Mode = agency.Mode

const (
	ModeUnknown   = agency.ModeUnknown
	ModeFocus     = agency.ModeFocus
	ModeExplore   = agency.ModeExplore
	ModeRest      = agency.ModeRest
	ModeCompanion = agency.ModeCompanion

	// DefaultMode is the owner's decision for an installation with no stored
	// choice: warm and conversational, without pushing work.
	DefaultMode = ModeCompanion
)

var (
	// ErrInvalidMode rejects values outside the closed vocabulary.
	ErrInvalidMode = errors.New("persistentmode: modo desconocido")
	// ErrDisabled means the mechanism was not enabled in this process.
	ErrDisabled = errors.New("persistentmode: modo persistente no habilitado")
)

// State is the durable owner decision.
type State struct {
	Mode      Mode      `json:"mode"`
	ChangedBy string    `json:"changed_by"`
	ChangedAt time.Time `json:"changed_at,omitempty"`
}

// DefaultState is the state used when no file exists yet.
func DefaultState() State {
	return State{Mode: DefaultMode, ChangedBy: "default"}
}

// Profile is the bounded behavior the harness applies for a mode. It is a
// posture, not an authority: every field is consumed by an owner that already
// exists.
type Profile struct {
	Label        string
	Description  string
	Warmth       float64
	Initiative   float64
	DetailLevel  float64
	Impulses     bool
	Proactive    bool
	PlannerQuota int
}

// ProfileFor returns the profile for one mode. An unknown mode is treated as
// the default so callers that already validated the state cannot widen
// authority by accident.
func ProfileFor(mode Mode) Profile {
	switch mode {
	case ModeFocus:
		return Profile{
			Label:        "foco",
			Description:  "Priorizo producción, detalle y metas propias.",
			Warmth:       0.40,
			Initiative:   0.10,
			DetailLevel:  0.85,
			Impulses:     true,
			Proactive:    true,
			PlannerQuota: 1,
		}
	case ModeExplore:
		return Profile{
			Label:        "exploración",
			Description:  "Abro alternativas y converso antes de cerrar una decisión.",
			Warmth:       0.60,
			Initiative:   0.70,
			DetailLevel:  0.70,
			Impulses:     true,
			Proactive:    true,
			PlannerQuota: 1,
		}
	case ModeRest:
		return Profile{
			Label:        "descanso",
			Description:  "No abro impulsos, iniciativa ni cupo de metas.",
			Warmth:       0.80,
			Initiative:   0.00,
			DetailLevel:  0.20,
			Impulses:     false,
			Proactive:    false,
			PlannerQuota: 0,
		}
	case ModeCompanion, ModeUnknown:
		return Profile{
			Label:        "compañía",
			Description:  "Soy cálida y conversacional, sin empujar trabajo ni iniciativa fuerte.",
			Warmth:       0.90,
			Initiative:   0.20,
			DetailLevel:  0.50,
			Impulses:     true,
			Proactive:    true,
			PlannerQuota: 1,
		}
	default:
		return ProfileFor(DefaultMode)
	}
}

// Label returns the Spanish label for a mode.
func Label(mode Mode) string { return ProfileFor(mode).Label }

// Normalize canonicalizes one raw mode string.
func Normalize(raw string) Mode { return agency.NormalizeMode(raw) }

// aliasEntry keeps the regex next to the mode it names. The slice is ordered,
// not a map, so ModesInText is deterministic.
type aliasEntry struct {
	mode Mode
	re   *regexp.Regexp
}

var aliasPatterns = []aliasEntry{
	{ModeFocus, regexp.MustCompile(`\b(focus|foco|concentracion|concentrado)\b`)},
	{ModeFocus, regexp.MustCompile(`\bmodo\s+(de\s+)?trabajo\b`)},
	{ModeExplore, regexp.MustCompile(`\b(explore|exploracion|explorar|explora|exploratorio)\b`)},
	{ModeRest, regexp.MustCompile(`\b(rest|descanso|descansar|descansa|reposo|reposar|reposa)\b`)},
	{ModeCompanion, regexp.MustCompile(`\b(companion|compania|acompanar|acompana|acompanamiento|acompanante)\b`)},
}

// ModesInText returns the distinct modes named in already-normalized text.
// The order is the package order, not the order of appearance; callers only
// need to know whether there is exactly one.
func ModesInText(norm string) []Mode {
	if strings.TrimSpace(norm) == "" {
		return nil
	}
	seen := make(map[Mode]bool, 4)
	out := make([]Mode, 0, 4)
	for _, entry := range aliasPatterns {
		if entry.re.MatchString(norm) && !seen[entry.mode] {
			seen[entry.mode] = true
			out = append(out, entry.mode)
		}
	}
	return out
}

// Store owns the durable state. It is safe for concurrent use.
type Store struct {
	mu    sync.Mutex
	path  string
	state State
}

// NewStore creates an in-memory store. Call Load to read an existing file.
func NewStore(path string) *Store {
	return &Store{path: path, state: DefaultState()}
}

// SetPath changes the durable location without changing the in-memory state.
func (s *Store) SetPath(path string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.path = path
	s.mu.Unlock()
}

// Path returns the current durable location.
func (s *Store) Path() string {
	if s == nil {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.path
}

// Current returns a copy of the current state.
func (s *Store) Current() State {
	if s == nil {
		return DefaultState()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}

// Load reads the durable state. A missing file is not an error: it installs
// the default mode. A malformed file is an error and leaves the default state
// untouched; the caller decides whether to disable the mechanism.
func (s *Store) Load() error {
	if s == nil {
		return ErrDisabled
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	path := strings.TrimSpace(s.path)
	if path == "" {
		s.state = DefaultState()
		return nil
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		s.state = DefaultState()
		return nil
	}
	if err != nil {
		return err
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return fmt.Errorf("persistentmode: estado inválido: %w", err)
	}
	if !state.Mode.Valid() {
		return fmt.Errorf("%w: %q", ErrInvalidMode, state.Mode)
	}
	if strings.TrimSpace(state.ChangedBy) == "" {
		state.ChangedBy = "unknown"
	}
	s.state = state
	return nil
}

// Ensure writes the current in-memory state when the durable location has no
// file yet. It never overwrites an existing file.
func (s *Store) Ensure() error {
	if s == nil {
		return ErrDisabled
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	path := strings.TrimSpace(s.path)
	if path == "" {
		return nil
	}
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return s.writeLocked(s.state)
}

// Set validates and persists one owner decision. The in-memory state changes
// only after the write succeeds.
func (s *Store) Set(mode Mode, by string) (State, error) {
	if s == nil {
		return State{}, ErrDisabled
	}
	if !mode.Valid() {
		return State{}, fmt.Errorf("%w: %q", ErrInvalidMode, mode)
	}
	by = strings.TrimSpace(by)
	if by == "" {
		by = "owner"
	}
	next := State{Mode: mode, ChangedBy: by, ChangedAt: time.Now().UTC()}

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.writeLocked(next); err != nil {
		return s.state, err
	}
	s.state = next
	return next, nil
}

func (s *Store) writeLocked(state State) error {
	path := strings.TrimSpace(s.path)
	if path == "" {
		return nil
	}
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// ParseModeWord maps one normalized word to a mode. It exists for callers
// that already isolated the token (for example an assignment).
func ParseModeWord(norm string) Mode {
	token := shared.Normalize(norm)
	if mode := agency.NormalizeMode(token); mode.Valid() {
		return mode
	}
	for _, mode := range ModesInText(token) {
		return mode
	}
	return ModeUnknown
}

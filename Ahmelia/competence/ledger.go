// Package competence convierte resultados VERIFICADOS en afirmaciones de
// mejora con respaldo estadístico. Nunca cuenta Success sin Verified.
package competence

import (
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

type Attempt struct {
	Capability   string    `json:"capability"`
	Operation    string    `json:"operation"`
	ContextKey   string    `json:"context_key"` // clase de proyecto/entorno (transferencia)
	Strategy     string    `json:"strategy"`    // versión de estrategia usada
	Verified     bool      `json:"verified"`
	FailureClass string    `json:"failure_class,omitempty"`
	At           time.Time `json:"at"`
	// Token es la clave ESTABLE deduplicación del feed idempotente (P2,
	// directiva Paso 1.2): el mismo intento real reconstruido en cada boot
	// lleva el mismo Token y EnsureAll no lo duplica. Vacío = registro
	// directo sin deduplicación (comportamiento de Record).
	Token string `json:"token,omitempty"`
}

type Claim string

const (
	ClaimInsufficient Claim = "insufficient_evidence"
	ClaimImproved     Claim = "improved"
	ClaimRegressed    Claim = "regressed"
	ClaimNoChange     Claim = "no_detectable_change"
)

type Report struct {
	N              int
	Window         int
	Baseline       Interval
	Recent         Interval
	Claim          Claim
	TransferCtx    int            // contextos DISTINTOS con éxito verificado
	FailureClasses map[string]int // dónde falla
}

type Interval struct{ Rate, Lo, Hi float64 }

// wilson: intervalo de Wilson al 90% (z=1.645), robusto con n pequeño.
func wilson(k, n int) Interval {
	if n == 0 {
		return Interval{}
	}
	z := 1.645
	p := float64(k) / float64(n)
	d := 1 + z*z/float64(n)
	c := (p + z*z/(2*float64(n))) / d
	m := z * math.Sqrt(p*(1-p)/float64(n)+z*z/(4*float64(n)*float64(n))) / d
	return Interval{Rate: p, Lo: math.Max(0, c-m), Hi: math.Min(1, c+m)}
}

type Ledger struct {
	mu   sync.Mutex
	path string
	data []Attempt
}

func Load(path string) (*Ledger, error) {
	l := &Ledger{path: path}
	if path == "" {
		return l, nil
	}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return l, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &l.data); err != nil {
		return nil, err // corrupción NO se traga: se reporta
	}
	return l, nil
}

func (l *Ledger) Record(a Attempt) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.data = append(l.data, a)
	return l.save()
}

// EnsureAll es el feed idempotente del bloque P2 (directiva DeepSeek Paso
// 1.2): reconstruye intentos de TRABAJO REAL persistido sin duplicar. Con
// Token, un intento ya visto se ignora — dos boots desde el mismo work store
// no remplazan el registro ni duplican intentos. Sin Token, el intento entra
// directo. Devuelve cuántos se AGREGARON; si el guardado falla, nada queda
// agregado en memoria (o el estado queda coherente con el disco) y se
// devuelve error — nunca una mitad registrada.
func (l *Ledger) EnsureAll(as []Attempt) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	existing := map[string]bool{}
	seen := map[string]bool{}
	for _, a := range l.data {
		if a.Token != "" {
			existing[a.Token] = true
			seen[a.Token] = true
		}
	}
	fresh := make([]Attempt, 0, len(as))
	for _, a := range as {
		if a.Token == "" || (!existing[a.Token] && !seen[a.Token]) {
			fresh = append(fresh, a)
			if a.Token != "" {
				seen[a.Token] = true
			}
		}
	}
	if len(fresh) == 0 {
		return 0, nil
	}
	backup := l.data
	l.data = append(append([]Attempt{}, backup...), fresh...)
	if err := l.save(); err != nil {
		l.data = backup
		return 0, err
	}
	return len(fresh), nil
}

func (l *Ledger) save() error {
	if l.path == "" {
		return nil
	}
	b, err := json.Marshal(l.data)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(l.path), 0o755); err != nil {
		return err
	}
	tmp := l.path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err = f.Write(b); err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	return os.Rename(tmp, l.path)
}

// Report afirma mejora SOLO si el intervalo reciente queda estrictamente por
// encima del de línea base (intervalos disjuntos) con ventanas disjuntas.
func (l *Ledger) Report(capability, operation string, window int) Report {
	if window <= 0 {
		window = 10
	}
	l.mu.Lock()
	var xs []Attempt
	for _, a := range l.data {
		if a.Capability == capability && a.Operation == operation {
			xs = append(xs, a)
		}
	}
	l.mu.Unlock()
	sort.SliceStable(xs, func(i, j int) bool { return xs[i].At.Before(xs[j].At) })
	r := Report{N: len(xs), Window: window, Claim: ClaimInsufficient, FailureClasses: map[string]int{}}
	ctx := map[string]bool{}
	for _, x := range xs {
		if x.Verified {
			ctx[x.ContextKey] = true
		} else if x.FailureClass != "" {
			r.FailureClasses[x.FailureClass]++
		}
	}
	r.TransferCtx = len(ctx)
	count := func(s []Attempt) int {
		k := 0
		for _, a := range s {
			if a.Verified {
				k++
			}
		}
		return k
	}
	if len(xs) >= window {
		r.Baseline = wilson(count(xs[:window]), window)
	}
	if len(xs) >= 2*window {
		r.Recent = wilson(count(xs[len(xs)-window:]), window)
		switch {
		case r.Recent.Lo > r.Baseline.Hi:
			r.Claim = ClaimImproved
		case r.Recent.Hi < r.Baseline.Lo:
			r.Claim = ClaimRegressed
		default:
			r.Claim = ClaimNoChange
		}
	}
	return r
}

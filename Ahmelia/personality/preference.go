package personality

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"
)

// Stance es lo que Ahmelia está autorizada a EXPRESAR sobre un criterio.
type Stance string

const (
	StanceUnformed Stance = "unformed" // poca evidencia: no hay gusto aún
	StanceLikes    Stance = "likes"
	StanceDislikes Stance = "dislikes"
	StanceMixed    Stance = "mixed"
)

// Posterior Beta(a,b) con olvido exponencial: la evidencia vieja pesa menos,
// de modo que un gusto es revisable por experiencia contraria.
type Posterior struct {
	A, B float64
	N    int
	Refs []string
	At   time.Time
}

type PreferenceModel struct {
	mu     sync.Mutex
	Forget float64 // factor por observación, p.ej. 0.97
	MinN   float64 // evidencia efectiva mínima para expresar
	post   map[string]*Posterior
}

func NewPreferenceModel() *PreferenceModel {
	return &PreferenceModel{Forget: 0.97, MinN: 5, post: map[string]*Posterior{}}
}

// Experience.Outcome en [-1,1]; Weight escala la fiabilidad de la fuente
// (verificado > autoevaluación > reacción ambigua).
type Experience struct {
	Key     string
	Outcome float64
	Weight  float64
	Ref     string // evidencia trazable (WorkID, turno)
	At      time.Time
}

func (m *PreferenceModel) Observe(x Experience) error {
	if x.Key == "" || x.Weight <= 0 || x.Weight > 1 {
		return errors.New("personality: invalid experience")
	}
	o := math.Max(-1, math.Min(1, x.Outcome))
	m.mu.Lock()
	defer m.mu.Unlock()
	p := m.post[x.Key]
	if p == nil {
		p = &Posterior{A: 1, B: 1}
		m.post[x.Key] = p
	}
	p.A = 1 + (p.A-1)*m.Forget
	p.B = 1 + (p.B-1)*m.Forget
	pos := (o + 1) / 2
	p.A += x.Weight * pos
	p.B += x.Weight * (1 - pos)
	p.N++
	p.At = x.At
	if x.Ref != "" {
		p.Refs = append(p.Refs, x.Ref)
		if len(p.Refs) > 5 {
			p.Refs = p.Refs[len(p.Refs)-5:]
		}
	}
	return nil
}

type View struct {
	Stance   Stance
	Mean     float64
	Lo, Hi   float64 // intervalo ~80% (z=1.28)
	EffN     float64
	Evidence []string
	// At es la marca de la ÚLTIMA observación aceptada del posterior
	// (Posterior.At). La proyección P4 la usa como ObservedAt para no fingir
	// que el derivado se observó "ahora". Cero = posterior sin marca.
	At time.Time
}

// Stance solo sale de "likes/dislikes" si el intervalo excluye 0.5; si no,
// "unformed" (poca evidencia) o "mixed" (evidencia suficiente y contradictoria).
func (m *PreferenceModel) View(key string) View {
	m.mu.Lock()
	defer m.mu.Unlock()
	p := m.post[key]
	if p == nil {
		return View{Stance: StanceUnformed}
	}
	n := p.A + p.B - 2
	mean := p.A / (p.A + p.B)
	sd := math.Sqrt(mean * (1 - mean) / (p.A + p.B + 1))
	v := View{Mean: mean, Lo: mean - 1.28*sd, Hi: mean + 1.28*sd, EffN: n, Evidence: append([]string(nil), p.Refs...), At: p.At}
	switch {
	case n < m.MinN:
		v.Stance = StanceUnformed
	case v.Lo > 0.5:
		v.Stance = StanceLikes
	case v.Hi < 0.5:
		v.Stance = StanceDislikes
	default:
		v.Stance = StanceMixed
	}
	return v
}

// Correct replaces the accumulated evidence for key with a fresh prior and one
// full negative observation, attributed to the owner's correction. A correction
// is stronger than an inference: the previous posterior for that key stops
// counting instead of being averaged against the owner's word. Without a
// traceable reference the correction is refused.
func (m *PreferenceModel) Correct(key, ref string, at time.Time) error {
	key = strings.TrimSpace(key)
	ref = strings.TrimSpace(ref)
	if key == "" || ref == "" {
		return errors.New("personality: corrección sin actividad o referencia")
	}
	if at.IsZero() {
		at = time.Now()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	p := &Posterior{A: 1, B: 1}
	m.post[key] = p
	// Full negative observation (outcome -1) at unit weight.
	p.B += 1
	p.N = 1
	p.At = at
	p.Refs = []string{ref}
	return nil
}

// P4 — Snapshot/Restore/Keys: persistencia del posterior SIN cambiar el
// contrato Observe/View. El motor carga el snapshot al arrancar (fail-open
// honesto) y lo guarda junto al modelo canónico de usuario cada vez que el
// Camino B96.34 acepta una preferencia explícita (extensión del mecanismo
// "guardado tras mutación" de B90.7). La autoridad sobre qué se recuerda
// sigue siendo el modelo canónico: esto solo persiste el posterior derivado.
func (m *PreferenceModel) Snapshot() map[string]Posterior {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make(map[string]Posterior, len(m.post))
	for k, p := range m.post {
		c := *p
		c.Refs = append([]string(nil), p.Refs...)
		out[k] = c
	}
	return out
}

// Restore acepta solo estados de posterior coherentes (llaves no vacías y
// conteos adecuados). Entradas inválidas o corrupción puntual no abortan:
// se devuelven en el error y el motor decide arrancar limpio (fail-open),
// nunca se inventa preferencia por estado dañado.
func (m *PreferenceModel) Restore(snap map[string]Posterior) error {
	if snap == nil {
		return errors.New("personality: nil preference snapshot")
	}
	clean := make(map[string]*Posterior, len(snap))
	var bad []string
	for k, p := range snap {
		key := k // nada de normalización aquí: el snap viene de Snapshot()
		if len(strings.TrimSpace(key)) == 0 || p.A < 1 || p.B < 1 || p.N < 0 {
			bad = append(bad, key)
			continue
		}
		c := p
		c.Refs = append([]string(nil), p.Refs...)
		clean[key] = &c
	}
	if len(bad) > 0 && len(clean) == 0 {
		return fmt.Errorf("personality: snapshot invalido (%d entradas corruptas)", len(bad))
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.post = clean
	if len(bad) > 0 {
		return fmt.Errorf("personality: snapshot con %d entradas corruptas ignoradas", len(bad))
	}
	return nil
}

// Keys devuelve las llaves con posterior registrado (orden estable).
func (m *PreferenceModel) Keys() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	keys := make([]string, 0, len(m.post))
	for k := range m.post {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

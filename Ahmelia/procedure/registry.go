package procedure

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"sync"
	"time"

	"ahmelia/memory"
)

type CheckError struct{ Issues []Issue }

func (e *CheckError) Error() string {
	return fmt.Sprintf("procedure: %d incidencia(s) de verificación", len(e.Issues))
}

type state struct {
	Procs   map[string][]Procedure `json:"procs"`
	Active  map[string]int         `json:"active"`
	History map[string][]int       `json:"history"` // pila de activaciones previas
}

// Registry guarda versiones inmutables y un puntero a la versión ACTIVA.
// Proponer ≠ activar: activar exige pasar la compuerta de regresión (trial).
type Registry struct {
	mu   sync.RWMutex
	path string
	st   state
}

func Open(path string) (*Registry, error) {
	r := &Registry{path: path, st: state{Procs: map[string][]Procedure{}, Active: map[string]int{}, History: map[string][]int{}}}
	if path == "" {
		return r, nil
	}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return r, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &r.st); err != nil {
		return nil, fmt.Errorf("procedure: registro ilegible: %w", err) // no se sobrescribe en silencio
	}
	if r.st.Procs == nil {
		r.st.Procs = map[string][]Procedure{}
	}
	if r.st.Active == nil {
		r.st.Active = map[string]int{}
	}
	if r.st.History == nil {
		r.st.History = map[string][]int{}
	}
	return r, nil
}

func (r *Registry) save() error {
	if r.path == "" {
		return nil
	}
	b, err := json.MarshalIndent(r.st, "", "  ")
	if err != nil {
		return err
	}
	return memory.WriteFileAtomic(r.path, b, 0o644)
}

// Propose valida contra el catálogo y guarda una versión NO activa.
func (r *Registry) Propose(p Procedure, cat Catalog) (Procedure, error) {
	if p.ID == "" {
		return Procedure{}, errors.New("procedure: id requerido")
	}
	if issues := p.Check(cat); len(issues) > 0 {
		return Procedure{}, &CheckError{Issues: issues}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	vs := r.st.Procs[p.ID]
	p.Version = len(vs) + 1
	if p.CreatedAt.IsZero() {
		p.CreatedAt = time.Now()
	}
	r.st.Procs[p.ID] = append(vs, p)
	return p, r.save()
}

func (r *Registry) Get(id string, version int) (Procedure, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	vs := r.st.Procs[id]
	if version < 1 || version > len(vs) {
		return Procedure{}, false
	}
	return vs[version-1], true
}

func (r *Registry) Active(id string) (Procedure, bool) {
	r.mu.RLock()
	v, ok := r.st.Active[id]
	r.mu.RUnlock()
	if !ok {
		return Procedure{}, false
	}
	return r.Get(id, v)
}

func (r *Registry) Activate(id string, version int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if version < 1 || version > len(r.st.Procs[id]) {
		return fmt.Errorf("procedure: %s@%d no existe", id, version)
	}
	if cur, ok := r.st.Active[id]; ok && cur != version {
		r.st.History[id] = append(r.st.History[id], cur)
	}
	r.st.Active[id] = version
	return r.save()
}

// Rollback vuelve a la versión activa anterior (o desactiva si no había).
func (r *Registry) Rollback(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	h := r.st.History[id]
	if len(h) == 0 {
		return fmt.Errorf("procedure: %s no tiene versión previa", id)
	}
	r.st.Active[id] = h[len(h)-1]
	r.st.History[id] = h[:len(h)-1]
	return r.save()
}

func (r *Registry) IDs() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.st.Procs))
	for id := range r.st.Procs {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

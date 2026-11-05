// Package procedure es el artefacto versionado que separa "saber que existe"
// (texto) de "saber hacerlo" (pasos tipados contra capacidades registradas).
// Un Procedure no ejecuta nada: produce BoundSteps que el binder/autoridad
// existentes validan y ejecutan.
package procedure

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

type OriginKind string

const (
	OriginAuthored   OriginKind = "authored"
	OriginFromClaims OriginKind = "derived_from_claims" // nace de información externa
	OriginRefined    OriginKind = "refined_from_experience"
)

type Origin struct {
	Kind          OriginKind `json:"kind"`
	ClaimIDs      []string   `json:"claim_ids,omitempty"`
	ParentVersion int        `json:"parent_version,omitempty"`
}

type Slot struct {
	Name     string `json:"name"`
	Required bool   `json:"required"`
}

// Step referencia una capacidad REGISTRADA. Los valores de Inputs son
// referencias "{slot}" o literales (los literales están prohibidos en
// procedimientos de origen externo: el texto web no aporta valores).
type Step struct {
	ID         string            `json:"id"`
	Capability string            `json:"capability"`
	Operation  string            `json:"operation"`
	Inputs     map[string]string `json:"inputs,omitempty"`
	After      []string          `json:"after,omitempty"`
}

type Procedure struct {
	ID        string    `json:"id"`
	Version   int       `json:"version"`
	Name      string    `json:"name"`
	Slots     []Slot    `json:"slots,omitempty"`
	Steps     []Step    `json:"steps"`
	Origin    Origin    `json:"origin"`
	CreatedAt time.Time `json:"created_at"`
}

// Ref es el identificador que se escribe en WorkAttempt.SkillID para atribuir
// cada intento a una versión concreta.
func (p Procedure) Ref() string { return fmt.Sprintf("proc:%s@%d", p.ID, p.Version) }

// Catalog es la costura hacia capability.Registry (sin importar el paquete).
type Catalog interface {
	Known(capability, operation string) bool
}

type IssueKind string

const (
	IssueEmpty           IssueKind = "empty"
	IssueDuplicateStep   IssueKind = "duplicate_step"
	IssueCapabilityGap   IssueKind = "capability_gap"
	IssueBadSlot         IssueKind = "bad_slot"
	IssueExternalLiteral IssueKind = "external_literal"
	IssueBadDependency   IssueKind = "bad_dependency"
	IssueCycle           IssueKind = "cycle"
)

type Issue struct {
	Kind   IssueKind
	Step   string
	Detail string
}

var slotRef = regexp.MustCompile(`^\{([a-z][a-z0-9_]*)\}$`)

func (p Procedure) Check(cat Catalog) []Issue {
	var out []Issue
	if len(p.Steps) == 0 {
		return []Issue{{Kind: IssueEmpty}}
	}
	slots := map[string]bool{}
	for _, s := range p.Slots {
		slots[s.Name] = true
	}
	ids := map[string]bool{}
	for _, s := range p.Steps {
		if ids[s.ID] || s.ID == "" {
			out = append(out, Issue{IssueDuplicateStep, s.ID, "id vacío o repetido"})
		}
		ids[s.ID] = true
	}
	for _, s := range p.Steps {
		if cat == nil || !cat.Known(s.Capability, s.Operation) {
			out = append(out, Issue{IssueCapabilityGap, s.ID, s.Capability + "/" + s.Operation})
		}
		keys := make([]string, 0, len(s.Inputs))
		for k := range s.Inputs {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			v := s.Inputs[k]
			if m := slotRef.FindStringSubmatch(v); m != nil {
				if !slots[m[1]] {
					out = append(out, Issue{IssueBadSlot, s.ID, "slot no declarado: " + m[1]})
				}
				continue
			}
			if strings.ContainsAny(v, "{}") {
				out = append(out, Issue{IssueBadSlot, s.ID, "referencia mal formada en " + k})
			} else if p.Origin.Kind == OriginFromClaims {
				out = append(out, Issue{IssueExternalLiteral, s.ID, "literal externo en " + k})
			}
		}
		for _, a := range s.After {
			if !ids[a] || a == s.ID {
				out = append(out, Issue{IssueBadDependency, s.ID, "after desconocido: " + a})
			}
		}
	}
	if len(out) == 0 {
		if _, err := p.order(); err != nil {
			out = append(out, Issue{IssueCycle, "", err.Error()})
		}
	}
	return out
}

func (p Procedure) order() ([]Step, error) {
	indeg := map[string]int{}
	by := map[string]Step{}
	for _, s := range p.Steps {
		indeg[s.ID] = len(s.After)
		by[s.ID] = s
	}
	var out []Step
	done := map[string]bool{}
	for len(out) < len(p.Steps) {
		progressed := false
		for _, s := range p.Steps { // orden de declaración = desempate estable
			if done[s.ID] {
				continue
			}
			ready := true
			for _, a := range s.After {
				if !done[a] {
					ready = false
					break
				}
			}
			if ready {
				out = append(out, by[s.ID])
				done[s.ID] = true
				progressed = true
			}
		}
		if !progressed {
			return nil, fmt.Errorf("dependencia cíclica")
		}
	}
	return out, nil
}

type BoundStep struct {
	StepID     string
	Capability string
	Operation  string
	Inputs     map[string]string
	After      []string
}

// Bind instancia el procedimiento con valores de slot del contexto REAL de la
// tarea (nunca del texto externo) y devuelve los pasos en orden ejecutable.
func (p Procedure) Bind(slots map[string]string) ([]BoundStep, error) {
	decl := map[string]Slot{}
	for _, s := range p.Slots {
		decl[s.Name] = s
	}
	for k := range slots {
		if _, ok := decl[k]; !ok {
			return nil, fmt.Errorf("procedure: slot desconocido %q", k)
		}
	}
	for _, s := range p.Slots {
		if s.Required && strings.TrimSpace(slots[s.Name]) == "" {
			return nil, fmt.Errorf("procedure: falta slot requerido %q", s.Name)
		}
	}
	ordered, err := p.order()
	if err != nil {
		return nil, err
	}
	var out []BoundStep
	for _, s := range ordered {
		in := map[string]string{}
		for k, v := range s.Inputs {
			if m := slotRef.FindStringSubmatch(v); m != nil {
				in[k] = slots[m[1]]
			} else {
				in[k] = v
			}
		}
		out = append(out, BoundStep{StepID: s.ID, Capability: s.Capability, Operation: s.Operation, Inputs: in, After: append([]string(nil), s.After...)})
	}
	return out, nil
}

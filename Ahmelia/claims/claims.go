// Package claims convierte documentos externos en AFIRMACIONES con procedencia
// y las compila a procedimientos tipados o a brechas de capacidad.
// Regla estructural: el texto externo es DATO. La única vía hacia una acción es
// claim → propuesta tipada → Procedure.Check → Trial → Release.
package claims

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"ahmelia/memory_intelligence/semindex"
	"ahmelia/procedure"
)

type Provenance struct {
	URL           string    `json:"url"`
	Host          string    `json:"host"`
	ContentSHA256 string    `json:"content_sha256"`
	LastModified  string    `json:"last_modified,omitempty"`
	ETag          string    `json:"etag,omitempty"`
	FetchedAt     time.Time `json:"fetched_at"`
	Truncated     bool      `json:"truncated,omitempty"`
}

type Kind string

const (
	KindStep         Kind = "step"
	KindPrerequisite Kind = "prerequisite"
	KindPitfall      Kind = "pitfall"
	KindFact         Kind = "fact"
)

type Claim struct {
	ID      string     `json:"id"`
	Subject string     `json:"subject"`
	Kind    Kind       `json:"kind"`
	Text    string     `json:"text"`
	Source  Provenance `json:"source"`
}

func NewClaim(subject string, kind Kind, text string, src Provenance) Claim {
	h := sha256.Sum256([]byte(subject + "\x00" + text + "\x00" + src.ContentSHA256))
	return Claim{ID: hex.EncodeToString(h[:8]), Subject: subject, Kind: kind, Text: strings.TrimSpace(text), Source: src}
}

func jaccard(a, b string) float64 {
	ta, tb := map[string]bool{}, map[string]bool{}
	for _, t := range semindex.Tokens(a) {
		ta[t] = true
	}
	for _, t := range semindex.Tokens(b) {
		tb[t] = true
	}
	if len(ta) == 0 || len(tb) == 0 {
		return 0
	}
	inter := 0
	for t := range ta {
		if tb[t] {
			inter++
		}
	}
	return float64(inter) / float64(len(ta)+len(tb)-inter)
}

// Corroboration: para cada claim, nº de HOSTS DISTINTOS con una afirmación
// equivalente (Jaccard de tokens >= minSim). Un host que repite no suma.
// Límite conocido: equivalencia léxica; no une paráfrasis sin embeddings.
func Corroboration(cs []Claim, minSim float64) map[string]int {
	out := map[string]int{}
	for _, a := range cs {
		hosts := map[string]bool{a.Source.Host: true}
		for _, b := range cs {
			if b.Source.Host != a.Source.Host && b.Subject == a.Subject && jaccard(a.Text, b.Text) >= minSim {
				hosts[b.Source.Host] = true
			}
		}
		out[a.ID] = len(hosts)
	}
	return out
}

// StepProposal es la salida de un Mapper (LLM o reglas). Es DATO no confiable.
type StepProposal struct {
	ClaimID    string
	StepID     string
	Capability string
	Operation  string
	Inputs     map[string]string
	After      []string
}

type Mapper interface {
	Map(ctx context.Context, subject string, cs []Claim) ([]StepProposal, error)
}

type Gap struct {
	ClaimID string
	Reason  string // capability_gap | external_literal | bad_slot | uncorroborated | depends_on_gap
	Detail  string
}

var ErrNothingCompilable = errors.New("claims: ningún claim compila a pasos ejecutables")

type Options struct {
	MinHosts int // hosts distintos exigidos por claim
	MinSim   float64
}

// Compile NUNCA ejecuta ni registra: devuelve un candidato (versión sin
// asignar) y las brechas. Pasos con capacidad inexistente, literales externos
// o claims no corroborados se descartan y se reportan como brechas, junto con
// todo lo que dependa de ellos.
func Compile(ctx context.Context, m Mapper, id, name string, slots []procedure.Slot, cs []Claim, cat procedure.Catalog, o Options) (procedure.Procedure, []Gap, error) {
	if o.MinHosts < 1 {
		o.MinHosts = 2
	}
	if o.MinSim <= 0 {
		o.MinSim = 0.6
	}
	if len(cs) == 0 {
		return procedure.Procedure{}, nil, ErrNothingCompilable
	}
	subject := cs[0].Subject
	props, err := m.Map(ctx, subject, cs)
	if err != nil {
		return procedure.Procedure{}, nil, err
	}
	corro := Corroboration(cs, o.MinSim)
	var gaps []Gap
	dropped := map[string]bool{}
	var steps []procedure.Step
	var claimIDs []string
	for _, p := range props {
		if corro[p.ClaimID] < o.MinHosts {
			gaps = append(gaps, Gap{p.ClaimID, "uncorroborated", fmt.Sprintf("%d host(s)", corro[p.ClaimID])})
			dropped[p.StepID] = true
			continue
		}
		steps = append(steps, procedure.Step{ID: p.StepID, Capability: p.Capability, Operation: p.Operation, Inputs: p.Inputs, After: p.After})
		claimIDs = append(claimIDs, p.ClaimID)
	}
	byStep := map[string]string{}
	for _, p := range props {
		byStep[p.StepID] = p.ClaimID
	}
	cand := procedure.Procedure{ID: id, Name: name, Slots: slots, Origin: procedure.Origin{Kind: procedure.OriginFromClaims}}
	for iter := 0; iter < len(props)+1; iter++ {
		cand.Steps = steps
		changed := false
		bad := map[string]procedure.Issue{}
		for _, is := range cand.Check(cat) {
			switch is.Kind {
			case procedure.IssueCapabilityGap, procedure.IssueExternalLiteral, procedure.IssueBadSlot, procedure.IssueBadDependency:
				if _, seen := bad[is.Step]; !seen && is.Step != "" {
					bad[is.Step] = is
				}
			case procedure.IssueEmpty:
				return procedure.Procedure{}, gaps, ErrNothingCompilable
			default:
				return procedure.Procedure{}, gaps, fmt.Errorf("claims: candidato incoherente: %s %s", is.Kind, is.Detail)
			}
		}
		var keep []procedure.Step
		for _, s := range steps {
			if is, isBad := bad[s.ID]; isBad {
				reason := string(is.Kind)
				if is.Kind == procedure.IssueBadDependency {
					reason = "depends_on_gap"
				}
				gaps = append(gaps, Gap{byStep[s.ID], reason, is.Detail})
				dropped[s.ID] = true
				changed = true
				continue
			}
			keep = append(keep, s)
		}
		steps = keep
		if !changed {
			break
		}
	}
	if len(steps) == 0 {
		return procedure.Procedure{}, gaps, ErrNothingCompilable
	}
	sort.Strings(claimIDs)
	cand.Steps = steps
	cand.Origin.ClaimIDs = claimIDs
	if issues := cand.Check(cat); len(issues) > 0 {
		return procedure.Procedure{}, gaps, fmt.Errorf("claims: candidato inválido: %v", issues[0])
	}
	return cand, gaps, nil
}

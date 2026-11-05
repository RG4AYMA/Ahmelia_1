package agency

// impulse.go — IMPULSOS DEL ARNÉS.
//
// Un impulso no es un deseo del modelo ni una orden de ejecución. Es una señal
// que el arnés deriva de su propio estado y que abre una ventana breve para que
// el modelo articule una meta propia. Sin impulso, el modelo no puede proponer
// metas: el arnés posee la ocasión, el modelo posee la articulación.
//
// Los impulsos son deterministas y baratos. No llaman al modelo, no leen
// archivos y no autorizan nada. Solo describen una presión observable:
//
//   - knowledge_gap   el arnés declaró incertidumbre sobre el turno (una
//                     pregunta pendiente para el usuario, por sí sola, no
//                     abre una meta propia);
//   - unfinished_work hay trabajo empezado que no cerró;
//   - failed_action   una acción corrió y no quedó verificada;
//   - blocked_goal    un objetivo del planificador quedó bloqueado;
//   - repeated_topic  el mismo tema vuelve una y otra vez sin resolverse.
//
// La fuerza es una prioridad relativa, no una probabilidad. El llamador puede
// aplicar un umbral; este paquete no decide por él.

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// ImpulseKind is the closed vocabulary of harness-owned pressures.
type ImpulseKind string

const (
	ImpulseKnowledgeGap   ImpulseKind = "knowledge_gap"
	ImpulseUnfinishedWork ImpulseKind = "unfinished_work"
	ImpulseFailedAction   ImpulseKind = "failed_action"
	ImpulseBlockedGoal    ImpulseKind = "blocked_goal"
	ImpulseRepeatedTopic  ImpulseKind = "repeated_topic"
)

// Impulse is one bounded pressure derived from harness state.
type Impulse struct {
	Kind     ImpulseKind `json:"kind"`
	Reason   string      `json:"reason"`
	Strength float64     `json:"strength"`
	Evidence []string    `json:"evidence,omitempty"`
}

// Text renders a compact line for the cognitive workspace.
func (i Impulse) Text() string {
	reason := strings.TrimSpace(i.Reason)
	if reason == "" {
		reason = string(i.Kind)
	}
	return fmt.Sprintf("%s: %s (fuerza %.2f)", i.Kind, reason, i.Strength)
}

// ImpulseInput is the harness-owned state an impulse may be derived from.
// Every field is already known at turn close; nothing here calls the model.
type ImpulseInput struct {
	Now time.Time

	// Uncertainty is the harness's own declared uncertainty for the turn.
	Uncertainty bool
	// PendingQuestion is the oldest unresolved clarification question, if any.
	// It enriches a knowledge_gap but never opens one by itself: a question the
	// user must answer is not a goal Ahmelia can pursue.
	PendingQuestion string

	// InteractionActive reports whether the turn touched a work session.
	InteractionActive bool
	// InteractionStatus is the work status as the harness sees it.
	InteractionStatus string

	// ActionRan reports whether an action was actually dispatched this turn.
	ActionRan bool
	// LastActionSuccess and LastActionVerified are the harness's own results.
	LastActionSuccess  bool
	LastActionVerified bool

	// BlockedGoals lists planner goals currently blocked, by name.
	BlockedGoals []string

	// Topic and TopicTurns describe a repeated conversational topic.
	Topic      string
	TopicTurns int
}

// terminalWorkStatuses are statuses that mean the work session is closed.
// Anything else with InteractionActive is treated as unfinished. The list is
// deliberately small and lowercase: an unknown status is not assumed closed.
var terminalWorkStatuses = map[string]bool{
	"completed": true,
	"complete":  true,
	"done":      true,
	"verified":  true,
	"closed":    true,
	"cancelled": true,
	"canceled":  true,
	"failed":    true,
}

// ComputeImpulses derives at most three impulses, strongest first. It is pure
// and deterministic: the same input always yields the same output.
func ComputeImpulses(in ImpulseInput) []Impulse {
	var out []Impulse

	if in.Uncertainty {
		strength := 0.70
		reason := "el arnés declaró incertidumbre sobre este turno"
		imp := Impulse{Kind: ImpulseKnowledgeGap, Reason: reason, Strength: strength}
		if q := strings.TrimSpace(in.PendingQuestion); q != "" {
			imp.Strength = 0.85
			imp.Reason = "el arnés declaró incertidumbre y además dejó una pregunta pendiente"
			imp.Evidence = append(imp.Evidence, "clarification:"+q)
		}
		out = append(out, imp)
	}

	if in.InteractionActive {
		status := strings.ToLower(strings.TrimSpace(in.InteractionStatus))
		if status == "" || !terminalWorkStatuses[status] {
			out = append(out, Impulse{
				Kind:     ImpulseUnfinishedWork,
				Reason:   "hay trabajo empezado que todavía no cerró",
				Strength: 0.60,
				Evidence: []string{"work_status:" + statusOrUnknown(status)},
			})
		}
	}

	if in.ActionRan && (!in.LastActionSuccess || !in.LastActionVerified) {
		strength := 0.65
		reason := "la última acción no quedó verificada"
		if !in.LastActionSuccess && !in.LastActionVerified {
			strength = 0.80
			reason = "la última acción falló y no quedó verificada"
		}
		out = append(out, Impulse{
			Kind:     ImpulseFailedAction,
			Reason:   reason,
			Strength: strength,
			Evidence: []string{
				fmt.Sprintf("action_success:%t", in.LastActionSuccess),
				fmt.Sprintf("action_verified:%t", in.LastActionVerified),
			},
		})
	}

	if len(in.BlockedGoals) > 0 {
		names := make([]string, 0, len(in.BlockedGoals))
		for _, name := range in.BlockedGoals {
			name = strings.TrimSpace(name)
			if name != "" {
				names = append(names, name)
			}
		}
		if len(names) > 0 {
			reason := "hay un objetivo bloqueado: " + names[0]
			if len(names) > 1 {
				reason = fmt.Sprintf("hay %d objetivos bloqueados; el primero es %s", len(names), names[0])
			}
			out = append(out, Impulse{
				Kind:     ImpulseBlockedGoal,
				Reason:   reason,
				Strength: 0.55,
				Evidence: append([]string(nil), names...),
			})
		}
	}

	topic := strings.TrimSpace(in.Topic)
	if topic != "" && in.TopicTurns >= 3 {
		strength := 0.40 + 0.10*float64(in.TopicTurns-3)
		if strength > 0.70 {
			strength = 0.70
		}
		out = append(out, Impulse{
			Kind:     ImpulseRepeatedTopic,
			Reason:   fmt.Sprintf("el tema %q vuelve desde hace %d turnos", topic, in.TopicTurns),
			Strength: strength,
			Evidence: []string{"topic:" + topic},
		})
	}

	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Strength == out[j].Strength {
			return out[i].Kind < out[j].Kind
		}
		return out[i].Strength > out[j].Strength
	})
	if len(out) > 3 {
		out = out[:3]
	}
	return out
}

// Strongest returns the first impulse at or above threshold, or false.
func Strongest(impulses []Impulse, threshold float64) (Impulse, bool) {
	for _, imp := range impulses {
		if imp.Strength >= threshold {
			return imp, true
		}
	}
	return Impulse{}, false
}

func statusOrUnknown(status string) string {
	if status == "" {
		return "unknown"
	}
	return status
}

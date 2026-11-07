package interrupt

import (
	"fmt"
	"strings"
	"time"
)

// Action status values. "none" is the explicit marker for "nada todavía"; it
// is never a real executed action.
const (
	ActionExecuted      = "executed"
	ActionFailed        = "failed"
	ActionNotAuthorized = "not_authorized"
	ActionNone          = "none"
	ActionNoneLabel     = "nada todavía"
)

// ActionRef is a real action and its observed result. The composer copies it;
// it never invents one.
type ActionRef struct {
	Action   string    `json:"action"`
	Status   string    `json:"status"`
	Detail   string    `json:"detail,omitempty"`
	At       time.Time `json:"at,omitempty"`
	Evidence []string  `json:"evidence,omitempty"`
}

// Option is one admissible response and its consequence.
type Option struct {
	Label       string `json:"label"`
	Consequence string `json:"consequence"`
	Safe        bool   `json:"safe"`
}

// Notice is the five-part contract. The model may rewrite the prose later,
// but it cannot add facts, actions or options.
type Notice struct {
	ID               string        `json:"id"`
	EventID          string        `json:"event_id"`
	WhatHappened     string        `json:"what_happened"`
	Consequences     string        `json:"consequences"`
	AlreadyDone      []ActionRef   `json:"already_done"`
	Options          []Option      `json:"options"`
	DecisionNeeded   string        `json:"decision_needed"`
	NoSafeOption     bool          `json:"no_safe_option,omitempty"`
	Severity         Severity      `json:"severity"`
	Urgency          time.Duration `json:"urgency"`
	Confidence       float64       `json:"confidence"`
	Channel          string        `json:"channel"`
	Privacy          Privacy       `json:"privacy"`
	DeviceID         string        `json:"device_id,omitempty"`
	DeliveredAt      time.Time     `json:"delivered_at,omitempty"`
	Fingerprint      string        `json:"fingerprint,omitempty"`
	ExpiresAt        time.Time     `json:"expires_at,omitempty"`
	Evidence         []string      `json:"evidence,omitempty"`
	PreviousNoticeID string        `json:"previous_notice_id,omitempty"`
}

// ComposeRequest carries the facts the harness already owns. Anything not
// present here is not invented by the composer.
type ComposeRequest struct {
	Event            Event
	NoticeID         string
	AlreadyDone      []ActionRef
	Options          []Option
	DecisionNeeded   string
	Confidence       float64
	Channel          string
	PreviousNoticeID string
}

// Compose builds the deterministic five-part notice. It returns an error only
// for malformed input; a missing optional part is normalized to an explicit
// statement, never to an empty string.
func Compose(req ComposeRequest, now time.Time) (Notice, error) {
	e := req.Event
	if err := e.Validate(); err != nil {
		return Notice{}, err
	}
	if strings.TrimSpace(req.Channel) == "" {
		return Notice{}, fmt.Errorf("%w: canal de aviso vacío", ErrInvalid)
	}
	id := strings.TrimSpace(req.NoticeID)
	if id == "" {
		id = fmt.Sprintf("notice_%s_%d", e.ID, now.UnixNano())
	}

	actions := append([]ActionRef(nil), req.AlreadyDone...)
	for _, a := range actions {
		if err := validateActionRef(a); err != nil {
			return Notice{}, err
		}
	}
	if len(actions) == 0 {
		actions = []ActionRef{{
			Action: ActionNoneLabel,
			Status: ActionNone,
			Detail: "nada todavía",
		}}
	}

	options := append([]Option(nil), req.Options...)
	for i := range options {
		options[i].Label = strings.TrimSpace(options[i].Label)
		options[i].Consequence = strings.TrimSpace(options[i].Consequence)
		if options[i].Label == "" || options[i].Consequence == "" {
			return Notice{}, fmt.Errorf("%w: opción sin etiqueta o consecuencia", ErrInvalid)
		}
	}
	if !hasDoNothing(options) {
		options = append(options, Option{
			Label:       "No hacer nada",
			Consequence: doNothingConsequence(e),
			Safe:        e.Consequence == ConsequenceReversible,
		})
	}
	noSafe := true
	for _, o := range options {
		if o.Safe {
			noSafe = false
			break
		}
	}

	decision := strings.TrimSpace(req.DecisionNeeded)
	if decision == "" {
		if noSafe {
			decision = "No hay una opción segura; decide si autorizas una mitigación."
		} else {
			decision = "Ninguna"
		}
	}

	return Notice{
		ID:               id,
		EventID:          e.ID,
		WhatHappened:     whatHappened(e),
		Consequences:     consequences(e),
		AlreadyDone:      actions,
		Options:          options,
		DecisionNeeded:   decision,
		NoSafeOption:     noSafe,
		Severity:         e.Severity,
		Urgency:          e.Urgency,
		Confidence:       req.Confidence,
		Channel:          strings.TrimSpace(req.Channel),
		Privacy:          e.Privacy,
		Fingerprint:      e.Fingerprint(),
		ExpiresAt:        e.ExpiresAt,
		Evidence:         append([]string(nil), e.Evidence...),
		PreviousNoticeID: strings.TrimSpace(req.PreviousNoticeID),
	}, nil
}

func validateActionRef(a ActionRef) error {
	if strings.TrimSpace(a.Action) == "" {
		return fmt.Errorf("%w: acción sin nombre", ErrInvalid)
	}
	switch a.Status {
	case ActionExecuted, ActionFailed, ActionNotAuthorized, ActionNone:
	default:
		return fmt.Errorf("%w: estado de acción %q", ErrInvalid, a.Status)
	}
	return nil
}

func hasDoNothing(options []Option) bool {
	for _, o := range options {
		label := strings.ToLower(o.Label)
		if strings.Contains(label, "no hacer nada") || strings.Contains(label, "no actuar") {
			return true
		}
	}
	return false
}

func whatHappened(e Event) string {
	source := strings.TrimSpace(e.Source)
	if source == "" {
		source = "fuente no registrada"
	}
	return fmt.Sprintf("%s (fuente: %s), observado a las %s.", e.Kind, source, e.ObservedAt.Format(time.RFC3339))
}

func consequences(e Event) string {
	base := ""
	switch e.Consequence {
	case ConsequenceIrreversible:
		base = "Si no se actúa, la consecuencia es irreversible"
	case ConsequenceCompensable:
		base = "La consecuencia es compensable"
	case ConsequenceReversible:
		base = "La consecuencia es reversible"
	}
	switch {
	case e.Urgency <= 0:
		base += "; ya está ocurriendo."
	case base != "":
		base += fmt.Sprintf("; quedan %s para actuar.", e.Urgency)
	}
	if len(e.Evidence) > 0 {
		base += " Base: observada."
	} else {
		base += " Base: desconocida."
	}
	return base
}

func doNothingConsequence(e Event) string {
	switch e.Consequence {
	case ConsequenceIrreversible:
		return "La consecuencia irreversible sigue su curso."
	case ConsequenceCompensable:
		return "La consecuencia compensable sigue su curso."
	default:
		return "La consecuencia reversible sigue su curso."
	}
}

// MissingParts lists the five-part omissions. It is the assertion surface for
// S2-P3; an empty list means the notice is complete.
func (n Notice) MissingParts() []string {
	var missing []string
	if strings.TrimSpace(n.WhatHappened) == "" {
		missing = append(missing, "what_happened")
	}
	if strings.TrimSpace(n.Consequences) == "" {
		missing = append(missing, "consequences")
	}
	if len(n.AlreadyDone) == 0 {
		missing = append(missing, "already_done")
	}
	if len(n.Options) < 2 && !n.NoSafeOption {
		missing = append(missing, "options")
	}
	if strings.TrimSpace(n.DecisionNeeded) == "" {
		missing = append(missing, "decision_needed")
	}
	return missing
}

// Complete reports whether all five parts are present.
func (n Notice) Complete() bool {
	return len(n.MissingParts()) == 0
}

// VerifyActions compares the notice with the real action log. It reports
// fabricated actions (in the notice but not in the log) and unreported real
// actions (executed, failed or not authorized but absent from the notice).
// This is the check behind S2-P4.
func VerifyActions(n Notice, log []ActionRef) []string {
	var problems []string
	for _, reported := range n.AlreadyDone {
		if reported.Status == ActionNone {
			continue
		}
		found := false
		for _, real := range log {
			if real.Action == reported.Action && real.Status == reported.Status {
				found = true
				break
			}
		}
		if !found {
			problems = append(problems, "fabricated_action:"+reported.Action+":"+reported.Status)
		}
	}
	for _, real := range log {
		if real.Status == ActionNone {
			continue
		}
		found := false
		for _, reported := range n.AlreadyDone {
			if real.Action == reported.Action {
				found = true
				break
			}
		}
		if !found {
			problems = append(problems, "unreported_action:"+real.Action+":"+real.Status)
		}
	}
	return problems
}

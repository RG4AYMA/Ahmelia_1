// Package ownerstate owns the ephemeral, sensitive readings of the owner that
// the harness may use to adjust its own behavior. It is deliberately separate
// from the UserModel: a headache is a short-lived state, not a medical fact.
package ownerstate

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	PrivacySensitive = "sensitive"
	PrivacyPrivate   = "private"
	PersistEphemeral = "ephemeral"
)

// ErrInvalid rejects a state or store operation that would break the
// ephemeral/sensitive contract.
var ErrInvalid = errors.New("ownerstate: estado inválido")

// State is one short-lived reading. It never becomes a durable UserModel fact.
type State struct {
	Kind       string    `json:"kind"`
	Intensity  float64   `json:"intensity"`
	Source     string    `json:"source"`
	ObservedAt time.Time `json:"observed_at"`
	ExpiresAt  time.Time `json:"expires_at"`
	Privacy    string    `json:"privacy"`
	Persist    string    `json:"persist"`
	Evidence   []string  `json:"evidence,omitempty"`
}

// Active reports whether the state is still usable at now.
func (s State) Active(now time.Time) bool {
	return !s.ObservedAt.IsZero() && now.Before(s.ExpiresAt)
}

// Validate rejects a state that could become a durable or public medical fact.
func (s State) Validate() error {
	if strings.TrimSpace(s.Kind) == "" {
		return errors.New("ownerstate: kind vacío")
	}
	if s.ObservedAt.IsZero() {
		return errors.New("ownerstate: observed_at vacío")
	}
	if !s.ExpiresAt.After(s.ObservedAt) {
		return errors.New("ownerstate: expires_at no es posterior a observed_at")
	}
	if s.Persist != "" && s.Persist != PersistEphemeral {
		return fmt.Errorf("ownerstate: persist %q no es ephemeral", s.Persist)
	}
	if s.Privacy != "" && s.Privacy != PrivacySensitive && s.Privacy != PrivacyPrivate {
		return fmt.Errorf("ownerstate: privacy %q no es sensible/privada", s.Privacy)
	}
	return nil
}

// Receptivity is the short-lived gate for optional proposals.
type Receptivity struct {
	Level      string    `json:"level"`
	Evidence   []string  `json:"evidence,omitempty"`
	ObservedAt time.Time `json:"observed_at"`
	ExpiresAt  time.Time `json:"expires_at"`
}

// Active reports whether the reading is still usable at now.
func (r Receptivity) Active(now time.Time) bool {
	return !r.ObservedAt.IsZero() && now.Before(r.ExpiresAt)
}

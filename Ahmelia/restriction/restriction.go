// Package restriction owns the durable prohibitions declared by the owner.
//
// A restriction is not a mode and not a fact about the owner. It is a bounded
// statement that remains in force across turns, with provenance, optional
// expiry and an explicit lift that is kept in history.
package restriction

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	// ErrInvalid rejects a malformed restriction before it reaches the store.
	ErrInvalid = errors.New("restriction: inválida")
	// ErrNotFound reports an unknown identifier.
	ErrNotFound = errors.New("restriction: no encontrada")
	// ErrNotActive reports a lift on a record that no longer governs.
	ErrNotActive = errors.New("restriction: no está activa")
)

// Scope identifies what a restriction governs. The two scopes are deliberately
// separate: one subtracts Ahmelia's own initiative, the other blocks an action
// the owner asked for. They have different admission rules.
type Scope string

const (
	ScopeWorkInitiative Scope = "work_initiative"
	ScopeActionBan      Scope = "action_ban"
)

// Status is the lifecycle of a restriction. Only StatusActive governs behavior.
type Status string

const (
	StatusPending Status = "pending"
	StatusActive  Status = "active"
	StatusLifted  Status = "lifted"
	StatusExpired Status = "expired"
)

// Source records how the restriction was admitted.
type Source string

const (
	SourceExplicitOwner      Source = "explicit_owner"
	SourceConfirmedInference Source = "confirmed_inference"
)

// OwnerRestriction is one owner-declared prohibition that survives the turn.
// A nil ExpiresAt means "until explicitly lifted"; a non-nil value means the
// restriction expires on its own at that instant.
type OwnerRestriction struct {
	ID          string     `json:"id"`
	Scope       Scope      `json:"scope"`
	Target      string     `json:"target,omitempty"` // only action_ban
	Statement   string     `json:"statement"`
	Source      Source     `json:"source"`
	Turn        string     `json:"turn,omitempty"`
	Since       time.Time  `json:"since"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	Status      Status     `json:"status"`
	ConfirmedAt *time.Time `json:"confirmed_at,omitempty"`
	LiftedAt    *time.Time `json:"lifted_at,omitempty"`
	LiftedBy    string     `json:"lifted_by,omitempty"` // owner | expiry
	Evidence    []string   `json:"evidence,omitempty"`
}

// IsExpired reports whether the clock has reached the declared expiry.
func (r OwnerRestriction) IsExpired(now time.Time) bool {
	return r.ExpiresAt != nil && !now.Before(*r.ExpiresAt)
}

// IsActive reports whether the restriction governs behavior at now. Pending,
// lifted and expired records never govern.
func (r OwnerRestriction) IsActive(now time.Time) bool {
	return r.Status == StatusActive && !r.IsExpired(now)
}

// Validate rejects malformed records before they reach the store.
func (r OwnerRestriction) Validate() error {
	if strings.TrimSpace(r.ID) == "" {
		return fmt.Errorf("%w: id vacío", ErrInvalid)
	}
	switch r.Scope {
	case ScopeWorkInitiative:
		if strings.TrimSpace(r.Target) != "" {
			return fmt.Errorf("%w: work_initiative no lleva target", ErrInvalid)
		}
	case ScopeActionBan:
		if strings.TrimSpace(r.Target) == "" {
			return fmt.Errorf("%w: action_ban exige target", ErrInvalid)
		}
	default:
		return fmt.Errorf("%w: scope desconocido %q", ErrInvalid, r.Scope)
	}
	switch r.Status {
	case StatusPending, StatusActive, StatusLifted, StatusExpired:
	default:
		return fmt.Errorf("%w: status desconocido %q", ErrInvalid, r.Status)
	}
	if strings.TrimSpace(string(r.Source)) == "" {
		return fmt.Errorf("%w: source vacío", ErrInvalid)
	}
	if strings.TrimSpace(r.Statement) == "" {
		return fmt.Errorf("%w: statement vacío", ErrInvalid)
	}
	if r.Since.IsZero() {
		return fmt.Errorf("%w: since vacío", ErrInvalid)
	}
	if r.ExpiresAt != nil && !r.ExpiresAt.After(r.Since) {
		return fmt.Errorf("%w: expires_at no es posterior a since", ErrInvalid)
	}
	return nil
}

// EndOfDay returns the first instant of the next local day.
func EndOfDay(t time.Time, loc *time.Location) time.Time {
	if loc == nil {
		loc = t.Location()
	}
	y, m, d := t.In(loc).Date()
	return time.Date(y, m, d+1, 0, 0, 0, 0, loc)
}

// NewID returns an opaque stable identifier. The scene sets IDs when it needs
// reproducible traces; production admission can use this helper.
func NewID(now time.Time) string {
	var b [6]byte
	if _, err := rand.Read(b[:]); err == nil {
		return "or_" + hex.EncodeToString(b[:])
	}
	return fmt.Sprintf("or_%d", now.UnixNano())
}

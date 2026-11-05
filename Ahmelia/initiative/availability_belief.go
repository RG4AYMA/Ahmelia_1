package initiative

import (
	"fmt"
	"math"
	"sync"
	"time"
)

// PresenceBelief estima P(el usuario es alcanzable ahora) combinando:
//   - persistencia de sesión: decaimiento exponencial desde la última actividad
//   - prior circadiano aprendido por hora-de-semana (Beta(a,b) por bucket)
//
// Reemplaza la ventana fija de 2 min que colapsaba a UNKNOWN y silenciaba la
// iniciativa. Los umbrales se aplican sobre una probabilidad, no sobre tiempo.
type PresenceBelief struct {
	mu        sync.Mutex
	a, b      [168]float64
	Tau       time.Duration
	AvailAt   float64 // P >= AvailAt  => AVAILABLE
	AwayAt    float64 // P <= AwayAt   => AWAY
	OutOfHour float64 // peso del prior circadiano fuera de sesión
}

func NewPresenceBelief() *PresenceBelief {
	p := &PresenceBelief{Tau: 20 * time.Minute, AvailAt: 0.55, AwayAt: 0.30, OutOfHour: 0.5}
	for i := range p.a {
		p.a[i], p.b[i] = 1, 1
	}
	return p
}

func bucket(t time.Time) int { return int(t.Weekday())*24 + t.Hour() }

// Observe registra si en ese momento el usuario respondió / estuvo activo.
// Es el único canal de aprendizaje: evidencia observada, nunca texto.
func (p *PresenceBelief) Observe(at time.Time, responded bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	i := bucket(at)
	if responded {
		p.a[i]++
	} else {
		p.b[i]++
	}
}

func (p *PresenceBelief) Prior(at time.Time) float64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	i := bucket(at)
	return p.a[i] / (p.a[i] + p.b[i])
}

func (p *PresenceBelief) Probability(last, now time.Time) float64 {
	age := now.Sub(last)
	if age < 0 {
		age = 0
	}
	pres := math.Exp(-age.Seconds() / p.Tau.Seconds())
	return pres + (1-pres)*p.Prior(now)*p.OutOfHour
}

// Assess requiere evidencia de actividad previa; sin ella devuelve ok=false
// (UNKNOWN honesto).
func (p *PresenceBelief) Assess(e AvailabilityEvidence) (AvailabilityAssessment, bool) {
	if p == nil || e.LastUserActivity.IsZero() {
		return AvailabilityAssessment{}, false
	}
	now := e.Now
	if now.IsZero() {
		now = time.Now()
	}
	pr := p.Probability(e.LastUserActivity, now)
	reason := fmt.Sprintf("presence_belief p=%.2f", pr)
	switch {
	case pr >= p.AvailAt:
		return AvailabilityAssessment{Availability: AvailabilityAvailable, Confidence: pr, Inferred: true, Reason: reason, ObservedAt: now}, true
	case pr <= p.AwayAt:
		return AvailabilityAssessment{Availability: AvailabilityAway, Confidence: 1 - pr, Inferred: true, Reason: reason, ObservedAt: now}, true
	}
	return AvailabilityAssessment{Availability: AvailabilityUnknown, Confidence: 0, Inferred: true, Reason: reason, ObservedAt: now}, true
}

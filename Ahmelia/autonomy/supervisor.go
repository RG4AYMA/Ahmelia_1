package autonomy

import (
	"fmt"
	"time"
)

type Verdict string

const (
	VerdictContinue Verdict = "continue"
	VerdictReplan   Verdict = "replan"   // cambiar de estrategia (no reintento ciego)
	VerdictEscalate Verdict = "escalate" // pedir criterio humano
	VerdictStop     Verdict = "stop"     // terminación endógena
)

// Budget es el presupuesto de UN objetivo. Los objetivos propios llevan uno más
// chico que los del usuario.
type Budget struct {
	MaxSteps    int
	MaxWall     time.Duration
	MaxReplans  int
	Stagnation  int     // pasos consecutivos sin avance verificado => replan
	SameFailure int     // misma clase de fallo consecutiva => replan
	MinDelta    float64 // avance verificado mínimo por paso para no estancarse
}

// Step es el hecho observado tras ejecutar/verificar. Solo el avance
// VERIFICADO cuenta: Delta de un efecto no verificado se ignora.
type Step struct {
	Delta        float64 // 0..1 avance estimado del objetivo
	Verified     bool
	FailureClass string
	At           time.Time
}

type Decision struct {
	Verdict Verdict
	Reason  string
}

// Supervisor decide continuar/replanificar/escalar/detener a partir de medidas,
// no de textos. Es puro y determinista: testeable sin entorno.
type Supervisor struct {
	b         Budget
	start     time.Time
	steps     int
	progress  float64
	replans   int
	sinceGain int
	lastClass string
	classRun  int
}

func NewSupervisor(b Budget, start time.Time) *Supervisor {
	if b.Stagnation <= 0 {
		b.Stagnation = 3
	}
	if b.SameFailure <= 0 {
		b.SameFailure = 2
	}
	if b.MinDelta <= 0 {
		b.MinDelta = 0.02
	}
	return &Supervisor{b: b, start: start}
}

func (s *Supervisor) Progress() float64 { return s.progress }

func (s *Supervisor) Observe(st Step) Decision {
	s.steps++
	gain := 0.0
	if st.Verified && st.Delta > 0 {
		gain = st.Delta
		s.progress += gain
	}
	if gain >= s.b.MinDelta {
		s.sinceGain = 0
	} else {
		s.sinceGain++
	}
	if st.FailureClass != "" && st.FailureClass == s.lastClass {
		s.classRun++
	} else if st.FailureClass != "" {
		s.lastClass, s.classRun = st.FailureClass, 1
	} else {
		s.lastClass, s.classRun = "", 0
	}

	switch {
	case s.progress >= 1:
		return Decision{VerdictStop, "goal_progress_verified_complete"}
	case s.b.MaxSteps > 0 && s.steps >= s.b.MaxSteps:
		return Decision{VerdictStop, "budget_steps_exhausted"}
	case s.b.MaxWall > 0 && !st.At.IsZero() && st.At.Sub(s.start) >= s.b.MaxWall:
		return Decision{VerdictStop, "budget_wall_exhausted"}
	}
	if s.classRun >= s.b.SameFailure || s.sinceGain >= s.b.Stagnation {
		why := fmt.Sprintf("stagnation=%d same_failure=%s×%d", s.sinceGain, s.lastClass, s.classRun)
		if s.replans >= s.b.MaxReplans {
			return Decision{VerdictEscalate, "strategies_exhausted:" + why}
		}
		s.replans++
		s.sinceGain, s.classRun = 0, 0
		return Decision{VerdictReplan, why}
	}
	return Decision{VerdictContinue, "progressing"}
}

package linguistics

import (
	"context"
	"fmt"

	"ahmelia/interaction"
)

// PipelineResult contiene el resultado completo de procesar una entrada
// a través del sistema lingüístico.
//
// Si UsedLLM es false, Interpretation será nil porque el pipeline decidió
// que la entrada no necesitaba análisis lingüístico adicional.
type PipelineResult struct {
	UsedLLM        bool
	Decision       RouteDecision
	Interpretation *Interpretation
	// OpenMeaning carries the open comprehension contract result when the
	// pipeline runs in open mode (live composition root). When it is set,
	// Interpretation stays nil: the runtime reasons over meaning instead of a
	// closed predicate dictionary (owner directive 2026-10-17 16:04).
	OpenMeaning *OpenMeaning
	Warnings    []string
}

// Pipeline coordina Router, Interpreter y Validator.
//
// No toma decisiones cognitivas.
// No ejecuta acciones.
// No reemplaza la comprensión existente de Ahmelia.
//
// Su única responsabilidad es coordinar el análisis lingüístico adicional
// cuando el Router determina que una entrada lo necesita.
type Pipeline struct {
	router            *Router
	interpreter       *Interpreter
	validator         *Validator
	openComprehension bool
}

// NewPipeline crea un pipeline lingüístico completo.
func NewPipeline(
	router *Router,
	interpreter *Interpreter,
	validator *Validator,
) *Pipeline {
	return &Pipeline{
		router:      router,
		interpreter: interpreter,
		validator:   validator,
	}
}

// SetOpenComprehension switches this pipeline from the closed interpretation
// contract to the open meaning contract (owner directive 2026-10-17 16:04).
// The live composition root enables it; suites and focused fixtures keep the
// closed contract by default.
func (p *Pipeline) SetOpenComprehension(enabled bool) {
	if p == nil {
		return
	}
	p.openComprehension = enabled
}

// Process procesa una entrada a través del pipeline lingüístico.
//
// Flujo:
//
//	Input
//	  ↓
//	Router
//	  ↓
//	¿UseLLM?
//	  ├── No → resultado inmediato
//	  │
//	  └── Sí
//	       ↓
//	    Interpreter
//	       ↓
//	    Validator
//	       ↓
//	    PipelineResult
func (p *Pipeline) Process(input string) (PipelineResult, error) {
	request := interaction.Analyze(input, interaction.Context{})
	return p.ProcessRequest(context.Background(), input, request)
}

// ProcessRequest is the single live linguistic orchestration path. The core
// supplies its already-built SemanticRequest so routing is based on semantic
// completeness, not a second set of lexical heuristics.
func (p *Pipeline) ProcessRequest(ctx context.Context, input string, request interaction.SemanticRequest) (PipelineResult, error) {
	return p.ProcessRequestWithSituation(ctx, input, request, nil)
}

// ProcessRequestWithSituation is the same orchestration with an optional,
// bounded snapshot of the harness state. The snapshot is read-only: it changes
// what the model may read, never what the pipeline may decide or execute.
func (p *Pipeline) ProcessRequestWithSituation(ctx context.Context, input string, request interaction.SemanticRequest, snapshot *SituationSnapshot) (PipelineResult, error) {
	if p == nil || p.router == nil {
		return PipelineResult{}, fmt.Errorf("linguistics pipeline: router no configurado")
	}
	decision := p.router.DecideRequest(request)
	result := PipelineResult{UsedLLM: false, Decision: decision, Interpretation: nil, Warnings: []string{}}
	if !decision.UseLLM {
		return result, nil
	}
	if p.interpreter == nil {
		return result, fmt.Errorf("linguistics pipeline: se requiere Interpreter para procesar esta entrada")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if p.openComprehension {
		meaning, err := p.interpreter.InterpretOpenContextWithSituation(ctx, input, snapshot)
		if err != nil {
			return result, err
		}
		result.UsedLLM = true
		result.OpenMeaning = &meaning
		return result, nil
	}
	interpretation, err := p.interpreter.InterpretContextWithSituation(ctx, input, snapshot)
	if err != nil {
		return result, err
	}
	if p.validator == nil {
		return result, fmt.Errorf("linguistics pipeline: se requiere Validator para validar la interpretación")
	}
	validation := p.validator.Validate(input, interpretation)
	result.UsedLLM = true
	result.Interpretation = &validation.Interpretation
	result.Warnings = validation.Warnings
	return result, nil
}

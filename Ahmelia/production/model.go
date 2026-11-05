package production

import (
	"fmt"
	"time"

	"ahmelia/autonomy"
	"ahmelia/capability"
	"ahmelia/planning"
	"ahmelia/tools"
)

// ============================================================================
// PRODUCTION MODEL — B95.1
// ============================================================================
//
// B95.1 no crea un segundo Goal, Task, Plan, Action, Capability, Execution,
// Observation o Verification. Esas entidades ya existen en B94 y en las
// capas anteriores. Production las ORQUESTA mediante referencias a sus
// objetos existentes.
//
// Regla de arquitectura:
//
//   Production Core = contexto + ciclo de producción
//   Existing systems = dueños de sus entidades y decisiones
//
// Por eso este paquete contiene únicamente el contexto que hace falta para
// unirlas y un resultado de producción que distingue "una acción se ejecutó"
// de "la producción quedó terminada".

// ProductionStatus representa el estado del trabajo productivo completo.
// No sustituye planning.TaskStatus ni planning.GoalStatus: describe el
// contexto de producción como conjunto.
type ProductionStatus string

const (
	ProductionPending   ProductionStatus = "pending"
	ProductionRunning   ProductionStatus = "running"
	ProductionBlocked   ProductionStatus = "blocked"
	ProductionCompleted ProductionStatus = "completed"
	ProductionCancelled ProductionStatus = "cancelled"
)

func (s ProductionStatus) IsValid() bool {
	switch s {
	case ProductionPending, ProductionRunning, ProductionBlocked, ProductionCompleted, ProductionCancelled:
		return true
	default:
		return false
	}
}

// Production reutiliza las entidades existentes en lugar de duplicarlas.
//
// Goal, Plan y Task pertenecen a planning.
// Capability pertenece a capability.
// Action, Execution y VerifiedResult pertenecen a tools.
// Environment/Observation/Verification siguen perteneciendo a sus contratos
// de ejecución/observación existentes.
//
// WorkspaceID es únicamente una referencia estable al Workspace que B95.2
// modelará. B95.1 no inventa todavía un segundo workspace ni copia el estado
// de environment.Manager/memory_intelligence.WorkspaceSnapshot.
type Production struct {
	ID string

	Goal *planning.Goal
	Plan *planning.Plan

	CurrentTask *planning.Task

	WorkspaceID string
	Status      ProductionStatus

	CreatedAt   time.Time
	StartedAt   time.Time
	CompletedAt time.Time
	UpdatedAt   time.Time
}

// NewProduction crea el contexto productivo alrededor de un Goal existente.
// No crea otro Goal y conserva la identidad del objeto recibido.
func NewProduction(id string, goal *planning.Goal) (*Production, error) {
	if id == "" {
		return nil, fmt.Errorf("production: ID vacío")
	}
	if goal == nil {
		return nil, fmt.Errorf("production: goal nil")
	}

	now := time.Now()
	return &Production{
		ID:        id,
		Goal:      goal,
		Status:    ProductionPending,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

// BindPlan vincula el Plan existente que corresponde al mismo Goal.
// No copia ni reconstruye el Plan.
func (p *Production) BindPlan(plan *planning.Plan) error {
	if p == nil {
		return fmt.Errorf("production: nil")
	}
	if plan == nil {
		return fmt.Errorf("production: plan nil")
	}
	if p.Goal == nil {
		return fmt.Errorf("production: goal nil")
	}
	if plan.GoalID != p.Goal.ID {
		return fmt.Errorf("production: plan %q pertenece a goal %q, esperado %q", plan.ID, plan.GoalID, p.Goal.ID)
	}
	p.Plan = plan
	p.UpdatedAt = time.Now()
	return nil
}

// SetCurrentTask vincula una Task existente del mismo Goal.
// La Task no se copia: Production observa la misma identidad que planning.
func (p *Production) SetCurrentTask(task *planning.Task) error {
	if p == nil {
		return fmt.Errorf("production: nil")
	}
	if task == nil {
		return fmt.Errorf("production: task nil")
	}
	if p.Goal == nil || task.GoalID != p.Goal.ID {
		return fmt.Errorf("production: task %q no pertenece al goal de production", task.ID)
	}
	p.CurrentTask = task
	p.UpdatedAt = time.Now()
	return nil
}

// Start marca el contexto productivo como iniciado. No inicia una Task:
// planning.Task.Start() sigue siendo responsabilidad del dueño de Tasks.
func (p *Production) Start() error {
	if p == nil {
		return fmt.Errorf("production: nil")
	}
	if p.Status != ProductionPending {
		return fmt.Errorf("production: no se puede iniciar desde estado %q", p.Status)
	}
	now := time.Now()
	p.Status = ProductionRunning
	p.StartedAt = now
	p.UpdatedAt = now
	return nil
}

// Complete cierra la producción como conjunto. No fuerza el estado del Goal
// ni de la Task: esas entidades siguen teniendo sus propios métodos y reglas.
func (p *Production) Complete() error {
	if p == nil {
		return fmt.Errorf("production: nil")
	}
	if p.Status != ProductionRunning {
		return fmt.Errorf("production: no se puede completar desde estado %q", p.Status)
	}
	now := time.Now()
	p.Status = ProductionCompleted
	p.CompletedAt = now
	p.UpdatedAt = now
	return nil
}

// SetWorkspaceID enlaza el contexto a un Workspace sin duplicar su estado.
func (p *Production) SetWorkspaceID(id string) error {
	if p == nil {
		return fmt.Errorf("production: nil")
	}
	if id == "" {
		return fmt.Errorf("production: workspace ID vacío")
	}
	p.WorkspaceID = id
	p.UpdatedAt = time.Now()
	return nil
}

// ReuseMap documenta de forma ejecutable qué entidad existente es dueña de
// cada concepto de producción. Se devuelve por valor y en orden fijo para
// que auditorías/tests puedan comprobar que B95.1 no está creando un segundo
// sistema de entidades.
type ReuseEntry struct {
	Concept string
	Owner   string
}

func ReuseMap() []ReuseEntry {
	return []ReuseEntry{
		{Concept: "Goal", Owner: "planning.Goal"},
		{Concept: "Plan", Owner: "planning.Plan"},
		{Concept: "Task", Owner: "planning.Task"},
		{Concept: "Capability", Owner: "capability.Capability"},
		{Concept: "Action", Owner: "tools.Action"},
		{Concept: "Execution", Owner: "tools.Result"},
		{Concept: "Observation", Owner: "tools.Observation"},
		{Concept: "Verification", Owner: "tools.Verification"},
		{Concept: "VerifiedExecution", Owner: "tools.VerifiedResult"},
		{Concept: "EnvironmentVerification", Owner: "autonomy.EnvironmentVerificationResult"},
	}
}

// Type aliases make the reuse explicit to callers without introducing new
// semantic entities. Production code can import production.Goal, for example,
// and still receive exactly *planning.Goal.
type Goal = planning.Goal
type Plan = planning.Plan
type Task = planning.Task
type Capability = capability.Capability
type Action = tools.Action
type Execution = tools.Result
type Observation = tools.Observation
type Verification = tools.Verification
type VerifiedExecution = tools.VerifiedResult
type EnvironmentVerification = autonomy.EnvironmentVerificationResult

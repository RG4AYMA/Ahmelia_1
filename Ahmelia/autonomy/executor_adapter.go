package autonomy

import "context"

// ActionRequest es la representación canónica de una acción que el ciclo
// autónomo solicita al ejecutor. B78 define el contrato; B79 conectará este
// contrato con solving.ExecutionEngine/core.Engine.
type ActionRequest struct {
	ID     string
	Action string
	// Objective is the higher-level goal this immediate action serves. It must not be inferred from Action.
	Objective  string
	Target     string
	Parameters map[string]string
}

// ActionResponse es el resultado normalizado devuelto por el ejecutor.
type ActionResponse struct {
	RequestID       string
	Executed        bool
	Output          string
	Error           error
	AuthorityResult AuthorityOutcome
}

// ExecutorAdapter desacopla B78 de cualquier motor de ejecución concreto.
type ExecutorAdapter interface {
	ExecuteAction(ctx context.Context, req ActionRequest) ActionResponse
}

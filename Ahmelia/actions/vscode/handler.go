package vscode

import (
	"fmt"

	"ahmelia/environment"
)

// Handler implementa actions.Handler para Visual Studio Code.
type Handler struct{}

func New() *Handler {
	return &Handler{}
}

func (h *Handler) AppName() string {
	return "code"
}

func (h *Handler) Actions() []string {
	return []string{"ejecutar", "depurar", "terminal"}
}

func (h *Handler) Execute(action string, world *environment.Manager) (string, error) {
	switch action {
	case "ejecutar":
		return run(world)
	case "depurar":
		return debug(world)
	case "terminal":
		return terminal(world)
	default:
		return "", fmt.Errorf("vscode no sabe hacer '%s'", action)
	}
}

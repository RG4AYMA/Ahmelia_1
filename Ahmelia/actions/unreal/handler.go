package unreal

import (
	"fmt"

	"ahmelia/environment"
)

// Handler implementa actions.Handler para Unreal Engine.
type Handler struct{}

func New() *Handler {
	return &Handler{}
}

func (h *Handler) AppName() string {
	return "unrealeditor"
}

func (h *Handler) Actions() []string {
	return []string{"compilar", "empaquetar"}
}

func (h *Handler) Execute(action string, world *environment.Manager) (string, error) {
	switch action {
	case "compilar":
		return compile(world)
	case "empaquetar":
		return pkg(world)
	default:
		return "", fmt.Errorf("unreal no sabe hacer '%s'", action)
	}
}

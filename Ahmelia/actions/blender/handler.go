package blender

import (
	"fmt"

	"ahmelia/environment"
)

// Handler implementa actions.Handler para Blender.
type Handler struct{}

func New() *Handler {
	return &Handler{}
}

func (h *Handler) AppName() string {
	return "blender"
}

func (h *Handler) Actions() []string {
	return []string{"renderizar", "exportar"}
}

func (h *Handler) Execute(action string, world *environment.Manager) (string, error) {
	switch action {
	case "renderizar":
		return render(world)
	case "exportar":
		return export(world)
	default:
		return "", fmt.Errorf("blender no sabe hacer '%s'", action)
	}
}

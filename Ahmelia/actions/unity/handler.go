package unity

import (
	"fmt"

	"ahmelia/environment"
)

// Handler implementa actions.Handler para Unity. No se importa el
// paquete actions aquí a propósito — Go permite satisfacer una interfaz
// sin importar el paquete que la declara, y esto evita un import
// circular si algún día actions/ necesita saber algo de unity/ (poco
// probable, pero gratis evitarlo desde ya).
type Handler struct{}

// New crea el handler de Unity. Se registra desde donde se arme el
// Dispatcher (core/engine.go, cuando este plugin deje de ser un
// esqueleto).
func New() *Handler {
	return &Handler{}
}

func (h *Handler) AppName() string {
	return "unity"
}

func (h *Handler) Actions() []string {
	return []string{"compilar", "reproducir", "detener"}
}

// Execute reparte por acción. Cada función (build.go, play.go, stop.go)
// es su propio archivo a propósito, siguiendo el layout que se acordó:
// una responsabilidad por archivo, para que cuando build.go deje de ser
// un stub y de verdad invoque el compilador de Unity, no quede enterrado
// en un solo archivo gigante junto con play/stop.
func (h *Handler) Execute(action string, world *environment.Manager) (string, error) {
	switch action {
	case "compilar":
		return build(world)
	case "reproducir":
		return play(world)
	case "detener":
		return stop(world)
	default:
		return "", fmt.Errorf("unity no sabe hacer '%s'", action)
	}
}

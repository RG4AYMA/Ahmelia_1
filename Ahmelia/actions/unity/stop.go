package unity

import (
	"fmt"

	"ahmelia/environment"
)

// stop sería "detén"/"para" para Unity — salir de Play Mode. Mismo TODO
// que play.go: depende de tener un puente real con el Editor, no de
// adivinar atajos de teclado.
func stop(world *environment.Manager) (string, error) {
	return "", fmt.Errorf("detener Unity todavía no está implementado")
}

package unity

import (
	"fmt"

	"ahmelia/environment"
)

// build sería "compílalo" para Unity. TODO cuando se implemente de
// verdad: Unity no se compila con un comando de shell simple — hay que
// invocar el Editor en modo batch (-batchmode -executeMethod ...) contra
// el Project de la app activa (world.ActiveApp().Project, que ya llena
// title_parser.go). Por ahora responde honesto: no finge compilar nada.
func build(world *environment.Manager) (string, error) {
	return "", fmt.Errorf("compilar en Unity todavía no está implementado")
}

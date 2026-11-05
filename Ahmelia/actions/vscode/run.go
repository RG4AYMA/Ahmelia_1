package vscode

import (
	"fmt"

	"ahmelia/environment"
)

// run sería "ejecútalo" para VSCode. TODO: VSCode no tiene una sola
// forma de "ejecutar" — depende del stack del proyecto activo
// (world.ActiveApp().Project / .File): un tasks.json propio, "go run",
// "npm start", el botón Run de una extensión, etc. Hace falta decidir
// primero cómo detectar el stack antes de fingir que hay un botón único.
func run(world *environment.Manager) (string, error) {
	return "", fmt.Errorf("ejecutar desde VSCode todavía no está implementado")
}

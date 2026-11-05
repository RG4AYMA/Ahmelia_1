package vscode

import (
	"fmt"

	"ahmelia/environment"
)

// terminal sería "abre una terminal" dentro de VSCode. TODO: esto
// probablemente sí sea alcanzable rápido más adelante (VSCode expone
// comandos vía su CLI, `code --command`), pero se deja sin implementar
// hasta que le toque su turno — no adivinar la sintaxis exacta ahora.
func terminal(world *environment.Manager) (string, error) {
	return "", fmt.Errorf("abrir terminal en VSCode todavía no está implementado")
}

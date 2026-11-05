package vscode

import (
	"fmt"

	"ahmelia/environment"
)

// debug sería "depúralo" para VSCode — lanzar una configuración de
// launch.json. TODO: necesita leer launch.json del proyecto activo para
// saber qué configuración correr (o preguntar cuál, si hay varias).
func debug(world *environment.Manager) (string, error) {
	return "", fmt.Errorf("depurar desde VSCode todavía no está implementado")
}

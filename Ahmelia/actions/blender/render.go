package blender

import (
	"fmt"

	"ahmelia/environment"
)

// render sería "renderízalo" para Blender. TODO: Blender sí tiene una
// vía real para esto (render vía línea de comandos con --background
// --render-anim contra el .blend activo), pero primero hace falta saber
// la ruta real del archivo — WorldState hoy solo guarda el nombre de
// ventana, no la ruta en disco.
func render(world *environment.Manager) (string, error) {
	return "", fmt.Errorf("renderizar en Blender todavía no está implementado")
}

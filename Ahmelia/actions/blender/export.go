package blender

import (
	"fmt"

	"ahmelia/environment"
)

// export sería "exporta esto" para Blender (FBX, glTF, etc.). TODO:
// necesita decidir formato de salida y ruta destino — no hay un default
// razonable que adivinar sin preguntarle al usuario.
func export(world *environment.Manager) (string, error) {
	return "", fmt.Errorf("exportar desde Blender todavía no está implementado")
}

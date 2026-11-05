package unreal

import (
	"fmt"

	"ahmelia/environment"
)

// compile sería "compílalo" para Unreal — normalmente vía
// UnrealBuildTool contra el .uproject del proyecto activo. TODO: mismo
// problema que Unity, hace falta la ruta real del proyecto en disco, no
// solo el nombre que aparece en el título de la ventana.
func compile(world *environment.Manager) (string, error) {
	return "", fmt.Errorf("compilar en Unreal todavía no está implementado")
}

package unreal

import (
	"fmt"

	"ahmelia/environment"
)

// pkg sería "empaquétalo" para Unreal (build final para distribución).
// Se llama "pkg" y no "package" porque "package" es palabra reservada de
// Go — el archivo igual se llama package.go, que es lo que importa para
// el layout acordado.
//
// TODO: un empaquetado real de Unreal puede tardar minutos/horas y correr
// en segundo plano — necesita un modelo de "tarea larga" (progreso,
// cancelación) que Ahmelia todavía no tiene para ninguna acción.
func pkg(world *environment.Manager) (string, error) {
	return "", fmt.Errorf("empaquetar en Unreal todavía no está implementado")
}

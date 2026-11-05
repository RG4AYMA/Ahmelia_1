package unity

import (
	"fmt"

	"ahmelia/environment"
)

// play sería "reproduce"/"ejecútalo" para Unity — entrar a Play Mode.
// TODO: esto necesita automatizar la UI del Editor (no hay un comando de
// consola para "entrar a Play Mode" en un Unity ya abierto); la vía
// realista es un paquete Editor propio con un endpoint que Ahmelia pueda
// llamar, no simular clicks a ciegas.
func play(world *environment.Manager) (string, error) {
	return "", fmt.Errorf("reproducir en Unity todavía no está implementado")
}

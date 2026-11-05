//go:build !windows

package system

import "errors"

// Fuera de Windows no hay escritorio Win32 que reconciliar; el respaldo
// PowerShell tampoco existe ahí, así que el camino rápido falla explícito
// y GetForegroundProcess propaga el error del respaldo.
func getForegroundProcessFast() (string, uint32, error) {
	return "", 0, errors.New("reconciliación de foco rápida no soportada fuera de Windows")
}

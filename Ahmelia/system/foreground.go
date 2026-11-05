package system

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// ============================================================================
// FOREGROUND.GO — ¿qué ventana tiene el foco real, sin pagar PowerShell?
// ============================================================================
//
// Medición 2026-10-17 (latido N0/N1): cada invocación del camino PowerShell
// original (Add-Type + Get-Process) cuesta ~300-500 ms — es un spawn de
// proceso por lectura. El latido necesita reconciliar el foco a 1 Hz, así
// que ese camino era imposible de pedalear sin comerse medio núcleo. El
// camino rápido usa solo syscalls de Win32 vía syscall.NewLazyDLL (stdlib,
// sin cgo y sin tocar go.mod) y cuesta microsegundos; el camino PowerShell
// queda como respaldo para cuando lo rápido no pueda resolver (p. ej.
// procesos protegidos que no acepten OpenProcess).

// GetForegroundProcess devuelve el nombre (sin ".exe") y el PID del proceso
// que tiene el foco real en Windows en este instante — lo que el usuario
// está usando de verdad. Camino rápido por syscalls con respaldo PowerShell
// de semántica idéntica al original.
func GetForegroundProcess() (name string, pid uint32, err error) {
	if fastName, fastPID, fastErr := getForegroundProcessFast(); fastErr == nil && fastName != "" {
		return fastName, fastPID, nil
	}
	return getForegroundProcessPowerShell()
}

// getForegroundProcessPowerShell es el camino original, conservado como
// respaldo: expone GetForegroundWindow + GetWindowThreadProcessId vía
// Add-Type y resuelve el nombre con Get-Process. Un solo llamado cuesta
// cientos de milisegundos — solo se usa cuando el camino rápido falla.
func getForegroundProcessPowerShell() (name string, pid uint32, err error) {
	script := `Add-Type -TypeDefinition '` + foregroundTypeDef + `'; ` +
		`$h = [WinQuery]::GetForegroundWindow(); ` +
		`$procId = 0; ` +
		`[WinQuery]::GetWindowThreadProcessId($h, [ref]$procId) | Out-Null; ` +
		`$p = Get-Process -Id $procId -ErrorAction SilentlyContinue; ` +
		`if ($p) { "$($p.Id)|$($p.ProcessName)" }`

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command", script).Output()
	if err != nil {
		return "", 0, err
	}
	line := strings.TrimSpace(string(out))
	if line == "" {
		return "", 0, fmt.Errorf("no se pudo determinar la ventana en foco")
	}
	parts := strings.SplitN(line, "|", 2)
	if len(parts) != 2 {
		return "", 0, fmt.Errorf("respuesta inesperada de PowerShell: %s", line)
	}
	pidInt, convErr := strconv.Atoi(strings.TrimSpace(parts[0]))
	if convErr != nil {
		return "", 0, convErr
	}
	return strings.TrimSpace(parts[1]), uint32(pidInt), nil
}

// foregroundTypeDef expone GetForegroundWindow + GetWindowThreadProcessId
// — las llamadas de Win32 para preguntar "¿qué ventana tiene el foco
// AHORA MISMO de verdad?", sin depender de lo que Ahmelia recuerde
// haber abierto o enfocado ella misma. Solo lo usa el respaldo PowerShell;
// el camino rápido declara las mismas llamadas por NewLazyDLL.
const foregroundTypeDef = `using System;using System.Runtime.InteropServices;public class WinQuery{[DllImport("user32.dll")]public static extern IntPtr GetForegroundWindow();[DllImport("user32.dll")]public static extern uint GetWindowThreadProcessId(IntPtr hWnd, out uint lpdwProcessId);}`

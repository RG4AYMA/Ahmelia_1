package environment

import (
	"encoding/csv"
	"os/exec"
	"strconv"
	"strings"
)

// ============================================================================
// SCAN.GO — lo que REALMENTE hay abierto, no solo lo que Ahmelia abrió
// ============================================================================
//
// Antes de este archivo, WorldState solo sabía de una aplicación si Ahmelia
// la había abierto ella misma (RegisterOpened). Eso deja fuera todo lo que
// el usuario abrió a mano, o lo que ya estaba abierto antes de arrancar
// Ahmelia — son dos fuentes de verdad distintas, y "qué tienes abierto"
// necesita las dos, no solo la que Ahmelia controla.
//
// Primera versión de esto usaba `tasklist /v` y filtraba por el título de
// ventana "N/A". Eso se rompía en Windows en español: la columna no dice
// "N/A", dice algo como "No disponible", así que el filtro nunca hacía
// nada y aparecía TODO el proceso del sistema (lsass, svchost, csrss...).
// tasklist es texto pensado para mostrarse en pantalla, no datos — y ese
// texto cambia con el idioma de Windows.
//
// La forma correcta: preguntarle a .NET directamente si el proceso tiene
// una ventana principal con título (Process.MainWindowTitle). Esa
// propiedad es una cadena vacía o no vacía — nunca un texto traducido —
// así que funciona igual sin importar el idioma de Windows.

// osWindow es una ventana principal visible detectada en el sistema.
type osWindow struct {
	ProcessID   int    // PID observado en el sistema operativo
	ProcessName string // "chrome", sin ".exe"
	WindowTitle string // título real de la ventana principal
}

// OSProcessObservation representa un proceso visible para el usuario actual.
// MainWindowTitle vacío significa que el proceso no expone una ventana principal
// con título; no significa que el proceso esté inactivo.
type OSProcessObservation struct {
	ProcessID   int
	ProcessName string
	WindowTitle string
}

// ScanOSProcesses devuelve una lectura puntual de los procesos que Windows
// permite enumerar al usuario actual. No usa el WorldState ni infiere procesos
// a partir de aplicaciones que Ahmelia haya abierto previamente.
func ScanOSProcesses() ([]OSProcessObservation, error) {
	script := `Get-Process | Select-Object Id,ProcessName,MainWindowTitle | ConvertTo-Csv -NoTypeInformation`
	out, err := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script).Output()
	if err != nil {
		return nil, err
	}
	reader := csv.NewReader(strings.NewReader(string(out)))
	records, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(records) <= 1 {
		return []OSProcessObservation{}, nil
	}
	observations := make([]OSProcessObservation, 0, len(records)-1)
	for _, rec := range records[1:] {
		if len(rec) < 3 {
			continue
		}
		id, parseErr := strconv.Atoi(strings.TrimSpace(rec[0]))
		name := strings.TrimSpace(rec[1])
		if parseErr != nil || id <= 0 || name == "" {
			continue
		}
		observations = append(observations, OSProcessObservation{ProcessID: id, ProcessName: name, WindowTitle: strings.TrimSpace(rec[2])})
	}
	return observations, nil
}

// scanOSWindows enumera los procesos que tienen una ventana principal
// visible con título — el mismo criterio que usa Alt+Tab, básicamente.
func scanOSWindows() ([]osWindow, error) {
	processes, err := ScanOSProcesses()
	if err != nil {
		return nil, err
	}
	windows := make([]osWindow, 0, len(processes))
	for _, process := range processes {
		if strings.TrimSpace(process.WindowTitle) == "" {
			continue
		}
		windows = append(windows, osWindow{ProcessID: process.ProcessID, ProcessName: process.ProcessName, WindowTitle: process.WindowTitle})
	}
	return windows, nil
}

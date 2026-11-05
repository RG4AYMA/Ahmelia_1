package environment

import (
	"fmt"
	"strings"
)

// ============================================================================
// WORLD_MODEL.GO — World Model v1: contexto de trabajo (no monitor de PID)
// ============================================================================
//
// La diferencia con RunningApps()/Describe() no es de datos — es de nivel.
// RunningApps() dice "chrome.exe, code.exe, unity.exe" (una lista). Esto
// arma UNA lectura coherente de qué está pasando: qué app tiene el foco,
// en qué proyecto/archivo, si hay cambios sin guardar, y a qué tipo de
// actividad corresponde (programar, navegar, jugar...) — el mismo dato
// crudo que ya recogían ActiveApp()/title_parser.go, pero leído como una
// sola frase con sentido en vez de campos sueltos que había que ir a
// preguntar uno por uno.
//
// v1 se queda deliberadamente en "contexto de trabajo": app activa,
// proyecto, archivo, tipo de actividad. CPU/RAM/batería (v2) y estado
// específico por app como errores de compilación o Git (v3) son pasos
// futuros — no se adivinan aquí.

// ActivityType es la actividad de alto nivel que corresponde a la app
// activa — lo que de verdad le sirve al Reasoner para decidir, no el
// nombre del proceso.
type ActivityType string

const (
	ActivityCoding   ActivityType = "programando"
	ActivityBrowsing ActivityType = "navegando"
	ActivityGaming   ActivityType = "jugando"
	ActivityMedia    ActivityType = "escuchando o viendo contenido"
	ActivityOffice   ActivityType = "trabajando en documentos"
	ActivitySystem   ActivityType = "en una tarea de sistema"
	ActivityUnknown  ActivityType = "sin actividad clara"
)

// activityByCategory traduce la categoría ya existente (friendly.go) a
// una actividad. Deliberadamente NO se infiere nada nuevo del proceso:
// reutiliza la categorización que ya existe para RunningApps/Describe,
// para no tener dos taxonomías distintas del mismo dato.
var activityByCategory = map[AppCategory]ActivityType{
	CategoryDev:     ActivityCoding,
	CategoryBrowser: ActivityBrowsing,
	CategoryGame:    ActivityGaming,
	CategoryMedia:   ActivityMedia,
	CategoryOffice:  ActivityOffice,
	CategorySystem:  ActivitySystem,
}

// WorldSnapshot es la lectura del "qué está pasando ahora" en un
// instante — el equivalente de una foto, no de un stream. Se arma bajo
// demanda a partir del estado que Manager ya mantiene, no se guarda
// aparte (una sola fuente de verdad: AppState).
type WorldSnapshot struct {
	HasActiveApp      bool
	AppName           string // nombre amigable ("Visual Studio Code")
	Category          AppCategory
	Activity          ActivityType
	Project           string // proyecto del ARCHIVO con foco ahora mismo (puede estar vacío)
	File              string
	HasUnsavedChanges bool

	// Workspace es el proyecto STICKY (ver CurrentProject en state.go):
	// se mantiene aunque el foco haya pasado a una app sin noción de
	// proyecto (Chrome, terminal...). WorkspaceIsCurrent indica si
	// coincide con Project (foco todavía en ese mismo proyecto) o si
	// quedó "atrás" (estás navegando pero el proyecto activo sigue
	// siendo otro).
	HasWorkspace       bool
	Workspace          string
	WorkspaceIsCurrent bool
}

// unsavedMarker es cómo VS Code marca un archivo con cambios sin
// guardar en el título de su ventana ("● archivo.go - proyecto - ...").
// parseVSCodeTitle ya lo recorta al extraer File; aquí se detecta ANTES
// de recortar, sobre el WindowTitle crudo, para no perder ese dato.
const unsavedMarker = "●"

// WorldSnapshot arma la foto actual del contexto de trabajo a partir de
// la app enfocada. ok=false si no hay ninguna app activa que Ahmelia
// pueda identificar (mismo criterio conservador que ActiveApp()): mejor
// no decir nada que inventar un contexto que no es cierto.
func (m *Manager) WorldSnapshot() (*WorldSnapshot, bool) {
	app, ok := m.ActiveApp()
	if !ok {
		return nil, false
	}

	cat := CategoryOf(app.Name)
	activity, known := activityByCategory[cat]
	if !known {
		activity = ActivityUnknown
	}

	snap := &WorldSnapshot{
		HasActiveApp: true,
		AppName:      FriendlyName(app.Name),
		Category:     cat,
		Activity:     activity,
		Project:      app.Project,
		File:         app.File,
	}
	if app.WindowTitle != "" && strings.Contains(app.WindowTitle, unsavedMarker) {
		snap.HasUnsavedChanges = true
	}

	// Workspace sticky: ActiveApp() ya lo actualizó (ver manager.go) si
	// app.Project venía lleno. Se lee acá aparte para poder distinguir
	// "sigues en el mismo proyecto" de "te moviste a otra cosa".
	if project, _, ok := m.CurrentWorkspace(); ok {
		snap.HasWorkspace = true
		snap.Workspace = project
		snap.WorkspaceIsCurrent = project == app.Project
	}

	return snap, true
}

// Describe arma la narrativa en español de la foto — lo que de verdad
// se le dice al usuario cuando pregunta "qué estás haciendo" o "cuál es
// tu contexto". Vive junto al snapshot para que la frase y los datos
// que la sostienen no se desincronicen entre archivos.
func (s *WorldSnapshot) Describe() string {
	if !s.HasActiveApp {
		return "No tengo suficiente información sobre lo que estás haciendo ahora mismo"
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("Estás %s", s.Activity))

	switch {
	case s.Project != "" && s.File != "":
		b.WriteString(fmt.Sprintf(", en el proyecto %s, editando %s", s.Project, s.File))
	case s.File != "":
		b.WriteString(fmt.Sprintf(", en el archivo %s", s.File))
	default:
		b.WriteString(fmt.Sprintf(" con %s", s.AppName))
	}

	if s.HasUnsavedChanges {
		b.WriteString(" (tienes cambios sin guardar)")
	}

	// El workspace solo se menciona cuando APORTA algo — si ya estás en
	// ese mismo proyecto (WorkspaceIsCurrent), decirlo sería repetir lo
	// mismo que ya dijo la frase de arriba. Solo vale la pena cuando el
	// foco se movió a algo sin proyecto propio (navegador, terminal) y
	// el proyecto en el que en verdad estabas trabajando sigue siendo
	// otro — ahí sí es información nueva.
	if s.HasWorkspace && !s.WorkspaceIsCurrent {
		b.WriteString(fmt.Sprintf(". Tu proyecto activo sigue siendo %s", s.Workspace))
	}

	return b.String()
}

package memory_intelligence

import (
	"path/filepath"

	"ahmelia/environment"
)

// ============================================================================
// WORKSPACE.GO — "¿en qué estoy trabajando ahora mismo?", como UN objeto
// ============================================================================
//
// Por qué existe este archivo, y por qué NO reimplementa nada: environment.
// Manager ya sabe responder cada pregunta suelta (ActiveApp, ActiveWork,
// BrowserTabs, DescribeByCategory). Lo que NO existía es un solo lugar que
// las junte en una sola foto. Sin esto, cualquier consumidor futuro —el
// Nivel 2 del Reference Resolver (resolveContent: "ese archivo", "el
// proyecto"), o un reporte tipo "Workspace: VSCode / core/engine.go"—
// tendría que conocer y llamar 3-4 métodos distintos de Manager por su
// cuenta. WorkspaceSnapshot es esa foto: un struct, una función, cero
// dato nuevo inventado.
//
// A propósito, esto NO cachea nada. Cada llamada a Snapshot() refiere el
// mismo estado en vivo que ya usa reference.Resolver, porque ambos pasan
// por el mismo *environment.Manager — no hay dos fuentes de verdad.

// WorkspaceSnapshot es la foto combinada del entorno de trabajo activo.
// Todo campo queda en su zero-value cuando el dato no existe todavía
// (HasActiveApp=false, o strings vacíos) — nunca se rellena con algo
// inventado.
type WorkspaceSnapshot struct {
	HasActiveApp bool

	// AppName ya viene en su forma amigable (environment.FriendlyName),
	// no el nombre crudo de proceso.
	AppName string

	// Project/File solo se llenan si el app activo los reporta (hoy:
	// Unity/Blender vía Project, VSCode vía File — ver title_parser.go).
	// Siguen vacíos para cualquier app sin ese soporte.
	Project string
	File    string

	// Folder es el único dato que este archivo SÍ deriva (no viene
	// directo de AppState): la carpeta contenedora de File. Responde
	// "¿en qué carpeta estás trabajando?" sin que environment/ necesite
	// saber qué es una carpeta de proyecto.
	Folder string

	// BrowserTab solo se llena si el app activo es un navegador
	// soportado con pestaña activa conocida (ver chrome_adapter.go).
	BrowserTab string

	// RelatedDevApps: otras apps de categoría Desarrollo abiertas AL
	// MISMO TIEMPO que la activa (ej.: VSCode activo + una terminal con
	// "go build" abierta aparte). No incluye la app activa misma.
	RelatedDevApps []string
}

// Snapshot arma la foto actual del workspace a partir del WorldState que
// ya mantiene environment.Manager. Devuelve zero-value (HasActiveApp
// false) si no hay ninguna app activa que Ahmelia recuerde — el caller
// decide qué decir en ese caso, este archivo no inventa un mensaje.
func Snapshot(world *environment.Manager) WorkspaceSnapshot {
	snap := WorkspaceSnapshot{}

	app, ok := world.ActiveApp()
	if !ok {
		return snap
	}

	snap.HasActiveApp = true
	snap.AppName = environment.FriendlyName(app.Name)
	snap.Project = app.Project
	snap.File = app.File
	snap.BrowserTab = app.ActiveTab

	if app.File != "" {
		snap.Folder = filepath.Dir(app.File)
	}

	for _, other := range world.RunningApps() {
		if other.Name == app.Name {
			continue // no reportarse a sí misma como "relacionada"
		}
		if environment.CategoryOf(other.Name) == environment.CategoryDev {
			snap.RelatedDevApps = append(snap.RelatedDevApps, environment.FriendlyName(other.Name))
		}
	}

	return snap
}

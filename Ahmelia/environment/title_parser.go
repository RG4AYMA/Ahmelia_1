package environment

import "strings"

// ============================================================================
// TITLE_PARSER.GO — Project/File a partir del título de ventana (Entorno v2a)
// ============================================================================
//
// scan.go ya lee MainWindowTitle de cada proceso con ventana visible — eso
// no es nuevo, ya se estaba haciendo para RunningApps()/Describe(). Lo que
// faltaba era ENTENDER ese título para editores conocidos: VSCode y las
// IDEs de JetBrains ya ponen el archivo activo y el proyecto/carpeta
// directamente en el título de la ventana, con un patrón fijo y
// predecible. No hace falta ningún hook nuevo de Windows para esto — es
// texto que Ahmelia ya leía y descartaba.
//
// Esto es deliberadamente conservador: Project/File solo se llenan para
// procesos con un patrón de título conocido con certeza. Si el título no
// calza exactamente con lo esperado (por ejemplo VSCode sin ninguna
// carpeta abierta), se deja vacío en vez de adivinar y arriesgar un dato
// falso — mismo criterio que ya se usa en el resto de environment/ (ver
// comentario de AppStatus en state.go).

// titleParser extrae (file, project) del título de un proceso conocido.
// ok=false si el título no calza con el patrón esperado para ese proceso.
type titleParser func(title string) (file, project string, ok bool)

// titleParsers mapea proceso normalizado -> cómo parsear su título. Solo
// se agregan procesos donde el patrón del título es estable y conocido —
// no es un parser genérico que "adivine" para cualquier app.
var titleParsers = map[string]titleParser{
	"code":      parseVSCodeTitle,
	"pycharm64": parseJetBrainsTitle,
	"idea64":    parseJetBrainsTitle,
}

// parseVSCodeTitle entiende el formato de título de VS Code:
//
//	"archivo.go - carpeta - Visual Studio Code"   (con carpeta/workspace abierto)
//	"archivo.go - Visual Studio Code"             (archivo suelto, sin carpeta)
//	"Visual Studio Code"                          (nada abierto)
//
// El separador es " - " (guion normal con espacios). Un archivo con
// cambios sin guardar antepone "● " al nombre; se recorta.
func parseVSCodeTitle(title string) (file, project string, ok bool) {
	const suffix = " - Visual Studio Code"
	if !strings.HasSuffix(title, suffix) {
		return "", "", false
	}
	body := strings.TrimSuffix(title, suffix)
	if body == "" {
		return "", "", false
	}
	parts := strings.Split(body, " - ")
	file = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(parts[0]), "●"))
	if len(parts) >= 2 {
		project = strings.TrimSpace(parts[len(parts)-1])
	}
	return file, project, file != ""
}

// parseJetBrainsTitle entiende el formato de título de las IDEs JetBrains
// (PyCharm, IntelliJ, GoLand...):
//
//	"archivo.go – NombreDelProyecto – PyCharm"
//
// A propósito usan un guion largo "–" (en dash), no un guion normal "-",
// así que el separador es distinto al de VSCode — de ahí parsers separados
// en vez de uno solo "genérico".
func parseJetBrainsTitle(title string) (file, project string, ok bool) {
	parts := strings.Split(title, " – ")
	if len(parts) < 2 {
		return "", "", false
	}
	file = strings.TrimSpace(parts[0])
	project = strings.TrimSpace(parts[1])
	return file, project, file != "" && project != ""
}

// titleAdapter es la primera implementación real de Adapter (v2a): sabe
// enriquecer procesos con un patrón de título conocido (VSCode,
// JetBrains) leyendo el WindowTitle que scan.go ya captura. Cuando llegue
// chromeAdapter, vivirá en su propio archivo con su propia lógica de
// conexión — no comparte nada con este salvo la interfaz.
type titleAdapter struct{}

func (t *titleAdapter) Name() string { return "window_title" }

func (t *titleAdapter) Supports(processName string) bool {
	_, known := titleParsers[processName]
	return known
}

// Collect aplica el parser correspondiente al proceso. Si el título deja
// de calzar con el patrón esperado (se cerró la carpeta, quedó en
// blanco...), limpia Project/File explícitamente en vez de dejar un dato
// viejo que ya no es cierto.
func (t *titleAdapter) Collect(app *AppState) bool {
	parser, known := titleParsers[app.Name]
	if !known {
		return false
	}
	if app.WindowTitle == "" {
		app.File, app.Project = "", ""
		return false
	}
	file, project, ok := parser(app.WindowTitle)
	if !ok {
		app.File, app.Project = "", ""
		return false
	}
	app.File = file
	app.Project = project
	return true
}

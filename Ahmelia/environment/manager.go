package environment

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"ahmelia/shared"
	"ahmelia/system"
)

// ============================================================================
// MANAGER.GO — único punto de entrada al WorldState
// ============================================================================
//
// Mismo principio que knowledge.Engine con el conocimiento: nadie debería
// tocar WorldState directamente, todo pasa por Manager. Es thread-safe a
// propósito — cuando exista el Task Manager (multitarea, otra estrella del
// roadmap) varios trabajos podrán actualizar el entorno al mismo tiempo que
// Ahmelia sigue conversando.

type Manager struct {
	mu                sync.Mutex
	state             WorldState
	collector         *Collector // v2b: adapters (VSCode hoy, Chrome/Discord futuro)
	foregroundProcess func() (string, uint32, error)
}

func NewManager() *Manager {
	return &Manager{
		state: WorldState{
			RunningApps: make(map[string]*AppState),
		},
		collector:         NewCollector(),
		foregroundProcess: system.GetForegroundProcess,
	}
}

// RegisterOpened marca una aplicación como abierta y la vuelve la
// "referencia activa": si cinco minutos después el usuario dice
// "ciérralo", Ahmelia ya sabe a quién se refiere sin que repita el nombre.
func (m *Manager) RegisterOpened(name, target string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := shared.Normalize(name)
	if key == "" {
		return
	}
	now := time.Now()
	if app, ok := m.state.RunningApps[key]; ok {
		app.Status = StatusAbierta
		app.Source = SourceAhmelia
		app.LastFocusAt = now
		if target != "" {
			app.Target = target
		}
	} else {
		m.state.RunningApps[key] = &AppState{
			Name:        key,
			Target:      target,
			Status:      StatusAbierta,
			Source:      SourceAhmelia,
			OpenedAt:    now,
			LastFocusAt: now,
		}
	}
	m.state.ActiveApp = key
}

// RegisterFocused vuelve una app la "referencia activa" (ej: "cambia a
// chrome"). Si Ahmelia no la conocía todavía —el caso típico: el usuario
// la abrió a mano y solo le pidió a Ahmelia "cambia a X"— la registra
// aquí mismo, mínimamente, en vez de dejar ActiveApp apuntando a una
// llave que no existe en el mapa. Antes de este fix, cambiar a una app
// desconocida rompía la referencia implícita silenciosamente: ActiveApp
// quedaba con un nombre que ActiveApp() nunca podía resolver.
func (m *Manager) RegisterFocused(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := shared.Normalize(name)
	if key == "" {
		return
	}
	now := time.Now()
	if app, ok := m.state.RunningApps[key]; ok {
		app.LastFocusAt = now
		app.Status = StatusAbierta // si acabamos de cambiarnos a ella, está abierta
	} else {
		m.state.RunningApps[key] = &AppState{
			Name:        key,
			Status:      StatusAbierta,
			Source:      SourceSistema, // Ahmelia no la abrió, solo la referenció
			OpenedAt:    now,
			LastFocusAt: now,
		}
	}
	m.state.ActiveApp = key
}

// RegisterClosed marca una app como cerrada. Si name viene vacío, cierra
// la que esté activa — así "ciérralo" no necesita saber el nombre.
// Devuelve el nombre normalizado de lo que cerró, y si de verdad existía.
//
// A propósito, esto NO borra ActiveApp cuando cierra la app que estaba
// activa. La primera versión sí lo hacía, y eso rompía "ábrelo": cerrar
// algo hacía que WorldState "se le olvidara" inmediatamente, así que no
// había a qué app volver. "Activa" pasó a significar "de la que estamos
// hablando ahora" (para referencias implícitas), no "está abierta ahora
// mismo" — eso último ya lo dice AppState.Status.
func (m *Manager) RegisterClosed(name string) (closedName string, ok bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := shared.Normalize(name)
	if key == "" {
		key = m.state.ActiveApp
	}
	if key == "" {
		return "", false
	}
	app, exists := m.state.RunningApps[key]
	if !exists {
		return "", false
	}
	app.Status = StatusCerrada
	return key, true
}

// refreshFromOS reconcilia el WorldState con lo que el sistema operativo
// realmente tiene abierto ahora mismo (escritorio del usuario), no solo
// lo que Ahmelia abrió ella misma. Las apps que Ahmelia SÍ controla
// (Source == SourceAhmelia) no se pisan con datos del escaneo — solo se
// confirma que siguen abiertas. Si el escaneo falla (por ejemplo,
// `tasklist` no disponible), simplemente no toca nada: WorldState sigue
// funcionando con lo que ya sabía.
func (m *Manager) refreshFromOS() {
	windows, err := scanOSWindows()
	if err != nil {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	seen := make(map[string]bool)
	for _, w := range windows {
		name := strings.ToLower(w.ProcessName)
		if name == "" {
			continue
		}
		seen[name] = true
		if app, ok := m.state.RunningApps[name]; ok {
			app.Status = StatusAbierta
			app.WindowTitle = w.WindowTitle
			m.collector.Enrich(app)
			continue
		}
		newApp := &AppState{
			Name:        name,
			Status:      StatusAbierta,
			Source:      SourceSistema,
			WindowTitle: w.WindowTitle,
			OpenedAt:    time.Now(),
			LastFocusAt: time.Now(),
		}
		m.collector.Enrich(newApp)
		m.state.RunningApps[name] = newApp
	}

	// Las apps detectadas por escaneo (SourceSistema) que el SO ya no ve,
	// se marcan cerradas. Las que Ahmelia abrió ella misma se dejan en
	// paz: no queremos que un escaneo parcial las "cierre" por error.
	for key, app := range m.state.RunningApps {
		if app.Source == SourceSistema && !seen[key] {
			app.Status = StatusCerrada
		}
	}
}

// consoleProcesses son procesos que alojan la propia consola de Ahmelia
// (o una terminal en general). Si Windows dice que el foco está ahí, no
// significa nada útil como "referencia activa" — el usuario está mirando
// una terminal, no una app en particular — así que se ignora y se deja
// lo que WorldState ya recordaba, en vez de sobreescribirlo con ruido.
var consoleProcesses = map[string]bool{
	"cmd": true, "powershell": true, "windowsterminal": true,
	"conhost": true, "go": true,
}

// syncActiveFromOS le pregunta a Windows qué ventana tiene el foco real
// ahora mismo y, si es una app identificable (no la propia consola),
// la vuelve la referencia activa — incluso si el usuario llegó ahí a
// mano, sin pasar por ningún comando de Ahmelia. Esto es lo que evita
// que "ábrelo" se quede pegado a la última app que Ahmelia manejó por
// voz si mientras tanto cambiaste de ventana con el mouse.
func (m *Manager) syncActiveFromOS() {
	getForeground := m.foregroundProcess
	if getForeground == nil {
		getForeground = system.GetForegroundProcess
	}
	name, _, err := getForeground()
	if err != nil || name == "" {
		return
	}
	key := shared.Normalize(name)
	if key == "" || consoleProcesses[key] {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	if app, ok := m.state.RunningApps[key]; ok {
		app.LastFocusAt = now
		app.Status = StatusAbierta
	} else {
		m.state.RunningApps[key] = &AppState{
			Name:        key,
			Status:      StatusAbierta,
			Source:      SourceSistema,
			OpenedAt:    now,
			LastFocusAt: now,
		}
	}
	m.state.ActiveApp = key
}

// SyncForeground es la percepción BARATA para trabajos periódicos (latido
// N0, 2026-10-17): reconcilia SOLO quién tiene el foco real ahora mismo
// (mismo syncActiveFromOS de siempre — ignora consolas, no pisa la
// semántica de referencia activa) sin disparar el escaneo completo de
// ventanas. Desde el mismo día system.GetForegroundProcess usa syscalls
// Win32 (microsegundos) en vez de PowerShell (~300-500 ms por spawn), así
// que esta llamada es apta para cadencia de 1 Hz.
func (m *Manager) SyncForeground() {
	m.syncActiveFromOS()
}

// ActiveAppKey devuelve la llave normalizada de la referencia activa SIN
// escanear el sistema operativo — lectura pura del estado ya reconciliado.
// El latido N0 la usa para detectar cambios de foco a 1 Hz sin costo.
// ok=false si todavía no hay ninguna referencia activa.
func (m *Manager) ActiveAppKey() (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state.ActiveApp == "" {
		return "", false
	}
	return m.state.ActiveApp, true
}

// ActiveApp devuelve la app que Ahmelia entiende como "de la que estamos
// hablando ahora" — la pieza que la resolución de referencias implícitas
// (ábrelo, vuelve a eso) consume directamente. Antes de responder,
// reconcilia con el foco real de Windows (ver syncActiveFromOS): la
// memoria interna solo se actualiza cuando el usuario pasa por un
// comando de voz, así que sin esto quedaría desactualizada en cuanto el
// usuario cambiara de ventana a mano.
func (m *Manager) ActiveApp() (*AppState, bool) {
	// Antes esto solo sincronizaba QUIÉN tiene el foco (syncActiveFromOS),
	// pero nunca releía el TÍTULO de esa ventana — eso solo pasaba dentro
	// de refreshFromOS(), y refreshFromOS() solo se llamaba desde
	// RunningApps() ("qué tienes abierto"). Resultado: si nadie preguntaba
	// "qué tienes abierto" primero, Project/File (ver title_parser.go)
	// nunca se llenaban, sin importar si la app la abrió Ahmelia o el
	// usuario a mano — el dato simplemente no se refrescaba por este
	// camino. refreshFromOS() ya distingue Source internamente (no pisa
	// apps SourceAhmelia, solo confirma que siguen abiertas), así que es
	// seguro llamarlo aquí también.
	m.refreshFromOS()
	m.syncActiveFromOS()

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.state.ActiveApp == "" {
		return nil, false
	}
	app, ok := m.state.RunningApps[m.state.ActiveApp]
	if !ok {
		return nil, false
	}

	// Workspace sticky (ver comentario de CurrentProject en state.go):
	// se actualiza acá, en la ÚNICA función que ya recorre el camino
	// completo (refreshFromOS + syncActiveFromOS) antes de leer el
	// estado — así no hace falta duplicar esa lógica en otro lado.
	if app.Project != "" {
		m.state.CurrentProject = app.Project
		m.state.ProjectLastSeenAt = time.Now()
	}

	appCopy := *app
	return &appCopy, true
}

// CurrentWorkspace devuelve el proyecto "activo" de forma persistente —
// a diferencia de AppState.Project (que solo existe mientras el foco
// esté literalmente en un editor con ese proyecto abierto), esto se
// mantiene aunque hayas tabulado a Chrome o a una terminal suelta. ok es
// false si Ahmelia todavía no ha visto NINGÚN proyecto en esta sesión.
func (m *Manager) CurrentWorkspace() (project string, lastSeen time.Time, ok bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state.CurrentProject == "" {
		return "", time.Time{}, false
	}
	return m.state.CurrentProject, m.state.ProjectLastSeenAt, true
}

// RunningApps devuelve una copia de las apps que Ahmelia cree abiertas
// ahora mismo — combinando lo que ella misma abrió con lo que detecta
// escaneando el sistema operativo (lo que el usuario abrió a mano).
func (m *Manager) RunningApps() []*AppState {
	m.refreshFromOS()

	m.mu.Lock()
	defer m.mu.Unlock()

	result := []*AppState{}
	for _, app := range m.state.RunningApps {
		if app.Status == StatusAbierta {
			appCopy := *app
			result = append(result, &appCopy)
		}
	}
	return result
}

// DescribeByCategory funciona igual que Describe() pero filtrado a una
// sola categoría — la base de "¿qué navegadores tienes abiertos?". El
// segundo valor de retorno distingue "no hay nada de esa categoría" (que
// sigue siendo una respuesta válida) de "no pude determinar nada".
//
// Dos casos especiales, salidos directamente de probar esto con datos
// reales (ver feedback: "una consola no representa que estés
// desarrollando" / "Steam es un launcher, no un juego"):
//
//  1. CategoryDev: si lo único abierto son terminales (cmd/powershell),
//     no se afirma "tienes un programa de desarrollo" — se dice, más
//     honesto, que solo hay una consola abierta.
//  2. CategoryGame: si no hay ningún juego pero SÍ hay un launcher
//     (Steam, Epic...) abierto, se lo dice explícitamente en vez de
//     responder un "no tengo nada" que ignora lo que sí está abierto.
func (m *Manager) DescribeByCategory(cat AppCategory) (string, bool) {
	all := m.RunningApps()

	apps := make([]*AppState, 0, len(all))
	for _, a := range all {
		if CategoryOf(a.Name) == cat {
			apps = append(apps, a)
		}
	}

	if cat == CategoryDev {
		primary := make([]*AppState, 0, len(apps))
		secondary := make([]*AppState, 0, len(apps))
		for _, a := range apps {
			if isSecondaryDev(a.Name) {
				secondary = append(secondary, a)
			} else {
				primary = append(primary, a)
			}
		}
		if len(primary) == 0 && len(secondary) > 0 {
			if len(secondary) == 1 {
				return fmt.Sprintf("Solo veo abierta %s, ningún editor ni motor", FriendlyName(secondary[0].Name)), true
			}
			return fmt.Sprintf("Solo veo abiertas consolas (%s), ningún editor ni motor", joinFriendlyNames(secondary)), true
		}
	}

	if cat == CategoryGame && len(apps) == 0 {
		launchers := make([]*AppState, 0)
		for _, a := range all {
			if CategoryOf(a.Name) == CategoryLauncher {
				launchers = append(launchers, a)
			}
		}
		if len(launchers) > 0 {
			return fmt.Sprintf("%s abierto, pero no hay ningún juego ejecutándose", joinFriendlyNames(launchers)), true
		}
	}

	if len(apps) == 0 {
		return fmt.Sprintf("No tengo ningún %s abierto que yo sepa", categoryLabel(cat, false)), true
	}
	if len(apps) == 1 {
		return fmt.Sprintf("Tienes %s abierto: %s", categoryLabel(cat, false), FriendlyName(apps[0].Name)), true
	}
	return fmt.Sprintf("Tienes %d %s abiertos: %s", len(apps), categoryLabel(cat, true), joinFriendlyNames(apps)), true
}

// joinFriendlyNames arma "Chrome, Steam, WinRAR" a partir de una lista de
// AppState, usando siempre el nombre amigable en vez del proceso crudo.
func joinFriendlyNames(apps []*AppState) string {
	names := ""
	for i, a := range apps {
		if i > 0 {
			names += ", "
		}
		names += FriendlyName(a.Name)
	}
	return names
}

// Describe genera una descripción en texto plano del estado actual,
// pensada para conectarse al Knowledge Core como una fuente más
// (ver knowledge.NewEnvironmentSource).
func (m *Manager) Describe() (string, bool) {
	apps := m.RunningApps()
	if len(apps) == 0 {
		return "", false
	}
	if len(apps) == 1 {
		return fmt.Sprintf("Tienes %s abierto", FriendlyName(apps[0].Name)), true
	}
	names := ""
	for i, a := range apps {
		if i > 0 {
			names += ", "
		}
		names += FriendlyName(a.Name)
	}
	return fmt.Sprintf("Tienes %d aplicaciones abiertas: %s", len(apps), names), true
}

// ActiveWork describe EN QUÉ está trabajando el usuario ahora mismo —
// no solo qué app tiene abierta, sino Project/File si el editor activo
// tiene un título reconocible (ver title_parser.go). Esta es la primera
// pieza de "Estado del entorno" (v2a): un nivel más allá de "referencia
// activa", que solo sabía CUÁL app, no QUÉ estás haciendo en ella.
//
// Se apoya en ActiveApp() (que ya reconcilia con el foco real de
// Windows) en vez de reinventar esa lógica — si el foco cambió a mano,
// esto también lo refleja.
func (m *Manager) ActiveWork() (string, bool) {
	app, ok := m.ActiveApp()
	if !ok {
		return "", false
	}
	if app.File == "" {
		// App activa conocida pero sin File/Project (o proceso sin
		// parser de título todavía) — no es un error, solo no hay
		// nada más específico que decir que el nombre de la app.
		return "", false
	}
	if app.Project != "" {
		return fmt.Sprintf(
			"Tienes %s abierto en el proyecto %s, en el archivo %s",
			FriendlyName(app.Name), app.Project, app.File,
		), true
	}
	return fmt.Sprintf(
		"Tienes %s abierto en el archivo %s",
		FriendlyName(app.Name), app.File,
	), true
}

// BrowserTabs describe las pestañas del navegador activo — el
// equivalente de ActiveWork() pero para Chrome (ver chrome_adapter.go)
// en vez de VSCode. ok=false cubre dos casos distintos a propósito:
//
//  1. La app activa no es un navegador soportado (o no hay app activa).
//  2. SÍ es Chrome, pero no tiene el puerto de depuración habilitado —
//     caso normal y esperado (ver comentario en chrome_adapter.go), no
//     un error. El caller decide qué decir en cada caso; este método
//     solo informa "no tengo datos", no por qué.
func (m *Manager) BrowserTabs() (string, bool) {
	app, ok := m.ActiveApp()
	if !ok || len(app.Tabs) == 0 {
		return "", false
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "En %s tienes %d pestañas abiertas: ", FriendlyName(app.Name), len(app.Tabs))
	for i, t := range app.Tabs {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(t.Title)
	}
	if app.ActiveTab != "" {
		fmt.Fprintf(&sb, ". La pestaña activa es: %s", app.ActiveTab)
	}
	return sb.String(), true
}

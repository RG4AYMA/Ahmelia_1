package environment

import "time"

// ============================================================================
// STATE.GO — Modelo del Entorno (WorldState)
// ============================================================================
//
// Idea central: hasta ahora Ahmelia EJECUTA acciones pero no recuerda qué
// quedó abierto ni de qué se estaba hablando a nivel de "mundo real". Por
// eso "ciérralo" o "vuelve a eso" no tenían a qué aferrarse. WorldState es
// el lugar único donde Ahmelia guarda "qué creo que existe ahora mismo".
//
// A propósito, este primer corte NO incluye CPU, GPU, batería, Bluetooth
// ni dispositivos conectados. Meterlos aquí sin una fuente real que los
// alimente daría una falsa sensación de que Ahmelia los mide, cuando en
// realidad nadie los está leyendo todavía. Se añaden cuando exista un
// sensor real (Fase 3), no antes.

// AppStatus representa lo que Ahmelia CREE sobre una aplicación — no es
// un espejo perfecto del sistema operativo (eso requeriría hooks más
// profundos de Windows), es lo que ella misma abrió/cerró o el usuario
// le confirmó.
type AppStatus string

const (
	StatusAbierta AppStatus = "abierta"
	StatusCerrada AppStatus = "cerrada"
)

// AppSource distingue CÓMO Ahmelia se enteró de esta app: si la abrió
// ella misma (control total, sabe Project/File) o si solo la detectó
// mirando el sistema operativo (el usuario la abrió a mano, o ya estaba
// abierta antes de arrancar Ahmelia).
type AppSource string

const (
	SourceAhmelia AppSource = "ahmelia" // Ahmelia la abrió/enfocó ella misma
	SourceSistema AppSource = "sistema" // detectada escaneando el SO
)

// AppState es lo que Ahmelia sabe de UNA aplicación en un momento dado.
// Project y File quedan listos para cuando Unity/Blender/VSCode (Fase 3)
// empiecen a reportar en qué proyecto/archivo están — hoy quedan vacíos.
type AppState struct {
	Name        string // nombre normalizado ("vlc", "chrome", "unity"...)
	Target      string // lo que se le pidió abrir (target original, sin normalizar)
	Status      AppStatus
	Source      AppSource
	WindowTitle string // solo se llena si viene de scanOSWindows
	OpenedAt    time.Time
	LastFocusAt time.Time
	Project     string // futuro: proyecto cargado (Unity/Blender)
	File        string // futuro: archivo activo (VSCode)

	// Tabs/ActiveTab son el equivalente de Project/File pero para
	// navegadores: en vez de "proyecto + archivo", "pestañas abiertas +
	// cuál está activa". Se llenan vía chromeAdapter (DevTools Protocol,
	// ver chrome_adapter.go) — quedan vacíos para cualquier app que no
	// sea un navegador soportado, o si Chrome no tiene el puerto de
	// depuración habilitado.
	Tabs      []TabInfo
	ActiveTab string
}

// TabInfo es UNA pestaña de navegador, tal como la reporta Chrome
// DevTools Protocol: título de la página y su URL.
type TabInfo struct {
	Title string
	URL   string
}

// WorldState es el estado del entorno que Ahmelia cree que existe ahora
// mismo. ActiveApp es la pieza clave: la última aplicación referenciada,
// de la que dependen las referencias implícitas ("ábrelo", "ciérralo")
// que se construirán sobre este módulo.
type WorldState struct {
	RunningApps map[string]*AppState // key: nombre normalizado
	ActiveApp   string               // key normalizada de la última app referenciada

	// CurrentProject es el "workspace" — un concepto DISTINTO del
	// archivo/app con foco en este instante. AppState.Project (de
	// title_parser.go) solo existe mientras el foco esté literalmente
	// en VSCode/JetBrains con ese proyecto abierto: en el momento en
	// que tabulas a Chrome para buscar algo, ese dato desaparece, y
	// Ahmelia "olvidaría" en qué proyecto estabas por algo tan trivial
	// como cambiar de ventana un momento.
	//
	// CurrentProject es sticky: se actualiza cada vez que SE VE un
	// proyecto real (foco en un editor con proyecto abierto), y se
	// queda con ese valor hasta que aparezca uno DISTINTO — no se
	// borra solo porque el foco pasó a una app sin noción de proyecto
	// (navegador, terminal suelta, etc.).
	CurrentProject    string
	ProjectLastSeenAt time.Time
}

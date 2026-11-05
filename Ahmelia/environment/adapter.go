package environment

// ============================================================================
// ADAPTER.GO — Environment Collector (interfaz de v2b)
// ============================================================================
//
// Hasta v2a, "entender qué está haciendo una app" y "leer su título de
// ventana" eran la misma cosa: title_parser.go tenía el conocimiento
// pegado directamente a refreshFromOS(). Eso funciona para VSCode, pero
// Chrome no va a dar su estado (pestañas, URL activa) por título de
// ventana — va a ser DevTools Protocol, una fuente completamente
// distinta. Si se sigue pegando cada fuente nueva directo en
// refreshFromOS(), cada app nueva (Chrome, luego Discord, luego Unity)
// es un parche más en el mismo lugar.
//
// Este archivo separa "CÓMO se detecta que una app existe" (scan.go, ya
// existía) de "CÓMO se entiende qué está haciendo" (este archivo, nuevo).
// El flujo completo queda así:
//
//	Sistema operativo
//	       │
//	       ▼
//	scanOSWindows()        — QUÉ hay abierto (ya existía, scan.go)
//	       │
//	       ▼
//	Collector.Enrich()     — QUÉ está haciendo cada app (este archivo)
//	 ├─ titleAdapter         VSCode/JetBrains, por título de ventana (v2a)
//	 └─ chromeAdapter        Chrome, por DevTools Protocol (v2b)
//	       │
//	       ▼
//	AppState                Project/File hoy; Tabs/URL cuando exista
//	       │                chromeAdapter — sin tocar este archivo otra vez
//	       ▼
//	knowledge.EnvironmentSource → Reasoner / lenguaje
//
// Agregar una app nueva (Discord, Unity...) es escribir un Adapter más y
// registrarlo en NewCollector() — no tocar manager.go ni refreshFromOS().

// Adapter enriquece una AppState con contexto más rico que "está abierta
// o no" (eso ya lo sabe scan.go). Cada Adapter obtiene su dato de una
// fuente distinta — título de ventana, protocolo de depuración, archivo
// de sesión — pero todos hablan el mismo idioma hacia el Collector.
type Adapter interface {
	// Name identifica el adapter para trazabilidad/debug — no el
	// proceso que soporta.
	Name() string

	// Supports indica si este adapter sabe enriquecer este proceso
	// (nombre normalizado, ej. "code", "chrome"). Un mismo proceso no
	// debería tener más de un adapter que lo soporte; si llega a pasar,
	// gana el primero registrado en el Collector.
	Supports(processName string) bool

	// Collect intenta enriquecer app con contexto más rico. Devuelve
	// ok=false si no pudo obtener nada confiable — en ese caso NO debe
	// dejar datos parciales ni inventados en app (mismo criterio que ya
	// usaba enrichFromTitle en v2a: mejor vacío que falso).
	Collect(app *AppState) (ok bool)
}

// Collector corre los Adapters registrados sobre una AppState. Es el
// único punto donde Manager necesita saber que existen adapters — así
// agregar uno nuevo (Chrome, Discord...) no toca manager.go, solo se
// registra aquí.
type Collector struct {
	adapters []Adapter
}

// NewCollector arma un Collector con los adapters conocidos hoy. Cuando
// exista chromeAdapter (DevTools Protocol), se agrega a esta lista y en
// ningún otro lado.
func NewCollector() *Collector {
	return &Collector{
		adapters: []Adapter{
			&titleAdapter{},
			newChromeAdapter(),
		},
	}
}

// Enrich busca el primer adapter que soporte este proceso y lo aplica.
// Si ninguno aplica, o el que aplica no obtuvo nada, no toca app.
func (c *Collector) Enrich(app *AppState) {
	for _, a := range c.adapters {
		if a.Supports(app.Name) {
			a.Collect(app)
			return
		}
	}
}

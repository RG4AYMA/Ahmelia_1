package environment

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

// ============================================================================
// CHROME_ADAPTER.GO — Chrome vía DevTools Protocol (v2b)
// ============================================================================
//
// A diferencia de titleAdapter (VSCode/JetBrains, que lee el título de
// ventana que scan.go ya capturaba), Chrome necesita una fuente distinta:
// el título de ventana solo da la pestaña ACTIVA, nunca la lista completa
// de pestañas. Para eso existe Chrome DevTools Protocol (CDP): si Chrome
// se lanzó con --remote-debugging-port=9222 (o el puerto que sea), expone
// un endpoint HTTP local con la lista de pestañas reales.
//
// LIMITACIÓN IMPORTANTE, y hay que ser honesta con el usuario sobre esto:
// Chrome NO habilita ese puerto por defecto — es una medida de seguridad
// intencional de Google, no una limitación de Ahmelia. Si el usuario abre
// Chrome haciendo doble clic normal, este adapter simplemente no va a
// encontrar nada que leer, y eso es lo esperado, no un bug. Para que
// funcione, Chrome tiene que arrancar con ese flag (típicamente hay que
// cerrar TODAS las ventanas de Chrome primero, porque el flag solo se
// aplica al arrancar un proceso nuevo).
//
// CDP tampoco marca directamente "esta es la pestaña con foco" — su
// endpoint /json solo lista pestañas, sin indicar cuál está activa. Por
// eso se combina con lo que YA se tenía de v2a: el WindowTitle de scan.go
// (que sí refleja la pestaña visible ahora mismo) se usa para encontrar,
// dentro de la lista de CDP, cuál pestaña es esa.

const chromeDebugPort = "9222" // puerto por defecto de --remote-debugging-port

// chromeAdapter implementa Adapter para Chrome, vía DevTools Protocol.
type chromeAdapter struct {
	// client con timeout corto: esto corre en el mismo hilo que responde
	// preguntas por voz, así que si el puerto no responde (lo normal si
	// Chrome no se lanzó con el flag), no puede dejar a Ahmelia colgada
	// esperando una conexión que nunca llega.
	client *http.Client
}

func newChromeAdapter() *chromeAdapter {
	return &chromeAdapter{
		client: &http.Client{Timeout: 300 * time.Millisecond},
	}
}

func (c *chromeAdapter) Name() string { return "chrome_devtools" }

func (c *chromeAdapter) Supports(processName string) bool {
	return processName == "chrome"
}

// cdpTarget es una entrada del JSON que devuelve el endpoint /json de
// Chrome DevTools Protocol. Chrome expone más campos, pero solo estos
// tres importan aquí.
type cdpTarget struct {
	Type  string `json:"type"`
	Title string `json:"title"`
	URL   string `json:"url"`
}

// Collect pide la lista de pestañas a CDP y, si responde, arma Tabs +
// ActiveTab en app. Si el puerto no está habilitado (caso normal y
// esperado, ver comentario arriba) devuelve ok=false sin dejar datos
// viejos o inventados.
func (c *chromeAdapter) Collect(app *AppState) bool {
	targets, err := c.fetchTargets()
	if err != nil || len(targets) == 0 {
		app.Tabs, app.ActiveTab = nil, ""
		return false
	}

	tabs := make([]TabInfo, 0, len(targets))
	for _, t := range targets {
		// type=="page" filtra pestañas reales — CDP también reporta
		// "background_page", "service_worker", "extension"... que no
		// son pestañas que el usuario esté viendo.
		if t.Type != "page" {
			continue
		}
		tabs = append(tabs, TabInfo{Title: t.Title, URL: t.URL})
	}
	if len(tabs) == 0 {
		app.Tabs, app.ActiveTab = nil, ""
		return false
	}

	app.Tabs = tabs
	app.ActiveTab = matchActiveTab(app.WindowTitle, tabs)
	return true
}

// fetchTargets llama al endpoint HTTP local de CDP. No usa el proceso
// PID para nada — CDP no distingue instancias por PID, solo por puerto.
func (c *chromeAdapter) fetchTargets() ([]cdpTarget, error) {
	resp, err := c.client.Get("http://localhost:" + chromeDebugPort + "/json")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var targets []cdpTarget
	if err := json.NewDecoder(resp.Body).Decode(&targets); err != nil {
		return nil, err
	}
	return targets, nil
}

// matchActiveTab reconcilia el WindowTitle (que sí refleja la pestaña
// visible ahora mismo, vía scan.go) contra la lista de pestañas de CDP
// (que no dice cuál está activa). Devuelve "" si no se pudo determinar
// con confianza — mejor no afirmar cuál está activa que adivinar mal.
func matchActiveTab(windowTitle string, tabs []TabInfo) string {
	pageTitle, ok := extractChromePageTitle(windowTitle)
	if !ok {
		return ""
	}
	// Coincidencia exacta primero.
	for _, tab := range tabs {
		if tab.Title == pageTitle {
			return tab.Title
		}
	}
	// Chrome recorta títulos largos en la barra de la ventana — de ahí
	// el segundo intento por contención, no solo igualdad exacta.
	for _, tab := range tabs {
		if strings.Contains(tab.Title, pageTitle) || strings.Contains(pageTitle, tab.Title) {
			return tab.Title
		}
	}
	return ""
}

// extractChromePageTitle recorta el sufijo que Chrome agrega al título
// de ventana ("Título de la página - Google Chrome"), para quedarse solo
// con el título de la página y poder compararlo contra CDP.
func extractChromePageTitle(windowTitle string) (string, bool) {
	for _, suffix := range []string{
		" - Google Chrome (Incognito)",
		" - Google Chrome",
	} {
		if strings.HasSuffix(windowTitle, suffix) {
			title := strings.TrimSpace(strings.TrimSuffix(windowTitle, suffix))
			return title, title != ""
		}
	}
	return "", false
}

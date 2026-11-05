package gateway

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"ahmelia/shared"
)

// ============================================================================
// KNOWLEDGE_GATEWAY.GO — Bloque 51, Fase 4.1
// ============================================================================
//
// Primer I/O real de todo el proyecto. Los 50 bloques anteriores son
// 100% deterministas, sin red, sin dependencias de terceros — esto
// rompe esa racha a propósito, y por eso queda AISLADO en un paquete
// nuevo (gateway/), separado de solving/: nada de lo anterior depende
// de esto, y esto no sabe nada de razonamiento.
//
// Por diseño explícito, esto NO es un "Internet Gateway" genérico tipo
// buscador — es un Knowledge Gateway con alcance deliberadamente
// angosto:
//
//   - NO decide QUÉ aprender (eso es Knowledge Discovery, Bloque 52)
//   - NO compara ni pondera fuentes (eso es Knowledge Validation, 53)
//   - NO genera capacidades nuevas (eso es Skill Builder, 54)
//   - Solo sabe: conectarse, descargar, normalizar, devolver documentos.
//
// La whitelist de dominios (AuthorizedSources) es explícita y estática
// — nunca fetchea un host que no esté en esa lista, nunca acepta http
// (solo https). Esto no es una limitación temporal a levantar después:
// es la arquitectura. Internet es una fuente de conocimiento
// verificable (documentación oficial de fabricantes/herramientas), no
// un sustituto del razonamiento propio de Ahmelia ni un buscador
// abierto a cualquier URL.

// AuthorizedSource es un dominio oficial habilitado para consulta. Host
// se compara EXACTO (case-insensitive), sin wildcards ni subdominios
// implícitos — agregar "docs.unity3d.com" no autoriza
// "evil.docs.unity3d.com.attacker.net" ni ningún otro subdominio.
type AuthorizedSource struct {
	Name string
	Host string
}

// AuthorizedSources es la whitelist inicial: documentación oficial de
// herramientas ya presentes en el resto del proyecto (Go) más los
// ejemplos de dominio que se acordaron para arrancar (Microsoft, Unity,
// Epic, GitHub, NVIDIA, Arduino, Blender, Python). Ampliar esta lista
// es la única forma de habilitar una fuente nueva — nunca se acepta un
// host fuera de acá, ni siquiera temporalmente.
var AuthorizedSources = []AuthorizedSource{
	{"Microsoft Learn", "learn.microsoft.com"},
	{"Unity Docs", "docs.unity3d.com"},
	{"Epic Developer Docs", "dev.epicgames.com"},
	{"Go Docs", "pkg.go.dev"},
	{"Go Docs (go.dev)", "go.dev"},
	{"Python Docs", "docs.python.org"},
	{"GitHub Docs", "docs.github.com"},
	{"GitHub", "github.com"},
	{"NVIDIA Developer", "developer.nvidia.com"},
	{"Arduino Docs", "docs.arduino.cc"},
	{"Blender Manual", "docs.blender.org"},
}

// MaxBodyBytes limita cuánto se lee de una sola respuesta — un
// documento de referencia no debería pesar decenas de MB de HTML; esto
// evita cargar a memoria una respuesta gigante (o maliciosa) entera.
const MaxBodyBytes = 5 * 1024 * 1024 // 5 MB

// Document es lo que el Gateway devuelve: texto normalizado, sin
// ninguna interpretación ni juicio de valor sobre el contenido — eso
// es trabajo de Knowledge Validation (53) y Skill Builder (54), no de
// este bloque.
type Document struct {
	URL        string
	Source     string // Name del AuthorizedSource que matcheó
	Title      string
	Text       string
	FetchedAt  time.Time
	StatusCode int

	// Procedencia (B96.46): lo mínimo para contrastar y detectar cambios.
	Host          string
	ContentSHA256 string // hash de los bytes crudos leídos
	LastModified  string
	ETag          string
	Truncated     bool // el cuerpo superó MaxBodyBytes y se cortó
}

// Gateway es el Knowledge Gateway en sí.
type Gateway struct {
	Client  *http.Client
	Sources []AuthorizedSource
	Retries int
	Backoff time.Duration
}

// NewGateway arma el Gateway con timeouts/reintentos por default y la
// whitelist estándar (AuthorizedSources).
func NewGateway() *Gateway {
	return NewGatewayWithSources(AuthorizedSources)
}

// NewGatewayWithSources arma el Gateway con una whitelist específica —
// para tests, o para restringir aún más qué fuentes están habilitadas
// en un contexto dado.
func NewGatewayWithSources(sources []AuthorizedSource) *Gateway {
	return &Gateway{
		Client:  &http.Client{Timeout: 10 * time.Second},
		Sources: sources,
		Retries: 2,
		Backoff: 500 * time.Millisecond,
	}
}

// authorize es la parte de "Seguridad" explícita del diseño: valida que
// la URL sea https y que el host esté en la whitelist EXACTA antes de
// hacer ninguna conexión real. Cortar acá, antes del request, es lo que
// garantiza que el Gateway nunca sale a buscar una URL arbitraria.
func (g *Gateway) authorize(rawURL string) (*url.URL, *AuthorizedSource, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, nil, fmt.Errorf("URL inválida: %w", err)
	}
	if u.Scheme != "https" {
		return nil, nil, fmt.Errorf("solo se permite https, la URL usa %q", u.Scheme)
	}
	for i := range g.Sources {
		if strings.EqualFold(u.Host, g.Sources[i].Host) {
			return u, &g.Sources[i], nil
		}
	}
	return nil, nil, fmt.Errorf("host %q no está en la whitelist de fuentes autorizadas", u.Host)
}

// Fetch descarga y normaliza UNA URL, con reintentos ante error de
// conexión (no ante un status HTTP de error — un 404 es una respuesta
// válida del servidor, no algo para reintentar).
func (g *Gateway) Fetch(rawURL string) (*Document, error) {
	u, source, err := g.authorize(rawURL)
	if err != nil {
		return nil, err
	}

	var resp *http.Response
	var lastErr error
	for attempt := 0; attempt <= g.Retries; attempt++ {
		if attempt > 0 {
			time.Sleep(g.Backoff * time.Duration(attempt))
		}
		req, reqErr := http.NewRequest(http.MethodGet, u.String(), nil)
		if reqErr != nil {
			return nil, reqErr
		}
		req.Header.Set("User-Agent", "Ahmelia-KnowledgeGateway/1.0")
		resp, lastErr = g.guardedClient().Do(req)
		if lastErr == nil {
			break
		}
	}
	if lastErr != nil {
		return nil, fmt.Errorf("no se pudo conectar a %s tras %d intento(s): %w", u.Host, g.Retries+1, lastErr)
	}
	defer resp.Body.Close()

	limited := io.LimitReader(resp.Body, MaxBodyBytes+1)
	raw, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("error leyendo respuesta de %s: %w", u.Host, err)
	}
	truncated := len(raw) > MaxBodyBytes
	if truncated {
		raw = raw[:MaxBodyBytes]
	}
	sum := sha256.Sum256(raw)
	rawStr := string(raw)

	return &Document{
		URL:        u.String(),
		Source:     source.Name,
		FetchedAt:  time.Now(),
		StatusCode: resp.StatusCode,
		Title:      extractTitle(rawStr),
		Text:       normalize(rawStr),

		Host:          u.Host,
		ContentSHA256: hex.EncodeToString(sum[:]),
		LastModified:  resp.Header.Get("Last-Modified"),
		ETag:          resp.Header.Get("ETag"),
		Truncated:     truncated,
	}, nil
}

// maxRedirects acota la cadena de redirecciones que el Gateway sigue.
const maxRedirects = 3

// guardedClient devuelve una copia del cliente cuya política de redirección
// re-aplica la whitelist en CADA salto. Sin esto, un host autorizado podía
// redirigir a cualquier destino y la whitelist dejaba de ser un límite real.
func (g *Gateway) guardedClient() *http.Client {
	c := *g.Client
	c.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) > maxRedirects {
			return fmt.Errorf("demasiadas redirecciones (>%d)", maxRedirects)
		}
		if _, _, err := g.authorize(req.URL.String()); err != nil {
			return fmt.Errorf("redirección bloqueada: %w", err)
		}
		return nil
	}
	return &c
}

// FetchAll descarga varias URLs sin cortar en el primer error — junta
// los documentos que sí funcionaron y los errores aparte, para que
// quien llame (Knowledge Discovery, 52) decida qué hacer con las
// fuentes que fallaron en vez de perder todo el lote.
func (g *Gateway) FetchAll(urls []string) ([]*Document, []error) {
	var docs []*Document
	var errs []error
	for _, u := range urls {
		doc, err := g.Fetch(u)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		docs = append(docs, doc)
	}
	return docs, errs
}

var (
	scriptStyleRe = regexp.MustCompile(`(?is)<(script|style)[^>]*>.*?</(script|style)>`)

	// chromeRe — Integration Fix, hallazgo real contra datos en vivo:
	// probamos "aprende el tutorial de python" y los "15 pasos" que
	// salieron eran, en su mayoría, basura de navegación del sitio —
	// "Report a bug", "Improve this page", el breadcrumb "Python »" —
	// mezclada con el índice de capítulos del tutorial. listItemRe
	// (abajo) convierte CUALQUIER <li> en línea de paso, sin
	// distinguir si viene del menú del sitio o del contenido real del
	// artículo — <nav>/<header>/<footer>/<aside> son exactamente las
	// etiquetas que HTML ya usa para marcar "esto es chrome del
	// sitio, no el artículo" (misma idea que scriptStyleRe: no
	// interpretamos el contenido, solo respetamos lo que el propio
	// HTML ya venía afirmando). Sacarlas ANTES de cualquier otro
	// procesamiento reduce el ruido en la raíz, no solo en Steps —
	// también en WhenToUse/Pitfalls/Prerequisites, que sufrían el
	// mismo problema en silencio.
	chromeRe = regexp.MustCompile(`(?is)<(nav|header|footer|aside)[^>]*>.*?</(nav|header|footer|aside)>`)

	tagRe   = regexp.MustCompile(`(?s)<[^>]*>`)
	titleRe = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	wsRe    = regexp.MustCompile(`\s+`)

	// listItemRe y blockBoundaryRe existen para un bug real, no
	// cosmético: antes, wsRe colapsaba TODOS los espacios en blanco
	// del documento — incluidos los saltos de línea — en uno solo, así
	// que el texto que salía de acá quedaba en una única línea gigante
	// sin \n. Eso significa que ninguna heurística del Bloque 54
	// basada en líneas (extractSteps, que busca "1. ", "- ", etc. al
	// PRINCIPIO de una línea) podía funcionar nunca sobre contenido
	// real fetcheado de Internet — solo sobre texto de test armado a
	// mano con \n explícitos. Confirmado en vivo: "aprende Unity" y
	// "aprende Blender" contra documentación real con listas
	// numeradas seguían saliendo sin Steps.
	//
	// Segundo problema, independiente: aunque se preserven los saltos
	// de línea, un <li> semántico de HTML casi nunca trae el "1." o
	// "-" como texto literal — el navegador lo DIBUJA vía CSS/List
	// Style, no está en el contenido. listItemRe hace explícito ese
	// marcador ("- " al principio de cada <li>) — no es inventar
	// nada: el propio <li> ya afirmaba "esto es un ítem de lista",
	// esto solo traduce esa afirmación semántica a texto literal, lo
	// mismo que haría un lector de pantalla.
	listItemRe      = regexp.MustCompile(`(?is)<li[^>]*>`)
	blockBoundaryRe = regexp.MustCompile(`(?is)</p>|<br\s*/?>|</h[1-6]>|</tr>|</li>|</ol>|</ul>`)
	intraLineWsRe   = regexp.MustCompile(`[ \t]+`)

	// headingOpenRe — Bloque 54 (extractProcedureTitle) necesita poder
	// distinguir un heading real ("<h1>Tutorial: Get started with
	// Go</h1>") de una línea de breadcrumb o párrafo que casualmente
	// tiene la misma pinta una vez aplanada a texto plano — hoy
	// blockBoundaryRe trata </h1>-</h6> igual que </p>/</tr>, así que
	// esa distinción se perdía por completo. Este regex marca el
	// ARRANQUE de un heading con shared.HeadingMarker (ver ese
	// comentario para el porqué de compartir el sentinel entre
	// paquetes) — blockBoundaryRe sigue encargándose del cierre de
	// línea vía </h1>-</h6>, esto solo agrega la marca de apertura.
	headingOpenRe = regexp.MustCompile(`(?is)<h[1-6][^>]*>`)
)

// extractTitle es el "parser básico" que pide el diseño — nada de un
// parser HTML real (eso es, a propósito, otro módulo: el Gateway "no
// piensa", solo normaliza lo mínimo para que el texto sea legible).
func extractTitle(html string) string {
	m := titleRe.FindStringSubmatch(html)
	if len(m) < 2 {
		return ""
	}
	return strings.TrimSpace(wsRe.ReplaceAllString(m[1], " "))
}

// normalize saca scripts/estilos y tags, pero a diferencia de antes
// PRESERVA los saltos de línea que el HTML ya marcaba (párrafos,
// <br>, encabezados, filas de tabla) y convierte cada <li> en una
// línea propia con viñeta ("- ") — sin eso, Steps (Bloque 54) nunca
// tenía nada real para encontrar, ver el comentario en listItemRe/
// blockBoundaryRe arriba. Sigue sin ser una extracción de contenido
// inteligente: no interpreta tablas, no distingue código de prosa —
// solo hace explícitos los límites de línea que el propio HTML ya
// declaraba, nada que la fuente no haya dicho.
func normalize(html string) string {
	text := scriptStyleRe.ReplaceAllString(html, " ")
	text = chromeRe.ReplaceAllString(text, " ")
	text = headingOpenRe.ReplaceAllString(text, "\n"+shared.HeadingMarker)
	text = listItemRe.ReplaceAllString(text, "\n- ")
	text = blockBoundaryRe.ReplaceAllString(text, "\n")
	text = tagRe.ReplaceAllString(text, " ")
	text = intraLineWsRe.ReplaceAllString(text, " ")

	var lines []string
	for _, line := range strings.Split(text, "\n") {
		// TrimSpace normal dejaría el sentinel pegado al texto real
		// (\x00H\x00Tutorial: ...) sin espacio — TrimLeft del propio
		// marcador primero, TrimSpace después, para que
		// extractProcedureTitle pueda hacer strings.HasPrefix(trimmed,
		// shared.HeadingMarker) de forma confiable línea por línea.
		hasHeading := strings.HasPrefix(line, shared.HeadingMarker)
		if hasHeading {
			line = strings.TrimPrefix(line, shared.HeadingMarker)
		}
		line = strings.TrimSpace(line)
		if hasHeading {
			line = shared.HeadingMarker + line
		}
		if line != "" && line != shared.HeadingMarker {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}

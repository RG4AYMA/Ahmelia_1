package gateway

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"ahmelia/shared"
)

// newTestGateway arma un Gateway apuntando a un servidor TLS local
// (httptest) en vez de a un dominio real — nada de esto sale a
// Internet. srv.Client() ya viene configurado para confiar en el
// certificado autofirmado del servidor de test.
func newTestGateway(t *testing.T, handler http.HandlerFunc) (*Gateway, *httptest.Server) {
	t.Helper()
	srv := httptest.NewTLSServer(handler)
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("no se pudo parsear la URL del server de test: %v", err)
	}

	gw := NewGatewayWithSources([]AuthorizedSource{{"Test Source", u.Host}})
	gw.Client = srv.Client()
	gw.Backoff = 0 // no esperar de verdad entre reintentos en los tests
	return gw, srv
}

// TestFetch_HostAutorizadoFunciona confirma la ruta central: un host en
// la whitelist, HTML de verdad, Fetch() lo trae normalizado.
func TestFetch_HostAutorizadoFunciona(t *testing.T) {
	gw, srv := newTestGateway(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><head><title>  Doc de prueba  </title></head><body><script>ignorar()</script><p>Hola   mundo</p></body></html>`)
	})
	defer srv.Close()

	doc, err := gw.Fetch(srv.URL + "/algo")
	if err != nil {
		t.Fatalf("Fetch() no debería fallar contra un host autorizado: %v", err)
	}
	if doc.Title != "Doc de prueba" {
		t.Errorf("esperaba Title='Doc de prueba', obtuve %q", doc.Title)
	}
	if !strings.Contains(doc.Text, "Hola mundo") {
		t.Errorf("esperaba que el texto normalizado incluya 'Hola mundo', obtuve %q", doc.Text)
	}
	if strings.Contains(doc.Text, "ignorar()") {
		t.Errorf("el contenido de <script> NO debería aparecer en el texto normalizado, obtuve %q", doc.Text)
	}
	if doc.Source != "Test Source" {
		t.Errorf("esperaba Source='Test Source', obtuve %q", doc.Source)
	}
}

// TestFetch_HostFueraDeWhitelistRechaza confirma la parte de seguridad
// central de todo el bloque: un host que NO está en Sources se
// rechaza ANTES de intentar cualquier conexión.
func TestFetch_HostFueraDeWhitelistRechaza(t *testing.T) {
	gw := NewGatewayWithSources([]AuthorizedSource{{"Solo Este", "docs.ejemplo-autorizado.com"}})

	_, err := gw.Fetch("https://sitio-no-autorizado.com/pagina")
	if err == nil {
		t.Fatal("esperaba que Fetch() rechace un host fuera de la whitelist, no debería llegar a conectar")
	}
	if !strings.Contains(err.Error(), "whitelist") {
		t.Errorf("esperaba un error que mencione la whitelist, obtuve: %v", err)
	}
}

// TestFetch_RechazaHTTP confirma que solo se acepta https, ni siquiera
// para un host que sí está en la whitelist.
func TestFetch_RechazaHTTP(t *testing.T) {
	gw := NewGatewayWithSources([]AuthorizedSource{{"Test", "docs.ejemplo-autorizado.com"}})

	_, err := gw.Fetch("http://docs.ejemplo-autorizado.com/pagina")
	if err == nil {
		t.Fatal("esperaba que Fetch() rechace http (no https), aunque el host esté autorizado")
	}
	if !strings.Contains(err.Error(), "https") {
		t.Errorf("esperaba un error que mencione https, obtuve: %v", err)
	}
}

// TestFetch_StatusHTTPDeErrorNoEsErrorDeGo confirma una distinción
// importante: un 404 es una respuesta VÁLIDA del servidor (Fetch no
// debería fallar como error de Go), simplemente StatusCode lo refleja
// — no reintenta un 404 pensando que es un problema de conexión.
func TestFetch_StatusHTTPDeErrorNoEsErrorDeGo(t *testing.T) {
	intentos := 0
	gw, srv := newTestGateway(t, func(w http.ResponseWriter, r *http.Request) {
		intentos++
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `<html><title>No encontrado</title></html>`)
	})
	defer srv.Close()

	doc, err := gw.Fetch(srv.URL + "/no-existe")
	if err != nil {
		t.Fatalf("un 404 no debería ser un error de Go, Fetch() debería devolver el Document con StatusCode=404: %v", err)
	}
	if doc.StatusCode != 404 {
		t.Errorf("esperaba StatusCode=404, obtuve %d", doc.StatusCode)
	}
	if intentos != 1 {
		t.Errorf("un 404 no debería disparar reintentos (no es un error de conexión), hubo %d intento(s)", intentos)
	}
}

// TestFetch_ReintentaAnteErrorDeConexion confirma que sí reintenta
// cuando el problema es de conexión — cerrando el server ANTES de
// llamar Fetch(), forzando que todos los intentos fallen, y contando
// cuántos hubo.
func TestFetch_ReintentaAnteErrorDeConexion(t *testing.T) {
	gw, srv := newTestGateway(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "no debería llegar acá")
	})
	authorizedHost := gw.Sources[0].Host
	srv.Close() // cierra el listener: cualquier intento de conexión falla

	gw.Retries = 2
	_, err := gw.Fetch("https://" + authorizedHost + "/algo")
	if err == nil {
		t.Fatal("esperaba error: el server de test está cerrado")
	}
	if !strings.Contains(err.Error(), "3 intento(s)") {
		t.Errorf("esperaba que el mensaje refleje Retries+1=3 intentos, obtuve: %v", err)
	}
}

// TestFetchAll_SigueAntePrimerErrorYJuntaLosDosGrupos confirma que un
// lote con URLs autorizadas y no autorizadas mezcladas no se corta en
// el primer error — trae lo que sí funcionó y separa los errores.
func TestFetchAll_SigueAntePrimerErrorYJuntaLosDosGrupos(t *testing.T) {
	gw, srv := newTestGateway(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><title>ok</title><body>contenido</body></html>`)
	})
	defer srv.Close()

	urls := []string{
		srv.URL + "/uno",
		"https://no-autorizado.com/otro",
		srv.URL + "/tres",
	}
	docs, errs := gw.FetchAll(urls)

	if len(docs) != 2 {
		t.Errorf("esperaba 2 documentos exitosos, obtuve %d", len(docs))
	}
	if len(errs) != 1 {
		t.Errorf("esperaba 1 error (el host no autorizado), obtuve %d", len(errs))
	}
}

// TestExtractTitle_SinTitleDevuelveVacio confirma que la ausencia de
// <title> no rompe nada, solo devuelve string vacío.
func TestExtractTitle_SinTitleDevuelveVacio(t *testing.T) {
	if got := extractTitle("<html><body>sin title</body></html>"); got != "" {
		t.Errorf("esperaba string vacío sin <title>, obtuve %q", got)
	}
}

// TestNormalize_PreservaLimitesDeLineaYConvierteListasEnViñetas
// confirma el fix del bug real: antes, wsRe colapsaba TODO el
// documento (incluidos los \n entre <li>) en una sola línea, así que
// ninguna heurística de Bloque 54 basada en líneas (extractSteps)
// podía encontrar nunca una lista real fetcheada de Internet — solo
// en texto de test armado a mano. Confirmado en vivo contra
// documentación real de Unity/Blender antes de este fix.
func TestNormalize_PreservaLimitesDeLineaYConvierteListasEnViñetas(t *testing.T) {
	gw, srv := newTestGateway(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><body>
			<p>Para instalar la herramienta, segui estos pasos:</p>
			<ol>
				<li>Descarga el instalador</li>
				<li>Ejecuta el instalador</li>
				<li>Abri la aplicacion</li>
			</ol>
		</body></html>`)
	})
	defer srv.Close()

	doc, err := gw.Fetch(srv.URL + "/tutorial")
	if err != nil {
		t.Fatalf("Fetch() no debería fallar: %v", err)
	}

	lines := strings.Split(doc.Text, "\n")
	var bulletLines []string
	for _, l := range lines {
		if strings.HasPrefix(l, "- ") {
			bulletLines = append(bulletLines, l)
		}
	}
	if len(bulletLines) != 3 {
		t.Fatalf("esperaba 3 líneas con viñeta (una por <li>), obtuve %d: %v — texto completo: %q", len(bulletLines), bulletLines, doc.Text)
	}
	if bulletLines[0] != "- Descarga el instalador" {
		t.Errorf("primera viñeta = %q, esperaba %q", bulletLines[0], "- Descarga el instalador")
	}
	if len(lines) < 2 {
		t.Errorf("el texto normalizado debería tener más de una línea (párrafo intro + items), obtuve una sola: %q", doc.Text)
	}
}

// TestNormalize_SacaNavHeaderFooterAsideDelContenido (Integration Fix)
// reproduce el hallazgo real contra datos en vivo: "aprende el
// tutorial de python" devolvió Steps mezclando el <nav> del sitio
// ("Report a bug", breadcrumbs) con el contenido real del artículo.
// Confirma que <nav>/<header>/<footer>/<aside> completos — incluidos
// sus <li> internos — ya no llegan a Skill Builder (54), sin tocar el
// contenido real del <article>/<body>.
func TestNormalize_SacaNavHeaderFooterAsideDelContenido(t *testing.T) {
	gw, srv := newTestGateway(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><body>
			<header><a href="/">Inicio</a> &raquo; Tutorial</header>
			<nav><ul><li>Reportar un error</li><li>Mejorar esta pagina</li></ul></nav>
			<p>Para instalar la herramienta, segui estos pasos:</p>
			<ol><li>Descarga el instalador</li><li>Ejecuta el instalador</li></ol>
			<aside><ul><li>Ver tambien: otra guia</li></ul></aside>
			<footer>Copyright 2026</footer>
		</body></html>`)
	})
	defer srv.Close()

	doc, err := gw.Fetch(srv.URL + "/tutorial")
	if err != nil {
		t.Fatalf("Fetch() no debería fallar: %v", err)
	}

	if strings.Contains(doc.Text, "Reportar un error") || strings.Contains(doc.Text, "Mejorar esta pagina") {
		t.Errorf("el contenido de <nav> no debería aparecer en el texto normalizado, obtuve: %q", doc.Text)
	}
	if strings.Contains(doc.Text, "Ver tambien") {
		t.Errorf("el contenido de <aside> no debería aparecer, obtuve: %q", doc.Text)
	}
	if strings.Contains(doc.Text, "Copyright") {
		t.Errorf("el contenido de <footer> no debería aparecer, obtuve: %q", doc.Text)
	}
	if strings.Contains(doc.Text, "Inicio") {
		t.Errorf("el contenido de <header> no debería aparecer, obtuve: %q", doc.Text)
	}
	if !strings.Contains(doc.Text, "Descarga el instalador") {
		t.Errorf("el contenido real del <ol> fuera del chrome SÍ debería sobrevivir, obtuve: %q", doc.Text)
	}
}

// TestNormalize_MarcaHeadingsConHeadingMarker confirma que un heading
// real (<h1>-<h6>) queda distinguible de una línea de prosa cualquiera
// una vez aplanado a texto — Bloque 54 (extractProcedureTitle) lo
// necesita para poder preferir un heading real sobre una heurística de
// puntuación. Antes de este fix, blockBoundaryRe trataba </h1>-</h6>
// igual que </p>, así que un heading y un párrafo eran indistinguibles.
func TestNormalize_MarcaHeadingsConHeadingMarker(t *testing.T) {
	gw, srv := newTestGateway(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><body>
			<h1>Tutorial: Get started with Go</h1>
			<p>In this tutorial, you will get a brief introduction.</p>
		</body></html>`)
	})
	defer srv.Close()

	doc, err := gw.Fetch(srv.URL + "/tutorial")
	if err != nil {
		t.Fatalf("Fetch() no debería fallar: %v", err)
	}

	if !strings.Contains(doc.Text, shared.HeadingMarker+"Tutorial: Get started with Go") {
		t.Errorf("esperaba el heading marcado con shared.HeadingMarker, obtuve: %q", doc.Text)
	}
	if strings.Contains(doc.Text, shared.HeadingMarker+"In this tutorial") {
		t.Error("un <p> no es un heading — no debería llevar el marcador")
	}
}

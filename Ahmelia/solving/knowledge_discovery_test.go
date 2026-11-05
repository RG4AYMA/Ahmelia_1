package solving

import (
	"ahmelia/interaction"
	"strings"
	"testing"

	"ahmelia/gateway"
)

// TestDiscover_DetectaDominioYCruzaFuentesReales confirma la ruta
// central: un objetivo que menciona "Unreal Engine" detecta el
// TechDomain correcto y trae SOLO las AuthorizedSources reales (no
// inventadas) que correspondan a ese host.
func TestDiscover_DetectaDominioYCruzaFuentesReales(t *testing.T) {
	sources := []gateway.AuthorizedSource{
		{Name: "Epic Developer Docs", Host: "dev.epicgames.com"},
		{Name: "Unity Docs", Host: "docs.unity3d.com"},
	}
	d := NewDiscoverer(sources)

	result := d.Discover("quiero aprender Unreal Engine")

	if result.Domain != "Unreal Engine" {
		t.Fatalf("esperaba dominio 'Unreal Engine', obtuve %q", result.Domain)
	}
	if len(result.Sources) != 1 || result.Sources[0].Name != "Epic Developer Docs" {
		t.Fatalf("esperaba solo la fuente de Epic, obtuve: %+v", result.Sources)
	}
	if !result.HasSources() {
		t.Fatal("HasSources() debería ser true con una fuente encontrada")
	}
}

// TestDiscover_DominioSinFuenteConfiguradaEsGapReal confirma un caso
// honesto: si el dominio matchea pero NINGUNA AuthorizedSource real
// tiene ese host (por ejemplo, la whitelist real no incluye Epic
// todavía), HasSources() debe ser false, no inventar una fuente.
func TestDiscover_DominioSinFuenteConfiguradaEsGapReal(t *testing.T) {
	sources := []gateway.AuthorizedSource{
		{Name: "Unity Docs", Host: "docs.unity3d.com"},
	}
	d := NewDiscoverer(sources)

	result := d.Discover("aprender Unreal Engine")

	if result.Domain != "Unreal Engine" {
		t.Fatalf("esperaba que el dominio SÍ se detecte, obtuve %q", result.Domain)
	}
	if result.HasSources() {
		t.Fatal("sin ninguna AuthorizedSource real para dev.epicgames.com, HasSources() debería ser false")
	}
}

// TestDiscover_SinDominioConocidoNoInventaNada confirma que un
// objetivo sin ningún TechDomain conocido no fuerza ningún match falso.
func TestDiscover_SinDominioConocidoNoInventaNada(t *testing.T) {
	d := NewDiscoverer(gateway.AuthorizedSources)
	result := d.Discover("aprender a hacer malabares")

	if result.Domain != "" {
		t.Fatalf("esperaba Domain vacío sin match, obtuve %q", result.Domain)
	}
	if result.HasSources() {
		t.Fatal("sin dominio detectado, no debería haber ninguna fuente")
	}
}

// TestDiscover_UsaWhitelistRealDelGateway confirma que Discoverer con
// gateway.AuthorizedSources (la whitelist real de Bloque 51) encuentra
// las fuentes reales — no una lista hardcodeada aparte que podría
// desincronizarse.
func TestDiscover_UsaWhitelistRealDelGateway(t *testing.T) {
	d := NewDiscoverer(gateway.AuthorizedSources)
	result := d.Discover("quiero aprender Python")

	if result.Domain != "Python" {
		t.Fatalf("esperaba dominio 'Python', obtuve %q", result.Domain)
	}
	if !result.HasSources() {
		t.Fatal("Python SÍ tiene fuente real en gateway.AuthorizedSources (docs.python.org) — debería encontrarla")
	}
}

// TestDescribe_TresCasosDistintos confirma que Describe() distingue
// sin-dominio, dominio-sin-fuente y éxito — tres mensajes distintos,
// no genéricos.
func TestDescribe_TresCasosDistintos(t *testing.T) {
	sinDominio := &DiscoveryResult{Goal: "malabares"}
	if !strings.Contains(sinDominio.Describe(), "No identifiqué") {
		t.Errorf("caso sin dominio: mensaje inesperado: %q", sinDominio.Describe())
	}

	sinFuente := &DiscoveryResult{Goal: "unreal", Domain: "Unreal Engine"}
	if !strings.Contains(sinFuente.Describe(), "no tengo ninguna fuente") {
		t.Errorf("caso dominio-sin-fuente: mensaje inesperado: %q", sinFuente.Describe())
	}

	exito := &DiscoveryResult{Goal: "python", Domain: "Python", Sources: []gateway.AuthorizedSource{{Name: "Python Docs", Host: "docs.python.org"}}}
	if !strings.Contains(exito.Describe(), "Python Docs") {
		t.Errorf("caso éxito: mensaje inesperado: %q", exito.Describe())
	}
}

// TestParseDiscoveryQuery_DetectaTriggers confirma los triggers
// principales sin falsos positivos sobre otros bloques de Fase 3/4.
func TestParseDiscoveryQuery_DetectaTriggers(t *testing.T) {
	if !ParseDiscoveryQuery(interaction.Analyze("que fuentes usarias para aprender unreal engine", interaction.Context{})) {
		t.Error("esperaba detectar el trigger de Knowledge Discovery")
	}
	if ParseDiscoveryQuery(interaction.Analyze("cual es tu plan de aprendizaje", interaction.Context{})) {
		t.Error("un pedido de Learning Planner no debería disparar Knowledge Discovery")
	}
}

// TestDiscover_SinResourceCuradoCaeALaRaiz (Integration Fix) confirma
// el fallback honesto: un objetivo sin ninguna palabra clave que
// matchee un Resource curado sigue trayendo la raíz del sitio — mismo
// comportamiento que Discovery tenía ANTES de este cambio, nunca peor.
func TestDiscover_SinResourceCuradoCaeALaRaiz(t *testing.T) {
	d := NewDiscoverer(gateway.AuthorizedSources)
	result := d.Discover("aprender Blender") // Blender no tiene Resources curados todavía

	if len(result.ResourceURLs) != 1 {
		t.Fatalf("esperaba 1 URL, obtuve %v", result.ResourceURLs)
	}
	if result.ResourceURLs[0] != "https://docs.blender.org/" {
		t.Errorf("ResourceURLs[0] = %q, esperaba la raíz del sitio", result.ResourceURLs[0])
	}
}

// TestDiscover_ConPalabraClaveUsaElRecursoCuradoEnVezDeLaRaiz
// (Integration Fix) confirma el fix real: un objetivo que menciona
// "tutorial" para Python ya no fetchea la portada del sitio, fetchea
// la página específica del tutorial — la causa raíz de por qué Steps
// salía siempre vacío contra fuentes reales.
func TestDiscover_ConPalabraClaveUsaElRecursoCuradoEnVezDeLaRaiz(t *testing.T) {
	d := NewDiscoverer(gateway.AuthorizedSources)
	result := d.Discover("quiero el tutorial de python")

	if len(result.ResourceURLs) != 1 {
		t.Fatalf("esperaba 1 URL, obtuve %v", result.ResourceURLs)
	}
	want := "https://docs.python.org/3/tutorial/index.html"
	if result.ResourceURLs[0] != want {
		t.Errorf("ResourceURLs[0] = %q, esperaba %q", result.ResourceURLs[0], want)
	}
}

// TestDiscover_RecursoCuradoNoSeMezclaEntreHostsDelMismoDominio
// (Integration Fix) confirma que un Resource de go.dev nunca aparece
// pegado a pkg.go.dev ni viceversa — Go tiene dos hosts con
// estructuras de URL totalmente distintas dentro del mismo TechDomain.
//
// Actualizado tras el fix de exclusión de host sin match (ver
// TestDiscover_ExcluyeHostSinRecursoCuradoCoincidenteCuandoOtroHostSiMatchea):
// antes pkg.go.dev cÍa a su raíz cuando ningún Resource matcheaba; esa
// raíz resultó ser, verificado en vivo, una página 100% UI de
// búsqueda — ahora ese host queda excluido en vez de aportar ruido.
func TestDiscover_RecursoCuradoNoSeMezclaEntreHostsDelMismoDominio(t *testing.T) {
	d := NewDiscoverer(gateway.AuthorizedSources)
	result := d.Discover("quiero aprender el tour basico de golang")

	if len(result.Sources) != 1 {
		t.Fatalf("esperaba 1 fuente (solo go.dev — pkg.go.dev no tiene ningún Resource que matchee), obtuve %d: %+v", len(result.Sources), result.Sources)
	}
	if result.Sources[0].Host != "go.dev" || result.ResourceURLs[0] != "https://go.dev/tour/welcome/1" {
		t.Errorf("esperaba go.dev con el Tour, obtuve Host=%q URL=%q", result.Sources[0].Host, result.ResourceURLs[0])
	}
}

// TestDiscover_ExcluyeHostSinRecursoCuradoCoincidenteCuandoOtroHostSiMatchea
// reproduce el bug real, encontrado verificando un skill_library.json
// en producción: "instalar goland paso a paso" matcheaba
// correctamente el Getting Started de go.dev, pero pkg.go.dev (sin
// ningún Resource que matcheara) caía a su raíz — la portada de
// búsqueda de pkg.go.dev, que resultó ser 100% UI (search tips,
// keyboard shortcuts, aviso de cookies), cero contenido de artículo.
// Confirmado trayendo la página real vía web_fetch antes de este fix.
func TestDiscover_ExcluyeHostSinRecursoCuradoCoincidenteCuandoOtroHostSiMatchea(t *testing.T) {
	d := NewDiscoverer(gateway.AuthorizedSources)
	result := d.Discover("aprende a instalar goland paso a paso")

	if len(result.Sources) != 1 {
		t.Fatalf("esperaba 1 fuente (solo go.dev), obtuve %d: %+v", len(result.Sources), result.Sources)
	}
	if result.Sources[0].Host != "go.dev" {
		t.Errorf("esperaba go.dev, obtuve %q — pkg.go.dev no debería aparecer sin Resource que matchee", result.Sources[0].Host)
	}
	if result.ResourceURLs[0] != "https://go.dev/doc/tutorial/getting-started" {
		t.Errorf("URL = %q, esperaba Getting Started", result.ResourceURLs[0])
	}
}

// TestDiscover_TraeRaizComoUltimoRecursoSiNingunHostTieneMatch confirma
// la degradación con gracia: si NINGÚN host del dominio matcheó ningún
// Resource curado, todos los hosts igual se incluyen con su raíz —
// mejor una fuente posiblemente ruidosa que cero fuentes para un
// dominio que sí se reconoció. La exclusión de
// TestDiscover_ExcluyeHostSinRecursoCuradoCoincidenteCuandoOtroHostSiMatchea
// solo aplica cuando ALGÚN otro host del mismo dominio sí tuvo match.
func TestDiscover_TraeRaizComoUltimoRecursoSiNingunHostTieneMatch(t *testing.T) {
	d := NewDiscoverer(gateway.AuthorizedSources)
	// "golang" solo — sin "tour"/"basico"/"paso a paso"/"std"/etc.,
	// ningún Resource de ningún host matchea.
	result := d.Discover("quiero saber sobre golang en general")

	if len(result.Sources) != 2 {
		t.Fatalf("esperaba 2 fuentes (go.dev y pkg.go.dev en su raíz, ningún Resource matcheó ninguno), obtuve %d: %+v", len(result.Sources), result.Sources)
	}
	got := make(map[string]string)
	for i, src := range result.Sources {
		got[src.Host] = result.ResourceURLs[i]
	}
	if got["go.dev"] != "https://go.dev/" {
		t.Errorf("go.dev = %q, esperaba la raíz (fallback de gracia)", got["go.dev"])
	}
	if got["pkg.go.dev"] != "https://pkg.go.dev/" {
		t.Errorf("pkg.go.dev = %q, esperaba la raíz (fallback de gracia)", got["pkg.go.dev"])
	}
}

// TestDiscover_GolandEsAliasDeVozDeGolang (Integration Fix) reproduce
// el hallazgo real: Google Speech Recognition transcribe "golang"
// como "goland" de forma consistente — sin este alias, decir "aprende
// golang" en voz alta nunca activaba el dominio Go.
func TestDiscover_GolandEsAliasDeVozDeGolang(t *testing.T) {
	d := NewDiscoverer(gateway.AuthorizedSources)
	result := d.Discover("quiero aprender goland")

	if result.Domain != "Go" {
		t.Errorf("Domain = %q, esperaba \"Go\" (goland debería reconocerse como alias de golang)", result.Domain)
	}
}

// TestDiscover_NombreCortoDelDominioMatcheaExacto (Integration Fix)
// reproduce el bug real: "qué sabes sobre Go" (el target extraído es
// solo "Go") nunca matcheaba ningún Keyword, porque "go" no puede
// CONTENER "golang" — dirección de substring al revés. "go" se
// excluyó a propósito de Keywords para no matchear como falso
// positivo dentro de "algo"/"amigo"/"tengo"; esta prueba confirma que
// la consulta EXACTA por el nombre del dominio ("Go" a secas) sigue
// funcionando sin reabrir ese falso positivo.
func TestDiscover_NombreCortoDelDominioMatcheaExacto(t *testing.T) {
	d := NewDiscoverer(gateway.AuthorizedSources)

	if got := d.Discover("Go").Domain; got != "Go" {
		t.Errorf("Discover(\"Go\").Domain = %q, esperaba \"Go\" (nombre exacto del dominio)", got)
	}
	if got := d.Discover("go").Domain; got != "Go" {
		t.Errorf("Discover(\"go\").Domain = %q, esperaba \"Go\" (case-insensitive)", got)
	}
	// Salvaguarda: el motivo original por el que "go" no es Keyword
	// sigue vigente — estas frases NO deberían matchear el dominio Go.
	noDebeMatchear := []string{"tengo hambre", "es mi amigo", "quiero aprender algo nuevo"}
	for _, frase := range noDebeMatchear {
		if got := d.Discover(frase).Domain; got == "Go" {
			t.Errorf("Discover(%q).Domain = \"Go\", NO debería matchear — reabriría el falso positivo que Keywords evitaba", frase)
		}
	}
}

// confirma que un objetivo de "primeros pasos paso a paso" elige la
// página de Getting Started (procedimiento real) y NO el Tour
// (interactivo, sin procedimiento en texto) — agregada específicamente
// para tener, por primera vez, un Resource curado que apunte a una
// página con una receta real de principio a fin, distinta de una
// página índice/tabla de contenidos (ver el hallazgo contra
// docs.python.org/3/tutorial/index.html).
func TestDiscover_GettingStartedEsDistintoDelTour(t *testing.T) {
	d := NewDiscoverer(gateway.AuthorizedSources)
	result := d.Discover("quiero instalar golang y crear mi primer proyecto paso a paso")

	if len(result.ResourceURLs) == 0 {
		t.Fatal("esperaba al menos 1 URL")
	}
	got := make(map[string]string)
	for i, src := range result.Sources {
		got[src.Host] = result.ResourceURLs[i]
	}
	want := "https://go.dev/doc/tutorial/getting-started"
	if got["go.dev"] != want {
		t.Errorf("go.dev = %q, esperaba %q (Getting Started, no el Tour)", got["go.dev"], want)
	}
}

package solving

import (
	"ahmelia/interaction"
	"fmt"
	"strings"

	"ahmelia/gateway"
)

// ============================================================================
// KNOWLEDGE_DISCOVERY.GO — Bloque 52, Fase 4.1
// ============================================================================
//
// Knowledge Gateway (51) sabe CÓMO traer un documento de una fuente
// autorizada, pero no decide DE CUÁL. Knowledge Discovery recibe un
// objetivo en lenguaje natural ("aprender Unreal Engine") y decide qué
// fuentes de gateway.AuthorizedSources son relevantes para ese
// dominio — nada de blogs, nada de foros, nada fuera de la whitelist
// que ya existe (Bloque 51). Todavía NO descarga nada (eso lo hace
// quien reciba el DiscoveryResult, llamando a Gateway.FetchAll con las
// URLs que arme a partir de las Sources) ni valida ni pondera nada
// (Knowledge Validation, Bloque 53).

// TechDomain asocia palabras clave de un objetivo con los hosts
// oficiales relevantes para ese dominio — capa intermedia entre
// "aprender Unreal Engine" y los Host EXACTOS de gateway.AuthorizedSources.
// Mismo espíritu que domainTemplates (goal_analyzer.go, Bloque 21): un
// punto de partida razonable, no exhaustivo — ampliable sin tocar el
// resto del bloque.
type TechDomain struct {
	Name      string
	Keywords  []string
	Hosts     []string   // deben existir en gateway.AuthorizedSources — Discovery NUNCA inventa un host nuevo
	Resources []Resource // páginas específicas conocidas dentro de esos Hosts — ver comentario del tipo
}

// Resource es una página conocida y curada a mano dentro de un Host de
// AuthorizedSources — la respuesta a un límite arquitectónico real
// (Integration Fix): antes, Discover() solo devolvía el Host, y quien
// arma la URL en core/engine.go siempre fetcheaba la RAÍZ del sitio
// ("https://"+Host+"/"). Eso significa que "aprende Blender" y
// "aprende a crear materiales en Blender" fetcheaban EXACTAMENTE la
// misma página — la portada del manual, que en la mayoría de los
// sitios de documentación es un índice/tabla de contenidos, no un
// procedimiento. Por eso Steps salía vacío incluso después del fix de
// normalize() en el Bloque 51: el fix de normalize() arregla "cómo
// interpreto la estructura de lo que bajé", esto arregla "qué bajo".
//
// Path va SIEMPRE atado a un Host específico (no es genérico entre
// hosts de un mismo TechDomain — un dominio como Go tiene go.dev Y
// pkg.go.dev, con estructuras de URL completamente distintas).
// Keywords son palabras del objetivo en lenguaje natural que, si
// aparecen, hacen que ESTE Resource se prefiera sobre la raíz.
//
// IMPORTANTE — honestidad sobre esta lista: son URLs conocidas al
// momento de escribir esto, curadas a mano, NO verificadas en vivo
// contra el sitio real (este entorno de trabajo no tiene acceso a
// red) — mismo criterio que Hosts en AuthorizedSources: si con el
// tiempo un sitio reestructura sus URLs y una queda rota, Knowledge
// Validation (53) la descarta como no confiable (ver IsReliable) en
// vez de que Skill Builder (54) reciba algo roto — degrada con
// gracia, no silenciosamente. La lista es deliberadamente chica: mejor
// pocos Resources verificables que muchos adivinados. Ampliarla es
// tan simple como agregar una entrada más, sin tocar el resto del
// bloque.
type Resource struct {
	Host     string
	Path     string
	Keywords []string
}

// techDomains cubre las mismas tecnologías que ya aparecen en
// gateway.AuthorizedSources (Bloque 51) — no tiene sentido reconocer un
// dominio para el que no hay ninguna fuente autorizada todavía.
var techDomains = []TechDomain{
	{Name: "Unreal Engine", Keywords: []string{"unreal engine", "unreal", "ue5", "ue4"}, Hosts: []string{"dev.epicgames.com"}},
	{Name: "Unity", Keywords: []string{"unity"}, Hosts: []string{"docs.unity3d.com"}},
	{
		// Integration Fix — "goland" agregado como alias real, no
		// como error de tipeo: Google Speech Recognition transcribe
		// "golang" dicho en voz alta como "goland" de forma
		// consistente (confirmado en vivo). "GoLand" es en realidad
		// el nombre de un IDE de JetBrains — no hay ningún
		// TechDomain/Resource para ese IDE en este proyecto, así que
		// hoy no hay ninguna ambigüedad real que se pierda al tratar
		// "goland" como sinónimo de voz de "golang". Si algún día se
		// agrega soporte para el IDE, esto hay que revisarlo.
		Name: "Go", Keywords: []string{"golang", "goland", "lenguaje go"}, Hosts: []string{"go.dev", "pkg.go.dev"},
		Resources: []Resource{
			{Host: "go.dev", Path: "/tour/welcome/1", Keywords: []string{"tour", "aprender", "basico", "introduccion"}},
			{Host: "go.dev", Path: "/doc/effective_go", Keywords: []string{"buenas practicas", "idiomatico", "estilo", "effective go"}},
			{Host: "pkg.go.dev", Path: "/std", Keywords: []string{"libreria estandar", "paquetes", "std", "standard library"}},
			// Integration Fix — a diferencia del Tour (interactivo, sin
			// procedimiento en texto) y de Effective Go (prosa de
			// estilo, no receta), esta página SÍ es un procedimiento
			// real paso a paso ("crea un directorio, corré go mod
			// init, escribí el código, corré go run...") — se agrega
			// específicamente para validar que Skill Builder (54)
			// puede extraer un procedimiento COHERENTE de una única
			// tarea contra una fuente real, a diferencia de una página
			// índice/tabla de contenidos (ver el hallazgo real contra
			// docs.python.org/3/tutorial/index.html: esa sí era solo
			// un índice, no un procedimiento).
			{Host: "go.dev", Path: "/doc/tutorial/getting-started", Keywords: []string{"empezar", "getting started", "instalar go", "crear un proyecto", "paso a paso", "primeros pasos"}},
		},
	},
	{
		Name: "Python", Keywords: []string{"python"}, Hosts: []string{"docs.python.org"},
		Resources: []Resource{
			{Host: "docs.python.org", Path: "/3/tutorial/index.html", Keywords: []string{"tutorial", "aprender", "empezar", "basico", "introduccion"}},
		},
	},
	{Name: "GitHub", Keywords: []string{"github"}, Hosts: []string{"docs.github.com", "github.com"}},
	{Name: "NVIDIA / CUDA", Keywords: []string{"nvidia", "cuda"}, Hosts: []string{"developer.nvidia.com"}},
	{Name: "Arduino", Keywords: []string{"arduino"}, Hosts: []string{"docs.arduino.cc"}},
	{Name: "Blender", Keywords: []string{"blender"}, Hosts: []string{"docs.blender.org"}},
	{Name: "Microsoft / .NET", Keywords: []string{"microsoft", ".net", "c#", "azure", "visual studio"}, Hosts: []string{"learn.microsoft.com"}},
}

// DiscoveryResult es lo que Knowledge Discovery decidió para un
// objetivo dado.
type DiscoveryResult struct {
	Goal    string
	Domain  string // "" si no matcheó ningún TechDomain conocido
	Sources []gateway.AuthorizedSource

	// ResourceURLs — Integration Fix — es la URL completa por cada
	// Source: la página específica curada (Resource) si alguna
	// coincidió por palabra clave con Goal, o la raíz del host
	// ("https://"+Host+"/") si no — MISMO fallback que el
	// comportamiento de antes, así que un dominio sin Resources
	// curados todavía no pierde nada. Quien fetchea (core/engine.go)
	// debería usar ESTO en vez de armar "https://"+src.Host+"/" a
	// mano — ver el comentario en el tipo Resource para el porqué.
	ResourceURLs []string
}

// HasSources es true si se encontró al menos una fuente autorizada
// real para el dominio detectado — no alcanza con detectar el dominio,
// tiene que existir la fuente en gateway.AuthorizedSources también.
func (dr *DiscoveryResult) HasSources() bool {
	return len(dr.Sources) > 0
}

// Discoverer es el Knowledge Discovery en sí. Recibe la whitelist REAL
// del Gateway (no una copia hardcodeada acá) para que nunca pueda
// quedar desincronizada de lo que Bloque 51 realmente tiene autorizado.
type Discoverer struct {
	AllSources []gateway.AuthorizedSource
}

// NewDiscoverer arma el Discoverer con la whitelist vigente.
func NewDiscoverer(allSources []gateway.AuthorizedSource) *Discoverer {
	return &Discoverer{AllSources: allSources}
}

// Discover recibe un objetivo en lenguaje natural, detecta el
// TechDomain que matchea (primero que encuentra, mismo criterio que
// detectDomain en goal_analyzer.go), y arma la lista de
// AuthorizedSources reales para ese dominio — cruzando Hosts contra
// AllSources, nunca inventando una Source que Gateway no tenga.
func (d *Discoverer) Discover(goalText string) *DiscoveryResult {
	norm := strings.ToLower(goalText)
	result := &DiscoveryResult{Goal: goalText}

	var matched *TechDomain
	for i := range techDomains {
		// Integration Fix — bug real encontrado en vivo: "qué sabes
		// sobre Go" nunca matcheaba, porque "go" (el target corto
		// extraído de la pregunta) no puede CONTENER "golang" — es la
		// dirección de substring al revés. "go" se excluyó a
		// propósito de Keywords para no matchear como falso positivo
		// dentro de "algo"/"amigo"/"tengo", pero esa misma protección
		// bloqueaba la consulta corta y directa por el nombre exacto
		// del dominio. EqualFold contra el Name entero es seguro
		// porque exige que TODA la consulta normalizada sea IGUAL al
		// nombre — "algo"/"amigo" no son iguales a "go", así que no
		// reabre el falso positivo que Keywords evitaba.
		if strings.EqualFold(strings.TrimSpace(norm), techDomains[i].Name) {
			matched = &techDomains[i]
			break
		}
		for _, kw := range techDomains[i].Keywords {
			if strings.Contains(norm, kw) {
				matched = &techDomains[i]
				break
			}
		}
		if matched != nil {
			break
		}
	}
	if matched == nil {
		return result
	}
	result.Domain = matched.Name

	// Integration Fix — hallazgo real verificando un skill_library.json
	// en producción: la consulta "instalar goland paso a paso" traía
	// go.dev (matcheó el Resource correcto, /doc/tutorial/getting-started)
	// Y pkg.go.dev (ningún Resource matcheó, cayó a la raíz "/") en el
	// mismo Content. La raíz de pkg.go.dev es la portada de búsqueda del
	// sitio — "Tip: Search for a package...", "Keyboard shortcuts",
	// aviso de cookies — cero contenido de artículo, confirmado
	// trayendo la página real. No es un problema de qué tan bien
	// normalize() (51) filtra chrome: esa página ES chrome de punta a
	// punta, por diseño.
	//
	// La distinción que importa: un host con Resources curados que NO
	// matchearon esta consulta puntual ya tiene evidencia (no solo
	// ausencia de dato) de que su raíz probablemente no es un
	// artículo — un host que nunca tuvo NINGÚN Resource curado no
	// tiene esa señal, así que para ese sigue valiendo más traer la
	// raíz que no traer nada (degradar con gracia, mismo criterio ya
	// documentado en el tipo Resource).
	type candidate struct {
		src  gateway.AuthorizedSource
		path string
		ok   bool
	}
	var candidates []candidate
	for _, host := range matched.Hosts {
		for _, src := range d.AllSources {
			if strings.EqualFold(src.Host, host) {
				path, ok := bestResourcePath(matched, host, norm)
				candidates = append(candidates, candidate{src: src, path: path, ok: ok})
			}
		}
	}

	anyMatched := false
	for _, c := range candidates {
		if c.ok {
			anyMatched = true
			break
		}
	}
	for _, c := range candidates {
		if c.ok {
			result.Sources = append(result.Sources, c.src)
			result.ResourceURLs = append(result.ResourceURLs, "https://"+c.src.Host+c.path)
			continue
		}
		if !anyMatched {
			// Ni siquiera UN host de este dominio tuvo un Resource que
			// matcheara — mejor traer la raíz de todos modos (posible
			// ruido) que devolver cero fuentes para un dominio que sí
			// se reconoció. Si algún host SÍ matcheó, en cambio, un
			// host sin match queda afuera (ver comentario arriba).
			result.Sources = append(result.Sources, c.src)
			result.ResourceURLs = append(result.ResourceURLs, "https://"+c.src.Host+"/")
		}
	}
	return result
}

// bestResourcePath elige, entre los Resources de matched que
// pertenecen a host, el que tenga MÁS palabras clave presentes en
// norm (el objetivo ya en minúsculas) — empate se resuelve por el
// primero en la lista (orden estable, mismo criterio que detectDomain
// más arriba).
//
// ok=false significa "este host tiene Resources curados pero ninguno
// matcheó" — señal para que Discover() lo excluya en vez de caer a la
// raíz (ver el comentario en Discover() sobre pkg.go.dev). ok=true
// cubre los otros dos casos: un Resource sí matcheó (path = ese
// Resource), o el host nunca tuvo Resources para empezar (path = "/",
// el fallback de siempre, sin cambios).
func bestResourcePath(domain *TechDomain, host, norm string) (path string, ok bool) {
	hasResourcesForHost := false
	bestPath := "/"
	bestScore := 0
	for _, r := range domain.Resources {
		if !strings.EqualFold(r.Host, host) {
			continue
		}
		hasResourcesForHost = true
		score := 0
		for _, kw := range r.Keywords {
			if strings.Contains(norm, kw) {
				score++
			}
		}
		if score > bestScore {
			bestScore = score
			bestPath = r.Path
		}
	}
	if hasResourcesForHost && bestScore == 0 {
		return "", false
	}
	return bestPath, true
}

// Describe narra qué decidió Discovery — tres casos distintos, no
// mezclados: sin dominio detectado, dominio detectado pero sin fuente
// configurada (gap real: sabría el nombre pero no de dónde traerlo), o
// éxito con la lista de fuentes.
func (dr *DiscoveryResult) Describe() string {
	if dr.Domain == "" {
		return fmt.Sprintf("No identifiqué un dominio conocido para \"%s\" — todavía no tengo fuentes autorizadas para eso.", dr.Goal)
	}
	if !dr.HasSources() {
		return fmt.Sprintf("Detecté el dominio %s, pero no tengo ninguna fuente autorizada configurada para él todavía.", dr.Domain)
	}
	var names []string
	for _, s := range dr.Sources {
		names = append(names, s.Name)
	}
	return fmt.Sprintf("Dominio: %s. Fuentes autorizadas: %s.", dr.Domain, strings.Join(names, ", "))
}

// ── Trigger de consulta ──────────────────────────────────────────────

var discoveryTriggers = []string{
	"que fuentes usarias para aprender",
	"de donde aprenderias sobre",
	"que fuentes tienes para",
	"donde buscarias informacion sobre",
}

// ParseDiscoveryQuery detecta si se pregunta específicamente por las
// FUENTES que se usarían para un tema — distinto de un pedido directo
// de aprender (eso todavía no está cableado: Discovery decide de
// dónde, no descarga ni aprende nada por sí sola).
func ParseDiscoveryQuery(req interaction.SemanticRequest) bool {
	return interaction.HasSemanticQuery(req, interaction.QuerySolvingDiscovery)
}

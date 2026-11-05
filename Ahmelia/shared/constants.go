package shared

// HeadingMarker — contrato compartido entre gateway (Bloque 51) y
// solving (Bloque 54). El Gateway antepone este sentinel a cada línea
// que vino de un <h1>-<h6> real en el HTML fuente, ANTES de aplanar
// todo lo demás a texto plano — porque una vez aplanado, un heading
// real ("Tutorial: Get started with Go") y una línea de breadcrumb o
// párrafo cualquiera son indistinguibles para quien lea el texto
// después (ver gateway/knowledge_gateway.go, blockBoundaryRe).
//
// Bloque 54 (extractProcedureTitle) lo usa para priorizar un heading
// real por sobre la heurística de "línea anterior a la lista que
// termina en :" — sin este marcador, esa priorización no tiene con
// qué funcionar, ninguna línea del texto aplanado dice "yo era un
// <h2>". Vive en shared, no en gateway ni en solving, precisamente
// porque las dos capas necesitan coincidir en el mismo literal — un
// string hardcodeado por separado en cada paquete se puede desincronizar
// silenciosamente si alguno cambia.
const HeadingMarker = "\x00H\x00"

var URLKeywords = map[string]bool{
	"youtube":       true,
	"google":        true,
	"facebook":      true,
	"twitter":       true,
	"whatsapp":      true,
	"instagram":     true,
	"tiktok":        true,
	"reddit":        true,
	"github":        true,
	"gmail":         true,
	"maps":          true,
	"spotify":       true,
	"netflix":       true,
	"twitch":        true,
	"discord":       true,
	"telegram":      true,
	"linkedin":      true,
	"wikipedia":     true,
	"chatgpt":       true,
	"claude":        true,
	"stackoverflow": true,
}

// URLAliasPhrases — variantes fonéticas conocidas que un STT puede
// transcribir para servicios cuyo nombre es una sigla en inglés dicha
// con acento en español (ej. "GPT" -> "hebede", "ge pe te"). Solo se
// agregan variantes CONFIRMADAS por uso real, no cualquier cosa que se
// nos ocurra — si aparece una nueva, se agrega acá.
var URLAliasPhrases = map[string]string{
	"chat hebede":   "chatgpt",
	"chat yipiti":   "chatgpt",
	"chat ge pe te": "chatgpt",
	"chatgepete":    "chatgpt",
}

// Wake words — palabras para activar Ahmelia
var WakeWords = []string{
	"ahmelia", "amelia", "a melia", "hamelia", "jamelia",
}

// Tags automáticos — clasifican Knowledge por tema
var TagMappings = map[string][]string{
	"volumen":  {"volumen", "sonido", "audio", "sube", "baja", "silencia"},
	"ventana":  {"ventana", "cambia", "alt tab", "minimiza", "cierra"},
	"archivo":  {"archivo", "busca", "mueve", "renombra", "crea"},
	"programa": {"abre", "ejecuta", "lanza", "inicia"},
	"youtube":  {"youtube"},
	"google":   {"google", "buscar"},
	"chrome":   {"chrome", "navegador", "edge"},
}

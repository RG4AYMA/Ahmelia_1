package shared

import "strings"

/* =======================
   PRONOMBRES (referencias implícitas)
   "ábrelo", "vuelve a eso" — el clasificador ya deja el pronombre como
   target (ver classifier.extractTarget: "abre"+"lo" -> queda "lo" al
   quitar el verbo). Esta lista es cómo el resto del sistema reconoce que
   ese target NO es el nombre real de un programa, sino una referencia a
   "lo último que WorldState recuerda" — y debe resolverse contra
   environment.Manager.ActiveApp() en vez de buscarse literalmente.
======================= */

var pronounWords = map[string]bool{
	"lo": true, "la": true, "los": true, "las": true,
	"eso": true, "esa": true, "ese": true,
	"esto": true, "este": true,
	"aquello": true, "aquel": true, "aquella": true,
}

// IsPronoun indica si target (ya normalizado) es una referencia implícita
// en vez de un nombre real — señal de que hay que resolverlo contra el
// estado del mundo (environment.Manager) antes de usarlo.
func IsPronoun(target string) bool {
	return pronounWords[strings.TrimSpace(target)]
}

/* =======================
   STOPWORDS
   Palabras que se ignoran al extraer keywords de un input.
   Antes vivían duplicadas en utils/nlp.go y, además, reimplementadas
   a mano (con su propio mapa de acentos) dentro de files/manager.go
   con el comentario "sin depender de utils". Ahora hay un solo lugar.
======================= */

var stopWords = map[string]bool{
	// Artículos
	"el": true, "la": true, "los": true, "las": true,
	"un": true, "una": true, "unos": true, "unas": true,

	// Preposiciones
	"de": true, "del": true, "a": true, "al": true,
	"en": true, "por": true, "para": true, "con": true,
	"sin": true, "sobre": true, "desde": true, "hasta": true,

	// Pronombres
	"mi": true, "mis": true, "tu": true, "tus": true,
	"su": true, "sus": true, "me": true, "te": true,
	"se": true, "le": true, "lo": true, "les": true,

	// Verbos auxiliares / muletillas
	"que": true, "como": true, "donde": true, "cuando": true,
	"cual": true, "porque": true, "si": true, "no": true,
	"oye": true, "hey": true, "mira": true,
	"por favor": true, "porfa": true, "porfavor": true,
	"gracias": true, "favor": true,

	// Verbos comunes que no aportan
	"es": true, "son": true, "fue": true, "era": true,
	"estar": true, "esta": true, "este": true, "estoy": true,
	"hacer": true, "hice": true, "hago": true,
	"tener": true, "tengo": true, "tiene": true,

	// Temporales
	"ayer": true, "hoy": true, "mañana": true,
	"semana": true, "pasada": true, "pasado": true,
	"mes": true, "año": true, "dia": true,
}

/* =======================
   EXTRACCIÓN DE KEYWORDS
======================= */

// ExtractKeywords extrae las palabras clave de un texto: normaliza (usando
// Normalize, la misma función que usa el resto del proyecto), quita
// stopwords generales y palabras muy cortas (<=2 caracteres).
//
// extraStopWords permite añadir palabras a ignorar específicas del
// contexto de la llamada — por ejemplo, files.SearchFileHuman ignora
// además "archivo" y "documento", que no tiene sentido tratar como
// keyword de búsqueda mientras que sí lo son en otros contextos.
func ExtractKeywords(text string, extraStopWords ...string) []string {
	normalized := Normalize(text)

	extra := make(map[string]bool, len(extraStopWords))
	for _, w := range extraStopWords {
		extra[w] = true
	}

	words := strings.Fields(normalized)
	keywords := make([]string, 0, len(words))

	for _, word := range words {
		if len(word) <= 2 {
			continue
		}
		if stopWords[word] || extra[word] {
			continue
		}
		keywords = append(keywords, word)
	}

	return keywords
}

/* =======================
   SIMILITUD
======================= */

// CalculateSimilarity cuenta cuántas keywords aparecen dentro de target
// (normalizado). Útil para puntuar qué tan bien un nombre de archivo,
// programa, etc. coincide con lo que el usuario pidió.
func CalculateSimilarity(keywords []string, target string) int {
	normTarget := Normalize(target)
	score := 0
	for _, keyword := range keywords {
		if strings.Contains(normTarget, keyword) {
			score++
		}
	}
	return score
}

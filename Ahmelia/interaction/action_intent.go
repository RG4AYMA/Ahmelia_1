package interaction

import (
	"sort"
	"strings"
)

// ActionIntent is the domain-neutral executable action extracted at the
// interaction boundary. The action name is a stable system capability key;
// the target remains human-level data and is resolved by the execution layer.
type ActionIntent struct {
	Name       string
	Target     string
	Parameters map[string]string
	Confidence float64
}

// actionAliases is deliberately language-agnostic at the architecture level:
// aliases are surface forms, while Name is the stable capability contract.
// Adding another language changes this table, not Engine or a provider.
var actionAliases = map[string][]string{
	"abrir_programa": {
		"abre", "abrir", "ejecuta", "lanza", "inicia",
		"open", "launch", "start", "run",
	},
	"buscar_archivo": {
		"busca", "buscar", "encuentra", "localiza",
		"find file", "search file", "find the file", "search for the file",
	},
	"mover_archivo": {
		"mueve", "mover", "traslada", "move", "move file", "move the file",
	},
	"renombrar_archivo": {
		"renombra", "renombrar", "cambia nombre", "rename", "rename file",
	},
	"crear_carpeta": {
		"crea carpeta", "crear carpeta", "nueva carpeta",
		"create folder", "make a folder", "new folder",
	},
	"reproducir_media": {
		"reproduce", "reproducir", "pon musica", "pon música",
		"play", "play music", "play song",
	},
	"salir": {
		"salir", "apagar", "adios", "adiós", "exit", "quit", "goodbye",
	},
	"volumen_subir": {
		"sube", "subir", "aumenta", "sube volumen", "sube el volumen",
		"turn up", "turn up the volume", "increase volume", "raise volume",
	},
	"volumen_bajar": {
		"baja", "bajar", "reduce", "baja volumen", "baja el volumen",
		"turn down", "turn down the volume", "decrease volume", "lower volume",
	},
	"volumen_silencio": {
		"silencio", "mutea", "silenciar", "sin sonido", "mute", "mute sound",
	},
	"volumen_max": {
		"maximo", "máximo", "al maximo", "al máximo", "a tope",
		"max volume", "volume maximum",
	},
	"volumen_set": {
		"pon el volumen", "volumen al", "set volume", "set the volume",
	},
	"minimizar": {
		"minimiza", "esconde", "minimize", "hide window",
	},
	"cambiar_ventana": {
		"cambia de ventana", "cambia ventana", "siguiente ventana",
		"cambiar ventana", "cambia a", "vuelve a", "regresa a",
		"switch window", "switch to", "change window",
	},
	"cerrar_ventana": {
		"cierra", "cerrar", "close", "close window",
	},
}

// inferAction resolves only an explicit executable interaction. It does not
// know anything about Unity, Go, Blender, URLs, filesystems, or providers.
// Longest phrase wins, and ties are deterministic.
func inferAction(n string) ActionIntent {
	n = normalize(n)
	if n == "" {
		return ActionIntent{}
	}

	type candidate struct {
		action string
		phrase string
	}
	var candidates []candidate
	tokens := commandTokens(n)
	start := commandStart(tokens)
	for action, aliases := range actionAliases {
		for _, alias := range aliases {
			// Una palabra de acción solo cuenta cuando abre la orden. Que aparezca
			// dentro de una frase ("las cosas empezaran a salir a flote", "creo que
			// deberiamos cambiar el sistema") es conversación, no una orden.
			end, ok := aliasEndsAtCommandStart(tokens, start, alias)
			if !ok {
				continue
			}
			// "salir" cierra el proceso entero: la orden debe ser toda la frase.
			if action == "salir" && !isWholeExitCommand(n, tokens, end) {
				continue
			}
			candidates = append(candidates, candidate{action: action, phrase: alias})
		}
	}
	if len(candidates) == 0 {
		return ActionIntent{}
	}

	sort.Slice(candidates, func(i, j int) bool {
		if len(candidates[i].phrase) != len(candidates[j].phrase) {
			return len(candidates[i].phrase) > len(candidates[j].phrase)
		}
		if candidates[i].action != candidates[j].action {
			return candidates[i].action < candidates[j].action
		}
		return candidates[i].phrase < candidates[j].phrase
	})

	best := candidates[0]
	target := extractActionTarget(n, best.action, best.phrase)
	confidence := 0.94
	if len(best.phrase) >= 8 {
		confidence = 0.98
	}
	return ActionIntent{Name: best.action, Target: target, Parameters: extractActionParameters(n, best.action), Confidence: confidence}
}

func extractActionParameters(n, action string) map[string]string {
	params := map[string]string{}
	if strings.TrimSpace(n) != "" {
		params["raw"] = n
	}
	if action == "mover_archivo" {
		if idx := strings.Index(n, " a "); idx >= 0 {
			params["destination"] = strings.TrimSpace(n[idx+3:])
		} else if idx := strings.Index(n, " al "); idx >= 0 {
			params["destination"] = strings.TrimSpace(n[idx+4:])
		}
	}
	if action == "volumen_set" {
		words := strings.Fields(n)
		for i, word := range words {
			if word == "a" || word == "al" {
				if i+1 < len(words) {
					value := strings.TrimSuffix(words[i+1], "%")
					if isDecimal(value) {
						params["percent"] = value
						break
					}
				}
			}
		}
	}
	return params
}

func isDecimal(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// commandLeadFillers son las palabras que pueden preceder a una orden sin
// cambiar que lo sea: llamar a Ahmelia, cortesía, "quiero/puedes...", conectores.
// Se prueban de más larga a más corta.
var commandLeadFillers = [][]string{
	{"i", "would", "like", "to"}, {"i", "want", "to"}, {"i", "need", "to"},
	{"me", "gustaria"}, {"me", "gustaría"}, {"por", "favor"}, {"vamos", "a"},
	{"can", "you"}, {"could", "you"}, {"would", "you"}, {"a", "melia"},
	{"amelia"}, {"ahmelia"}, {"oye"}, {"hey"}, {"ok"}, {"okay"}, {"bueno"},
	{"entonces"}, {"ahora"}, {"ya"}, {"pues"}, {"y"}, {"luego"}, {"despues"},
	{"después"}, {"tambien"}, {"también"}, {"porfa"}, {"please"},
	{"puedes"}, {"puede"}, {"podrias"}, {"podrías"}, {"podria"}, {"podría"},
	{"quiero"}, {"quisiera"}, {"necesito"}, {"deseo"},
	{"hola"}, {"buenas"}, {"buenos", "dias"}, {"buenos", "días"}, {"hello"}, {"hi"},
}

// exitCommandClosers son las únicas palabras que pueden acompañar a "salir" sin
// quitarle el carácter de orden de salida total.
var exitCommandClosers = map[string]bool{
	"ahmelia": true, "amelia": true, "ahora": true, "ahorita": true, "ya": true,
	"mismo": true, "por": true, "favor": true, "porfa": true, "gracias": true,
	"y": true, "please": true, "now": true, "del": true, "programa": true,
	"de": true, "la": true, "el": true, "aplicacion": true, "aplicación": true,
	"aqui": true, "aquí": true, "hasta": true, "luego": true, "nos": true,
	"vemos": true, "mañana": true, "manana": true, "pronto": true, "bye": true,
}

// commandTokens separa el texto en palabras sin la puntuación de los bordes.
func commandTokens(n string) []string {
	fields := strings.Fields(n)
	tokens := make([]string, 0, len(fields))
	for _, f := range fields {
		f = strings.Trim(f, ".,;:!?¡¿\"'()")
		if f != "" {
			tokens = append(tokens, f)
		}
	}
	return tokens
}

// commandStart devuelve el índice de la primera palabra que no es de relleno.
func commandStart(tokens []string) int {
	i := 0
	for i < len(tokens) {
		matched := false
		for _, filler := range commandLeadFillers {
			if tokensHavePrefix(tokens[i:], filler) {
				i += len(filler)
				matched = true
				break
			}
		}
		if !matched {
			break
		}
	}
	return i
}

func tokensHavePrefix(tokens, prefix []string) bool {
	if len(prefix) == 0 || len(tokens) < len(prefix) {
		return false
	}
	for i, p := range prefix {
		if tokens[i] != p {
			return false
		}
	}
	return true
}

// aliasEndsAtCommandStart dice si el alias abre la orden y, si es así, en qué
// posición termina.
func aliasEndsAtCommandStart(tokens []string, start int, alias string) (int, bool) {
	aliasTokens := commandTokens(strings.ToLower(alias))
	if start > len(tokens) || !tokensHavePrefix(tokens[start:], aliasTokens) {
		return 0, false
	}
	return start + len(aliasTokens), true
}

// isWholeExitCommand exige que lo que sigue a "salir" sean solo cierres de
// cortesía, y que la frase no sea una pregunta.
func isWholeExitCommand(n string, tokens []string, end int) bool {
	if strings.ContainsAny(n, "?¿") {
		return false
	}
	for _, t := range tokens[end:] {
		if !exitCommandClosers[t] {
			return false
		}
	}
	return true
}

func phrasePresent(text, phrase string) bool {
	text = " " + strings.TrimSpace(text) + " "
	phrase = strings.TrimSpace(phrase)
	if phrase == "" {
		return false
	}
	return strings.Contains(text, " "+phrase+" ")
}

func extractActionTarget(n, action, matchedPhrase string) string {
	target := strings.TrimSpace(strings.Replace(n, matchedPhrase, " ", 1))
	target = strings.Trim(target, " .,!?:;¿¡")
	target = cleanActionTargetPrefix(target, action)

	switch action {
	case "volumen_subir", "volumen_bajar", "volumen_silencio", "volumen_max", "volumen_set", "minimizar":
		return ""
	case "salir":
		return ""
	}

	if action == "cambiar_ventana" {
		for _, prefix := range []string{"al ", "a ", "the ", "to "} {
			target = strings.TrimPrefix(target, prefix)
		}
	}
	if action == "mover_archivo" {
		// Preserve the destination when the utterance uses "to/a"; the
		// execution layer can interpret the structured destination later.
		for _, marker := range []string{" a ", " al ", " to ", " into "} {
			if idx := strings.Index(target, marker); idx >= 0 {
				target = strings.TrimSpace(target[idx+len(marker):])
				break
			}
		}
	}

	return strings.TrimSpace(target)
}

func cleanActionTargetPrefix(target, action string) string {
	// These are semantic filler words, not domain names. They are removed
	// only from the beginning so an artifact called "the project" is not
	// mangled in the middle of a request.
	prefixes := []string{"el ", "la ", "los ", "las ", "the ", "a ", "al "}
	if action == "buscar_archivo" || action == "abrir_programa" || action == "cerrar_ventana" {
		for _, p := range prefixes {
			target = strings.TrimPrefix(target, p)
		}
	}
	return target
}

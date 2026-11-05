package classifier

import (
	"encoding/json"
	"strconv"
	"strings"

	"ahmelia/shared"
)

type ClassificationResult struct {
	Action     string            `json:"action"`
	Target     string            `json:"target"`
	Parameters map[string]string `json:"parameters"`
	Confidence float64           `json:"confidence"`
}

/* =======================
   PREPROCESAMIENTO
======================= */

func cleanInput(text string) string {
	text = strings.ToLower(text)
	text = strings.TrimSpace(text)
	return text
}

// hasExplicitVolumeNumber detecta si el texto ya normalizado trae un
// número 0-100 (ej: "a 50", "al 60", "70%") — señal de que el usuario
// pide un volumen absoluto, no relativo.
func hasExplicitVolumeNumber(normalized string) bool {
	for _, w := range strings.Fields(normalized) {
		w = strings.TrimSuffix(w, "%")
		if n, err := strconv.Atoi(w); err == nil && n >= 0 && n <= 100 {
			return true
		}
	}
	return false
}

/* =======================
   DETECCIÓN (CORREGIDA)
======================= */

func extractAction(input string) (string, float64) {
	normalized := shared.Normalize(input)

	// PRIORIDAD 1: Comandos de volumen (detección específica)
	if strings.Contains(normalized, "volumen") || strings.Contains(normalized, "sonido") || strings.Contains(normalized, "audio") {
		// Un número explícito ("sube el volumen A 40", "baja el volumen A 40",
		// "volumen a 40") siempre gana sobre la palabra relativa: el usuario
		// está pidiendo un valor absoluto, no un empujón relativo. Antes esto
		// se revisaba AL FINAL, así que "baja el volumen a 40" se clasificaba
		// como volumen_bajar normal y el "a 40" se descartaba por completo.
		if hasExplicitVolumeNumber(normalized) {
			return "volumen_set", 1.0
		}
		if strings.Contains(normalized, "sube") || strings.Contains(normalized, "subir") || strings.Contains(normalized, "aumenta") {
			return "volumen_subir", 1.0
		}
		if strings.Contains(normalized, "baja") || strings.Contains(normalized, "bajar") || strings.Contains(normalized, "reduce") {
			return "volumen_bajar", 1.0
		}
		if strings.Contains(normalized, "silencio") || strings.Contains(normalized, "mutea") || strings.Contains(normalized, "mute") {
			return "volumen_silencio", 1.0
		}
		if strings.Contains(normalized, "maximo") || strings.Contains(normalized, "tope") {
			return "volumen_max", 1.0
		}
	}

	// PRIORIDAD 2: Detección general por patrones
	//
	// Antes "score" era 1.0 fijo para cualquier match, así que el primer
	// action que el for..range recorriera con AL MENOS un patrón
	// encontrado ganaba — y Go aleatoriza a propósito el orden de
	// iteración de un map en cada corrida del programa. Con inputs que
	// matchean más de una acción (ej. "pon el volumen" matchea "pon" de
	// reproducir_media Y "pon el volumen" de volumen_set), el resultado
	// cambiaba entre ejecuciones sin que el usuario cambiara nada de lo
	// que dijo. Ahora se usa la LONGITUD del patrón como score: el
	// patrón más específico ("pon el volumen", 14 caracteres) siempre le
	// gana al más genérico ("pon", 3 caracteres), sin importar en qué
	// orden el map los entregue.
	bestAction := "desconocido"
	bestLen := 0

	for action, patterns := range shared.ActionPatterns {
		for _, pattern := range patterns {
			if strings.Contains(normalized, pattern) {
				if len(pattern) > bestLen {
					bestLen = len(pattern)
					bestAction = action
				}
			}
		}
	}

	if bestLen == 0 {
		return "desconocido", 0.0
	}

	return bestAction, 1.0
}

/* =======================
   EXTRACT TARGET (AHORA IMPLEMENTADO)
======================= */

func extractTarget(input, action string) string {
	normalized := shared.Normalize(input)

	// Caso especial: las acciones de volumen no tienen concepto de "target".
	// Sin este caso, caían al camino genérico de abajo, que solo limpia
	// verbos como "abre"/"busca"/etc. — ninguno de "baja", "reduce", "sube",
	// "silencio", "maximo" está en esa lista, así que el target terminaba
	// siendo el input completo normalizado. Eso rompía cualquier
	// comparación por Action+Target exacto (ej. RecallBySameCommand) entre
	// frases distintas para el mismo comando ("baja el volumen" vs "reduce
	// el volumen" -> mismo Action volumen_bajar, pero Target distinto).
	// engine.go nunca lee Target para estas acciones (volumen_bajar,
	// volumen_silencio y volumen_max lo ignoran directamente;
	// volumen_subir tampoco lo usa; volumen_set solo usa
	// params["percent"]/params["raw"], nunca Target), así que "" es seguro.
	if action == "volumen_subir" || action == "volumen_bajar" ||
		action == "volumen_silencio" || action == "volumen_max" ||
		action == "volumen_set" {
		return ""
	}

	// Caso especial: cambiar_ventana puede tener target opcional
	if action == "cambiar_ventana" {
		// Patrones: "cambia a chrome", "cambiar a vscode", "vuelve a eso",
		// y sus variantes con "al" (contracción obligatoria de "a"+"el"
		// en español: "cambia AL administrador de tareas", no "cambia a
		// el administrador..."). Sin "al" por separado, cualquier target
		// que lleve artículo masculino ("el administrador", "el bloc de
		// notas") no calzaba con ningún patrón y el target quedaba
		// vacío, cayendo al Alt+Tab genérico en vez de buscar la
		// ventana específica.
		swapPatterns := []string{
			"cambia al ", "cambiar al ", "vuelve al ", "regresa al ", "vamos al ",
			"cambia a ", "cambiar a ", "switch to ", "vamos a ", "vuelve a ", "regresa a ",
		}
		for _, pattern := range swapPatterns {
			if strings.Contains(normalized, pattern) {
				target := strings.ReplaceAll(normalized, pattern, "")
				// Remover otras palabras que pueden estar
				target = strings.ReplaceAll(target, "cambia", "")
				target = strings.ReplaceAll(target, "cambiar", "")
				target = strings.ReplaceAll(target, "vuelve", "")
				target = strings.ReplaceAll(target, "regresa", "")
				target = strings.TrimSpace(target)
				// Quitar artículo suelto al inicio ("el administrador de
				// tareas" -> "administrador de tareas"). Hace falta
				// porque el patrón "cambia a " (sin contraer) deja el
				// artículo pegado al target cuando Whisper transcribe
				// "cambia a el administrador de tareas" en vez de
				// "cambia AL administrador de tareas" — ambas formas son
				// válidas en español hablado, pero solo "al" ya viene
				// sin artículo separado.
				for _, article := range []string{"el ", "la ", "los ", "las "} {
					target = strings.TrimPrefix(target, article)
				}
				target = strings.TrimSpace(target)
				if target != "" && target != "ventana" && target != "de" && target != "app" {
					return target
				}
			}
		}
		// Si no tiene "a algo", retorna vacío (Alt+Tab normal)
		return ""
	}

	// Eliminar palabras de acción comunes
	verbs := []string{
		// Spanish command forms.
		"abre", "abrir", "ejecuta", "lanza", "inicia",
		"busca", "encuentra", "localiza",
		"mueve", "traslada",
		"renombra", "cambia nombre",
		"crea carpeta", "nueva carpeta",
		"reproduce", "pon",
		"cierra", "cerrar",
		// English command forms. The semantic classifier is language-neutral;
		// target extraction must therefore be language-neutral too. Otherwise
		// "open unity" is correctly classified as abrir_programa but the
		// target remains the whole sentence and downstream resolution loses it.
		"open", "launch", "start", "run", "execute",
		"find", "search", "move", "rename", "play", "close",
	}

	target := normalized
	for _, verb := range verbs {
		target = strings.ReplaceAll(target, verb, "")
		target = strings.ReplaceAll(target, verb+" ", "")
	}

	target = strings.TrimSpace(target)

	// Si es vacío después de limpiar, retornar vacío
	if target == "" {
		return ""
	}

	// Detectar si es una URL (palabra clave)
	for keyword := range shared.URLKeywords {
		if strings.Contains(target, keyword) {
			// Es una URL, formatear correctamente
			if !strings.Contains(target, ".") {
				return keyword + ".com"
			}
			return target
		}
	}

	// Detectar URLs explícitas
	if strings.Contains(target, ".com") || strings.Contains(target, ".org") ||
		strings.Contains(target, ".net") || strings.Contains(target, ".es") ||
		strings.Contains(target, ".io") || strings.Contains(target, ".dev") ||
		strings.Contains(target, ".tv") || strings.Contains(target, ".co") {
		return target
	}

	// Si tiene puntos pero no es dominio, es probablemente un archivo
	if strings.Contains(target, ".") {
		return target
	}

	// Es un programa o búsqueda
	return target
}

/* =======================
   EXTRACT PARAMETERS
======================= */

func extractParameters(input, action string) map[string]string {
	params := make(map[string]string)
	params["raw"] = input

	// Generar params["url"] cuando la acción es abrir y el target es una URL
	if action == "abrir_programa" {
		normalized := shared.Normalize(input)
		verbs := []string{"abre ", "abrir ", "ejecuta ", "lanza ", "inicia "}
		target := normalized
		for _, verb := range verbs {
			target = strings.ReplaceAll(target, verb, "")
		}
		target = strings.TrimSpace(target)

		// Alias fonéticos conocidos primero (ej. "chat hebede" -> chatgpt)
		// — se revisan antes que el match por substring porque estos
		// casos ya NO se parecen lo suficiente al nombre real como
		// para que Contains() los encuentre.
		for alias, canonical := range shared.URLAliasPhrases {
			if strings.Contains(target, alias) {
				params["url"] = canonical + ".com"
				break
			}
		}

		// Buscar palabra clave de URL. Se compara también contra el
		// target SIN espacios: las claves del diccionario están todas
		// juntas ("chatgpt", "stackoverflow"), pero así es raro que
		// alguien lo diga o que un STT lo transcriba — lo natural es
		// "chat gpt", "stack overflow", con espacio. Sin esto, "abre
		// chat gpt" nunca calzaba con "chatgpt" y terminaba buscando
		// (sin éxito) un programa instalado llamado "chat gpt".
		targetNoSpaces := strings.ReplaceAll(target, " ", "")
		if params["url"] == "" {
			for keyword := range shared.URLKeywords {
				if strings.Contains(target, keyword) || strings.Contains(targetNoSpaces, keyword) {
					if !strings.Contains(target, ".") {
						params["url"] = keyword + ".com"
					} else {
						params["url"] = target
					}
					break
				}
			}
		}

		// Verificar dominios explícitos si no encontró palabra clave
		if params["url"] == "" {
			domainSuffixes := []string{".com", ".org", ".net", ".es", ".io", ".dev", ".tv", ".co"}
			for _, suffix := range domainSuffixes {
				if strings.Contains(target, suffix) {
					params["url"] = target
					break
				}
			}
		}
	}

	// Extraer destino para "mover" (ej: "mueve archivo a documentos")
	if action == "mover_archivo" {
		if strings.Contains(strings.ToLower(input), " a ") {
			parts := strings.Split(strings.ToLower(input), " a ")
			if len(parts) > 1 {
				params["destination"] = strings.TrimSpace(parts[1])
			}
		}
	}

	// Extraer porcentaje de volumen (ej: "volumen al 50")
	if action == "volumen_set" {
		words := strings.Fields(strings.ToLower(input))
		for i, word := range words {
			if word == "al" || word == "a" {
				if i+1 < len(words) {
					// Verificar si siguiente es número
					if _, err := strconv.Atoi(strings.TrimSuffix(words[i+1], "%")); err == nil {
						params["percent"] = strings.TrimSuffix(words[i+1], "%")
						break
					}
				}
			}
		}
	}

	return params
}

/* =======================
   MAIN CLASSIFICATION
======================= */

func ClassifyCommand(input string) ClassificationResult {
	cleaned := cleanInput(shared.RemoveWakeWord(input))
	action, confidence := extractAction(cleaned)
	target := extractTarget(cleaned, action)
	params := extractParameters(input, action)

	return ClassificationResult{
		Action:     action,
		Target:     target,
		Parameters: params,
		Confidence: confidence,
	}
}

/* =======================
   HELPER FUNCTIONS
======================= */

func isURL(target string) bool {
	if target == "" {
		return false
	}

	// Verificar palabras clave de URL
	normalized := shared.Normalize(target)
	for keyword := range shared.URLKeywords {
		if strings.Contains(normalized, keyword) {
			return true
		}
	}

	// Verificar dominios explícitos
	if strings.Contains(target, ".com") || strings.Contains(target, ".org") ||
		strings.Contains(target, ".net") || strings.Contains(target, ".es") ||
		strings.Contains(target, ".io") || strings.Contains(target, ".dev") ||
		strings.Contains(target, ".tv") || strings.Contains(target, ".co") {
		return true
	}

	return false
}

func ClassifyCommandJSON(input string) string {
	result := ClassifyCommand(input)
	jsonResult, err := json.Marshal(result)
	if err != nil {
		return "{}"
	}
	return string(jsonResult)
}

func IsValidCommand(input string) bool {
	result := ClassifyCommand(input)
	return result.Action != "desconocido" && result.Confidence >= 0.4
}

/* =======================
   CLASIFICACIÓN CON CONTEXTO
======================= */

// ContextHint es información del contexto que ayuda al clasificador
type ContextHint struct {
	LastAction  string // Última acción ejecutada
	LastTarget  string // Último target
	ActiveTopic string // Tema activo ("sistema_audio", "archivos", etc.)
}

// ClassifyWithContext usa el contexto para mejorar la clasificación
// cuando el comando solo es ambiguo (confidence < 0.7)
func ClassifyWithContext(input string, hint ContextHint) ClassificationResult {
	result := ClassifyCommand(input)

	// Si ya tiene alta confianza, no necesita contexto
	if result.Confidence >= 0.7 {
		return result
	}

	normalized := shared.Normalize(strings.ToLower(input))

	// Desambiguación por tema activo
	switch hint.ActiveTopic {
	case "sistema_audio":
		// Comandos cortos de volumen sin contexto
		if matchesAny(normalized, []string{"sube", "mas", "arriba"}) {
			return ClassificationResult{Action: "volumen_subir", Target: "", Parameters: map[string]string{"raw": input}, Confidence: 0.8}
		}
		if matchesAny(normalized, []string{"baja", "menos", "abajo"}) {
			return ClassificationResult{Action: "volumen_bajar", Target: "", Parameters: map[string]string{"raw": input}, Confidence: 0.8}
		}
		if matchesAny(normalized, []string{"silencio", "mute", "calla"}) {
			return ClassificationResult{Action: "volumen_silencio", Target: "", Parameters: map[string]string{"raw": input}, Confidence: 0.8}
		}

	case "archivos":
		// Comandos cortos de archivos sin contexto
		if matchesAny(normalized, []string{"abre", "abralo", "ver"}) {
			return ClassificationResult{Action: "buscar_archivo", Target: hint.LastTarget, Parameters: map[string]string{"raw": input}, Confidence: 0.75}
		}

	case "navegacion_ventanas":
		// Comandos cortos de ventanas
		if matchesAny(normalized, []string{"otra", "siguiente", "cambia", "siguiente"}) {
			return ClassificationResult{Action: "cambiar_ventana", Target: "", Parameters: map[string]string{"raw": input}, Confidence: 0.8}
		}
	}

	// Desambiguación por última acción
	switch hint.LastAction {
	case "volumen_subir", "volumen_bajar", "volumen_set":
		if matchesAny(normalized, []string{"mas", "sube", "arriba"}) {
			return ClassificationResult{Action: "volumen_subir", Target: "", Parameters: map[string]string{"raw": input}, Confidence: 0.75}
		}
		if matchesAny(normalized, []string{"menos", "baja", "abajo"}) {
			return ClassificationResult{Action: "volumen_bajar", Target: "", Parameters: map[string]string{"raw": input}, Confidence: 0.75}
		}
	}

	return result
}

// matchesAny verifica si el texto contiene alguna de las palabras
func matchesAny(text string, words []string) bool {
	return shared.MatchesAny(text, words)
}

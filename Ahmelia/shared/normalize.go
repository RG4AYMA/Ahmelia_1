package shared

import (
	"strings"
)

// Normalize limpia y normaliza texto — ÚNICA versión
func Normalize(text string) string {
	text = strings.ToLower(strings.TrimSpace(text))

	// Eliminar signos de puntuación
	replacements := map[string]string{
		"¿": "", "?": "", "¡": "", "!": "",
		",": "", ".": "", ";": "", ":": "",
	}

	for old, new := range replacements {
		text = strings.ReplaceAll(text, old, new)
	}

	// Normalizar acentos
	replacer := strings.NewReplacer(
		"á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u",
		"Á", "a", "É", "e", "Í", "i", "Ó", "o", "Ú", "u",
		"ü", "u", "Ü", "u", "ñ", "n", "Ñ", "n",
	)
	text = replacer.Replace(text)

	return strings.TrimSpace(text)
}

// RemoveWakeWord elimina la palabra de activación
func RemoveWakeWord(input string) string {
	text := strings.ToLower(input)
	for _, w := range WakeWords {
		text = strings.ReplaceAll(text, w, "")
	}
	return strings.TrimSpace(text)
}

// InferTags deduce los tags de un comando automáticamente
func InferTags(input, action string) []string {
	tags := []string{}
	normalized := Normalize(input)

	// Mapeo automático — si contiene estas palabras, agregar estos tags
	for tag, keywords := range TagMappings {
		for _, keyword := range keywords {
			if strings.Contains(normalized, keyword) {
				tags = append(tags, tag)
				break // agregar tag solo una vez
			}
		}
	}

	// Si no tiene tags, usar el action como tag
	if len(tags) == 0 && action != "" {
		tags = append(tags, action)
	}

	return tags
}

// MatchesAny verifica si texto contiene alguna de las palabras
func MatchesAny(text string, words []string) bool {
	for _, w := range words {
		if strings.Contains(text, w) {
			return true
		}
	}
	return false
}

// Contains verifica si un slice contiene un string
func Contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

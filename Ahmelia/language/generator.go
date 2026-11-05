package language

import "strings"

// LanguageGenerator es la fachada de la CAPA 6 (Generación de Lenguaje Natural)
// que el Engine usa para convertir respuestas simples en respuestas naturales,
// con expresión de confianza. Internamente reutiliza el OpinionExpressor.
type LanguageGenerator struct {
	opinion *OpinionExpressor
}

// NewLanguageGenerator crea el generador de lenguaje natural de la Capa 6
func NewLanguageGenerator() *LanguageGenerator {
	return &LanguageGenerator{
		opinion: NewOpinionExpressor(),
	}
}

// GenerateQuick genera una expresión natural rápida de una conclusión,
// adaptando el tono según el nivel de confianza.
//
// IMPORTANTE: si el texto ya viene "pre-armado" (porque otra capa ya expresó
// su propia incertidumbre, o ya es una respuesta estructurada con varias
// líneas/secciones), NO lo envolvemos de nuevo. Envolver un texto que ya es
// un hedge, o un texto multi-línea con evidencia/advertencias, produce dobles
// negaciones ("no estoy segura... no estoy segura...") o contradicciones
// ("estoy muy segura de que... no tengo suficientes datos").
func (lg *LanguageGenerator) GenerateQuick(conclusion string, confidence float64, confidenceDesc string) string {
	if lg == nil || lg.opinion == nil {
		return conclusion
	}
	if alreadyHedged(conclusion) || alreadyStructured(conclusion) {
		return conclusion
	}
	return lg.opinion.Express(conclusion, confidence, confidenceDesc)
}

// hedgePrefixes son aperturas que ya expresan incertidumbre por sí mismas.
// Si el texto ya empieza así, envolverlo otra vez duplica la duda.
var hedgePrefixes = []string{
	"no estoy segur", // segura/seguro
	"no se ",         // "no sé" ya normalizado sin tilde
	"no sé ",
	"no tengo suficiente",
	"no tengo certeza",
	"no puedo estar segur",
	"tengo dudas",
	"con mucha incertidumbre",
}

func alreadyHedged(text string) bool {
	lower := strings.ToLower(strings.TrimSpace(text))
	for _, prefix := range hedgePrefixes {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return false
}

// alreadyStructured detecta respuestas que ya vienen formateadas por otra
// capa (múltiples líneas, secciones marcadas con emojis de evidencia /
// advertencia, o texto que ya es una oración completa por sí mismo).
// Esas respuestas deben decirse tal cual, no reducirse a una plantilla de
// opinión como "Creo que X es la mejor opción" — esa plantilla asume que el
// texto es un fragmento tipo "usar Python", no una oración ya terminada
// como "Soy Ahmelia v3.1. Última actualización: ...".
func alreadyStructured(text string) bool {
	if strings.Contains(text, "\n") {
		return true
	}
	structuredMarkers := []string{"📊", "⚠️", "**", "- "}
	for _, marker := range structuredMarkers {
		if strings.Contains(text, marker) {
			return true
		}
	}
	// Texto con varias oraciones (". " en medio) ya viene completo/armado.
	if strings.Contains(text, ". ") {
		return true
	}
	// Autodescripciones ("Soy Ahmelia...") no son "conclusiones" a envolver.
	lower := strings.ToLower(strings.TrimSpace(text))
	selfDeclarationPrefixes := []string{"soy ", "yo soy "}
	for _, prefix := range selfDeclarationPrefixes {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return false
}

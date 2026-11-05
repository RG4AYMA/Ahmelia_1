package language

import "ahmelia/knowledge"

// ============================================================================
// FROM_KNOWLEDGE.GO - Puente entre el Knowledge Core y la Capa 6
// ============================================================================
//
// GenerateQuick (en language_generator.go) tiene que ADIVINAR con
// strings.Contains si un texto es un hecho, una opinión, o algo ya armado.
// Eso funciona como parche, pero es frágil: cada nuevo tipo de respuesta
// puede necesitar una heurística nueva.
//
// GenerateFromKnowledge es el camino correcto hacia adelante: cuando la
// respuesta viene del Knowledge Core (knowledge.Result), YA sabemos su Kind
// de forma explícita — no hay que adivinar nada.

// GenerateFromKnowledge convierte un knowledge.Result en texto hablado,
// respetando su Kind:
//   - KindFact / KindStructured / KindUnknown → se dice tal cual (ya es
//     una autodescripción, un hecho, o contenido ya formateado).
//   - KindOpinion → sí se envuelve con lenguaje natural de opinión,
//     porque es una conclusión propia de Ahmelia, no un hecho fijo.
func (lg *LanguageGenerator) GenerateFromKnowledge(r *knowledge.Result) string {
	if r == nil {
		return ""
	}
	if lg == nil || lg.opinion == nil {
		return r.Content
	}

	switch r.Kind {
	case knowledge.KindOpinion:
		return lg.opinion.Express(r.Content, r.Confidence, "")
	default: // KindFact, KindStructured, KindUnknown
		return r.Content
	}
}

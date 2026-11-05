package linguistics

// open_meaning_explicit.go — B2: REPRESENTACIÓN EXPLÍCITA del significado
// (directriz del propietario, principio 2; interfaz propuesta por DeepSeek
// 18:38 en el tablero, semántica firmada; nombres/tipos ajustables hasta el
// sellado del hito).
//
// Reglas de la familia BF (mínimas, no negociables):
//   - Ninguna marca autoriza por sí misma. Los mecanismos leen la marca y
//     deciden con honradez (guarda de DeepSeek: candidato con negation /
//     condition / modality≠affirmed / incompatibility NO se captura como
//     hecho llano ni autoriza).
//   - Las marcas las PROPONE el modelo lingüístico, las NORMALIZA el
//     validador y las CONSUME Ahmelia como hechos tipados. No son verdad:
//     son las lecturas lingüísticas explícitas del turno.

import "strings"

// OpenModality describe cómo se presenta el dato del turno.
type OpenModality string

const (
	ModalityHypothesis OpenModality = "hypothesis"
	ModalityAffirmed   OpenModality = "affirmed"
	ModalityReported   OpenModality = "reported"
	ModalityObserved   OpenModality = "observed"
	ModalityHistorical OpenModality = "historical"
	ModalityUnknown    OpenModality = "unknown"
)

// OpenProvenance describe la fuente del dato (guarda anti-fabricación):
// user|llm|observed|external.
type OpenProvenance string

const (
	ProvenanceUser     OpenProvenance = "user"
	ProvenanceLLM      OpenProvenance = "llm"
	ProvenanceObserved OpenProvenance = "observed"
	ProvenanceExternal OpenProvenance = "external"
)

// OpenNegation marca prohibiciones y negados ("No abras el archivo").
type OpenNegation struct {
	Scope  string `json:"scope"`  // accion | estado | cantidad
	Target string `json:"target"` // lo que NO debe o no es
}

// OpenCondition marca acciones condicionadas ("Si termina la descarga, abre...").
type OpenCondition struct {
	Antecedent string `json:"antecedent"`
	Consequent string `json:"consequent"`
}

// OpenException marca excepciones ("Todos... excepto los visitantes").
type OpenException struct {
	Set      string `json:"set"`      // a quién aplica la regla
	Excluded string `json:"excluded"` // quién NO entra
}

// OpenIncompatibility marca contradicciones SIN suavizarlas ("Nunca...; fui ayer").
type OpenIncompatibility struct {
	A      string `json:"a"`
	B      string `json:"b"`
	Reason string `json:"reason"`
}

// OpenMarks agrupa las marcas explícitas del turno. Todas opcionales.
type OpenMarks struct {
	Negation        *OpenNegation         `json:"negation,omitempty"`
	Condition       *OpenCondition        `json:"condition,omitempty"`
	Exception       *OpenException        `json:"exception,omitempty"`
	Modality        OpenModality          `json:"modality,omitempty"`
	Provenance      OpenProvenance        `json:"provenance,omitempty"`
	Incompatibility []OpenIncompatibility `json:"incompatibility,omitempty"`
}

// normalizeOpenMarks recorta cadenas, cae dominios fuera-de-set en estados
// honestos (modality→unknown; provenance→vacío, que el consumidor lee como
// desconocido) y descarta marcas vacías (una negación sin target no es marca).
func normalizeOpenMarks(in OpenMarks) OpenMarks {
	out := OpenMarks{}
	if in.Negation != nil && strings.TrimSpace(in.Negation.Target) != "" {
		out.Negation = &OpenNegation{Scope: strings.TrimSpace(in.Negation.Scope), Target: strings.TrimSpace(in.Negation.Target)}
	}
	if in.Condition != nil && strings.TrimSpace(in.Condition.Antecedent) != "" && strings.TrimSpace(in.Condition.Consequent) != "" {
		out.Condition = &OpenCondition{Antecedent: strings.TrimSpace(in.Condition.Antecedent), Consequent: strings.TrimSpace(in.Condition.Consequent)}
	}
	if in.Exception != nil && strings.TrimSpace(in.Exception.Set) != "" && strings.TrimSpace(in.Exception.Excluded) != "" {
		out.Exception = &OpenException{Set: strings.TrimSpace(in.Exception.Set), Excluded: strings.TrimSpace(in.Exception.Excluded)}
	}
	switch {
	case in.Modality == "":
		// sin declaración del modelo: no inventamos una
	default:
		out.Modality = normalizeOpenModality(in.Modality)
	}
	switch {
	case in.Provenance == "":
	default:
		switch OpenProvenance(strings.TrimSpace(string(in.Provenance))) {
		case ProvenanceUser, ProvenanceLLM, ProvenanceObserved, ProvenanceExternal:
			out.Provenance = OpenProvenance(strings.TrimSpace(string(in.Provenance)))
		}
	}
	for _, inc := range in.Incompatibility {
		a, b, reason := strings.TrimSpace(inc.A), strings.TrimSpace(inc.B), strings.TrimSpace(inc.Reason)
		if a != "" && b != "" {
			out.Incompatibility = append(out.Incompatibility, OpenIncompatibility{A: a, B: b, Reason: reason})
		}
	}
	return out
}

func normalizeOpenModality(m OpenModality) OpenModality {
	switch OpenModality(strings.TrimSpace(string(m))) {
	case ModalityHypothesis, ModalityAffirmed, ModalityReported, ModalityObserved, ModalityHistorical:
		return OpenModality(strings.TrimSpace(string(m)))
	default:
		return ModalityUnknown
	}
}

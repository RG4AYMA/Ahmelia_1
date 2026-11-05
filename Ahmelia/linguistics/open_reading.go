package linguistics

// open_reading.go — B1: LECTOR del significado (propuesta DeepSeek 18:38).
// Función PURA: OpenMeaning → Reading{Acto, Owner, Valor, Evidence, Marks}.
// Sin LLM, 100 % testeable offline. El consumo (core) la llama en el punto
// ya localizado: respuesta a pregunta abierta registrada o declaración
// exenta admitida. Cambia la FUENTE de lectura (significado abierto), no
// la autoridad: la decisión sigue siendo del mecanismo con sus guardas.

import "strings"

// Reading es la lectura postfixa consumible por los mecanismos.
type Reading struct {
	Acto     string    // consulta | captura | solicitud | entrega | evaluacion | social | operativa | desconocido
	Owner    OpenOwner // del dato del turno (misma regla del User Model)
	Valor    string    // candidato verbatim: primera mention ("" si ninguna)
	Evidence []string  // TODAS las mentions verbatim + el meaning (contexto completo)
	Marks    OpenMarks // marcas explícitas del turno (BF1/BF2: nunca autorizan solas)
}

// deriveActo es el enrutador mínimo purpose→acto de conducta. La guarda de
// DeepSeek vive en el CONSUMO: un Reading con Marks.Negation != nil NO se
// captura como hecho llano aunque Acto sea "captura".
func deriveActo(p OpenPurpose) string {
	switch p {
	case OpenPurposeQuestion:
		return "consulta"
	case OpenPurposeDeclaration:
		return "captura"
	case OpenPurposeRequest:
		return "solicitud"
	case OpenPurposeDelivery:
		return "entrega"
	case OpenPurposeFeedback:
		return "evaluacion"
	case OpenPurposeSocial:
		return "social"
	case OpenPurposeOperational:
		return "operativa"
	default:
		return "desconocido"
	}
}

// pickValor elige el valor verbatim candidato: la PRIMERA mention en el orden
// del modelo (que en la captura suele ser el objeto del dato). Devuelve ""
// si no hay mention. NINGÚN ajuste ni derivado: verbatim como el ciclo
// honesto ya verificado en vivo (17:09).
func pickValor(m OpenMeaning) string {
	for _, s := range m.Mentions {
		if t := strings.TrimSpace(s); t != "" {
			return t
		}
	}
	return ""
}

// DeriveReading es la puerta única del lector. Pura, sin sorpresas: lo único
// que hace es traducir el contrato abierto al gramo que el mecanismo ya usa.
func DeriveReading(om OpenMeaning) Reading {
	r := Reading{
		Acto:  deriveActo(om.Purpose),
		Owner: om.OwnerOfData,
		Valor: pickValor(om),
		Marks: om.Marks,
	}
	r.Evidence = append(r.Evidence, om.Mentions...)
	if m := om.Meaning; strings.TrimSpace(m) != "" {
		r.Evidence = append(r.Evidence, strings.TrimSpace(m))
	}
	return r
}

// RestrictiveMarkers lista los marcadores de un Reading que, por la firma B2
// (DeepSeek 18:38, directiva del propietario), requieren tratamiento no-llano
// en el consumo: negation, condition, incompatibility y modality declarada
// distinta de affirmed. DESCRIPTOR puro — no es la decisión de autorización;
// esa vive en la guard de core.
func RestrictiveMarkers(r Reading) []string {
	var out []string
	if r.Marks.Negation != nil {
		out = append(out, "negation")
	}
	if r.Marks.Condition != nil {
		out = append(out, "condition")
	}
	if len(r.Marks.Incompatibility) > 0 {
		out = append(out, "incompatibility")
	}
	if r.Marks.Modality != "" && r.Marks.Modality != ModalityAffirmed {
		out = append(out, "modality-"+string(r.Marks.Modality))
	}
	return out
}

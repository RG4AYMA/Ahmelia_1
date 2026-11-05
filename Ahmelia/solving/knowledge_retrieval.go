package solving

import (
	"fmt"
	"strings"
)

// ============================================================================
// KNOWLEDGE_RETRIEVAL.GO — Bloque 56, Fase 4.1
// ============================================================================
//
// Skill Integration (55) consulta Skill Library, pero solo para
// INFORMAR junto al curriculum — nunca evita una salida real a
// Internet. Cada vez que alguien pide "guarda una nota sobre X"
// (Bloque 54), Ahmelia repite la cadena completa Discover→Fetch→
// Validate, aunque ya tenga una nota guardada sobre ese mismo dominio
// de hace un minuto.
//
// Knowledge Retrieval es el que efectivamente EVITA la salida a
// Internet cuando no hace falta: antes de tocar la red, pregunta si
// ya hay una nota guardada para el dominio detectado. Si la hay, la
// devuelve directo — cero llamadas de red, cero Fetch, cero Validate.
//
// LÍMITE HONESTO: esto NO decide si la nota sigue siendo VÁLIDA (¿la
// documentación de Unreal Engine cambió desde que se guardó?). Esa es
// una pregunta distinta —"¿sigue siendo cierto lo que tengo?"— que
// necesitaría fecha de publicación del contenido original, algo que
// Document (gateway, 51) no trae, mismo límite que Bloque 53 ya
// documentó sobre no inventar antigüedad. El criterio acá es más
// simple y 100% real: "¿tengo algo guardado para este dominio, sí o
// no?" Nada de heurísticas de vencimiento. Ese refinamiento —cuándo
// una nota guardada debería considerarse vieja— es Skill Update
// Detector (58, todavía no existe), un bloque distinto a propósito.

// RetrievalResult es lo que Knowledge Retrieval decidió: usar lo que
// ya está guardado, o que hace falta salir a buscar.
type RetrievalResult struct {
	Goal   string
	Domain string // "" si no se reconoció el dominio
	Skill  *Skill // nil si no había nada guardado para ese dominio
}

// NeedsFetch es true si Retrieval NO encontró nada usable — quien
// llama tiene que seguir con la cadena real
// Discover→Fetch→Validate→Build. Atajo booleano, mismo criterio que
// HasSources()/HasLead()/HasAnalogy()/HasKnownSkill() en el resto del
// proyecto.
func (rr *RetrievalResult) NeedsFetch() bool {
	return rr.Skill == nil
}

// Describe narra qué decidió Retrieval — tres casos, igual que
// DiscoveryResult.Describe() (52) y SkillContext.Describe() (55): sin
// dominio reconocido, dominio reconocido sin nada guardado, o
// resuelto desde lo ya guardado sin tocar la red.
func (rr *RetrievalResult) Describe() string {
	if rr.Skill == nil {
		if rr.Domain == "" {
			return fmt.Sprintf("No reconozco un dominio técnico conocido para \"%s\" — no hay nada guardado que consultar.", rr.Goal)
		}
		return fmt.Sprintf("Detecté el dominio %s, pero no tengo ninguna nota guardada — hace falta salir a buscar.", rr.Domain)
	}
	corrob := "sin corroborar"
	if rr.Skill.Corroborated {
		corrob = "corroborada"
	}
	return fmt.Sprintf("Ya tenía una nota %s guardada sobre %s (fuentes: %s) — no hizo falta volver a Internet.",
		corrob, rr.Domain, strings.Join(rr.Skill.Sources, ", "))
}

// KnowledgeRetriever decide si hace falta salir a Internet o si ya
// hay algo guardado que sirve. Sin estado propio — la biblioteca real
// vive en SkillLibrary (54), esto solo consulta.
type KnowledgeRetriever struct{}

// NewKnowledgeRetriever arma el retriever.
func NewKnowledgeRetriever() *KnowledgeRetriever {
	return &KnowledgeRetriever{}
}

// Retrieve intenta resolver goalText desde Skill Library ANTES de
// tocar la red — ninguna llamada de este método hace I/O real, mismo
// espíritu que Discoverer.Discover() (52), que tampoco toca la red
// por sí solo. discoverer y skills pueden ser nil — defensivo.
func (kr *KnowledgeRetriever) Retrieve(goalText string, discoverer *Discoverer, skills *SkillLibrary) *RetrievalResult {
	rr := &RetrievalResult{Goal: goalText}
	if discoverer == nil || goalText == "" {
		return rr
	}

	discovery := discoverer.Discover(goalText)
	rr.Domain = discovery.Domain
	if rr.Domain == "" || skills == nil {
		return rr
	}

	if note, ok := skills.Get(rr.Domain); ok {
		rr.Skill = note
	}
	return rr
}

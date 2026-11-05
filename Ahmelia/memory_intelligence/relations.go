package memory_intelligence

import (
	"ahmelia/interaction"
	"fmt"
	"strings"

	"ahmelia/memory"
	"ahmelia/shared"
)

// ============================================================================
// RELATIONS.GO — Bloque 2, Nivel 2: conectar comandos que hacen lo mismo
// ============================================================================
//
// learning/pattern.go ya tenía Relate()/GetByTag()/GetByType() construidos
// sobre memory.LongMemory desde antes — la infraestructura para conectar
// comandos relacionados existía, pero engine.go nunca la llamaba (huérfano,
// mismo patrón que mentalAgenda y que Timeline antes de Nivel 1). Esto no
// reemplaza esa infraestructura: la usa. El engine sigue siendo quien
// decide CUÁNDO conectar dos inputs (ver core/engine.go, execute()); este
// archivo solo se encarga de CONSULTAR lo ya conectado en lenguaje natural.

// relationTriggers son las frases que disparan una consulta de relaciones.
// Cada una debe terminar justo antes del sujeto ("...con ", "...que ") para
// poder cortar el texto ahí y quedarnos con lo que sigue.
var relationTriggers = []string{
	"que esta relacionado con ",
	"que mas hace lo mismo que ",
	"de que otra forma puedo decir ",
	"de que otra forma digo ",
	"con que mas se conecta ",
}

// ParseRelationsQuery detecta si la pregunta pide navegar relaciones entre
// comandos ("qué está relacionado con subir el volumen") y devuelve el
// sujeto ya recortado (el comando sobre el que se pregunta). normalizedText
// ya debe venir de shared.Normalize — mismo contrato que ParseTimelineQuery.
func ParseRelationsQuery(req interaction.SemanticRequest) (subject string, ok bool) {
	if !interaction.HasSemanticQuery(req, interaction.QueryHistoryRelations) {
		return "", false
	}
	if req.Topic == "" {
		return "", false
	}
	return req.Topic, true
}

// DescribeRelations arma la narrativa en español de los comandos conectados
// a subject. No inventa relaciones nuevas: solo recorre Knowledge.Relations,
// que engine.go llena en cada ejecución exitosa vía learner.Relate() cuando
// detecta que dos inputs distintos producen la misma acción+target.
func DescribeRelations(long *memory.LongMemory, subject string) (string, bool) {
	if long == nil {
		return "", false
	}

	key := shared.Normalize(subject)
	k, ok := long.GetByID(key)
	if !ok || len(k.Relations) == 0 {
		return "", false
	}

	seen := map[string]bool{key: true}
	var parts []string
	for _, id := range k.Relations {
		if seen[id] {
			continue
		}
		seen[id] = true
		if rk, ok := long.GetByID(id); ok && rk.Input != "" {
			parts = append(parts, rk.Input)
		}
	}
	if len(parts) == 0 {
		return "", false
	}

	return fmt.Sprintf("%s es lo mismo que: %s", k.Input, strings.Join(parts, ", ")), true
}

package memory_intelligence

import "strings"

// ============================================================================
// SEQUENCE.GO — Fase 2 (Relación secuencial) de Pattern Learning
// ============================================================================
//
// OJO — esto NO es lo mismo que memory.LongMemory.Relate() / relations.go.
// Relate() conecta dos formas de decir el MISMO comando ("baja el volumen"
// = "reduce el volumen"): equivalencia, sin orden ni tiempo. Esto conecta
// pasos DISTINTOS que tienden a ocurrir uno después del otro: secuencia,
// con orden.
//
// Primera versión de este archivo solo contaba PARES (A→B). Eso alcanzaba
// para validar el mecanismo, pero un flujo de trabajo real casi nunca es de
// dos pasos — "Unity → compila → VSCode → prueba" es una cadena de cuatro,
// y contar solo pares nunca la reconoce como UN patrón, la deja repartida
// en fragmentos sueltos. Esta versión cuenta subsecuencias de cualquier
// largo (n-gramas contiguos) sobre las mismas corridas de eventos exitosos.

// MaxChainLength topa cuántos pasos como máximo se consideran una sola
// cadena candidata. Es un límite práctico (evita explosión combinatoria en
// runsFromLog largas), no un límite conceptual — si más adelante hacen
// falta cadenas más largas, se sube este número, no hay otra lógica atada
// a "6" en particular.
const MaxChainLength = 6

// stepKey identifica un paso de forma estable y legible. Target puede venir
// vacío para varias acciones desde el fix de extractTarget (volumen_*, ver
// classifier.go) — en ese caso el Action solo ya identifica el paso sin
// ambigüedad.
func stepKey(action, target string) string {
	if target == "" {
		return action
	}
	return action + ":" + target
}

// runsFromLog reconstruye, a partir del EventLog completo, las "corridas"
// de pasos exitosos consecutivos dentro de una misma sesión. Reglas
// explícitas:
//
//   - Una corrida se corta cuando cambia la sesión, cuando un comando
//     falla, o cuando la acción viene vacía/"desconocido" — un fallo no
//     conecta lo de antes con lo de después salteándolo (eso es material
//     para Fase 5 — Experiencia, no para esta fase).
//   - Un paso repetido inmediatamente seguido de sí mismo (ej. "sube el
//     volumen" dos veces) se colapsa a uno solo — ya lo mide
//     Knowledge.Uses, no aporta nada nuevo acá.
//   - Corridas de un solo paso no sirven para nada (no hay secuencia que
//     contar) y se descartan.
//
// dateRun es una corrida de pasos con la fecha calendario en que empezó
// (formato "2006-01-02", tomada del primer evento). Separado de un simple
// []string porque Fase 4 (Habits) necesita saber en cuántos DÍAS DISTINTOS
// se vio una cadena, no solo cuántas veces en total — eso es lo que separa
// "patrón" (se repitió) de "hábito" (se sostiene en el tiempo).
type dateRun struct {
	Steps []string
	Date  string
}

// runsFromLog reconstruye, a partir del EventLog completo, las "corridas"
// de pasos exitosos consecutivos dentro de una misma sesión. Reglas
// explícitas:
//
//   - Una corrida se corta cuando cambia la sesión, cuando un comando
//     falla, o cuando la acción viene vacía/"desconocido" — un fallo no
//     conecta lo de antes con lo de después salteándolo (eso es material
//     para Fase 5 — Experiencia, no para esta fase).
//   - Un paso repetido inmediatamente seguido de sí mismo (ej. "sube el
//     volumen" dos veces) se colapsa a uno solo — ya lo mide
//     Knowledge.Uses, no aporta nada nuevo acá.
//   - Corridas de un solo paso no sirven para nada (no hay secuencia que
//     contar) y se descartan.
//
// Envoltorio fino sobre runsWithDatesFromLog — mismo comportamiento que la
// versión original (validada con la prueba sintética de cadenas), solo que
// ahora la fecha se calcula una sola vez y la reusan tanto Fase 3
// (Patrones, que no la necesita) como Fase 4 (Habits, que sí).
func runsFromLog(log *EventLog) [][]string {
	dateRuns := runsWithDatesFromLog(log)
	runs := make([][]string, 0, len(dateRuns))
	for _, dr := range dateRuns {
		runs = append(runs, dr.Steps)
	}
	return runs
}

func runsWithDatesFromLog(log *EventLog) []dateRun {
	var runs []dateRun
	var current []string
	var currentDate string
	var prevSession string
	havePrev := false

	flush := func() {
		if len(current) >= 2 {
			runs = append(runs, dateRun{Steps: current, Date: currentDate})
		}
		current = nil
		currentDate = ""
	}

	for _, ev := range log.Events {
		if !ev.Outcome.Success || ev.Action == "" || ev.Action == "desconocido" {
			flush()
			havePrev = false
			continue
		}
		if havePrev && ev.SessionID != prevSession {
			flush()
		}
		step := stepKey(ev.Action, ev.Target)
		if len(current) == 0 {
			currentDate = ev.Timestamp.Format("2006-01-02")
		}
		if len(current) == 0 || current[len(current)-1] != step {
			current = append(current, step)
		}
		prevSession = ev.SessionID
		havePrev = true
	}
	flush()

	return runs
}

// Chain es una subsecuencia contigua de pasos observada, con cuántas veces
// apareció en total (sumando todas las corridas).
type Chain struct {
	Steps []string
	Count int
}

func chainKey(steps []string) string {
	return strings.Join(steps, "→")
}

// BuildChains cuenta, sobre las corridas reconstruidas del EventLog,
// cuántas veces aparece cada subsecuencia contigua de largo 2..maxLen. No
// filtra por umbral acá — eso es responsabilidad de DetectPatterns
// (patterns.go); esta función es "el conteo crudo", para poder reusarla si
// hace falta sin arrastrar la lógica de qué cuenta como patrón.
func BuildChains(log *EventLog, maxLen int) []Chain {
	counts := map[string]int{}
	order := []string{}
	stepsByKey := map[string][]string{}

	for _, run := range runsFromLog(log) {
		for length := 2; length <= maxLen && length <= len(run); length++ {
			for start := 0; start+length <= len(run); start++ {
				sub := run[start : start+length]
				key := chainKey(sub)
				if _, seen := counts[key]; !seen {
					order = append(order, key)
					cp := make([]string, len(sub))
					copy(cp, sub)
					stepsByKey[key] = cp
				}
				counts[key]++
			}
		}
	}

	chains := make([]Chain, 0, len(order))
	for _, key := range order {
		chains = append(chains, Chain{Steps: stepsByKey[key], Count: counts[key]})
	}
	return chains
}

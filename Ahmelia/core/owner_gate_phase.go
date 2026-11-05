package core

// owner_gate_phase.go — DECISIONES EXPLÍCITAS DEL PROPIETARIO.
//
// Las tres puertas del paso 3 comparten la misma disciplina: reconocimiento
// determinista, fallos cerrados y respuesta verbatim. Agruparlas aquí mantiene
// Process como un orden de fases legible y deja el límite de tamaño en un solo
// lugar.
//
// El orden es deliberado: primero la postura durable (modo), después las
// decisiones sobre gustos y metas. Ninguna de las tres interpreta el turno: o
// reconocen una orden explícita o lo dejan pasar intacto.

func (e *Engine) ownerGatePhase(t *processTurn) bool {
	if e.persistentModePhase(t) {
		return true
	}
	if e.selfTastePhase(t) {
		return true
	}
	return e.agentGoalPhase(t)
}

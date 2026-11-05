package language

import (
	"strings"
	"testing"
)

// ============================================================================
// RESPONSE_COMPOSER_BLOQUE35_TEST.GO — Tests para Bloque 35
// ============================================================================
//
// Bloque 35 cierra el último hueco de las 6 decisiones de ResponsePlan:
// Formality/Warmth/Initiative (Tone) se calculaban en CreatePlan desde
// Bloque 20, pero ComposeResponse nunca los leía — solo aparecían en
// Describe(), para debugging. Estos tests confirman que Initiative
// ahora gatea el follow-up y Warmth ahora agrega un cierre cálido,
// sin romper ninguna de las otras 5 decisiones.

// TestComposeResponse_InitiativeAltaPermiteFollowUp confirma que, con
// Initiative por encima del umbral, el follow-up se agrega igual que
// antes de Bloque 35.
func TestComposeResponse_InitiativeAltaPermiteFollowUp(t *testing.T) {
	rc := NewResponseComposer(nil, nil, nil, nil, nil, nil)

	plan := ResponsePlan{
		Initiative:          0.6, // por encima de proactiveInitiativeThreshold (0.5)
		HasFollowUp:         true,
		FollowUpQuestions:   []string{"¿Seguimos con el siguiente paso?"},
		SelectedFollowUpIdx: 0,
	}

	composed := rc.ComposeResponse("Respuesta base.", plan)

	if !strings.Contains(composed, "¿Seguimos con el siguiente paso?") {
		t.Errorf("con Initiative=0.6 esperaba el follow-up incluido, obtuve: %q", composed)
	}
}

// TestComposeResponse_InitiativeBajaSuprimeFollowUp confirma el caso
// central del bloque: con Initiative por debajo del umbral, el
// follow-up NO se agrega aunque ConversationFlow lo tuviera listo —
// antes de Bloque 35 se agregaba siempre que HasFollowUp fuera true,
// sin mirar Initiative.
func TestComposeResponse_InitiativeBajaSuprimeFollowUp(t *testing.T) {
	rc := NewResponseComposer(nil, nil, nil, nil, nil, nil)

	plan := ResponsePlan{
		Initiative:          0.2, // por debajo de proactiveInitiativeThreshold (0.5) — como Task
		HasFollowUp:         true,
		FollowUpQuestions:   []string{"¿Seguimos con el siguiente paso?"},
		SelectedFollowUpIdx: 0,
	}

	composed := rc.ComposeResponse("Respuesta base.", plan)

	if strings.Contains(composed, "¿Seguimos con el siguiente paso?") {
		t.Errorf("con Initiative=0.2 (modo Task) no esperaba follow-up, obtuve: %q", composed)
	}
}

// TestComposeResponse_WarmthAltaAgregaCierre confirma que, con Warmth
// por encima del umbral y sin challenge ni follow-up ya presentes, se
// agrega el cierre cálido de Bloque 35.
func TestComposeResponse_WarmthAltaAgregaCierre(t *testing.T) {
	rc := NewResponseComposer(nil, nil, nil, nil, nil, nil)

	plan := ResponsePlan{
		Warmth: 0.9, // como Emotional
	}

	composed := rc.ComposeResponse("Respuesta base.", plan)

	if !strings.Contains(composed, warmClosers[0]) {
		t.Errorf("con Warmth=0.9 esperaba el cierre cálido, obtuve: %q", composed)
	}
}

// TestComposeResponse_WarmthBajaNoAgregaCierre confirma que, con Warmth
// bajo (como Task), no se agrega ningún cierre de más.
func TestComposeResponse_WarmthBajaNoAgregaCierre(t *testing.T) {
	rc := NewResponseComposer(nil, nil, nil, nil, nil, nil)

	plan := ResponsePlan{
		Warmth: 0.4, // como Task
	}

	composed := rc.ComposeResponse("Respuesta base.", plan)

	if strings.Contains(composed, warmClosers[0]) {
		t.Errorf("con Warmth=0.4 no esperaba cierre cálido, obtuve: %q", composed)
	}
}

// TestComposeResponse_ChallengeYaCierraNoSumaCalidez confirma que,
// aunque Warmth sea alto, si ya hay un challenge la respuesta no suma
// un segundo cierre — evitar saturar la respuesta con dos "remates"
// distintos es parte deliberada del diseño de este bloque.
func TestComposeResponse_ChallengeYaCierraNoSumaCalidez(t *testing.T) {
	rc := NewResponseComposer(nil, nil, nil, nil, nil, nil)

	plan := ResponsePlan{
		Warmth:          0.9,
		ShouldChallenge: true,
		ChallengeText:   "¿Estás seguro de eso?",
	}

	composed := rc.ComposeResponse("Respuesta base.", plan)

	if strings.Contains(composed, warmClosers[0]) {
		t.Errorf("con un challenge ya presente no esperaba también el cierre cálido, obtuve: %q", composed)
	}
	if !strings.Contains(composed, "¿Estás seguro de eso?") {
		t.Errorf("el challenge original debería seguir presente, obtuve: %q", composed)
	}
}

// TestComposeResponse_FollowUpYaCierraNoSumaCalidez confirma lo mismo
// que el test anterior, pero para el caso donde el cierre ya lo puso el
// follow-up en vez del challenge.
func TestComposeResponse_FollowUpYaCierraNoSumaCalidez(t *testing.T) {
	rc := NewResponseComposer(nil, nil, nil, nil, nil, nil)

	plan := ResponsePlan{
		Warmth:              0.9,
		Initiative:          0.8,
		HasFollowUp:         true,
		FollowUpQuestions:   []string{"¿Querés que sigamos?"},
		SelectedFollowUpIdx: 0,
	}

	composed := rc.ComposeResponse("Respuesta base.", plan)

	if strings.Contains(composed, warmClosers[0]) {
		t.Errorf("con un follow-up ya presente no esperaba también el cierre cálido, obtuve: %q", composed)
	}
	if !strings.Contains(composed, "¿Querés que sigamos?") {
		t.Errorf("el follow-up original debería seguir presente, obtuve: %q", composed)
	}
}

// TestComposeResponse_ToneNoRompeLasOtrasCincoDecisiones confirma que,
// con Tone en valores neutros (los mismos 0.5/0.5/0.5 por defecto de
// NewResponseComposer cuando mode=nil), humor/memoria/challenge siguen
// funcionando exactamente igual que antes de Bloque 35 — este bloque
// solo agrega gates nuevos, no cambia las otras 5 decisiones.
func TestComposeResponse_ToneNoRompeLasOtrasCincoDecisiones(t *testing.T) {
	rc := NewResponseComposer(nil, nil, nil, nil, nil, nil)

	plan := ResponsePlan{
		ShouldIncludeHumor: true,
		JokeText:           "un chiste cualquiera",
		JokeTier:           "wordplay",
		HasRecentMemory:    true,
		RecentMemoryText:   "el clima",
	}

	composed := rc.ComposeResponse("Respuesta base.", plan)

	if !strings.Contains(composed, "un chiste cualquiera") {
		t.Errorf("esperaba que el humor siguiera funcionando, obtuve: %q", composed)
	}
	if !strings.Contains(composed, "el clima") {
		t.Errorf("esperaba que la mención de memoria siguiera funcionando, obtuve: %q", composed)
	}
}

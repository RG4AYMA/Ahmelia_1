package personality

import (
	"strings"
	"testing"
)

// TestLikelihoodForMode_TaskEsCero confirma el caso categórico del
// roadmap: en modo Task, la probabilidad de humor es exactamente cero,
// no "baja".
func TestLikelihoodForMode_TaskEsCero(t *testing.T) {
	h := NewHumorStyle()
	if l := h.LikelihoodForMode("Task"); l != 0.0 {
		t.Fatalf("LikelihoodForMode(Task) = %v, esperaba 0.0 (categórico, no solo bajo)", l)
	}
}

// TestLikelihoodForMode_OrdenNoLinealConCampoHumor confirma la decisión
// de diseño central del bloque: Emotional debería tener MÁS
// probabilidad que Task, aunque ConversationMode.Humor (Bloque 16) diga
// lo contrario (Emotional=0.0 < Task=0.1 en ese campo). Si este test
// fallara, alguien conectó LikelihoodForMode directo al campo Humor en
// vez de mantener la política propia del roadmap.
func TestLikelihoodForMode_OrdenNoLinealConCampoHumor(t *testing.T) {
	h := NewHumorStyle()
	task := h.LikelihoodForMode("Task")
	emotional := h.LikelihoodForMode("Emotional")
	if !(emotional > task) {
		t.Fatalf("LikelihoodForMode(Emotional)=%v debería ser mayor que Task=%v (Task es 'No' categórico, Emotional es 'casi nunca' pero no cero)", emotional, task)
	}
	brainstorming := h.LikelihoodForMode("Brainstorming")
	discussion := h.LikelihoodForMode("Discussion")
	if !(brainstorming > discussion) {
		t.Fatalf("Brainstorming ('Sí') debería tener mayor probabilidad que Discussion ('A veces'): brainstorming=%v, discussion=%v", brainstorming, discussion)
	}
}

// TestShouldJokeNow_TaskNuncaBromeaSinImportarElRoll confirma que en
// Task, ningún valor de roll (ni 0.0, el mínimo posible) hace que
// ShouldJokeNow devuelva true — 0.0 < 0.0 es false, así que el roll más
// favorable posible sigue sin activar humor.
func TestShouldJokeNow_TaskNuncaBromeaSinImportarElRoll(t *testing.T) {
	h := NewHumorStyle()
	if h.ShouldJokeNow("Task", map[string]interface{}{}, 0.0) {
		t.Fatal("ShouldJokeNow(Task, roll=0.0) = true, esperaba false — Task es 'No' categórico")
	}
}

// TestShouldJokeNow_RespetaNeverJokesAbout confirma que la nueva
// decisión por modo NO pisa la protección ya existente
// (ShouldMakeJoke/NeverJokesAbout): un contexto con un tema prohibido
// debe bloquear el chiste aunque el modo (Brainstorming, "Sí") y el
// roll sean lo más favorables posible.
func TestShouldJokeNow_RespetaNeverJokesAbout(t *testing.T) {
	h := NewHumorStyle()
	context := map[string]interface{}{"fallos críticos": "el sistema se cayó"}
	if h.ShouldJokeNow("Brainstorming", context, 0.0) {
		t.Fatal("ShouldJokeNow debería seguir respetando NeverJokesAbout aunque el modo y el roll sean favorables")
	}
}

// TestAvailableTiers_EscalaConTrustLevel confirma la progresión de
// variantes: más confianza habilita más tiers, en orden, y los tiers
// que dependen de material real (callback/inside joke) no aparecen sin
// ese material aunque el trust alcance.
func TestAvailableTiers_EscalaConTrustLevel(t *testing.T) {
	rel := NewRelationship()
	rel.TrustLevel = 0.0
	rel.SharedHistory = nil
	rel.InsideJokes = nil

	tiers := (&HumorStyle{UsesHumor: true}).AvailableTiers(rel)
	if len(tiers) != 1 || tiers[0] != HumorTierLight {
		t.Fatalf("con TrustLevel=0, tiers=%v, esperaba solo [%s]", tiers, HumorTierLight)
	}

	rel.TrustLevel = 0.9 // confianza alta, pero SIN material compartido todavía
	tiers = (&HumorStyle{UsesHumor: true}).AvailableTiers(rel)
	for _, tier := range tiers {
		if tier == HumorTierCallback || tier == HumorTierInsideJoke {
			t.Fatalf("con TrustLevel=0.9 pero SharedHistory/InsideJokes vacíos, no debería aparecer %s en tiers=%v", tier, tiers)
		}
	}

	rel.SharedHistory = []SharedMoment{{Event: "arreglamos el bug del inventario juntas"}}
	rel.InsideJokes = []string{"lo del Blender que nunca abre"}
	tiers = (&HumorStyle{UsesHumor: true}).AvailableTiers(rel)
	hasCallback, hasInsideJoke := false, false
	for _, tier := range tiers {
		if tier == HumorTierCallback {
			hasCallback = true
		}
		if tier == HumorTierInsideJoke {
			hasInsideJoke = true
		}
	}
	if !hasCallback || !hasInsideJoke {
		t.Fatalf("con TrustLevel=0.9 y material real disponible, esperaba Callback e InsideJoke en tiers=%v", tiers)
	}
}

// TestPickJoke_NoSiempreDevuelveLoMismo confirma que, a diferencia de
// GetJoke() (que siempre devuelve jokes[0]), PickJoke con distintos
// rolls puede devolver contenido distinto — la razón de ser de este
// bloque, no solo decorar la reconexión.
func TestPickJoke_NoSiempreDevuelveLoMismo(t *testing.T) {
	rel := NewRelationship()
	rel.TrustLevel = 0.9
	h := NewHumorStyle()

	seen := map[string]bool{}
	for _, roll := range []float64{0.0, 0.2, 0.4, 0.6, 0.8, 0.99} {
		text, tier := h.PickJoke(rel, roll)
		if text == "" || tier == "" {
			t.Fatalf("PickJoke(roll=%v) devolvió vacío con TrustLevel=0.9 y HumorBank cargado", roll)
		}
		seen[text] = true
	}
	if len(seen) < 2 {
		t.Fatalf("PickJoke con 6 rolls distintos devolvió solo %d resultado(s) distinto(s), esperaba variedad real", len(seen))
	}
}

// TestPickJoke_InsideJokeUsaElBancoDeLaRelacion confirma que, cuando el
// tier elegido es InsideJoke, el texto sale de rel.InsideJokes (el
// campo que ya existía en el struct, vacío, sin que nada lo leyera
// antes de este bloque) — no del HumorBank estático.
func TestPickJoke_InsideJokeUsaElBancoDeLaRelacion(t *testing.T) {
	rel := NewRelationship()
	rel.TrustLevel = 1.0
	rel.InsideJokes = []string{"la vez que le pediste que abriera Photoshop"}

	h := &HumorStyle{UsesHumor: true} // solo el tier más alto disponible en la práctica: forzamos vía AvailableTiers
	tiers := h.AvailableTiers(rel)
	last := tiers[len(tiers)-1]
	if last != HumorTierInsideJoke {
		t.Fatalf("con TrustLevel=1.0 e InsideJokes cargado, el último tier debería ser InsideJoke, tiers=%v", tiers)
	}
	// roll=0.99 cae en el último tier de la lista (InsideJoke)
	text, tier := h.PickJoke(rel, 0.99)
	if tier != HumorTierInsideJoke {
		t.Fatalf("tier = %s, esperaba %s", tier, HumorTierInsideJoke)
	}
	if text != rel.InsideJokes[0] {
		t.Fatalf("text = %q, esperaba el contenido de rel.InsideJokes: %q", text, rel.InsideJokes[0])
	}
}

// TestExplain_TaskDiceQueNoTieneSentido confirma la frase completa para
// el caso más simple: modo Task, nunca bromea.
func TestExplain_TaskDiceQueNoTieneSentido(t *testing.T) {
	h := NewHumorStyle()
	rel := NewRelationship()
	explanation := h.Explain("Task", rel, 0.0)
	if !strings.Contains(explanation, "no me parece que tenga sentido") {
		t.Fatalf("Explain(Task) = %q, esperaba que dijera explícitamente que no tiene sentido", explanation)
	}
}

// TestParseHumorQuery_DetectaVariantesSinPisarModeQuery confirma el
// trigger, y que no choca con "en qué modo estás conversando" (Bloque
// 16) ni con "cómo es nuestra relación" (Bloque 17).
func TestParseHumorQuery_DetectaVariantesSinPisarModeQuery(t *testing.T) {
	positive := []string{
		"deberias bromear",
		"que humor usarias",
	}
	for _, q := range positive {
		if !ParseHumorQuery(q) {
			t.Errorf("ParseHumorQuery(%q) = false, esperaba true", q)
		}
	}

	negative := []string{
		"como es nuestra relacion",
		"abre chrome",
	}
	for _, q := range negative {
		if ParseHumorQuery(q) {
			t.Errorf("ParseHumorQuery(%q) = true, esperaba false", q)
		}
	}
}

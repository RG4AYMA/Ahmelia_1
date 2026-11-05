package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"ahmelia/interaction"
	"ahmelia/personality"
)

func preferenceObservationForTest(key, value string, confidence float64, sourceID string) interaction.UserObservation {
	return interaction.UserObservation{
		Subject:    interaction.UserObservationSubjectUser,
		Domain:     interaction.UserModelDomainPreference,
		Key:        key,
		Value:      value,
		Confidence: confidence,
		Provenance: interaction.UserModelSourceRef{Kind: interaction.UserSourceExplicitUser, SourceID: sourceID, ActorID: "user", ObservedAt: time.Now()},
	}
}

func TestNormalizePreferenceKeyStripsArticles(t *testing.T) {
	casos := map[string]string{
		"me gusta la gaseosa de piña": "gaseosa de piña", // articulante + artículo
		"me gustan los libros viejos": "libros viejos",
		"prefiero el café con leche":  "café con leche",
		"mi favorito es el negro":     "negro",
		"la gaseosa de piña":          "gaseosa de piña", // solo artículo (compatibilidad)
		"El café con leche":           "café con leche",
		"  el   negro  ":              "negro",
		"una historia larga":          "historia larga",
		"los libros viejos":           "libros viejos",
		"la":                          "", // artículo solo: no hay objeto, no entra al posterior
		"me gusta":                    "", // articulante solo: sin objeto
		"prefiero":                    "",
	}
	for in, want := range casos {
		if got := normalizePreferenceKey(in); got != want {
			t.Fatalf("normalizar %q: got %q want %q", in, got, want)
		}
	}
}

// R11/P4 honestidad: UNA declaración explícita nunca produce "likes".
func TestPreferenceObservationSingleDeclarationStaysUnformed(t *testing.T) {
	e := &Engine{prefModel: personality.NewPreferenceModel()}
	e.observeAcceptedPreference(preferenceObservationForTest("bebida favorita", "me gusta la gaseosa de piña", 0.9, "turn-5"), time.Now())
	view := e.prefModel.View("gaseosa de piña")
	if view.Stance != personality.StanceUnformed {
		t.Fatalf("una sola declaración no debe dar stance formado: got %s", view.Stance)
	}
	snap := e.prefModel.Snapshot()
	if len(snap) != 1 {
		t.Fatalf("snapshot debe registrar la única experiencia: len=%d", len(snap))
	}
	p := snap["gaseosa de piña"]
	if p.N != 1 || p.Refs[len(p.Refs)-1] != "turn-5" {
		t.Fatalf("experiencia mal registrada: %+v", p)
	}
}

func TestPreferenceObservationAccumulatesAndFormStance(t *testing.T) {
	e := &Engine{prefModel: personality.NewPreferenceModel()}
	for i := 0; i < 6; i++ {
		e.observeAcceptedPreference(preferenceObservationForTest("bebida favorita", "me gusta la gaseosa de piña", 0.9, "turn-x"), time.Now())
	}
	view := e.prefModel.View("gaseosa de piña")
	if view.Stance != personality.StanceLikes {
		t.Fatalf("posterior declarado +1 con evidencia suficiente debe dar likes: got %s (%.2f..%.2f, effN=%.2f)", view.Stance, view.Lo, view.Hi, view.EffN)
	}
	if view.Mean <= 0.5 {
		t.Fatalf("media posterior mal calculada: %.2f", view.Mean)
	}
}

// Polaridad (Paso 2, directiva DeepSeek 19/10): las negaciones del gusto y la
// ambigüedad NO producen experiencia — la familia explícita positiva sí.
func TestPreferencePolarityGuardRejectsNegations(t *testing.T) {
	e := &Engine{prefModel: personality.NewPreferenceModel()}
	negated := []string{
		"ya no me gusta la gaseosa",
		"no me gusta el café",
		"nunca me gusta madrugar",
		"odio el café",
		"detesto el clima frío",
		"sí", // afirmación sin cabeza de gusto: fail-closed
	}
	for _, v := range negated {
		e.observeAcceptedPreference(preferenceObservationForTest("bebida favorita", v, 0.9, "turn-neg"), time.Now())
	}
	if snap := e.prefModel.Snapshot(); len(snap) != 0 {
		t.Fatalf("negaciones/ambigüedades no deben producir experiencia: %v", snap)
	}
}

func TestPreferencePolarityGuardAcceptsPositiveFamily(t *testing.T) {
	e := &Engine{prefModel: personality.NewPreferenceModel()}
	cases := []string{
		"me gusta la gaseosa",
		"me encantan los atardeceres",
		"mi favorito es el negro",
	}
	for _, v := range cases {
		e.observeAcceptedPreference(preferenceObservationForTest("bebida favorita", v, 0.9, "turn-pos"), time.Now())
	}
	if snap := e.prefModel.Snapshot(); len(snap) != 3 {
		t.Fatalf("familia positiva explícita debe registrar 3 claves: %v", snap)
	}
}

// B90.7/P4 ciclo completo: guardar tras mutación → cargar limpio → stance vive.
func TestPreferenceSnapshotRoundtrip(t *testing.T) {
	e := &Engine{prefModel: personality.NewPreferenceModel()}
	for i := 0; i < 6; i++ {
		e.observeAcceptedPreference(preferenceObservationForTest("bebida favorita", "me gusta la gaseosa de piña", 0.9, "turn-x"), time.Now())
	}
	path := filepath.Join(t.TempDir(), "prefs.json")
	e.saveUserPreferenceSnapshotTo(path)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("snapshot no persistido: %v", err)
	}
	snap := map[string]personality.Posterior{}
	if err := json.Unmarshal(raw, &snap); err != nil {
		t.Fatalf("snapshot no parseable: %v\n%s", err, raw)
	}
	revived := personality.NewPreferenceModel()
	if err := revived.Restore(snap); err != nil {
		t.Fatalf("restauración falló: %v", err)
	}
	if revived.View("gaseosa de piña").Stance != personality.StanceLikes {
		t.Fatalf("el stance no sobrevivió al ciclo guardar→cargar")
	}
}

func TestPreferenceHooksAreNilSafe(t *testing.T) {
	var empty *Engine
	empty.observeAcceptedPreference(preferenceObservationForTest("k", "v", 1, "s"), time.Now())
	empty.saveUserPreferenceSnapshot()
	lazy := &Engine{}
	lazy.observeAcceptedPreference(preferenceObservationForTest("k", "v", 1, "s"), time.Now())
	if got := derivedPreferenceStances(lazy.prefModel); got != nil {
		t.Fatalf("engine sin prefModel no debe derivar stances, got %d", len(got))
	}
}

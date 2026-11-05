package core

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ahmelia/agency"
	"ahmelia/cognition"
	"ahmelia/deliberation"
	"ahmelia/interaction"
	"ahmelia/persistentmode"
)

func persistentModeTestEngine(t *testing.T, path string) *Engine {
	t.Helper()
	e := &Engine{}
	e.enablePersistentModeAt(path)
	if err := e.LastPersistentModeError(); err != "" {
		t.Fatalf("enable: %s", err)
	}
	if _, ok := e.PersistentModeState(); !ok {
		t.Fatal("el modo persistente no quedó habilitado")
	}
	return e
}

func TestParsePersistentModeCommandExplicitForms(t *testing.T) {
	cases := []struct {
		raw  string
		kind persistentModeCommandKind
		mode agency.Mode
	}{
		{"¿cuál es el modo persistente?", persistentModeQuery, agency.ModeUnknown},
		{"modo actual", persistentModeQuery, agency.ModeUnknown},
		{"¿qué modo por defecto tienes?", persistentModeQuery, agency.ModeUnknown},
		{"cambia el modo a focus", persistentModeSet, agency.ModeFocus},
		{"modo: exploración", persistentModeSet, agency.ModeExplore},
		{"ponte en modo descanso", persistentModeSet, agency.ModeRest},
		{"cambia el modo persistente a compañía", persistentModeSet, agency.ModeCompanion},
		{"pon el modo trabajo", persistentModeSet, agency.ModeFocus},
	}
	for _, tc := range cases {
		cmd := parsePersistentModeCommand(tc.raw)
		if cmd.Kind != tc.kind || cmd.Mode != tc.mode {
			t.Fatalf("%q -> %+v, esperaba %s/%s", tc.raw, cmd, tc.kind, tc.mode)
		}
		if cmd.Problem != "" {
			t.Fatalf("%q no debía fallar: %s", tc.raw, cmd.Problem)
		}
	}
}

func TestParsePersistentModeCommandFailsClosed(t *testing.T) {
	cases := []struct {
		raw     string
		problem string
	}{
		{"no cambies el modo a focus", "negación"},
		{"nunca pongas el modo descanso", "negación"},
		{"cambia el modo a focus o descanso", "más de un modo"},
		{"modo: turbo", "No reconozco"},
		{"cambia el modo persistente", "necesito un modo exacto"},
	}
	for _, tc := range cases {
		cmd := parsePersistentModeCommand(tc.raw)
		if cmd.Problem == "" || !strings.Contains(cmd.Problem, tc.problem) {
			t.Fatalf("%q -> %+v, esperaba problema con %q", tc.raw, cmd, tc.problem)
		}
	}
}

func TestParsePersistentModeCommandLeavesNormalTalkAlone(t *testing.T) {
	cases := []string{
		"me gusta el modo en que trabajo",
		"cambia el modo en que trabajo",
		"no hables de trabajo",
		"estoy en modo focus",
	}
	for _, raw := range cases {
		if cmd := parsePersistentModeCommand(raw); cmd.Kind != persistentModeNone {
			t.Fatalf("%q fue interceptado: %+v", raw, cmd)
		}
	}
}

func TestPersistentModeGateSetsAndPersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory", "persistent_mode.json")
	e := persistentModeTestEngine(t, path)

	text, handled := e.applyPersistentModeCommand(parsePersistentModeCommand("cambia el modo a focus"))
	if !handled || !strings.Contains(text, "foco") {
		t.Fatalf("respuesta = %q handled=%t", text, handled)
	}
	state, ok := e.PersistentModeState()
	if !ok || state.Mode != agency.ModeFocus || state.ChangedBy != "owner" {
		t.Fatalf("estado = %+v ok=%t", state, ok)
	}

	reloaded := persistentModeTestEngine(t, path)
	state, ok = reloaded.PersistentModeState()
	if !ok || state.Mode != agency.ModeFocus {
		t.Fatalf("tras reiniciar: %+v ok=%t", state, ok)
	}

	text, handled = reloaded.applyPersistentModeCommand(parsePersistentModeCommand("¿cuál es el modo persistente?"))
	if !handled || !strings.Contains(text, "foco") {
		t.Fatalf("consulta = %q handled=%t", text, handled)
	}
}

func TestPersistentModePhaseConsumesOwnerCommand(t *testing.T) {
	e := persistentModeTestEngine(t, filepath.Join(t.TempDir(), "persistent_mode.json"))
	var spoken []string
	e.onResponse = func(msg string) { spoken = append(spoken, msg) }

	raw := "cambia el modo a focus"
	turn := &processTurn{
		input: raw,
		semanticReq: interaction.SemanticRequest{
			Raw:       raw,
			Objective: interaction.Objective{Text: raw},
		},
	}
	if !e.persistentModePhase(turn) {
		t.Fatal("la fase debía consumir la orden")
	}
	if len(spoken) == 0 || !strings.Contains(strings.Join(spoken, " "), "foco") {
		t.Fatalf("no se habló el cambio: %v", spoken)
	}
	state, _ := e.PersistentModeState()
	if state.Mode != agency.ModeFocus {
		t.Fatalf("estado=%s", state.Mode)
	}

	plain := &processTurn{input: "hola, ¿cómo estás?"}
	if e.persistentModePhase(plain) {
		t.Fatal("una frase corriente no puede ser interceptada")
	}
}

func TestPersistentModeGateFailsClosedWithoutChangingState(t *testing.T) {
	e := persistentModeTestEngine(t, filepath.Join(t.TempDir(), "persistent_mode.json"))
	for _, raw := range []string{
		"no cambies el modo a focus",
		"cambia el modo a focus o descanso",
		"modo: turbo",
	} {
		text, handled := e.applyPersistentModeCommand(parsePersistentModeCommand(raw))
		if !handled || text == "" {
			t.Fatalf("%q no se respondió", raw)
		}
	}
	state, _ := e.PersistentModeState()
	if state.Mode != persistentmode.DefaultMode {
		t.Fatalf("un fallo cerrado cambió el modo: %+v", state)
	}
}

func TestPersistentModeDisablesOnInvalidFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "persistent_mode.json")
	if err := os.WriteFile(path, []byte(`{"mode":"turbo","changed_by":"owner"}`), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	e := &Engine{}
	e.enablePersistentModeAt(path)
	if err := e.LastPersistentModeError(); err == "" {
		t.Fatal("un archivo inválido debía dejar error visible")
	}
	if _, ok := e.PersistentModeState(); ok {
		t.Fatal("el mecanismo debía quedar apagado")
	}
	if e.persistentModePhase(&processTurn{input: "cambia el modo a focus"}) {
		t.Fatal("una puerta apagada no debe consumir el turno")
	}
}

func TestPersistentModeBiasesConversationMode(t *testing.T) {
	base := &cognition.ConversationMode{Name: "Discussion", Warmth: 0.5, Initiative: 0.8, DetailLevel: 0.3, CanBringTopics: true}

	companion := persistentModeTestEngine(t, filepath.Join(t.TempDir(), "companion.json"))
	biased := companion.applyPersistentModeToConversationMode(base)
	if biased == base {
		t.Fatal("el sesgo debía devolver una copia")
	}
	if biased.Initiative >= base.Initiative {
		t.Fatalf("compañía debía bajar la iniciativa: %.2f -> %.2f", base.Initiative, biased.Initiative)
	}
	if biased.Warmth <= base.Warmth {
		t.Fatalf("compañía debía subir la calidez: %.2f -> %.2f", base.Warmth, biased.Warmth)
	}
	if base.Initiative != 0.8 {
		t.Fatal("el modo del turno no debe mutarse")
	}

	focus := persistentModeTestEngine(t, filepath.Join(t.TempDir(), "focus.json"))
	if _, err := focus.SetPersistentMode(agency.ModeFocus, "owner"); err != nil {
		t.Fatalf("set focus: %v", err)
	}
	biased = focus.applyPersistentModeToConversationMode(base)
	if biased.DetailLevel <= base.DetailLevel {
		t.Fatalf("foco debía subir el detalle: %.2f -> %.2f", base.DetailLevel, biased.DetailLevel)
	}

	rest := persistentModeTestEngine(t, filepath.Join(t.TempDir(), "rest.json"))
	if _, err := rest.SetPersistentMode(agency.ModeRest, "owner"); err != nil {
		t.Fatalf("set rest: %v", err)
	}
	biased = rest.applyPersistentModeToConversationMode(base)
	if biased.Initiative != 0 || biased.CanBringTopics {
		t.Fatalf("descanso debía cerrar iniciativa y temas: %+v", biased)
	}
}

func TestRestSuppressesImpulsesAndPlannerQuota(t *testing.T) {
	turn := &processTurn{input: "hola"}

	companion := persistentModeTestEngine(t, filepath.Join(t.TempDir(), "companion.json"))
	companion.lastBehavioralDecision.Uncertainty = true
	if got := companion.buildImpulses(turn); len(got) == 0 {
		t.Fatal("sin descanso, la incertidumbre debía abrir un impulso")
	}
	if got := companion.agentGoalPlannerQuota(); got != 1 {
		t.Fatalf("cupo companion = %d, quería 1", got)
	}

	rest := persistentModeTestEngine(t, filepath.Join(t.TempDir(), "rest.json"))
	if _, err := rest.SetPersistentMode(agency.ModeRest, "owner"); err != nil {
		t.Fatalf("set rest: %v", err)
	}
	rest.lastBehavioralDecision.Uncertainty = true
	if got := rest.buildImpulses(turn); len(got) != 0 {
		t.Fatalf("descanso abrió impulsos: %+v", got)
	}
	if got := rest.agentGoalPlannerQuota(); got != 0 {
		t.Fatalf("cupo rest = %d, quería 0", got)
	}
	if rest.persistentModeAllowsInitiative() {
		t.Fatal("descanso no debe permitir iniciativa")
	}
}

func TestModelModeSuggestionIsRecordedButNeverApplied(t *testing.T) {
	e := persistentModeTestEngine(t, filepath.Join(t.TempDir(), "persistent_mode.json"))
	e.deliberationClient = &shadowFakeClient{raw: `{
		"reading":"el turno pide compañía",
		"strategy":"answer",
		"critique":"",
		"expectation":"",
		"evidence_ids":["w1"],
		"confidence":0.8,
		"abstain":false,
		"mode_suggestion":"focus"
	}`}
	ws := &deliberation.Workspace{}
	ws.Add("turno_actual", "hola", "interaction", 1)

	rec := e.deliberateShadow(context.Background(), ws, deliberation.HarnessDecision{}, "hola", "t1")
	if rec.Proposal.ModeSuggestion != agency.ModeFocus {
		t.Fatalf("la sugerencia no se registró: %+v", rec.Proposal)
	}
	state, _ := e.PersistentModeState()
	if state.Mode != agency.ModeCompanion {
		t.Fatalf("la sugerencia del modelo cambió el modo: %+v", state)
	}
	if !strings.Contains(strings.Join(rec.Reasons, " "), "owner decides") {
		t.Fatalf("el registro no deja claro quién decide: %v", rec.Reasons)
	}
}

func TestPersistentModeIsVisibleToModelProjections(t *testing.T) {
	e := persistentModeTestEngine(t, filepath.Join(t.TempDir(), "persistent_mode.json"))
	if _, err := e.SetPersistentMode(agency.ModeExplore, "owner"); err != nil {
		t.Fatalf("set: %v", err)
	}

	ws := e.buildDeliberationWorkspace(&processTurn{input: "hola"})
	found := false
	for _, item := range ws.Items {
		if item.Kind == "modo_persistente" && strings.Contains(item.Text, "exploración") {
			found = true
		}
	}
	if !found {
		t.Fatalf("el workspace no muestra el modo: %s", ws.Render())
	}

	snap := e.buildLinguisticSituationSnapshot(time.Now(), "hola")
	if snap == nil {
		t.Fatal("la proyección lingüística no debía estar vacía")
	}
	found = false
	for _, fact := range snap.Facts {
		if fact.Kind == "modo_persistente" && strings.Contains(fact.Text, "exploración") {
			found = true
		}
	}
	if !found {
		t.Fatalf("el snapshot no muestra el modo: %+v", snap.Facts)
	}

	facts := e.buildConversationalRuntimeFacts(interaction.ConversationSnapshot{})
	if !strings.Contains(facts.PersistentMode, "exploración") {
		t.Fatalf("el generador conversacional no ve el modo: %q", facts.PersistentMode)
	}
}

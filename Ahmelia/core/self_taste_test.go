package core

import (
	"fmt"
	"strings"
	"testing"

	"ahmelia/autonomy"
	"ahmelia/interaction"
	"ahmelia/planning"
	"ahmelia/taste"
)

func selfTasteEngine(t *testing.T) *Engine {
	t.Helper()
	return &Engine{
		selfTaste:       taste.NewModel(taste.NewLedger("")),
		selfTasteOn:     true,
		planner:         planning.NewPlanner(),
		autonomousState: autonomy.New(),
	}
}

var seedTasteSeq int

func seedConfirmedTaste(t *testing.T, e *Engine, key string, n int) []taste.Candidate {
	t.Helper()
	seedTasteSeq++
	out := make([]taste.Candidate, 0, n)
	for i := 0; i < n; i++ {
		// Cada llamada usa referencias nuevas: la misma experiencia no puede
		// confirmarse dos veces.
		ref := fmt.Sprintf("%s-work-%d-%d", key, seedTasteSeq, i)
		rec, _, err := e.selfTaste.RecordVerified(key, ref)
		if err != nil {
			t.Fatalf("record %d: %v", i, err)
		}
		if _, err := e.selfTaste.Confirm(rec.ID); err != nil {
			t.Fatalf("confirm %d: %v", i, err)
		}
		out = append(out, rec)
	}
	return out
}

func TestParseSelfTasteCommandExplicitForms(t *testing.T) {
	cases := []struct {
		raw  string
		kind selfTasteCommandKind
		id   string
		key  string
	}{
		{"¿qué gustos propios tienes?", selfTasteList, "", ""},
		{"lista tus gustos", selfTasteList, "", ""},
		{"confirma el gusto tg-123", selfTasteConfirm, "tg-123", ""},
		{"Aprueba el gusto tg-1793566854741221700, por favor", selfTasteConfirm, "tg-1793566854741221700", ""},
		{"rechaza el gusto tg-42-1", selfTasteReject, "tg-42-1", ""},
		{"corrige tu gusto por explorar", selfTasteCorrect, "", "explorar"},
		{"corrige: no te gusta blender", selfTasteCorrect, "", "blender"},
	}
	for _, tc := range cases {
		cmd := parseSelfTasteCommand(tc.raw)
		if cmd.Kind != tc.kind || cmd.ID != tc.id || cmd.Key != tc.key || cmd.Problem != "" {
			t.Fatalf("%q -> %+v, esperaba %s/%s/%s", tc.raw, cmd, tc.kind, tc.id, tc.key)
		}
	}
}

func TestParseSelfTasteCommandFailsClosed(t *testing.T) {
	cases := []struct {
		raw     string
		problem string
	}{
		{"no me gusta el café", ""},        // preferencia del usuario: ruta normal
		{"¿te gusta el café?", ""},         // pregunta, no orden
		{"aprueba el plan de trabajo", ""}, // no es un gusto
		{"confirma el gusto", "ID exacto"},
		{"confirma el gusto tg-1 y tg-2", "más de un identificador"},
		{"confirma y rechaza el gusto tg-1", "confirmación y un rechazo"},
		{"no confirmes el gusto tg-1", "negación"},
		{"corrige tu gusto", "actividad exacta"},
	}
	for _, tc := range cases {
		cmd := parseSelfTasteCommand(tc.raw)
		if tc.problem == "" {
			if cmd.Kind != selfTasteNone {
				t.Fatalf("%q no debía interceptarse: %+v", tc.raw, cmd)
			}
			continue
		}
		if cmd.Kind == selfTasteNone || !strings.Contains(strings.ToLower(cmd.Problem), strings.ToLower(tc.problem)) {
			t.Fatalf("%q -> %+v, esperaba problema con %q", tc.raw, cmd, tc.problem)
		}
	}
}

func TestApplySelfTasteCommandListsConfirmsAndRejects(t *testing.T) {
	e := selfTasteEngine(t)
	rec, _, err := e.selfTaste.RecordVerified("blender", "work-1")
	if err != nil {
		t.Fatalf("record: %v", err)
	}

	list, handled := e.applySelfTasteCommand(parseSelfTasteCommand("¿qué gustos propios tienes?"))
	if !handled {
		t.Fatal("la consulta debía manejarse")
	}
	if !strings.Contains(list, rec.ID) || !strings.Contains(list, "blender") || !strings.Contains(list, "work-1") {
		t.Fatalf("lista incompleta: %s", list)
	}

	text, _ := e.applySelfTasteCommand(parseSelfTasteCommand("confirma el gusto " + rec.ID))
	if !strings.Contains(text, "cuenta como evidencia") {
		t.Fatalf("respuesta=%s", text)
	}
	got, ok := e.selfTaste.Ledger().Get(rec.ID)
	if !ok || got.Status != taste.CandidateConfirmed {
		t.Fatalf("candidate=%+v ok=%v", got, ok)
	}

	second, _, err := e.selfTaste.RecordVerified("explorar", "work-2")
	if err != nil {
		t.Fatalf("record 2: %v", err)
	}
	text, _ = e.applySelfTasteCommand(parseSelfTasteCommand("rechaza el gusto " + second.ID))
	if !strings.Contains(text, "no cuenta como evidencia") {
		t.Fatalf("respuesta=%s", text)
	}
	got, _ = e.selfTaste.Ledger().Get(second.ID)
	if got.Status != taste.CandidateRejected {
		t.Fatalf("estado=%s", got.Status)
	}

	text, _ = e.applySelfTasteCommand(parseSelfTasteCommand("confirma el gusto tg-999"))
	if !strings.Contains(text, "No encuentro") {
		t.Fatalf("respuesta=%s", text)
	}
}

func TestApplySelfTasteCommandFormsAndCorrects(t *testing.T) {
	e := selfTasteEngine(t)
	seedConfirmedTaste(t, e, "explorar", 6)
	if got := e.selfTaste.Stances(); len(got) != 1 || got[0].Stance != "likes" {
		t.Fatalf("gusto formado: %+v", got)
	}
	text, _ := e.applySelfTasteCommand(parseSelfTasteCommand("corrige tu gusto por explorar"))
	if !strings.Contains(text, "deja de contar") {
		t.Fatalf("respuesta=%s", text)
	}
	if got := e.selfTaste.Stances(); len(got) != 0 {
		t.Fatalf("la corrección debe revocar el gusto: %+v", got)
	}
}

func TestSweepSelfTasteCandidatesUsesVerifiedEvidence(t *testing.T) {
	e := selfTasteEngine(t)
	goal := planning.NewGoal("Explorar Blender", "probar el flujo", planning.PriorityLow, "ahmelia")
	goal.Status = planning.GoalCompleted
	goal.AddNote("modo explore")
	e.planner.RegisterGoal(goal)
	userGoal := planning.NewGoal("Ordenar", "del usuario", planning.PriorityLow, "user")
	userGoal.Status = planning.GoalCompleted
	e.planner.RegisterGoal(userGoal)

	turn := &processTurn{
		interactionHasWork: true,
		interactionWorkContext: interaction.WorkContext{
			ID: "work-1", Provider: "Blender", LastActionVerified: true, AttemptNumber: 2,
		},
	}
	e.sweepSelfTasteCandidates(turn)
	pending := e.selfTaste.Pending()
	if len(pending) != 2 {
		t.Fatalf("esperaba 2 candidatos: %+v", pending)
	}
	keys := map[string]string{}
	for _, c := range pending {
		keys[c.Key] = c.Ref
	}
	if keys["blender"] != "work-1#2" || keys["explorar"] != goal.ID {
		t.Fatalf("candidatos=%+v", pending)
	}

	// Replay del mismo turno: no duplica.
	e.sweepSelfTasteCandidates(turn)
	if got := len(e.selfTaste.Pending()); got != 2 {
		t.Fatalf("dedupe roto: %d", got)
	}

	// Trabajo no verificado: no registra.
	unverified := &processTurn{
		interactionHasWork:     true,
		interactionWorkContext: interaction.WorkContext{ID: "work-2", Provider: "blender"},
	}
	e.sweepSelfTasteCandidates(unverified)
	if got := len(e.selfTaste.Pending()); got != 2 {
		t.Fatalf("lo no verificado no debe registrar: %d", got)
	}
}

func TestSelfTastePhaseConsumesOwnerCommand(t *testing.T) {
	e := selfTasteEngine(t)
	rec, _, err := e.selfTaste.RecordVerified("blender", "work-1")
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	var spoken []string
	e.onResponse = func(msg string) { spoken = append(spoken, msg) }

	turn := &processTurn{
		input: "confirma el gusto " + rec.ID,
		semanticReq: interaction.SemanticRequest{
			Raw:       "confirma el gusto " + rec.ID,
			Objective: interaction.Objective{Text: "confirma el gusto " + rec.ID},
		},
	}
	if !e.selfTastePhase(turn) {
		t.Fatal("la fase debía consumir la orden")
	}
	if len(spoken) == 0 || !strings.Contains(strings.Join(spoken, " "), "Confirmo") {
		t.Fatalf("no se habló la decisión: %v", spoken)
	}

	plain := &processTurn{input: "hola, ¿cómo estás?"}
	if e.selfTastePhase(plain) {
		t.Fatal("una frase corriente no puede ser interceptada")
	}
}

func TestSelfTastePhaseOffByDefault(t *testing.T) {
	e := &Engine{selfTaste: taste.NewModel(taste.NewLedger(""))}
	if e.selfTastePhase(&processTurn{input: "confirma el gusto tg-1"}) {
		t.Fatal("sin habilitar no debe consumir el turno")
	}
}

func TestWorkspaceOnlyCarriesFormedTastes(t *testing.T) {
	e := selfTasteEngine(t)
	seedConfirmedTaste(t, e, "explorar", 5)
	ws := e.buildDeliberationWorkspace(&processTurn{input: "hola"})
	for _, item := range ws.Items {
		if item.Kind == "gusto_propio" {
			t.Fatalf("con 5 confirmaciones no debe entrar al espacio: %+v", item)
		}
	}

	seedConfirmedTaste(t, e, "explorar", 1)
	ws = e.buildDeliberationWorkspace(&processTurn{input: "hola"})
	found := false
	for _, item := range ws.Items {
		if item.Kind == "gusto_propio" && strings.Contains(item.Text, "explorar") {
			found = true
		}
	}
	if !found {
		t.Fatalf("el gusto formado debe entrar al espacio: %+v", ws.Items)
	}
}

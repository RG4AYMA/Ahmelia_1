package core

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ahmelia/agency"
	"ahmelia/autonomy"
	"ahmelia/interaction"
	"ahmelia/planning"
)

func gateGoalProposal(title string) agency.GoalProposal {
	return agency.GoalProposal{
		Title:           title,
		Motive:          "el turno dejó una incógnita",
		NextStep:        "revisar la evidencia local",
		ExpectedMinutes: 15,
		Risk:            agency.RiskLow,
		Mode:            agency.ModeExplore,
		EvidenceIDs:     []string{"w1"},
		Confidence:      0.7,
	}
}

func gateEngineWithProposal(t *testing.T, title string) (*Engine, agency.GoalRecord) {
	t.Helper()
	e := &Engine{
		agentGoalLedger: agency.NewLedger(""),
		agentGoalOn:     true,
		planner:         planning.NewPlanner(),
		autonomousState: autonomy.New(),
	}
	rec, err := e.agentGoalLedger.Propose("t1", agency.Impulse{
		Kind:     agency.ImpulseKnowledgeGap,
		Reason:   "el turno dejó una incógnita",
		Strength: 0.7,
		Evidence: []string{"clarification:¿qué versión?"},
	}, gateGoalProposal(title))
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	return e, rec
}

func TestParseAgentGoalCommandExplicitForms(t *testing.T) {
	cases := []struct {
		raw  string
		kind agentGoalCommandKind
		id   string
	}{
		{"autoriza la meta ag-123", agentGoalAuthorize, "ag-123"},
		{"Aprueba la meta ag-1793566854741221700, por favor", agentGoalAuthorize, "ag-1793566854741221700"},
		{"rechaza la meta ag-42-1 porque no es el momento", agentGoalReject, "ag-42-1"},
		{"¿qué metas propias tienes?", agentGoalList, ""},
		{"lista tus metas propias", agentGoalList, ""},
	}
	for _, tc := range cases {
		cmd := parseAgentGoalCommand(tc.raw)
		if cmd.Kind != tc.kind || cmd.ID != tc.id {
			t.Fatalf("%q -> %+v, esperaba %s/%s", tc.raw, cmd, tc.kind, tc.id)
		}
		if cmd.Problem != "" {
			t.Fatalf("%q no debía fallar: %s", tc.raw, cmd.Problem)
		}
	}
}

func TestParseAgentGoalCommandFailsClosed(t *testing.T) {
	cases := []struct {
		raw     string
		problem string
	}{
		{"aprueba el plan de trabajo", ""}, // no es una meta propia: no se intercepta
		{"autoriza la meta", "ID exacto"},
		{"autoriza la meta ag-1 y ag-2", "más de un identificador"},
		{"autoriza y rechaza la meta ag-1", "autorización y un rechazo"},
		{"no autorices la meta ag-1", "negación"},
	}
	for _, tc := range cases {
		cmd := parseAgentGoalCommand(tc.raw)
		if tc.problem == "" {
			if cmd.Kind != agentGoalNone {
				t.Fatalf("%q no debía interceptarse: %+v", tc.raw, cmd)
			}
			continue
		}
		if cmd.Kind == agentGoalNone || !strings.Contains(strings.ToLower(cmd.Problem), strings.ToLower(tc.problem)) {
			t.Fatalf("%q -> %+v, esperaba problema con %q", tc.raw, cmd, tc.problem)
		}
	}
}

func TestApplyAgentGoalCommandListsAndAuthorizes(t *testing.T) {
	e, rec := gateEngineWithProposal(t, "Aprender a verificar Blender")

	list, handled := e.applyAgentGoalCommand(parseAgentGoalCommand("¿qué metas propias tienes?"))
	if !handled {
		t.Fatal("la consulta debía manejarse")
	}
	if !strings.Contains(list, rec.ID) || !strings.Contains(list, "Aprender a verificar Blender") {
		t.Fatalf("lista incompleta: %s", list)
	}

	text, handled := e.applyAgentGoalCommand(parseAgentGoalCommand("autoriza la meta " + rec.ID))
	if !handled {
		t.Fatal("la autorización debía manejarse")
	}
	if !strings.Contains(text, "planificador") || !strings.Contains(text, rec.ID) {
		t.Fatalf("respuesta=%s", text)
	}
	got, ok := e.AgentGoalRecord(rec.ID)
	if !ok || got.Status != agency.GoalRegistered {
		t.Fatalf("registro=%+v ok=%v", got, ok)
	}
	goals := e.planner.Goals()
	if len(goals) != 1 {
		t.Fatalf("esperaba 1 goal en el planificador: %+v", goals)
	}
	if goals[0].Owner != "ahmelia" || goals[0].Name != "Aprender a verificar Blender" || goals[0].Status != planning.GoalActive {
		t.Fatalf("goal=%+v", goals[0])
	}
	if got.PlannerGoalID != goals[0].ID {
		t.Fatalf("el libro no está vinculado: %s != %s", got.PlannerGoalID, goals[0].ID)
	}
	if current := e.autonomousState.CurrentGoal(); current != nil {
		t.Fatalf("registrar no debe activar autonomía: %+v", current)
	}
	if pending := e.autonomousState.PendingGoals(); len(pending) != 0 {
		t.Fatalf("registrar no debe encolar autonomía: %+v", pending)
	}
}

func TestApplyAgentGoalCommandAuthorizeRespectsPlannerQuota(t *testing.T) {
	e, first := gateEngineWithProposal(t, "Primera meta")
	text, _ := e.applyAgentGoalCommand(parseAgentGoalCommand("autoriza la meta " + first.ID))
	if !strings.Contains(text, "planificador") {
		t.Fatalf("respuesta=%s", text)
	}

	second, err := e.agentGoalLedger.Propose("t2", agency.Impulse{
		Kind:     agency.ImpulseUnfinishedWork,
		Reason:   "quedó trabajo sin cerrar",
		Strength: 0.8,
		Evidence: []string{"work:pending"},
	}, gateGoalProposal("Segunda meta"))
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	text, _ = e.applyAgentGoalCommand(parseAgentGoalCommand("autoriza la meta " + second.ID))
	if !strings.Contains(text, "No la registré") || !strings.Contains(text, "cupo") {
		t.Fatalf("respuesta=%s", text)
	}
	got, _ := e.AgentGoalRecord(second.ID)
	if got.Status != agency.GoalAuthorized {
		t.Fatalf("la segunda meta debe quedar autorizada sin registrar: %s", got.Status)
	}
	if goals := e.planner.Goals(); len(goals) != 1 {
		t.Fatalf("el cupo debe mantener un solo goal: %+v", goals)
	}
}

func TestApplyAgentGoalCommandBlockedGoalStillOccupiesQuota(t *testing.T) {
	e, first := gateEngineWithProposal(t, "Primera meta")
	if text, _ := e.applyAgentGoalCommand(parseAgentGoalCommand("autoriza la meta " + first.ID)); !strings.Contains(text, "planificador") {
		t.Fatalf("respuesta=%s", text)
	}
	goals := e.planner.Goals()
	if len(goals) != 1 {
		t.Fatalf("esperaba 1 goal: %+v", goals)
	}
	goals[0].SetBlocked("esperando decisión del propietario")

	second, err := e.agentGoalLedger.Propose("t2", agency.Impulse{
		Kind:     agency.ImpulseUnfinishedWork,
		Reason:   "quedó trabajo sin cerrar",
		Strength: 0.8,
		Evidence: []string{"work:pending"},
	}, gateGoalProposal("Segunda meta"))
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	text, _ := e.applyAgentGoalCommand(parseAgentGoalCommand("autoriza la meta " + second.ID))
	if !strings.Contains(text, "No la registré") || !strings.Contains(text, "cupo") {
		t.Fatalf("respuesta=%s", text)
	}
	if got, _ := e.AgentGoalRecord(second.ID); got.Status != agency.GoalAuthorized {
		t.Fatalf("la segunda meta debe quedar autorizada sin registrar: %s", got.Status)
	}
}

func TestApplyAgentGoalCommandRejectsAndReportsUnknown(t *testing.T) {
	e, rec := gateEngineWithProposal(t, "Ordenar la biblioteca")

	text, _ := e.applyAgentGoalCommand(parseAgentGoalCommand("rechaza la meta " + rec.ID))
	if !strings.Contains(text, "Rechacé") {
		t.Fatalf("respuesta=%s", text)
	}
	got, _ := e.AgentGoalRecord(rec.ID)
	if got.Status != agency.GoalRejected {
		t.Fatalf("estado=%s", got.Status)
	}

	text, _ = e.applyAgentGoalCommand(parseAgentGoalCommand("autoriza la meta ag-999"))
	if !strings.Contains(text, "No encuentro") {
		t.Fatalf("respuesta=%s", text)
	}
}

func TestAgentGoalGateHidesExpiredProposals(t *testing.T) {
	e, rec := gateEngineWithProposal(t, "Una meta que vence")
	now := time.Now()
	e.agentGoalLedger.SetClock(func() time.Time { return now.Add(73 * time.Hour) })

	list, _ := e.applyAgentGoalCommand(parseAgentGoalCommand("¿qué metas propias tienes?"))
	if strings.Contains(list, rec.ID) {
		t.Fatalf("la lista no debe mostrar propuestas vencidas: %s", list)
	}
	if !strings.Contains(list, "ninguna") {
		t.Fatalf("lista=%s", list)
	}

	text, _ := e.applyAgentGoalCommand(parseAgentGoalCommand("autoriza la meta " + rec.ID))
	if !strings.Contains(text, string(agency.GoalExpired)) {
		t.Fatalf("la decisión sobre una propuesta vencida debe informar el estado: %s", text)
	}
}

func TestAgentGoalPhaseConsumesOwnerCommand(t *testing.T) {
	e, rec := gateEngineWithProposal(t, "Aprender a verificar Blender")
	var spoken []string
	e.onResponse = func(msg string) { spoken = append(spoken, msg) }

	turn := &processTurn{
		input: "autoriza la meta " + rec.ID,
		semanticReq: interaction.SemanticRequest{
			Raw:       "autoriza la meta " + rec.ID,
			Objective: interaction.Objective{Text: "autoriza la meta " + rec.ID},
		},
	}
	if !e.agentGoalPhase(turn) {
		t.Fatal("la fase debía consumir la orden")
	}
	if len(spoken) == 0 || !strings.Contains(strings.Join(spoken, " "), "planificador") {
		t.Fatalf("no se habló la decisión: %v", spoken)
	}
	got, _ := e.AgentGoalRecord(rec.ID)
	if got.Status != agency.GoalRegistered {
		t.Fatalf("estado=%s", got.Status)
	}

	plain := &processTurn{input: "hola, ¿cómo estás?"}
	if e.agentGoalPhase(plain) {
		t.Fatal("una frase corriente no puede ser interceptada")
	}
}

func TestAgentGoalGateRefusesAuthorizationInRest(t *testing.T) {
	e, rec := gateEngineWithProposal(t, "Aprender a verificar Blender")
	e.enablePersistentModeAt(filepath.Join(t.TempDir(), "persistent_mode.json"))
	if _, err := e.SetPersistentMode(agency.ModeRest, "owner"); err != nil {
		t.Fatalf("set rest: %v", err)
	}
	text, handled := e.applyAgentGoalCommand(agentGoalCommand{Kind: agentGoalAuthorize, ID: rec.ID})
	if !handled || !strings.Contains(text, "descanso") {
		t.Fatalf("respuesta=%q handled=%t", text, handled)
	}
	if got, _ := e.AgentGoalRecord(rec.ID); got.Status != agency.GoalProposed {
		t.Fatalf("descanso no debe autorizar: %s", got.Status)
	}
	if goals := e.planner.Goals(); len(goals) != 0 {
		t.Fatalf("descanso no debe registrar: %+v", goals)
	}
}

func TestAgentGoalPhaseDoesNotConsumeWithoutLedger(t *testing.T) {
	e := &Engine{agentGoalOn: true}
	var spoken []string
	e.onResponse = func(msg string) { spoken = append(spoken, msg) }
	turn := &processTurn{input: "autoriza la meta ag-1"}
	if e.agentGoalPhase(turn) {
		t.Fatal("sin libro la fase no debe consumir el turno")
	}
	if len(spoken) != 0 {
		t.Fatalf("no debía hablar: %v", spoken)
	}
}

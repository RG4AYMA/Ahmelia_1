package deliberation

import (
	"strings"
	"testing"

	"ahmelia/agency"
)

func testWorkspace() *Workspace {
	ws := &Workspace{}
	ws.Add("turno_actual", "abre eso", "engine", 1)
	ws.Add("situacion", "el proyecto de Unity sigue abierto", "situation", 0.9)
	return ws
}

func TestParseProposalBindsEvidence(t *testing.T) {
	ws := testWorkspace()
	p, err := ParseProposal(`{"reading":"El usuario retoma el proyecto de Unity","strategy":"continue","critique":"","expectation":"El proyecto sigue abierto","evidence_ids":["w1","w2","w9","w1"],"confidence":1.4,"abstain":false}`)
	if err != nil {
		t.Fatalf("ParseProposal devolvió error: %v", err)
	}
	p.Bind(ws)
	if p.Strategy != StrategyContinue {
		t.Fatalf("estrategia inesperada: %q", p.Strategy)
	}
	if p.Confidence != 1 {
		t.Fatalf("la confianza debe acotarse a 1: %v", p.Confidence)
	}
	if len(p.EvidenceIDs) != 2 || p.EvidenceIDs[0] != "w1" || p.EvidenceIDs[1] != "w2" {
		t.Fatalf("evidencia válida mal filtrada: %+v", p.EvidenceIDs)
	}
	if len(p.RejectedEvidenceIDs) != 1 || p.RejectedEvidenceIDs[0] != "w9" {
		t.Fatalf("la cita inventada debe registrarse como rechazada: %+v", p.RejectedEvidenceIDs)
	}
	if !p.Grounded {
		t.Fatal("una propuesta con evidencia válida debe quedar fundada")
	}
}

func TestParseProposalWithoutValidEvidenceIsUngrounded(t *testing.T) {
	ws := testWorkspace()
	p, err := ParseProposal(`{"reading":"Algo pasa","strategy":"answer","critique":"","expectation":"","evidence_ids":["w99"],"confidence":0.7,"abstain":false}`)
	if err != nil {
		t.Fatalf("ParseProposal devolvió error: %v", err)
	}
	p.Bind(ws)
	if p.Grounded {
		t.Fatal("una cita inexistente no puede fundar la propuesta")
	}
	if len(p.RejectedEvidenceIDs) != 1 {
		t.Fatalf("la cita rechazada debe quedar registrada: %+v", p.RejectedEvidenceIDs)
	}
}

func TestParseProposalUnknownStrategyStaysUnknown(t *testing.T) {
	p, err := ParseProposal(`{"reading":"x","strategy":"responder_con_entusiasmo","critique":"","expectation":"","evidence_ids":["w1"],"confidence":0.5,"abstain":false}`)
	if err != nil {
		t.Fatalf("ParseProposal devolvió error: %v", err)
	}
	p.Bind(testWorkspace())
	if p.Strategy != StrategyUnknown {
		t.Fatalf("una estrategia fuera del vocabulario no puede adivinarse: %q", p.Strategy)
	}
}

func TestParseProposalModeSuggestionIsValidated(t *testing.T) {
	p, err := ParseProposal(`{"reading":"x","strategy":"answer","critique":"","expectation":"","evidence_ids":["w1"],"confidence":0.5,"abstain":false,"mode_suggestion":"focus"}`)
	if err != nil {
		t.Fatalf("ParseProposal devolvió error: %v", err)
	}
	if p.ModeSuggestion != agency.ModeFocus {
		t.Fatalf("sugerencia = %q, quería focus", p.ModeSuggestion)
	}

	q, err := ParseProposal(`{"reading":"x","strategy":"answer","critique":"","expectation":"","evidence_ids":["w1"],"confidence":0.5,"abstain":false,"mode_suggestion":"turbo"}`)
	if err != nil {
		t.Fatalf("ParseProposal devolvió error: %v", err)
	}
	if q.ModeSuggestion != agency.ModeUnknown || q.ModeSuggestionRejected == "" {
		t.Fatalf("una sugerencia inválida debe registrarse como rechazada: %+v", q)
	}
}

func TestParseProposalToleratesFencesAndRejectsEmpty(t *testing.T) {
	p, err := ParseProposal("```json\n{\"reading\":\"x\",\"strategy\":\"wait\",\"evidence_ids\":[\"w1\"],\"abstain\":false}\n```")
	if err != nil {
		t.Fatalf("ParseProposal debe tolerar cercas: %v", err)
	}
	if p.Strategy != StrategyWait {
		t.Fatalf("estrategia inesperada: %q", p.Strategy)
	}
	if _, err := ParseProposal("   "); err == nil {
		t.Fatal("una respuesta vacía debe ser error")
	}
	if _, err := ParseProposal(`{"reading":"","strategy":"unknown","critique":"","expectation":"","evidence_ids":[],"confidence":0,"abstain":false}`); err == nil {
		t.Fatal("una propuesta sin contenido debe ser error")
	}
}

func TestProposalBindWithoutWorkspaceNeverGrounds(t *testing.T) {
	p := Proposal{EvidenceIDs: []string{"w1"}}
	p.Bind(nil)
	if p.Grounded || len(p.EvidenceIDs) != 0 || len(p.RejectedEvidenceIDs) != 1 {
		t.Fatalf("sin workspace no puede haber fundamento: %+v", p)
	}
}

func TestBuildPromptContainsWorkspaceDecisionAndProhibition(t *testing.T) {
	ws := testWorkspace()
	prompt := BuildPrompt(ws, HarnessDecision{Action: "respond", Strategy: StrategyAnswer, Intent: "answer", Speak: true})
	for _, frag := range []string{"ESPACIO DE TRABAJO", "[w1]", "[w2]", "DECISIÓN YA TOMADA", "strategy: answer", "No ejecutas acciones", "evidence_ids", "self_goal", "impulso"} {
		if !strings.Contains(prompt, frag) {
			t.Fatalf("el prompt pierde %q", frag)
		}
	}
}

func TestParseProposalSelfGoalBindsAgainstWorkspace(t *testing.T) {
	ws := testWorkspace()
	raw := `{"reading":"Hay una incógnita","strategy":"answer","critique":"","expectation":"","evidence_ids":["w1"],"confidence":0.8,"abstain":false,` +
		`"self_goal":{"title":"Aprender a verificar el estado de Unity","motive":"El usuario preguntó dos veces y no hay evidencia","next_step":"Revisar la documentación local","expected_minutes":20,"risk":"low","mode":"explore","evidence_ids":["w1","w9"],"confidence":0.7}}`
	p, err := ParseProposal(raw)
	if err != nil {
		t.Fatalf("ParseProposal: %v", err)
	}
	p.Bind(ws)
	if p.SelfGoal == nil {
		t.Fatalf("la meta propia válida se perdió: %q", p.SelfGoalRejected)
	}
	if len(p.SelfGoal.EvidenceIDs) != 1 || p.SelfGoal.EvidenceIDs[0] != "w1" {
		t.Fatalf("evidencia de meta=%v", p.SelfGoal.EvidenceIDs)
	}
	if len(p.SelfGoal.RejectedEvidenceIDs) != 1 || p.SelfGoal.RejectedEvidenceIDs[0] != "w9" {
		t.Fatalf("rechazadas de meta=%v", p.SelfGoal.RejectedEvidenceIDs)
	}
}

func TestParseProposalInvalidSelfGoalKeepsCritique(t *testing.T) {
	raw := `{"reading":"Hay una incógnita","strategy":"answer","critique":"la decisión ignora el costo","expectation":"","evidence_ids":["w1"],"confidence":0.8,"abstain":false,` +
		`"self_goal":{"title":"Algo","motive":"motivo","next_step":"paso","expected_minutes":10,"risk":"extreme","mode":"explore","evidence_ids":["w1"],"confidence":0.5}}`
	p, err := ParseProposal(raw)
	if err != nil {
		t.Fatalf("ParseProposal: %v", err)
	}
	if p.SelfGoal != nil {
		t.Fatalf("una meta con riesgo inválido no puede sobrevivir: %+v", p.SelfGoal)
	}
	if p.SelfGoalRejected == "" {
		t.Fatal("el rechazo de la meta debe quedar visible")
	}
	if p.Critique == "" {
		t.Fatal("la crítica debe sobrevivir al rechazo de la meta")
	}
}

func TestParseProposalSelfGoalWithoutValidEvidenceIsDropped(t *testing.T) {
	ws := testWorkspace()
	raw := `{"reading":"Hay una incógnita","strategy":"answer","critique":"","expectation":"","evidence_ids":["w1"],"confidence":0.8,"abstain":false,` +
		`"self_goal":{"title":"Aprender algo","motive":"motivo","next_step":"paso","expected_minutes":10,"risk":"low","mode":"focus","evidence_ids":["w99"],"confidence":0.5}}`
	p, err := ParseProposal(raw)
	if err != nil {
		t.Fatalf("ParseProposal: %v", err)
	}
	p.Bind(ws)
	if p.SelfGoal != nil {
		t.Fatalf("sin cita válida la meta no puede quedar fundada: %+v", p.SelfGoal)
	}
	if p.SelfGoalRejected == "" {
		t.Fatal("el rechazo debe quedar registrado")
	}
}

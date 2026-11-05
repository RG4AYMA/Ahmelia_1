package agency

import (
	"strings"
	"testing"
)

func validGoal() GoalProposal {
	return GoalProposal{
		Title:           "Aprender a verificar el estado de Blender",
		Motive:          "El usuario preguntó dos veces y el arnés no tenía evidencia.",
		NextStep:        "Revisar la documentación local y registrar una prueba.",
		ExpectedMinutes: 20,
		Risk:            RiskLow,
		Mode:            ModeExplore,
		EvidenceIDs:     []string{"w1"},
		Confidence:      0.7,
	}
}

func TestGoalProposalValidateAcceptsBoundedGoal(t *testing.T) {
	g := validGoal()
	if err := g.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if g.Risk != RiskLow || g.Mode != ModeExplore {
		t.Fatalf("normalización inesperada: %+v", g)
	}
}

func TestGoalProposalNormalizesTextAndEnums(t *testing.T) {
	g := validGoal()
	g.Title = "  Aprender   algo  "
	g.Risk = " HIGH "
	g.Mode = "Explore"
	g.Confidence = 1.7
	g.ExpectedMinutes = 999
	if err := g.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if g.Title != "Aprender   algo" {
		t.Fatalf("title=%q", g.Title)
	}
	if g.Risk != RiskHigh || g.Mode != ModeExplore {
		t.Fatalf("enums=%q/%q", g.Risk, g.Mode)
	}
	if g.Confidence != 1 || g.ExpectedMinutes != MaxExpectedMinutes {
		t.Fatalf("clamp conf=%v min=%d", g.Confidence, g.ExpectedMinutes)
	}
}

func TestModeVocabulary(t *testing.T) {
	for _, mode := range []Mode{ModeFocus, ModeExplore, ModeRest, ModeCompanion} {
		if !mode.Valid() {
			t.Fatalf("%q debía ser válido", mode)
		}
		if got := NormalizeMode("  " + strings.ToUpper(string(mode)) + " "); got != mode {
			t.Fatalf("normalize %q -> %q", mode, got)
		}
	}
	for _, raw := range []string{"", "turbo", "mode-focus"} {
		if got := NormalizeMode(raw); got != ModeUnknown {
			t.Fatalf("%q -> %q, quería desconocido", raw, got)
		}
	}
}

func TestGoalProposalValidateRejectsBrokenContracts(t *testing.T) {
	cases := []struct {
		name string
		mut  func(*GoalProposal)
		want string
	}{
		{"empty title", func(g *GoalProposal) { g.Title = "" }, "título"},
		{"long title", func(g *GoalProposal) { g.Title = strings.Repeat("x", MaxGoalTitleRunes+1) }, "título"},
		{"empty motive", func(g *GoalProposal) { g.Motive = "" }, "motivo"},
		{"empty next", func(g *GoalProposal) { g.NextStep = "" }, "siguiente paso"},
		{"zero minutes", func(g *GoalProposal) { g.ExpectedMinutes = 0 }, "costo"},
		{"bad risk", func(g *GoalProposal) { g.Risk = "extreme" }, "riesgo"},
		{"bad mode", func(g *GoalProposal) { g.Mode = "party" }, "modo"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := validGoal()
			tc.mut(&g)
			err := g.Validate()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err=%v, esperaba mención de %q", err, tc.want)
			}
		})
	}
}

func TestGoalProposalBindRequiresValidEvidence(t *testing.T) {
	g := validGoal()
	g.EvidenceIDs = []string{"w1", "w9", "w1", ""}
	ok := g.Bind(func(id string) bool { return id == "w1" })
	if !ok {
		t.Fatal("esperaba fundamento válido")
	}
	if len(g.EvidenceIDs) != 1 || g.EvidenceIDs[0] != "w1" {
		t.Fatalf("evidencia=%v", g.EvidenceIDs)
	}
	if len(g.RejectedEvidenceIDs) != 1 || g.RejectedEvidenceIDs[0] != "w9" {
		t.Fatalf("rechazadas=%v", g.RejectedEvidenceIDs)
	}

	g = validGoal()
	g.EvidenceIDs = []string{"w9"}
	if g.Bind(func(string) bool { return false }) {
		t.Fatal("sin cita válida no debe quedar fundada")
	}
}

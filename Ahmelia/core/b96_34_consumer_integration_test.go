package core

import (
	"strings"
	"testing"
	"time"

	"ahmelia/interaction"
)

func seedB9634Model(t *testing.T, e *Engine, mutate func(*interaction.UserModel)) {
	t.Helper()
	view, err := e.userModelStore.Read(interaction.UserModelAccessRequest{Actor: interaction.UserModelActorSystem, Mode: interaction.UserModelAccessRead})
	if err != nil {
		t.Fatal(err)
	}
	model := view.Model
	mutate(&model)
	model.UpdatedAt = time.Now()
	if err := e.userModelStore.Replace(interaction.UserModelAccessRequest{Actor: interaction.UserModelActorSystem, Mode: interaction.UserModelAccessModify}, model); err != nil {
		t.Fatal(err)
	}
}

func TestB9634InitiativeUsesCanonicalUserModelCompatibilityView(t *testing.T) {
	e := newB963334AuditEngine(t)
	legacy := interaction.NewUserProfile()
	legacy.Remember("preference", "favorite_sweet", "chocolate", "legacy", 0.99)
	e.userProfile = legacy

	seedB9634Model(t, e, func(m *interaction.UserModel) {
		m.Preferences = []interaction.UserPreference{{Key: "color favorito", Value: "azul", Confidence: 1, Status: interaction.UserModelStatusActive, Provenance: interaction.UserModelSourceRef{Kind: interaction.UserSourceExplicitUser, SourceID: "test"}, CreatedAt: time.Now(), UpdatedAt: time.Now()}}
	})

	authority := legacy.PrimaryAuthority
	projected, err := interaction.UserProfileCompatibilityView(e.userModelStore, authority)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := projected.Preferences["favorite_sweet"]; ok {
		t.Fatal("legacy-only preference leaked into compatibility view")
	}
	if got := projected.Preferences["color favorito"].Value; got != "azul" {
		t.Fatalf("compatibility view used wrong source: %q", got)
	}

	req := interaction.SemanticRequest{Kind: interaction.KindConversation, Raw: "quiero conversar"}
	ctx := interaction.ConversationSnapshot{}
	canonicalDecision := interaction.DefaultInitiativePolicy().EvaluateInitiative(req, ctx, projected)
	if canonicalDecision.ShouldInitiate {
		t.Fatalf("canonical UserModel should not invent a proactive personal question from legacy-only data: %+v", canonicalDecision)
	}
	legacyDecision := interaction.DefaultInitiativePolicy().EvaluateInitiative(req, ctx, legacy)
	if legacyDecision.ShouldInitiate {
		t.Fatalf("legacy compatibility data must not be treated as a current-belief source by the canonical initiative path: %+v", legacyDecision)
	}
}

func newConsumerEngine(t *testing.T) *Engine {
	t.Helper()
	e := newB963334AuditEngine(t)
	e.SetResponseCallback(func(string) {})
	return e
}

func TestB9634GoalConsumerChangesEngineBehavior(t *testing.T) {
	e := newConsumerEngine(t)
	now := time.Now()
	seedB9634Model(t, e, func(m *interaction.UserModel) {
		m.Goals = []interaction.UserGoal{{Key: "aprender Blender", Value: "long_term", Confidence: 1, Status: interaction.UserModelStatusActive, Provenance: interaction.UserModelSourceRef{Kind: interaction.UserSourceExplicitUser, SourceID: "test"}, CreatedAt: now, UpdatedAt: now}}
	})
	var got string
	e.SetResponseCallback(func(s string) { got = s })
	e.Process("¿Cuál es mi objetivo con Blender?")
	if !strings.Contains(strings.ToLower(got), "aprender blender") {
		t.Fatalf("goal query did not consume B96.34 goal model: %q", got)
	}
}

func TestB9634KnowledgeConsumerChangesEngineBehavior(t *testing.T) {
	e := newConsumerEngine(t)
	now := time.Now()
	seedB9634Model(t, e, func(m *interaction.UserModel) {
		m.Knowledge = []interaction.UserKnowledgeItem{{Key: "Blender", Value: "advanced", Confidence: 1, Status: interaction.UserModelStatusActive, Provenance: interaction.UserModelSourceRef{Kind: interaction.UserSourceExplicitUser, SourceID: "test"}, CreatedAt: now, UpdatedAt: now}}
	})
	var got string
	e.SetResponseCallback(func(s string) { got = s })
	e.Process("¿Qué sé sobre Blender?")
	if !strings.Contains(strings.ToLower(got), "advanced") {
		t.Fatalf("knowledge query did not consume B96.34 knowledge model: %q", got)
	}
}

func TestB9634CapabilityConsumerChangesEngineBehavior(t *testing.T) {
	e := newConsumerEngine(t)
	now := time.Now()
	seedB9634Model(t, e, func(m *interaction.UserModel) {
		m.Capabilities = []interaction.UserCapability{{Key: "Blender", Value: "expert", Confidence: 1, Status: interaction.UserModelStatusActive, Provenance: interaction.UserModelSourceRef{Kind: interaction.UserSourceExplicitUser, SourceID: "test"}, CreatedAt: now, UpdatedAt: now}}
	})
	var got string
	e.SetResponseCallback(func(s string) { got = s })
	e.Process("¿Qué nivel tengo en Blender?")
	if !strings.Contains(strings.ToLower(got), "expert") {
		t.Fatalf("capability query did not consume B96.34 capability model: %q", got)
	}
}

func TestB9634CommunicationConsumerChangesEngineBehavior(t *testing.T) {
	e := newConsumerEngine(t)
	now := time.Now()
	seedB9634Model(t, e, func(m *interaction.UserModel) {
		m.InteractionProfile.Values = map[string]interaction.UserInteractionProfileValue{
			string(interaction.CommunicationDetailTolerance): {Value: "brief", Confidence: 1, Status: interaction.UserModelStatusActive, Provenance: interaction.UserModelSourceRef{Kind: interaction.UserSourceExplicitUser, SourceID: "test"}, UpdatedAt: now},
		}
	})
	e.Process("hola")
	if e.contextAdapter.PreferredLength != "brief" {
		t.Fatalf("communication profile did not affect response behavior: %q", e.contextAdapter.PreferredLength)
	}
}

func TestB9634MultiTurnPreferenceConflictFlowsThroughEngine(t *testing.T) {
	e := newConsumerEngine(t)
	e.Process("Mi color favorito es azul")
	e.Process("Mi color favorito es rojo")
	view, err := e.userModelStore.Read(interaction.UserModelAccessRequest{Actor: interaction.UserModelActorCognition, Mode: interaction.UserModelAccessRead})
	if err != nil {
		t.Fatal(err)
	}
	var live []interaction.UserPreference
	for _, pref := range view.Model.Preferences {
		if pref.Key == "color favorito" && (pref.Status == interaction.UserModelStatusActive || pref.Status == interaction.UserModelStatusUncertain) {
			live = append(live, pref)
		}
	}
	if len(live) != 2 {
		t.Fatalf("expected B96.34 conflict resolution to preserve both live competing beliefs, got %d: %+v", len(live), live)
	}
	for _, pref := range live {
		if pref.Status != interaction.UserModelStatusUncertain {
			t.Fatalf("expected unresolved contradiction to be uncertain, got %+v", pref)
		}
	}
}

func TestB9634UserObservationCapabilityE2E(t *testing.T) {
	e := newConsumerEngine(t)
	e.Process("Mi nivel de Blender es experto")
	view, err := e.userModelStore.Read(interaction.UserModelAccessRequest{Actor: interaction.UserModelActorCognition, Mode: interaction.UserModelAccessRead})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range view.Model.Capabilities {
		if strings.EqualFold(c.Key, "Blender") && c.Value == "expert" && c.Status == interaction.UserModelStatusActive {
			found = true
		}
	}
	if !found {
		t.Fatalf("explicit capability did not traverse Engine -> UserObservation -> B96.34 -> UserModelStore: %+v", view.Model.Capabilities)
	}
	var got string
	e.SetResponseCallback(func(s string) { got = s })
	e.Process("¿Qué nivel tengo en Blender?")
	if !strings.Contains(strings.ToLower(got), "expert") {
		t.Fatalf("capability was persisted but not consumed by query: %q", got)
	}
}

func TestB9634UserObservationGoalKnowledgeCommunicationAttributeE2E(t *testing.T) {
	e := newConsumerEngine(t)
	cases := []string{
		"Mi objetivo es aprender Blender",
		"Mi nivel de conocimiento de Blender es avanzado",
		"Prefiero que me hables de forma directa",
		"Mi ciudad es Barranquilla",
	}
	for _, input := range cases {
		e.Process(input)
	}
	view, err := e.userModelStore.Read(interaction.UserModelAccessRequest{Actor: interaction.UserModelActorCognition, Mode: interaction.UserModelAccessRead})
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Model.Goals) == 0 {
		t.Fatalf("goal observation was not persisted: %+v", view.Model.Goals)
	}
	if len(view.Model.Knowledge) == 0 {
		t.Fatalf("knowledge observation was not persisted: %+v", view.Model.Knowledge)
	}
	if len(view.Model.Attributes) == 0 {
		t.Fatalf("attribute observation was not persisted: %+v", view.Model.Attributes)
	}
	if v := view.Model.InteractionProfile.Values[string(interaction.CommunicationDirectness)]; v.Value == "" {
		t.Fatalf("communication observation was not persisted: %+v", view.Model.InteractionProfile.Values)
	}
}

func TestB9634GenericAttributeRoundTripUsesSameSemanticKey(t *testing.T) {
	e := newConsumerEngine(t)
	var responses []string
	e.SetResponseCallback(func(s string) { responses = append(responses, s) })

	e.Process("Mi nombre es Adrian Yesid Marin Angulo")
	e.Process("¿Cuál es mi nombre?")

	if len(responses) < 2 {
		t.Fatalf("expected declaration and query responses, got %v", responses)
	}
	if !strings.Contains(strings.ToLower(responses[len(responses)-1]), "adrian yesid marin angulo") {
		t.Fatalf("generic name attribute was not recovered: %q", responses[len(responses)-1])
	}
}

func TestB9634GenericUnseenAttributesDoNotRequireEngineCases(t *testing.T) {
	cases := []struct {
		declaration string
		query       string
		want        string
	}{
		{"Mi ciudad es Barranquilla", "¿Cuál es mi ciudad?", "barranquilla"},
		{"Mi profesión es desarrollador", "¿Cuál es mi profesión?", "desarrollador"},
		{"Mi edad es 17", "¿Cuál es mi edad?", "17"},
	}
	for _, tc := range cases {
		t.Run(tc.query, func(t *testing.T) {
			e := newConsumerEngine(t)
			var responses []string
			e.SetResponseCallback(func(s string) { responses = append(responses, s) })
			e.Process(tc.declaration)
			e.Process(tc.query)
			if len(responses) < 2 || !strings.Contains(strings.ToLower(responses[len(responses)-1]), strings.ToLower(tc.want)) {
				t.Fatalf("generic attribute round trip failed: declaration=%q query=%q responses=%v", tc.declaration, tc.query, responses)
			}
		})
	}
}

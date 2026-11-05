package interaction

import (
	"strings"
	"testing"
)

func TestParseConvergesVoiceAndTextToSameSemanticBoundary(t *testing.T) {
	voice := Parse("Amelia, crea una imagen y guárdala", ChannelVoice)
	text := Parse("crea una imagen y guárdala", ChannelText)
	if voice.Kind != KindProduction || text.Kind != KindProduction {
		t.Fatalf("kinds: voice=%q text=%q", voice.Kind, text.Kind)
	}
	if voice.Objective.Text == "" || text.Objective.Text == "" {
		t.Fatal("objective text missing")
	}
}

func TestParseFollowUpIsDomainNeutral(t *testing.T) {
	for _, q := range []string{"¿Dónde está lo que acabas de crear?", "¿Cómo vas?", "¿Qué falta?", "Continúa con lo que estabas haciendo"} {
		if !Parse(q, ChannelText).FollowUp {
			t.Fatalf("expected follow-up for %q", q)
		}
	}
}

func TestReasoningDoesNotBecomeSearch(t *testing.T) {
	r := Parse("2 + 2 = ?", ChannelText)
	if r.Kind != KindReasoning {
		t.Fatalf("kind=%q", r.Kind)
	}
}

func TestReferenceAndLanguageContext(t *testing.T) {
	m := NewManager()
	r := m.Analyze("¿Dónde está lo que acabas de crear?", Context{
		Language: "es", LastArtifactName: "hello.exe", LastArtifactPath: `C:\out\hello.exe`,
	})
	if len(r.References) == 0 {
		t.Fatal("expected reference")
	}
	if r.Language != "es" {
		t.Fatalf("language=%q", r.Language)
	}
}

func TestLanguageDoesNotFlipOnProperNoun(t *testing.T) {
	m := NewManager()
	r := m.Analyze("Abre Unity", Context{Language: "es"})
	if r.Language != "es" {
		t.Fatalf("language=%q", r.Language)
	}
}

func TestLanguageCanChangeWithEvidence(t *testing.T) {
	m := NewManager()
	r := m.Analyze("Where is the project you created?", Context{Language: "es"})
	if r.Language != "en" {
		t.Fatalf("language=%q", r.Language)
	}
}

func TestContinuationIsConversationContext(t *testing.T) {
	m := NewManager()
	r := m.Analyze("continúa con eso", Context{PreviousKind: KindAction})
	if !r.Continues || r.Kind != KindContinuation {
		t.Fatalf("request=%+v", r)
	}
}

func TestAnalyzeResolvesCurrentWorkWithoutDomainKnowledge(t *testing.T) {
	m := NewManager()
	r := m.Analyze("Continúa con lo que estabas haciendo.", Context{
		PreviousInput:   "crea el proyecto",
		PreviousKind:    KindProduction,
		ActiveObjective: "crear un proyecto",
		ActiveWorkID:    "work_123",
		Language:        "es",
	})
	if r.Kind != KindContinuation {
		t.Fatalf("kind=%q", r.Kind)
	}
	if !r.Continues || !r.AddressesCurrent {
		t.Fatalf("continuity=%v current=%v", r.Continues, r.AddressesCurrent)
	}
	found := false
	for _, ref := range r.References {
		if ref.Kind == ReferenceRecentWork && ref.Resolved && ref.Value == "work_123" {
			found = true
		}
	}
	if !found {
		t.Fatalf("current work reference not resolved: %+v", r.References)
	}
}

func TestAnalyzeEnglishFollowUpUsesSameSemanticBoundary(t *testing.T) {
	r := Analyze("Where is the project you created?", Context{
		Language:         "es",
		LastArtifactPath: `C:\out\project`,
	})
	if r.Kind != KindQuestion {
		t.Fatalf("kind=%q", r.Kind)
	}
	if r.Language != "en" {
		t.Fatalf("language=%q", r.Language)
	}
	if !r.FollowUp || len(r.References) == 0 {
		t.Fatalf("follow-up/reference missing: %+v", r)
	}
}

func TestProperNounDoesNotChangeLanguage(t *testing.T) {
	r := Analyze("Abre Unity", Context{Language: "es"})
	if r.Language != "es" {
		t.Fatalf("language=%q", r.Language)
	}
}

func TestGenericIntentKindsDoNotMentionDomains(t *testing.T) {
	cases := map[string]RequestKind{
		"corrige eso":             KindCorrection,
		"te propongo otra opción": KindProposal,
		"cancel":                  KindStop,
		"thanks":                  KindConversation,
		"continue with that":      KindContinuation,
	}
	for input, want := range cases {
		if got := Parse(input, ChannelText).Kind; got != want {
			t.Fatalf("%q => %q, want %q", input, got, want)
		}
	}
}

func TestGeneralReferencesResolveWorkResultAndTest(t *testing.T) {
	ctx := Context{
		ActiveWorkID: "work_1", ActiveObjective: "crear proyecto",
		LastResult: "tests passed", LastArtifactName: "main_test.go",
		Language: "es",
	}
	cases := []struct {
		input string
		kind  ReferenceKind
		value string
	}{
		{"¿Y el test?", ReferenceRecentArtifact, "main_test.go"},
		{"¿Y qué resultado tuvo?", ReferenceCurrentInput, "tests passed"},
		{"¿Qué estábamos haciendo?", ReferenceRecentWork, "work_1"},
	}
	for _, tc := range cases {
		r := Analyze(tc.input, ctx)
		found := false
		for _, ref := range r.References {
			if ref.Kind == tc.kind && ref.Resolved && ref.Value == tc.value {
				found = true
			}
		}
		if !found {
			t.Fatalf("%q references=%+v", tc.input, r.References)
		}
		if !r.FollowUp || !r.AddressesCurrent {
			t.Fatalf("%q lost continuity: %+v", tc.input, r)
		}
	}
}

func TestEnglishActionKeepsConversationLanguage(t *testing.T) {
	r := Analyze("Open Unity.", Context{Language: "es"})
	if r.Kind != KindAction {
		t.Fatalf("kind=%q", r.Kind)
	}
	if r.Language != "en" {
		t.Fatalf("language=%q", r.Language)
	}
}

func TestProductionWithGreetingPrefixStillProduces(t *testing.T) {
	r := Parse(`Hola Ahmelia, crea un programa que imprima "Hola mundo".`, ChannelText)
	if r.Kind != KindProduction {
		t.Fatalf("kind = %q, want production", r.Kind)
	}
}

func TestQuotedSocialWordsDoNotBecomeConversation(t *testing.T) {
	for _, input := range []string{
		`Crea un archivo que contenga "hola"`,
		`Crea un archivo que contenga "gracias"`,
		`Crea un programa que imprima "how are you"`,
	} {
		r := Parse(input, ChannelText)
		if r.Kind != KindProduction {
			t.Fatalf("%q => %q, want production", input, r.Kind)
		}
	}
}

func TestRawObjectiveIsPreservedAfterClassificationMask(t *testing.T) {
	input := `Crea un programa que muestre "Hola contexto".`
	r := Parse(input, ChannelText)
	if r.Raw != input || r.Objective.Text != input {
		t.Fatalf("raw/objective were modified: raw=%q objective=%q", r.Raw, r.Objective.Text)
	}
	if !strings.Contains(r.Normalized, "hola contexto") {
		t.Fatalf("normalized request unexpectedly lost literal: %q", r.Normalized)
	}
}

func TestGreetingMustBeWholeTurn(t *testing.T) {
	for _, input := range []string{
		"Hola",
		"Hola.",
		"Buenas",
		"Hello",
	} {
		r := Parse(input, ChannelText)
		if r.Kind != KindConversation {
			t.Fatalf("%q => %q, want conversation", input, r.Kind)
		}
	}
	for _, input := range []string{
		"Hola Ahmelia, abre Unity.",
		"Hello Ahmelia, open Unity.",
	} {
		r := Parse(input, ChannelText)
		if r.Kind != KindAction {
			t.Fatalf("%q => %q, want action", input, r.Kind)
		}
	}
}

func TestAnalyze_OpenEnglishProducesSemanticAction(t *testing.T) {
	r := Analyze("Open Unity.", Context{})
	if r.Kind != KindAction {
		t.Fatalf("Kind = %q, want action", r.Kind)
	}
	if r.Action.Name != "abrir_programa" {
		t.Fatalf("Action.Name = %q, want abrir_programa", r.Action.Name)
	}
	if r.Action.Target != "unity" {
		t.Fatalf("Action.Target = %q, want unity", r.Action.Target)
	}
}

func TestAnalyze_QuotedHelloDoesNotCreateGreeting(t *testing.T) {
	r := Analyze(`Crea un pequeño programa Go llamado prueba que muestre "Hola contexto".`, Context{})
	if r.Kind != KindProduction {
		t.Fatalf("Kind = %q, want production", r.Kind)
	}
	if r.Kind == KindConversation {
		t.Fatal("quoted literal hijacked the interaction kind")
	}
}

func TestInferAction_LongestSpecificPhraseWins(t *testing.T) {
	r := Analyze("Turn down the volume.", Context{})
	if r.Action.Name != "volumen_bajar" {
		t.Fatalf("Action.Name = %q, want volumen_bajar", r.Action.Name)
	}
	if r.Action.Target != "" {
		t.Fatalf("volume action target = %q, want empty", r.Action.Target)
	}
}

func TestInferAction_ClosePreservesTarget(t *testing.T) {
	r := Analyze("Close Chrome.", Context{})
	if r.Action.Name != "cerrar_ventana" {
		t.Fatalf("Action.Name = %q, want cerrar_ventana", r.Action.Name)
	}
	if r.Action.Target != "chrome" {
		t.Fatalf("Action.Target = %q, want chrome", r.Action.Target)
	}
}

func TestParseSalirIsStop(t *testing.T) {
	r := Parse("salir", ChannelText)
	if r.Kind != KindStop {
		t.Fatalf("salir debe ser KindStop, recibió %q", r.Kind)
	}
}

func TestAnalyzeAssignsQuestionOwnership(t *testing.T) {
	cases := []struct {
		input string
		want  UserObservationSubject
	}{
		{"¿Cuál es mi color favorito?", UserObservationSubjectUser},
		{"¿Tú tienes color favorito?", UserObservationSubjectSelf},
		{"¿Qué haces?", UserObservationSubjectSelf},
		{"¿Qué puedes hacer?", UserObservationSubjectSelf},
		{"¿Cómo estás?", UserObservationSubjectSelf},
	}
	for _, tc := range cases {
		r := Analyze(tc.input, Context{Language: "es"})
		if r.Subject != tc.want {
			t.Fatalf("%q subject=%q want=%q", tc.input, r.Subject, tc.want)
		}
	}
}

func TestInferQuestionSubject_CanonicalSelfQueries(t *testing.T) {
	cases := []struct {
		input string
		want  UserObservationSubject
	}{
		{"¿Tú tienes color favorito?", UserObservationSubjectSelf},
		{"¿Qué haces?", UserObservationSubjectSelf},
		{"¿Cómo estás?", UserObservationSubjectSelf},
		{"¿Cuál es mi color favorito?", UserObservationSubjectUser},
	}
	for _, tc := range cases {
		r := Analyze(tc.input, Context{Language: "es"})
		if r.Subject != tc.want {
			t.Fatalf("input=%q subject=%q want=%q", tc.input, r.Subject, tc.want)
		}
	}
}

func TestFeedback_CoversNaturalEvaluationOfProgress(t *testing.T) {
	cases := []string{
		"me encanta que hayas mejorado tu comportamiento últimamente, aunque hay cosas por mejorar aún",
		"has mejorado mucho, aunque todavía falta",
	}
	for _, input := range cases {
		r := Analyze(input, Context{Language: "es"})
		if !r.CommunicativeActs.Has(ActFeedback) {
			t.Fatalf("feedback not detected: %q acts=%+v", input, r.CommunicativeActs)
		}
	}
}

func TestContextClosure_DoesNotCreateStaleReference(t *testing.T) {
	r := Analyze("no estoy comprobando tu comportamiento actual eso es todo", Context{CurrentTopic: "color favorito", ActiveObjective: "color favorito"})
	if len(r.References) != 0 {
		t.Fatalf("closure statement resurrected stale references: %+v", r.References)
	}
	if r.AddressesCurrent || r.FollowUp {
		t.Fatalf("closure statement inherited current context: addresses=%t followup=%t", r.AddressesCurrent, r.FollowUp)
	}
}

func TestIndirectColorQuestionNormalizesAccentsAndOwnership(t *testing.T) {
	cases := []string{
		"nada queria saber si sabias mi color favorito",
		"nada quería saber si sabías mi color favorito",
	}
	for _, input := range cases {
		r := Analyze(input, Context{Language: "es"})
		if r.Kind != KindQuestion {
			t.Fatalf("%q kind=%q want=%q normalized=%q", input, r.Kind, KindQuestion, r.Normalized)
		}
		if r.Subject != UserObservationSubjectUser {
			t.Fatalf("%q subject=%q want user", input, r.Subject)
		}
		if !r.CommunicativeActs.Has(ActQuestion) {
			t.Fatalf("%q acts=%+v missing question", input, r.CommunicativeActs)
		}
	}
}

func TestClassificationSurfaceUsesSharedNormalization(t *testing.T) {
	r := SemanticRequest{Raw: "¿Podrías explicar qué está pasando?", Normalized: "¿Podrías explicar qué está pasando?"}
	acts := ClassifyCommunicativeActs(r)
	if !acts.Has(ActQuestion) {
		t.Fatalf("acts=%+v missing question after shared normalization", acts)
	}
}

func TestOwnershipResolverGeneralizesAcrossVocabulariesAndSpeechActs(t *testing.T) {
	cases := []struct {
		input string
		want  UserObservationSubject
	}{
		{"tu color favorito", UserObservationSubjectSelf},
		{"tranquila pronto sabras tu color favorito", UserObservationSubjectSelf},
		{"tu animal favorito", UserObservationSubjectSelf},
		{"algun dia tendras tu propio animal favorito", UserObservationSubjectSelf},
		{"tu nombre", UserObservationSubjectSelf},
		{"tu proposito", UserObservationSubjectSelf},
	}
	for _, tc := range cases {
		r := Analyze(tc.input, Context{Language: "es"})
		if r.Subject != tc.want {
			t.Fatalf("%q subject=%q want=%q confidence=%.2f evidence=%+v", tc.input, r.Subject, tc.want, r.SubjectConfidence, r.SubjectEvidence)
		}
		if r.SubjectConfidence <= 0 || len(r.SubjectEvidence) == 0 {
			t.Fatalf("%q missing graded ownership evidence: confidence=%.2f evidence=%+v", tc.input, r.SubjectConfidence, r.SubjectEvidence)
		}
	}
}

func TestOwnershipResolverLeavesMixedAndOwnerlessTurnsUnresolved(t *testing.T) {
	for _, input := range []string{
		"me gusta tu color",
		"el sistema esta listo",
		"que esta pasando",
	} {
		r := Analyze(input, Context{Language: "es"})
		if r.Subject != "" {
			t.Fatalf("%q subject=%q want unresolved; evidence=%+v", input, r.Subject, r.SubjectEvidence)
		}
	}
}

func TestOwnershipResolverPreservesUserKnowledgeQueries(t *testing.T) {
	for _, input := range []string{"¿Qué nivel tengo en Blender?", "¿Qué sé sobre Blender?"} {
		r := Analyze(input, Context{Language: "es"})
		if r.Subject != UserObservationSubjectUser {
			t.Fatalf("%q subject=%q want user confidence=%.2f evidence=%+v", input, r.Subject, r.SubjectConfidence, r.SubjectEvidence)
		}
	}
}

func TestSearchRequestIsAnswerableCommunicativeAct(t *testing.T) {
	for _, input := range []string{
		"busca información sobre Go",
		"search for information about Go",
		"look up Go on the web",
	} {
		t.Run(input, func(t *testing.T) {
			req := Analyze(input, Context{})
			if req.Kind != KindSearch {
				t.Fatalf("request kind = %q, want %q", req.Kind, KindSearch)
			}
			if !req.CommunicativeActs.Has(ActQuestion) {
				t.Fatalf("search communicative acts = %+v, want question act", req.CommunicativeActs)
			}
			intent := DetermineResponseIntent(req, ConversationSnapshot{})
			if intent.Kind != ResponseIntentAnswer || !intent.ShouldSpeak || intent.ShouldAct {
				t.Fatalf("search response intent = %+v, want answer/speak/not-act", intent)
			}
		})
	}
}

func TestProductionObjectiveDoesNotExtractEmbeddedComputerAction(t *testing.T) {
	input := `Quiero que implementes una pequeña utilidad de programación.

Objetivo:
Crea una función Go llamada IsEven(n int) bool que devuelva true si el número es par y false si es impar.

Condiciones:
- Trabaja dentro del workspace/proyecto actual.
- Crea o modifica los archivos necesarios.
- Añade tests para números positivos, negativos y cero.
- Ejecuta los tests.
- Verifica que el resultado funciona realmente.`
	r := Analyze(input, Context{})
	if r.Kind != KindProduction {
		t.Fatalf("kind=%q, want production", r.Kind)
	}
	if r.Action.Name != "" {
		t.Fatalf("production objective leaked embedded action %q", r.Action.Name)
	}
	decision := ResolveCapabilityInteraction(r)
	if !decision.Executable || decision.Request.Operation != "produce" {
		t.Fatalf("production objective lost generic execution boundary: %+v", decision)
	}
}

func TestSimpleComputerActionStillExtractsOutsideProduction(t *testing.T) {
	r := Analyze("Ejecuta VLC", Context{})
	if r.Kind != KindAction {
		t.Fatalf("kind=%q, want action", r.Kind)
	}
	if r.Action.Name != "abrir_programa" {
		t.Fatalf("action=%q, want abrir_programa", r.Action.Name)
	}
}

func TestNeedsObjectiveAssemblyRecognizesDelegationPreamble(t *testing.T) {
	r := Analyze("Quiero que implementes una pequeña utilidad de programación.", Context{})
	if !NeedsObjectiveAssembly(r) {
		t.Fatalf("expected delegation preamble to await specification: %+v", r)
	}
}

func TestNeedsObjectiveAssemblyAllowsCompleteSingleSentence(t *testing.T) {
	r := Analyze("Quiero que implementes una función Go que devuelva true si el número es par.", Context{})
	if NeedsObjectiveAssembly(r) {
		t.Fatalf("complete single-sentence objective was incorrectly deferred: %+v", r)
	}
}

func TestNeedsObjectiveAssemblyAllowsCompoundObjective(t *testing.T) {
	r := Analyze(`Quiero que implementes una pequeña utilidad de programación.

Objetivo:
Crea una función Go llamada IsEven(n int) bool que devuelva true si el número es par y false si es impar.

Condiciones:
- Ejecuta los tests.
- Verifica el resultado.`, Context{})
	if NeedsObjectiveAssembly(r) {
		t.Fatalf("compound objective was incorrectly deferred: %+v", r)
	}
}

func TestProductionObjectivePreservesPrimaryOperationAcrossRecoveryAndVerification(t *testing.T) {
	input := `Quiero que implementes una pequeña utilidad de programación.

Objetivo:
Crea una función Go llamada IsEven(n int) bool que devuelva true si el número es par y false si es impar.

Condiciones:
- Trabaja dentro del workspace/proyecto actual.
- Añade tests para números positivos, negativos y cero.
- Ejecuta los tests.
- Verifica que el resultado funciona realmente.
- Si encuentras un problema durante la ejecución, analízalo y corrígelo antes de finalizar.`
	r := Analyze(input, Context{})
	if r.RequestedAction != "create" || r.Frame.PrimaryOperation != "create" {
		t.Fatalf("primary operation was displaced: requested=%q frame=%q", r.RequestedAction, r.Frame.PrimaryOperation)
	}
	if r.FollowUp || r.AddressesCurrent {
		t.Fatalf("subordinate requirements incorrectly changed discourse continuity: follow_up=%v addresses_current=%v", r.FollowUp, r.AddressesCurrent)
	}
	if r.Frame.Workspace != "current_workspace" {
		t.Fatalf("workspace=%q, want current_workspace", r.Frame.Workspace)
	}
	if len(r.Frame.RequiredActions) == 0 || len(r.Frame.Verification) == 0 || len(r.Frame.Recovery) == 0 {
		t.Fatalf("objective frame lost subordinate requirements: %+v", r.Frame)
	}
	for _, ref := range r.References {
		if !ref.Resolved && ref.Role != ReferenceRoleRequirement && ref.Role != ReferenceRoleRecoveryTarget {
			t.Fatalf("unexpected blocking reference: %+v", ref)
		}
	}
}

func TestProductionReferenceRolesDoNotTurnRequirementsIntoTargets(t *testing.T) {
	r := Analyze("Implementa la función y ejecuta los tests; si falla, corrígelo.", Context{})
	if r.RequestedAction != "create" {
		t.Fatalf("requested action=%q, want create", r.RequestedAction)
	}
	for _, ref := range r.References {
		if ref.Expression == "test artifact" && ref.Role != ReferenceRoleRequirement {
			t.Fatalf("test reference role=%q, want requirement", ref.Role)
		}
		if ref.Expression == "current referent" && ref.Role != ReferenceRoleRecoveryTarget {
			t.Fatalf("recovery reference role=%q, want recovery_target", ref.Role)
		}
	}
}

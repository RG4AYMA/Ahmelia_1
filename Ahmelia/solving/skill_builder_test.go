package solving

import (
	"ahmelia/interaction"
	"strings"
	"testing"
	"time"

	"ahmelia/gateway"
)

// TestBuild_ValidationSetNilDevuelveNil confirma el criterio defensivo:
// sin ValidationSet no hay nada que construir.
func TestBuild_ValidationSetNilDevuelveNil(t *testing.T) {
	sb := NewSkillBuilder()
	note := sb.Build(LearningItem{TaskID: "T1", TaskDesc: "algo"}, nil)
	if note != nil {
		t.Fatalf("esperaba nil con ValidationSet nil, obtuve %+v", note)
	}
}

// TestBuild_SinValidacionesDevuelveNil confirma que un ValidationSet
// vacío (ninguna URL fetcheada) tampoco produce nota.
func TestBuild_SinValidacionesDevuelveNil(t *testing.T) {
	sb := NewSkillBuilder()
	vs := &ValidationSet{Goal: "unity"}
	note := sb.Build(LearningItem{TaskID: "T1", TaskDesc: "unity"}, vs)
	if note != nil {
		t.Fatalf("esperaba nil sin Validations, obtuve %+v", note)
	}
}

// TestBuild_SinNingunDocumentoConfiableDevuelveNil confirma que no se
// fabrica una nota sin contenido real detrás — el límite honesto
// central de este bloque.
func TestBuild_SinNingunDocumentoConfiableDevuelveNil(t *testing.T) {
	sb := NewSkillBuilder()
	vs := &ValidationSet{
		Goal: "unity",
		Validations: []*SourceValidation{
			{
				Document:   &gateway.Document{URL: "https://docs.unity3d.com/", Source: "Unity Docs", StatusCode: 404},
				StatusOK:   false,
				HasContent: false,
			},
		},
	}
	note := sb.Build(LearningItem{TaskID: "T1", TaskDesc: "unity"}, vs)
	if note != nil {
		t.Fatalf("esperaba nil sin ningún documento confiable, obtuve %+v", note)
	}
}

// TestBuild_ConDocumentoConfiableArmaNota confirma el camino feliz:
// al menos un documento confiable produce una Skill con contenido
// y procedencia real.
func TestBuild_ConDocumentoConfiableArmaNota(t *testing.T) {
	sb := NewSkillBuilder()
	vs := &ValidationSet{
		Goal: "unity",
		Validations: []*SourceValidation{
			{
				Document:   &gateway.Document{URL: "https://docs.unity3d.com/", Source: "Unity Docs", Text: strings.Repeat("contenido real. ", 20), StatusCode: 200},
				StatusOK:   true,
				HasContent: true,
			},
		},
		Corroborated: false,
	}
	note := sb.Build(LearningItem{TaskID: "unity", TaskDesc: "aprender unity"}, vs)
	if note == nil {
		t.Fatal("esperaba una nota con un documento confiable, obtuve nil")
	}
	if note.SkillID != "unity" {
		t.Errorf("SkillID = %q, esperaba %q", note.SkillID, "unity")
	}
	if !strings.Contains(note.Content, "contenido real") {
		t.Errorf("Content no incluye el texto del documento confiable: %q", note.Content)
	}
	if len(note.Sources) != 1 || note.Sources[0] != "Unity Docs" {
		t.Errorf("Sources = %v, esperaba [Unity Docs]", note.Sources)
	}
	if note.Corroborated {
		t.Error("Corroborated debe reflejar vs.Corroborated (false acá), no inventar true")
	}
	if note.BuiltAt == "" || note.UpdatedAt == "" {
		t.Error("BuiltAt/UpdatedAt no deben quedar vacíos")
	}
}

// TestBuild_IgnoraDocumentosNoConfiablesPeroUsaLosConfiables confirma
// que un documento con StatusCode de error no contamina la nota,
// aunque haya otro sí confiable en el mismo lote.
func TestBuild_IgnoraDocumentosNoConfiablesPeroUsaLosConfiables(t *testing.T) {
	sb := NewSkillBuilder()
	vs := &ValidationSet{
		Goal: "unity",
		Validations: []*SourceValidation{
			{
				Document:   &gateway.Document{URL: "https://docs.unity3d.com/roto", Source: "Unity Docs", StatusCode: 500},
				StatusOK:   false,
				HasContent: false,
			},
			{
				Document:   &gateway.Document{URL: "https://docs.unity3d.com/ok", Source: "Unity Docs", Text: strings.Repeat("bueno. ", 30), StatusCode: 200},
				StatusOK:   true,
				HasContent: true,
			},
		},
	}
	note := sb.Build(LearningItem{TaskID: "unity", TaskDesc: "unity"}, vs)
	if note == nil {
		t.Fatal("esperaba una nota, hay un documento confiable en el lote")
	}
	if strings.Count(note.Content, "[Unity Docs]") != 1 {
		t.Errorf("esperaba un único bloque [Unity Docs] (solo el confiable), Content=%q", note.Content)
	}
}

// TestBuild_RecortaContenidoLargo confirma que el contenido se acota a
// MaxNoteContentLength — no crece sin límite.
func TestBuild_RecortaContenidoLargo(t *testing.T) {
	sb := NewSkillBuilder()
	longText := strings.Repeat("a", MaxNoteContentLength*2)
	vs := &ValidationSet{
		Goal: "unity",
		Validations: []*SourceValidation{
			{
				Document:   &gateway.Document{URL: "https://docs.unity3d.com/", Source: "Unity Docs", Text: longText, StatusCode: 200},
				StatusOK:   true,
				HasContent: true,
			},
		},
	}
	note := sb.Build(LearningItem{TaskID: "unity", TaskDesc: "unity"}, vs)
	if len(note.Content) > MaxNoteContentLength+50 { // margen por el prefijo "[Source] " y el "…"
		t.Errorf("Content no se recortó: len=%d, límite=%d", len(note.Content), MaxNoteContentLength)
	}
}

// TestBuild_ContenidoDeterministicoPorOrdenDeFuente confirma que el
// orden del contenido armado no depende del orden en que llegaron los
// Validations — se ordena por Source antes de concatenar.
func TestBuild_ContenidoDeterministicoPorOrdenDeFuente(t *testing.T) {
	sb := NewSkillBuilder()
	vsA := &ValidationSet{
		Validations: []*SourceValidation{
			{Document: &gateway.Document{Source: "Z Source", Text: strings.Repeat("z. ", 20), StatusCode: 200}, StatusOK: true, HasContent: true},
			{Document: &gateway.Document{Source: "A Source", Text: strings.Repeat("a. ", 20), StatusCode: 200}, StatusOK: true, HasContent: true},
		},
	}
	vsB := &ValidationSet{
		Validations: []*SourceValidation{
			{Document: &gateway.Document{Source: "A Source", Text: strings.Repeat("a. ", 20), StatusCode: 200}, StatusOK: true, HasContent: true},
			{Document: &gateway.Document{Source: "Z Source", Text: strings.Repeat("z. ", 20), StatusCode: 200}, StatusOK: true, HasContent: true},
		},
	}
	noteA := sb.Build(LearningItem{TaskID: "t", TaskDesc: "t"}, vsA)
	noteB := sb.Build(LearningItem{TaskID: "t", TaskDesc: "t"}, vsB)
	if noteA.Content != noteB.Content {
		t.Errorf("Content debe ser determinístico sin importar el orden de entrada:\nA=%q\nB=%q", noteA.Content, noteB.Content)
	}
	if !strings.HasPrefix(noteA.Content, "[A Source]") {
		t.Errorf("Content debe empezar con la fuente ordenada alfabéticamente primero, obtuve: %q", noteA.Content[:20])
	}
}

// TestInstall_ConservaBuiltAtOriginalAlActualizar confirma que
// reconstruir una nota existente no pisa BuiltAt, solo UpdatedAt —
// mismo espíritu que FirstValidatedAt en Knowledge Memory (49).
func TestInstall_ConservaBuiltAtOriginalAlActualizar(t *testing.T) {
	sl := NewSkillLibrary()
	first := &Skill{SkillID: "unity", Label: "unity", BuiltAt: "2020-01-01T00:00:00Z", UpdatedAt: "2020-01-01T00:00:00Z"}
	sl.Install(first)

	time.Sleep(10 * time.Millisecond)
	second := &Skill{SkillID: "unity", Label: "unity actualizado", BuiltAt: "2099-12-31T00:00:00Z"}
	sl.Install(second)

	got, ok := sl.Get("unity")
	if !ok {
		t.Fatal("esperaba encontrar la nota instalada")
	}
	if got.BuiltAt != "2020-01-01T00:00:00Z" {
		t.Errorf("BuiltAt no debe pisarse al reinstalar, quedó en %q", got.BuiltAt)
	}
	if got.UpdatedAt == "2020-01-01T00:00:00Z" {
		t.Error("UpdatedAt sí debe actualizarse al reinstalar")
	}
	if got.Label != "unity actualizado" {
		t.Errorf("el resto del contenido sí debe reflejar la versión nueva, Label=%q", got.Label)
	}
}

// TestInstall_NilONoteVaciaEsNoOp confirma el criterio defensivo.
func TestInstall_NilONoteVaciaEsNoOp(t *testing.T) {
	sl := NewSkillLibrary()
	sl.Install(nil)
	if len(sl.Skills) != 0 {
		t.Error("Install(nil) no debe agregar nada")
	}
	sl.Install(&Skill{SkillID: ""})
	if len(sl.Skills) != 0 {
		t.Error("Install con SkillID vacío no debe agregar nada")
	}
}

// TestSaveLoad_PersisteYRecuperaCorrecto confirma el ciclo completo de
// persistencia — mismo patrón que Knowledge Memory (49).
func TestSaveLoad_PersisteYRecuperaCorrecto(t *testing.T) {
	tmpFile := t.TempDir() + "/skill_library_test.json"

	sl := NewSkillLibrary()
	sl.Install(&Skill{SkillID: "unity", Label: "Unity", Content: "texto", Sources: []string{"Unity Docs"}})
	if err := sl.Save(tmpFile); err != nil {
		t.Fatalf("Save falló: %v", err)
	}

	loaded := LoadSkillLibrary(tmpFile)
	note, ok := loaded.Get("unity")
	if !ok {
		t.Fatal("esperaba recuperar la nota 'unity' tras Load")
	}
	if note.Content != "texto" {
		t.Errorf("Content tras Load = %q, esperaba %q", note.Content, "texto")
	}
}

// TestLoadSkillLibrary_ArchivoInexistenteArrancaVacia confirma el
// criterio defensivo al cargar (mismo patrón que LoadKnowledgeMemory).
func TestLoadSkillLibrary_ArchivoInexistenteArrancaVacia(t *testing.T) {
	sl := LoadSkillLibrary("/ruta/que/no/existe/nunca.json")
	if sl == nil || sl.Skills == nil {
		t.Fatal("esperaba una biblioteca vacía inicializada, no nil")
	}
	if len(sl.Skills) != 0 {
		t.Errorf("esperaba 0 notas, obtuve %d", len(sl.Skills))
	}
}

// TestDescribe_BibliotecaVacia confirma la narrativa sin notas.
func TestDescribe_SkillLibraryVacia(t *testing.T) {
	sl := NewSkillLibrary()
	desc := sl.Describe()
	if !strings.Contains(desc, "Todavía no hay ninguna nota") {
		t.Errorf("Describe() vacío = %q", desc)
	}
}

// TestParseSkillBuilderQuery_vs_ParseSkillBuildQuery_NoSeConfunden
// confirma que consultar la biblioteca ya guardada y pedir construir
// una nota nueva son triggers distintos, sin solapamiento — mismo
// tipo de test que confirmó SourceValidator/Validator en Bloque 53.
func TestParseSkillBuilderQuery_vs_ParseSkillBuildQuery_NoSeConfunden(t *testing.T) {
	queryOnly := "que notas de estudio tienes sobre unity"
	if !ParseSkillBuilderQuery(interaction.Analyze(queryOnly, interaction.Context{})) {
		t.Errorf("ParseSkillBuilderQuery debería detectar %q", queryOnly)
	}
	if ParseSkillBuildQuery(interaction.Analyze(queryOnly, interaction.Context{})) {
		t.Errorf("ParseSkillBuildQuery NO debería disparar con %q", queryOnly)
	}

	buildOnly := "guarda una nota sobre unity engine"
	if !ParseSkillBuildQuery(interaction.Analyze(buildOnly, interaction.Context{})) {
		t.Errorf("ParseSkillBuildQuery debería detectar %q", buildOnly)
	}
	if ParseSkillBuilderQuery(interaction.Analyze(buildOnly, interaction.Context{})) {
		t.Errorf("ParseSkillBuilderQuery NO debería disparar con %q", buildOnly)
	}
}

// TestParseSkillBuildQuery_FormaImperativa (Integration Fix, post
// Bloque 63) confirma que "aprende X" — la forma natural de un
// comando de voz — dispara la misma cadena que "guarda una nota
// sobre X", sin necesitar una frase completa en infinitivo.
func TestParseSkillBuildQuery_FormaImperativa(t *testing.T) {
	casos := []string{"aprende blender", "aprende golang", "aprende sobre blender"}
	for _, c := range casos {
		if !ParseSkillBuildQuery(interaction.Analyze(c, interaction.Context{})) {
			t.Errorf("ParseSkillBuildQuery debería detectar %q", c)
		}
	}
}

// TestParseSkillBuildQuery_FormaInfinitiva (Integration Fix) — mismo
// caso que la imperativa, pero para "aprender X": la forma que arma
// PendingClarification (memory/session.go) al fusionar un objetivo
// original dicho en infinitivo con la aclaración del usuario.
func TestParseSkillBuildQuery_FormaInfinitiva(t *testing.T) {
	casos := []string{"aprender blender", "aprender golang", "aprender sobre blender"}
	for _, c := range casos {
		if !ParseSkillBuildQuery(interaction.Analyze(c, interaction.Context{})) {
			t.Errorf("ParseSkillBuildQuery debería detectar %q", c)
		}
	}
}

// TestStripTrigger_AprendeYAprenderNoCortanAMitadDePalabra confirma que
// tanto "aprende " (imperativo) como "aprender " (infinitivo, agregado
// después por Integration Fix — ver comentario en skillBuildTriggers)
// cortan por límite de palabra real, no por substring a mitad de
// palabra. El riesgo concreto: sin el espacio final en el trigger,
// "aprende " matchea como substring de "aprender" (aprendER unity) y
// stripTrigger corta mal, dejando "R unity" en vez de "Unity" — ese
// bug apareció de verdad al escribir el fix original, antes de agregar
// el espacio. Ahora que "aprender " también es trigger (para que un
// objetivo fusionado por PendingClarification en infinitivo dispare
// ejecución igual que en imperativo), las dos formas deben stripear
// simétricamente.
func TestStripTrigger_AprendeYAprenderNoCortanAMitadDePalabra(t *testing.T) {
	a := NewAnalyzer()

	imperativo := a.Analyze("aprende blender")
	if imperativo.MainGoal != "Blender" {
		t.Errorf("MainGoal para %q = %q, esperaba %q", "aprende blender", imperativo.MainGoal, "Blender")
	}

	infinitivo := a.Analyze("aprender unity")
	if infinitivo.MainGoal != "Unity" {
		t.Errorf("MainGoal para %q = %q, esperaba %q (aprender ahora es trigger de ejecución, simétrico a aprende)", "aprender unity", infinitivo.MainGoal, "Unity")
	}
}

// TestSkillLibrarySave_DirectorioInexistenteDevuelveError confirma que
// un error real de filesystem se propaga en vez de fallar en silencio.
func TestSkillLibrarySave_DirectorioInexistenteDevuelveError(t *testing.T) {
	sl := NewSkillLibrary()
	sl.Install(&Skill{SkillID: "x", Label: "x"})
	err := sl.Save("/ruta/que/no/existe/nunca/skill_library.json")
	if err == nil {
		t.Error("esperaba error al guardar en un directorio inexistente")
	}
}

func TestIsInstructionTriggerRejectsIncidentalEnglishMention(t *testing.T) {
	if isInstructionTrigger("Use the go command to run your code.", "run") {
		t.Fatal("run inside 'to run your code' must not become a standalone operation")
	}
	if !isInstructionTrigger("Run your code.", "run") {
		t.Fatal("run at the beginning must be recognized as an operation")
	}
	if !isInstructionTrigger("Open the editor and run the program.", "run") {
		t.Fatal("run after a clause connector must be recognized")
	}
}

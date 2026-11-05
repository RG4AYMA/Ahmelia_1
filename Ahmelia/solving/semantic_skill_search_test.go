package solving

import (
	"ahmelia/interaction"
	"strings"
	"testing"
)

// TestSearch_QueryVacioDevuelveSinMatches confirma el criterio
// defensivo con texto vacío.
func TestSearch_QueryVacioDevuelveSinMatches(t *testing.T) {
	s := NewSemanticSkillSearcher()
	skills := NewSkillLibrary()
	skills.Install(&Skill{SkillID: "Unity", Label: "Unity", Content: "rendering pipeline"})

	result := s.Search("", skills)
	if result.HasMatches() {
		t.Error("con query vacío no debe haber matches")
	}
}

// TestSearch_QuerySoloEspaciosSeConsideraVacio confirma que un query
// de puros espacios se trata igual que uno vacío.
func TestSearch_QuerySoloEspaciosSeConsideraVacio(t *testing.T) {
	s := NewSemanticSkillSearcher()
	skills := NewSkillLibrary()
	skills.Install(&Skill{SkillID: "Unity", Label: "Unity", Content: "contenido real"})

	result := s.Search("   ", skills)
	if result.HasMatches() {
		t.Error("un query de solo espacios debe tratarse como vacío, sin matches")
	}
}

// TestSearch_SkillLibraryNilNoRompe confirma el criterio defensivo
// con la biblioteca en nil.
func TestSearch_SkillLibraryNilNoRompe(t *testing.T) {
	s := NewSemanticSkillSearcher()
	result := s.Search("rendering", nil)
	if result == nil {
		t.Fatal("Search no debe devolver nil")
	}
	if result.HasMatches() {
		t.Error("sin biblioteca no puede haber matches")
	}
}

// TestSearch_BibliotecaVaciaSinMatches confirma que una biblioteca sin
// notas no rompe y simplemente no encuentra nada.
func TestSearch_BibliotecaVaciaSinMatches(t *testing.T) {
	s := NewSemanticSkillSearcher()
	result := s.Search("rendering", NewSkillLibrary())
	if result.HasMatches() {
		t.Error("una biblioteca vacía no debe tener matches")
	}
}

// TestSearch_EncuentraTerminoEnContent confirma el camino feliz: una
// nota cuyo Content real menciona el término buscado aparece en los
// resultados.
func TestSearch_EncuentraTerminoEnContent(t *testing.T) {
	s := NewSemanticSkillSearcher()
	skills := NewSkillLibrary()
	skills.Install(&Skill{
		SkillID: "Unity",
		Label:   "Unity",
		Content: "Unity offers a powerful rendering pipeline for real-time rendering.",
	})

	result := s.Search("rendering", skills)
	if !result.HasMatches() {
		t.Fatal("esperaba encontrar la nota de Unity, que menciona 'rendering' en su Content")
	}
	if len(result.Matches) != 1 || result.Matches[0].Domain != "Unity" {
		t.Errorf("Matches = %+v, esperaba [Unity]", result.Matches)
	}
}

// TestSearch_EsCaseInsensitive confirma que la búsqueda no distingue
// mayúsculas/minúsculas — ni en el query ni en el Content.
func TestSearch_EsCaseInsensitive(t *testing.T) {
	s := NewSemanticSkillSearcher()
	skills := NewSkillLibrary()
	skills.Install(&Skill{SkillID: "Unity", Label: "Unity", Content: "Advanced RENDERING techniques"})

	result := s.Search("Rendering", skills)
	if !result.HasMatches() {
		t.Error("la búsqueda debe ser case-insensitive")
	}
}

// TestSearch_CuentaMencionesReales confirma que Hits refleja el
// conteo real de apariciones, no solo 0/1.
func TestSearch_CuentaMencionesReales(t *testing.T) {
	s := NewSemanticSkillSearcher()
	skills := NewSkillLibrary()
	skills.Install(&Skill{
		SkillID: "Unity",
		Label:   "Unity",
		Content: "rendering rendering rendering — Unity's rendering pipeline",
	})

	result := s.Search("rendering", skills)
	if len(result.Matches) != 1 {
		t.Fatalf("esperaba 1 match, obtuve %d", len(result.Matches))
	}
	if result.Matches[0].Hits != 4 {
		t.Errorf("Hits = %d, esperaba 4 apariciones reales de 'rendering'", result.Matches[0].Hits)
	}
}

// TestSearch_OrdenaPorHitsDescendente confirma que las notas con más
// menciones aparecen primero — relevancia real, no orden de mapa.
func TestSearch_OrdenaPorHitsDescendente(t *testing.T) {
	s := NewSemanticSkillSearcher()
	skills := NewSkillLibrary()
	skills.Install(&Skill{SkillID: "Unity", Label: "Unity", Content: "rendering mencionado una vez"})
	skills.Install(&Skill{SkillID: "Unreal Engine", Label: "Unreal Engine", Content: "rendering rendering rendering rendering"})
	skills.Install(&Skill{SkillID: "Go", Label: "Go", Content: "nada relacionado acá"})

	result := s.Search("rendering", skills)
	if len(result.Matches) != 2 {
		t.Fatalf("esperaba 2 matches (Unity, Unreal Engine), obtuve %d: %+v", len(result.Matches), result.Matches)
	}
	if result.Matches[0].Domain != "Unreal Engine" {
		t.Errorf("el primer match debe ser Unreal Engine (4 menciones), obtuve %s", result.Matches[0].Domain)
	}
	if result.Matches[1].Domain != "Unity" {
		t.Errorf("el segundo match debe ser Unity (1 mención), obtuve %s", result.Matches[1].Domain)
	}
}

// TestSearch_EmpateEnHitsOrdenaAlfabeticamentePorDomain confirma que,
// a igual conteo, el orden es determinístico (alfabético), no
// dependiente del orden de iteración del map interno.
func TestSearch_EmpateEnHitsOrdenaAlfabeticamentePorDomain(t *testing.T) {
	s := NewSemanticSkillSearcher()
	skills := NewSkillLibrary()
	skills.Install(&Skill{SkillID: "Unreal Engine", Label: "Unreal Engine", Content: "rendering"})
	skills.Install(&Skill{SkillID: "DirectX", Label: "DirectX", Content: "rendering"})
	skills.Install(&Skill{SkillID: "Unity", Label: "Unity", Content: "rendering"})

	result := s.Search("rendering", skills)
	if len(result.Matches) != 3 {
		t.Fatalf("esperaba 3 matches, obtuve %d", len(result.Matches))
	}
	got := []string{result.Matches[0].Domain, result.Matches[1].Domain, result.Matches[2].Domain}
	want := []string{"DirectX", "Unity", "Unreal Engine"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("orden = %v, esperaba %v (alfabético a igual Hits)", got, want)
		}
	}
}

// TestSearch_TambienBuscaEnLabel confirma que el término también se
// busca en Label, no solo en Content.
func TestSearch_TambienBuscaEnLabel(t *testing.T) {
	s := NewSemanticSkillSearcher()
	skills := NewSkillLibrary()
	skills.Install(&Skill{SkillID: "X", Label: "motor de renderizado", Content: "sin el término acá"})

	result := s.Search("renderizado", skills)
	if !result.HasMatches() {
		t.Error("la búsqueda debe considerar también el Label, no solo el Content")
	}
}

// TestSearch_TerminoEnOtroIdiomaNoMatcheaSinTraduccion confirma
// honestamente el límite documentado: sin traducción, un término en
// español no encuentra contenido guardado en inglés, aunque hable del
// mismo tema.
func TestSearch_TerminoEnOtroIdiomaNoMatcheaSinTraduccion(t *testing.T) {
	s := NewSemanticSkillSearcher()
	skills := NewSkillLibrary()
	skills.Install(&Skill{SkillID: "Unity", Label: "Unity", Content: "Unity's real-time rendering engine"})

	result := s.Search("renderizado", skills)
	if result.HasMatches() {
		t.Error("sin traducción, 'renderizado' (español) no debe matchear contenido en inglés — límite honesto del bloque")
	}
}

// TestDescribe_SinMatches confirma la narrativa cuando no se
// encuentra nada.
func TestDescribe_SkillSearchResult_SinMatches(t *testing.T) {
	sr := &SkillSearchResult{Query: "quantum computing"}
	desc := sr.Describe()
	if !strings.Contains(desc, "No encontré ninguna nota") || !strings.Contains(desc, "quantum computing") {
		t.Errorf("Describe() = %q", desc)
	}
}

// TestDescribe_ConMatchesListaLosDominiosYMenciones confirma que la
// narrativa incluye tanto el dominio como el conteo de menciones.
func TestDescribe_SkillSearchResult_ConMatches(t *testing.T) {
	sr := &SkillSearchResult{
		Query: "rendering",
		Matches: []SkillMatch{
			{Domain: "Unreal Engine", Hits: 4, Note: &Skill{Label: "Unreal Engine"}},
			{Domain: "Unity", Hits: 1, Note: &Skill{Label: "Unity"}},
		},
	}
	desc := sr.Describe()
	if !strings.Contains(desc, "Unreal Engine") || !strings.Contains(desc, "Unity") {
		t.Errorf("Describe() debe listar todos los dominios encontrados: %q", desc)
	}
	if !strings.Contains(desc, "4 mención") {
		t.Errorf("Describe() debe incluir el conteo de menciones: %q", desc)
	}
}

// TestDescribe_SkillSearchResult_NotaNilNoPanica es defensivo: Search()
// siempre puebla Note en uso real, pero Describe() no debería asumirlo
// — un SkillMatch armado a mano (como este test, o cualquier código
// futuro que no pase por Search()) con Note nil debe caer al Domain
// crudo, nunca panicar.
func TestDescribe_SkillSearchResult_NotaNilNoPanica(t *testing.T) {
	sr := &SkillSearchResult{
		Query:   "go",
		Matches: []SkillMatch{{Domain: "Go", Hits: 2}},
	}
	desc := sr.Describe()
	if !strings.Contains(desc, "Go") || !strings.Contains(desc, "2 mención") {
		t.Errorf("Describe() con Note nil debería caer al Domain crudo: %q", desc)
	}
}

// TestParseSemanticSkillSearchQuery_SabesUsarYSabesHacer (Integration
// Fix) confirma que el fraseo natural "sabes usar X"/"sabes hacer X"
// dispara la misma búsqueda que "que sabes sobre X", sin necesitar la
// frase completa.
func TestParseSemanticSkillSearchQuery_SabesUsarYSabesHacer(t *testing.T) {
	casos := []string{"sabes usar blender", "sabes hacer blender", "que sabes sobre blender"}
	for _, c := range casos {
		if !ParseSemanticSkillSearchQuery(interaction.Analyze(c, interaction.Context{})) {
			t.Errorf("ParseSemanticSkillSearchQuery debería detectar %q", c)
		}
	}
}

// TestParseSemanticSkillSearchQuery_NoSeConfundeConOtrasRamasDeFase4Punto1
// confirma que el trigger de búsqueda no colisiona con los triggers
// de las otras tres ramas relacionadas con Skill Library.
func TestParseSemanticSkillSearchQuery_NoSeConfundeConOtrasRamasDeFase4Punto1(t *testing.T) {
	searchOnly := "que sabes sobre renderizado"
	if !ParseSemanticSkillSearchQuery(interaction.Analyze(searchOnly, interaction.Context{})) {
		t.Errorf("ParseSemanticSkillSearchQuery debería detectar %q", searchOnly)
	}
	if ParseSkillBuilderQuery(interaction.Analyze(searchOnly, interaction.Context{})) {
		t.Errorf("ParseSkillBuilderQuery NO debería disparar con %q", searchOnly)
	}
	if ParseSkillBuildQuery(interaction.Analyze(searchOnly, interaction.Context{})) {
		t.Errorf("ParseSkillBuildQuery NO debería disparar con %q", searchOnly)
	}
	if ParseSkillContextQuery(interaction.Analyze(searchOnly, interaction.Context{})) {
		t.Errorf("ParseSkillContextQuery NO debería disparar con %q", searchOnly)
	}

	builderOnly := "que hay guardado sobre unity"
	if ParseSemanticSkillSearchQuery(interaction.Analyze(builderOnly, interaction.Context{})) {
		t.Errorf("ParseSemanticSkillSearchQuery NO debería disparar con %q", builderOnly)
	}
}

// TestSearch_RecuperaSkillYaAprendidaSinRedEsGarantiaEstructural
// (Integration Fix — validación de recuperación offline, post-cierre
// del Bloque 54) reproduce el escenario real completo: una Skill de Go
// con la forma exacta que dejó producción (mismos Steps,
// ProcedureTitle, Operations que salieron de skill_library.json
// contra go.dev real) ya guardada en la biblioteca, y una consulta de
// texto equivalente a lo que Process() recibiría después de que
// Google Speech Recognition ya convirtió la voz — "amelia qué sabes
// sobre instalar go" — normalizado igual que hace core/engine.go antes
// de esta rama.
//
// La garantía que prueba este test no es solo "dio la respuesta
// correcta": es que Search(query string, skills *SkillLibrary) —ver su
// firma— NO recibe ningún *gateway.Gateway ni *Discoverer. Es
// estructuralmente imposible que esta función dispare una llamada de
// red, sin importar qué tan mal calibrada esté una heurística interna
// — el compilador lo garantiza, no una prueba de comportamiento en
// runtime. Esto es lo que responde, con evidencia y no solo con
// lectura de logs, a la pregunta real: "¿Ahmelia necesita Internet
// para usar algo que ya aprendió?" No, esta ruta ni siquiera tiene
// cómo pedirlo.
func TestSearch_RecuperaSkillYaAprendidaSinRedEsGarantiaEstructural(t *testing.T) {
	skills := NewSkillLibrary()
	skills.Install(&Skill{
		SkillID:        "Go",
		Label:          "A instalar goland paso a paso",
		Problem:        "A instalar goland paso a paso",
		ProcedureTitle: "Tutorial: Get started with Go",
		Steps: []string{
			"Install Go (if you haven't already).",
			"Write some simple \"Hello, world\" code.",
			"Use the go command to run your code.",
			"Call functions of an external module.",
		},
		Operations: []Operation{
			{Text: "Install Go (if you haven't already).", Status: OpUnknown},
			{Text: "Write some simple \"Hello, world\" code.", Status: OpUnknown},
			{Text: "Use the go command to run your code.", Status: OpUnknown},
			{Text: "Call functions of an external module.", Status: OpUnknown},
		},
		State:        StateProcedural,
		Sources:      []string{"Go Docs (go.dev)"},
		Corroborated: false,
	})

	s := NewSemanticSkillSearcher()
	// Search() recibe analysis.MainGoal ya en producción (ver
	// core/engine.go línea ~1064) — el trigger ("que sabes sobre") ya
	// viene descartado por Goal Analyzer antes de llegar acá, Search()
	// no lo strippea internamente. Mismo texto que quedaría del
	// comando de voz "amelia qué sabes sobre instalar go" después de
	// ese paso.
	result := s.Search("instalar go", skills)

	if !result.HasMatches() {
		t.Fatal("esperaba encontrar la Skill de Go ya guardada, sin ningún acceso a red")
	}
	found := false
	for _, m := range result.Matches {
		if m.Note != nil && m.Note.SkillID == "Go" {
			found = true
			if m.Note.ProcedureTitle != "Tutorial: Get started with Go" {
				t.Errorf("ProcedureTitle recuperado = %q, no coincide con lo guardado", m.Note.ProcedureTitle)
			}
			if len(m.Note.Steps) != 4 {
				t.Errorf("esperaba los 4 Steps guardados, obtuve %d", len(m.Note.Steps))
			}
		}
	}
	if !found {
		t.Error("el resultado no incluye la Skill \"Go\" instalada")
	}
}

// TestDescribe_VariosMatches_UsaEffectiveLabelNoElDominioCrudo
// (Integration Fix) confirma que, cuando hay más de un match, el
// listado usa EffectiveLabel() (54) — el ProcedureTitle si el texto
// fuente lo traía, no el Domain genérico. Mismo criterio que ya
// aplican Skill.Describe() y SkillContext.Describe() (55).
func TestDescribe_VariosMatches_UsaEffectiveLabelNoElDominioCrudo(t *testing.T) {
	sr := &SkillSearchResult{
		Query: "go",
		Matches: []SkillMatch{
			{Domain: "Go", Hits: 3, Note: &Skill{SkillID: "Go", Label: "Go", ProcedureTitle: "Tutorial: Get started with Go"}},
			{Domain: "Unity", Hits: 1, Note: &Skill{SkillID: "Unity", Label: "Unity"}},
		},
	}
	desc := sr.Describe()
	if !strings.Contains(desc, "Tutorial: Get started with Go (3 mención(es))") {
		t.Errorf("Describe() debería usar el ProcedureTitle para Go, obtuve: %q", desc)
	}
	if !strings.Contains(desc, "Unity (1 mención(es))") {
		t.Errorf("Describe() debería caer a Label para Unity (sin ProcedureTitle), obtuve: %q", desc)
	}
}

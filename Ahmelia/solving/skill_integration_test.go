package solving

import (
	"ahmelia/interaction"
	"strings"
	"testing"

	"ahmelia/gateway"
)

// TestIntegrate_CurriculumNilNoRompeYDevuelveContextoVacio confirma el
// criterio defensivo con Curriculum nil.
func TestIntegrate_CurriculumNilNoRompeYDevuelveContextoVacio(t *testing.T) {
	si := NewSkillIntegrator()
	discoverer := NewDiscoverer(gateway.AuthorizedSources)
	sc := si.Integrate(nil, discoverer, NewSkillLibrary())
	if sc == nil {
		t.Fatal("Integrate(nil, ...) no debe devolver nil")
	}
	if sc.Domain != "" || sc.KnownSkill != nil {
		t.Errorf("sin Curriculum no debe haber Domain ni KnownSkill, obtuve Domain=%q KnownSkill=%v", sc.Domain, sc.KnownSkill)
	}
}

// TestIntegrate_DiscovererNilNoRompe confirma el criterio defensivo
// con Discoverer nil.
func TestIntegrate_DiscovererNilNoRompe(t *testing.T) {
	si := NewSkillIntegrator()
	c := &Curriculum{Goal: "aprender unity"}
	sc := si.Integrate(c, nil, NewSkillLibrary())
	if sc.Domain != "" {
		t.Errorf("sin Discoverer no debe detectarse ningún Domain, obtuve %q", sc.Domain)
	}
	if sc.Curriculum != c {
		t.Error("Curriculum debe conservarse en el contexto aunque Discoverer sea nil")
	}
}

// TestIntegrate_SkillLibraryNilNoRompe confirma el criterio defensivo
// con SkillLibrary nil — Domain sí se detecta (Discoverer no depende
// de la biblioteca), pero KnownSkill se queda en nil.
func TestIntegrate_SkillLibraryNilNoRompe(t *testing.T) {
	si := NewSkillIntegrator()
	testSources := []gateway.AuthorizedSource{{Name: "Unity Docs", Host: "docs.unity3d.com"}}
	discoverer := NewDiscoverer(testSources)
	c := &Curriculum{Goal: "quiero aprender unity"}
	sc := si.Integrate(c, discoverer, nil)
	if sc.Domain != "Unity" {
		t.Errorf("Domain = %q, esperaba detectar Unity aunque SkillLibrary sea nil", sc.Domain)
	}
	if sc.KnownSkill != nil {
		t.Error("KnownSkill debe quedar nil si SkillLibrary es nil")
	}
}

// TestIntegrate_DominioNoReconocidoNoConsultaLaBiblioteca confirma que
// si Discovery no reconoce el dominio, ni siquiera se intenta buscar
// en Skill Library — no tiene sentido buscar con una clave vacía.
func TestIntegrate_DominioNoReconocidoNoConsultaLaBiblioteca(t *testing.T) {
	si := NewSkillIntegrator()
	discoverer := NewDiscoverer(gateway.AuthorizedSources)
	skills := NewSkillLibrary()
	skills.Install(&Skill{SkillID: "", Label: "no debería matchear nada"})

	c := &Curriculum{Goal: "quiero aprender a cocinar"}
	sc := si.Integrate(c, discoverer, skills)
	if sc.Domain != "" {
		t.Errorf("Domain debe quedar vacío para un objetivo sin dominio técnico reconocido, obtuve %q", sc.Domain)
	}
	if sc.KnownSkill != nil {
		t.Error("KnownSkill debe quedar nil si no hay Domain reconocido")
	}
}

// TestIntegrate_DominioReconocidoSinNotaGuardada confirma el caso
// intermedio: se reconoce el dominio, pero Skill Library todavía no
// tiene nada guardado sobre él.
func TestIntegrate_DominioReconocidoSinNotaGuardada(t *testing.T) {
	si := NewSkillIntegrator()
	discoverer := NewDiscoverer(gateway.AuthorizedSources)
	c := &Curriculum{Goal: "quiero aprender python"}
	sc := si.Integrate(c, discoverer, NewSkillLibrary())
	if sc.Domain != "Python" {
		t.Fatalf("Domain = %q, esperaba Python", sc.Domain)
	}
	if sc.HasKnownSkill() {
		t.Error("HasKnownSkill debe ser false sin ninguna nota guardada")
	}
}

// TestIntegrate_DominioReconocidoConNotaGuardadaLaEncuentra es el
// camino feliz completo: dominio reconocido Y nota guardada bajo esa
// misma clave — el caso que Bloque 55 existe para resolver.
func TestIntegrate_DominioReconocidoConNotaGuardadaLaEncuentra(t *testing.T) {
	si := NewSkillIntegrator()
	discoverer := NewDiscoverer(gateway.AuthorizedSources)
	skills := NewSkillLibrary()
	skills.Install(&Skill{
		SkillID:      "Unreal Engine",
		Label:        "Unreal Engine",
		Content:      "contenido real",
		Sources:      []string{"Epic Developer Docs"},
		Corroborated: true,
	})

	c := &Curriculum{Goal: "quiero hacer un juego en unreal engine"}
	sc := si.Integrate(c, discoverer, skills)
	if sc.Domain != "Unreal Engine" {
		t.Fatalf("Domain = %q, esperaba Unreal Engine", sc.Domain)
	}
	if !sc.HasKnownSkill() {
		t.Fatal("esperaba encontrar la nota guardada bajo la clave Unreal Engine")
	}
	if sc.KnownSkill.Label != "Unreal Engine" {
		t.Errorf("KnownSkill.Label = %q, esperaba Unreal Engine", sc.KnownSkill.Label)
	}
}

// TestIntegrate_ClaveNoMatcheaPorMayusculasOTextoDistinto confirma que
// la búsqueda es por clave EXACTA (Domain tal cual lo arma
// Discoverer) — una nota guardada con una clave distinta (ej. texto
// libre de una versión anterior del wiring) NO debe aparecer como
// encontrada por accidente.
func TestIntegrate_ClaveNoMatcheaPorMayusculasOTextoDistinto(t *testing.T) {
	si := NewSkillIntegrator()
	discoverer := NewDiscoverer(gateway.AuthorizedSources)
	skills := NewSkillLibrary()
	// Guardada con una clave de texto libre, NO con el Domain real
	// ("Unity") — simula el bug que Bloque 55 corrigió en el wiring
	// de Bloque 54 (usar analysis.MainGoal en vez de discovery.Domain).
	skills.Install(&Skill{SkillID: "quiero aprender unity para hacer un juego", Label: "unity"})

	c := &Curriculum{Goal: "quiero aprender unity para hacer un juego"}
	sc := si.Integrate(c, discoverer, skills)
	if sc.Domain != "Unity" {
		t.Fatalf("Domain = %q, esperaba Unity", sc.Domain)
	}
	if sc.HasKnownSkill() {
		t.Error("no debe encontrar la nota — está guardada bajo una clave de texto libre, no bajo el Domain real 'Unity'")
	}
}

// TestDescribe_SinDominioReconocido confirma la narrativa cuando no
// hay dominio técnico reconocido en absoluto.
func TestDescribe_SkillContext_SinDominioReconocido(t *testing.T) {
	sc := &SkillContext{Goal: "aprender a cocinar", Curriculum: &Curriculum{Goal: "aprender a cocinar"}}
	desc := sc.Describe()
	if !strings.Contains(desc, "No reconozco un dominio técnico conocido") {
		t.Errorf("Describe() = %q", desc)
	}
}

// TestDescribe_DominioSinNota confirma la narrativa del caso
// intermedio.
func TestDescribe_SkillContext_DominioSinNota(t *testing.T) {
	sc := &SkillContext{Goal: "aprender python", Domain: "Python", Curriculum: &Curriculum{Goal: "aprender python"}}
	desc := sc.Describe()
	if !strings.Contains(desc, "Detecté el dominio Python") || !strings.Contains(desc, "todavía no tengo ninguna nota") {
		t.Errorf("Describe() = %q", desc)
	}
}

// TestDescribe_DominioConNotaNoOmiteElCurriculum confirma que, aunque
// haya una nota guardada, el curriculum SIGUE apareciendo completo en
// la narrativa — el límite honesto central del bloque: la nota no
// reemplaza ni oculta los gaps genuinos.
func TestDescribe_SkillContext_DominioConNotaNoOmiteElCurriculum(t *testing.T) {
	curriculum := &Curriculum{
		Goal: "aprender unreal engine",
		Items: []LearningItem{
			{TaskID: "T1", TaskDesc: "Implementar la lógica de juego", Reason: "sin pista ni antecedente", Priority: 2},
		},
	}
	sc := &SkillContext{
		Goal:       "aprender unreal engine",
		Domain:     "Unreal Engine",
		KnownSkill: &Skill{Label: "Unreal Engine", Sources: []string{"Epic Developer Docs"}, Corroborated: true},
		Curriculum: curriculum,
	}
	desc := sc.Describe()
	if !strings.Contains(desc, "Detecté el dominio Unreal Engine") {
		t.Errorf("Describe() no menciona el dominio detectado: %q", desc)
	}
	if !strings.Contains(desc, "no reduce los gaps genuinos del plan") {
		t.Errorf("Describe() no incluye el límite honesto explícito: %q", desc)
	}
	if !strings.Contains(desc, "Implementar la lógica de juego") {
		t.Errorf("Describe() debe seguir listando los items del curriculum sin ocultarlos: %q", desc)
	}
}

// TestDescribe_NotaSinCorroborarSeDistingue confirma que la narrativa
// distingue una nota corroborada de una que no lo está — no las trata
// como si fueran la misma confiabilidad.
func TestDescribe_SkillContext_NotaSinCorroborarSeDistingue(t *testing.T) {
	sc := &SkillContext{
		Goal:       "aprender go",
		Domain:     "Go",
		KnownSkill: &Skill{Label: "Go", Sources: []string{"Go Docs"}, Corroborated: false},
		Curriculum: &Curriculum{Goal: "aprender go"},
	}
	desc := sc.Describe()
	if !strings.Contains(desc, "sin corroborar") {
		t.Errorf("Describe() debe decir 'sin corroborar' para una nota no corroborada: %q", desc)
	}
}

// TestDescribe_SkillContext_ProcedimientoConOperationsTotalmenteReconocidas
// (Integration Fix, Bloque 55) confirma que cuando TODAS las
// Operations de un procedimiento son Wired, la narrativa lo dice sin
// ambigüedad — la distinción "conocimiento vs. capacidad respaldada"
// que motivó el rediseño del 54, ahora visible también al integrar
// con el curriculum.
func TestDescribe_SkillContext_ProcedimientoConOperationsTotalmenteReconocidas(t *testing.T) {
	sc := &SkillContext{
		Goal:   "aprender go",
		Domain: "Go",
		KnownSkill: &Skill{
			Label: "Go", Steps: []string{"Abrir el programa"}, State: StateProcedural,
			Operations: []Operation{{Text: "Abrir el programa", Action: "abrir_programa", Status: OpWired}},
			Sources:    []string{"Go Docs"},
		},
		Curriculum: &Curriculum{Goal: "aprender go"},
	}
	desc := sc.Describe()
	if !strings.Contains(desc, "ya están todas reconocidas") {
		t.Errorf("Describe() debería confirmar cobertura total de Operations: %q", desc)
	}
}

// TestDescribe_SkillContext_ProcedimientoSinNingunaOperationReconocida
// (Integration Fix, Bloque 55) confirma el caso KnowledgeOnly: hay
// procedimiento, pero cero Operations reconocidas — la narrativa debe
// decir explícitamente que Ahmelia no puede ejecutar nada de esto
// todavía, sin insinuar lo contrario.
func TestDescribe_SkillContext_ProcedimientoSinNingunaOperationReconocida(t *testing.T) {
	sc := &SkillContext{
		Goal:   "aprender go",
		Domain: "Go",
		KnownSkill: &Skill{
			Label: "Go", Steps: []string{"Install Go"}, State: StateProcedural,
			Operations: []Operation{{Text: "Install Go", Action: "", Status: OpUnknown}},
			Sources:    []string{"Go Docs"},
		},
		Curriculum: &Curriculum{Goal: "aprender go"},
	}
	desc := sc.Describe()
	if !strings.Contains(desc, "Ninguna de sus operaciones está reconocida") {
		t.Errorf("Describe() debería aclarar que no hay ninguna Operation reconocida: %q", desc)
	}
}

// TestDescribe_SkillContext_ProcedimientoConOperationsParcialmenteReconocidas
// (Integration Fix, Bloque 55) confirma el caso intermedio: reporta la
// proporción real (wired/total), sin redondear para arriba ni para
// abajo.
func TestDescribe_SkillContext_ProcedimientoConOperationsParcialmenteReconocidas(t *testing.T) {
	sc := &SkillContext{
		Goal:   "aprender unity",
		Domain: "Unity",
		KnownSkill: &Skill{
			Label: "Unity", Steps: []string{"Abrir Unity", "Crear proyecto"}, State: StateProcedural,
			Operations: []Operation{
				{Text: "Abrir Unity", Action: "abrir_programa", Status: OpWired},
				{Text: "Crear proyecto", Action: "", Status: OpUnknown},
			},
			Sources: []string{"Unity Docs"},
		},
		Curriculum: &Curriculum{Goal: "aprender unity"},
	}
	desc := sc.Describe()
	if !strings.Contains(desc, "1 de 2 operación(es)") {
		t.Errorf("Describe() debería reportar la proporción exacta 1 de 2: %q", desc)
	}
}

// TestDescribe_SkillContext_SinProcedimientoNoMencionaOperations
// (Integration Fix, Bloque 55) confirma que una Skill sin Steps (solo
// conocimiento, o solo condición de uso) no arma ninguna frase de
// Operations — no hay nada que reportar, y no debe inventarse una.
func TestDescribe_SkillContext_SinProcedimientoNoMencionaOperations(t *testing.T) {
	sc := &SkillContext{
		Goal:       "aprender python",
		Domain:     "Python",
		KnownSkill: &Skill{Label: "Python", Sources: []string{"Python Docs"}},
		Curriculum: &Curriculum{Goal: "aprender python"},
	}
	desc := sc.Describe()
	if strings.Contains(desc, "operación(es)") {
		t.Errorf("Describe() no debería mencionar Operations sin ningún Step extraído: %q", desc)
	}
}

// TestParseSkillContextQuery_NoSeConfundeConCurriculumNiConSkillBuilder
// confirma que los tres triggers de Fase 4/4.1 relacionados con
// aprendizaje (Curriculum crudo, Skill Library sola, Curriculum +
// Skill Library) son mutuamente excluyentes.
func TestParseSkillContextQuery_NoSeConfundeConCurriculumNiConSkillBuilder(t *testing.T) {
	contextOnly := "que te falta aprender realmente para hacer un juego"
	if !ParseSkillContextQuery(interaction.Analyze(contextOnly, interaction.Context{})) {
		t.Errorf("ParseSkillContextQuery debería detectar %q", contextOnly)
	}
	if ParseCurriculumQuery(interaction.Analyze(contextOnly, interaction.Context{})) {
		t.Errorf("ParseCurriculumQuery NO debería disparar con %q", contextOnly)
	}
	if ParseSkillBuilderQuery(interaction.Analyze(contextOnly, interaction.Context{})) {
		t.Errorf("ParseSkillBuilderQuery NO debería disparar con %q", contextOnly)
	}
	if ParseSkillBuildQuery(interaction.Analyze(contextOnly, interaction.Context{})) {
		t.Errorf("ParseSkillBuildQuery NO debería disparar con %q", contextOnly)
	}

	curriculumOnly := "que necesito aprender para hacer un juego"
	if ParseSkillContextQuery(interaction.Analyze(curriculumOnly, interaction.Context{})) {
		t.Errorf("ParseSkillContextQuery NO debería disparar con %q", curriculumOnly)
	}
	if !ParseCurriculumQuery(interaction.Analyze(curriculumOnly, interaction.Context{})) {
		t.Errorf("ParseCurriculumQuery debería detectar %q", curriculumOnly)
	}
}

// TestHasKnownSkill_NilEsFalse confirma el atajo booleano defensivo.
func TestHasKnownSkill_NilEsFalse(t *testing.T) {
	sc := &SkillContext{}
	if sc.HasKnownSkill() {
		t.Error("HasKnownSkill debe ser false con KnownSkill nil")
	}
}

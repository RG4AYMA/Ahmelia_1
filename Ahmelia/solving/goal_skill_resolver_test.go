package solving

import (
	"strings"
	"testing"
)

// goResolverGoSkill — mismo fixture de Go usado en producción (ver
// semantic_skill_search_test.go), reusado acá para no inventar datos
// nuevos por cada archivo de test.
//
// Operations se calcula explícitamente con detectOperations(Steps) —
// a diferencia de Build() (54), armar un &Skill{} a mano NO llena
// Operations solo (no hay ningún hook automático desde Steps), y
// CapabilityCoverage() lee Operations, no Steps. Sin esto, cualquier
// fixture armado a mano quedaría con total=0 siempre, sin importar
// cuántos Steps tenga.
func goResolverGoSkill() *Skill {
	steps := []string{
		"Install Go (if you haven't already).",
		"Write some simple \"Hello, world\" code.",
		"Use the go command to run your code.",
		"Call functions of an external module.",
	}
	return &Skill{
		SkillID:        "Go",
		Label:          "Tutorial: Get started with Go",
		ProcedureTitle: "Tutorial: Get started with Go",
		Steps:          steps,
		Operations:     detectOperations(steps),
		Sources:        []string{"Go Docs (go.dev)"},
		State:          StateProcedural,
	}
}

func TestResolveGoal_AccionLocalWiredNoNecesitaSkill(t *testing.T) {
	skills := NewSkillLibrary()
	res := ResolveGoal("abre chrome", skills)
	if res.Kind != KindLocalAction {
		t.Fatalf("esperaba KindLocalAction, dio %s", res.Kind)
	}
	if res.LocalAction != "abrir_programa" {
		t.Errorf("LocalAction = %q, esperaba abrir_programa", res.LocalAction)
	}
}

func TestResolveGoal_AccionLocalWiredResuelveComoLocalAction(t *testing.T) {
	// La acción tiene trigger, capability y binding real; el resolutor debe
	// conservar ese ownership y no degradarla a una skill aprendida.
	skills := NewSkillLibrary()
	res := ResolveGoal("renombra el archivo", skills)
	if res.Kind != KindLocalAction {
		t.Fatalf("renombrar_archivo debería resolver como KindLocalAction, dio %v", res.Kind)
	}
}

func TestResolveGoal_ProceduralUnbackedCuandoNingunaOperationEstaWired(t *testing.T) {
	// Los Steps del tutorial de Go están en inglés y no matchean
	// ningún ActionPattern en español ni ningún verbo de actions/* —
	// misma honestidad que ya demostró producción (Operations: Unknown).
	skills := NewSkillLibrary()
	skills.Install(goResolverGoSkill())

	res := ResolveGoal("instala go", skills)
	if res.Kind != KindProceduralUnbacked {
		t.Fatalf("esperaba KindProceduralUnbacked, dio %s (wired=%d total=%d)", res.Kind, res.WiredOps, res.TotalOps)
	}
	if res.Skill == nil || res.Skill.SkillID != "Go" {
		t.Fatalf("esperaba resolver a la Skill de Go")
	}
}

func TestResolveGoal_ProceduralFullCuandoTodasLasOperationsEstanWired(t *testing.T) {
	// "crea" está en actionVerbs, pero la frase completa NO contiene
	// "crea carpeta" ni "nueva carpeta" (los únicos triggers de
	// crear_carpeta) — el Paso 1 (acción local directa) no se dispara,
	// y el caso ejercita la resolución real vía Skill + Operations.
	steps := []string{"abre el editor", "sube el volumen"} // ambos matchean acciones Wired
	skill := &Skill{
		SkillID:    "PrepararEntorno",
		Label:      "Crear un entorno de trabajo",
		Steps:      steps,
		Operations: detectOperations(steps),
		State:      StateProcedural,
		Sources:    []string{"Nota local"},
	}
	skills := NewSkillLibrary()
	skills.Install(skill)

	res := ResolveGoal("crea un entorno de trabajo", skills)
	if res.Kind != KindProceduralFull {
		t.Fatalf("esperaba KindProceduralFull, dio %s (wired=%d total=%d)", res.Kind, res.WiredOps, res.TotalOps)
	}
}

func TestResolveGoal_KnowledgeOnlyCuandoLaSkillNoTieneProcedimiento(t *testing.T) {
	skill := &Skill{
		SkillID: "Unity",
		Label:   "Unity",
		Content: "Unity es un motor para crear videojuegos.",
		State:   "",
		Sources: []string{"Unity Docs"},
	}
	skills := NewSkillLibrary()
	skills.Install(skill)

	res := ResolveGoal("instala unity", skills)
	if res.Kind != KindKnowledgeOnly {
		t.Fatalf("esperaba KindKnowledgeOnly, dio %s", res.Kind)
	}
}

func TestResolveGoal_UnknownCuandoNoHayNadaGuardado(t *testing.T) {
	skills := NewSkillLibrary()
	res := ResolveGoal("instala blender", skills)
	if res.Kind != KindUnknown {
		t.Fatalf("esperaba KindUnknown, dio %s", res.Kind)
	}
}

func TestResolveGoal_AmbiguousConMasDeUnMatchNoElige(t *testing.T) {
	a := &Skill{SkillID: "UnityBasico", Label: "Unity", Content: "Unity Unity Unity", State: "", Sources: []string{"Doc A"}}
	b := &Skill{SkillID: "UnityAvanzado", Label: "Unity avanzado", Content: "Unity Unity", State: "", Sources: []string{"Doc B"}}
	skills := NewSkillLibrary()
	skills.Install(a)
	skills.Install(b)

	res := ResolveGoal("instala unity", skills)
	if res.Kind != KindAmbiguous {
		t.Fatalf("esperaba KindAmbiguous, dio %s", res.Kind)
	}
	if len(res.Ambiguous) != 2 {
		t.Errorf("esperaba 2 matches ambiguos, dio %d", len(res.Ambiguous))
	}
}

func TestParseGoalCapabilityQuery_ReconoceVerboImperativoAlPrincipio(t *testing.T) {
	cases := map[string]bool{
		"instala go":                 true,
		"crea una carpeta proyectos": true,
		"abre chrome":                true,
		"que sabes sobre go":         false,
		"como instalo go":            false, // le corresponde al 61 (Skill Procedure), no a este bloque
		"el procedimiento de go":     false,
		// Límite honesto conocido: actionVerbs solo cubre imperativo
		// ("instala"), no infinitivo — "quiero instalar go" (lo que
		// queda después de stripFillers("quiero ")) NO matchea hoy.
		// No es un bug de este test, es lo mismo que ya documenta 54
		// para startsWithActionVerb — se deja como deuda conocida en
		// vez de fingir que el heurístico cubre conjugación.
		"quiero instalar go": false,
	}
	for text, want := range cases {
		got := ParseGoalCapabilityQuery(text)
		if got != want {
			t.Errorf("ParseGoalCapabilityQuery(%q) = %v, esperaba %v", text, got, want)
		}
	}
}

func TestGoalResolution_DescribeNuncaAfirmaHaberEjecutado(t *testing.T) {
	skills := NewSkillLibrary()
	skills.Install(goResolverGoSkill())
	res := ResolveGoal("instala go", skills)
	desc := res.Describe()
	if desc == "" {
		t.Fatal("Describe() no debería devolver vacío")
	}
	// Límite honesto central del bloque: nunca debe sonar a que ya
	// pasó algo — solo a que Ahmelia SABE o PUEDE, nunca "hice".
	forbidden := []string{"ejecuté", "ejecute", "hice", "instalé", "instale"}
	lowerDesc := strings.ToLower(desc)
	for _, f := range forbidden {
		if strings.Contains(lowerDesc, f) {
			t.Errorf("Describe() no debería sonar a ejecución ya realizada, contiene %q: %s", f, desc)
		}
	}
}

// TestGoalResolution_DescribeMencionaLimitationsAntesDeOfrecerEjecutar
// (Integration Fix, Skill Model Refinement) confirma que, si la Skill
// declara una limitación de alcance conocida, aparece en la narrativa
// junto con la oferta de ejecutar — no se calla algo que la fuente
// dijo explícitamente.
func TestGoalResolution_DescribeMencionaLimitationsAntesDeOfrecerEjecutar(t *testing.T) {
	skill := goResolverGoSkill()
	skill.Limitations = []string{"no funciona en Windows 7"}
	skills := NewSkillLibrary()
	skills.Install(skill)

	desc := ResolveGoal("instala go", skills).Describe()
	if !strings.Contains(desc, "no funciona en Windows 7") {
		t.Errorf("Describe() debería mencionar la limitación conocida, obtuve: %q", desc)
	}
}

// TestGoalResolution_DescribeDesglosaCadaOperacionIndividual (Bloque
// 64 — Capability Resolver) confirma el hallazgo real: Describe() ya
// daba el agregado ("2 de 4 operación(es)") pero nunca el desglose por
// operación individual que sí necesitaba mostrarse — reproduce
// exactamente el ejemplo de "Organizar archivos" (listar/detectar/
// crear/mover), con 3 Wired y 1 Named.
func TestGoalResolution_DescribeDesglosaCadaOperacionIndividual(t *testing.T) {
	steps := []string{"Listar archivos", "Detectar extensiones", "Crear carpetas", "Mover archivos"}
	skill := &Skill{
		SkillID: "OrganizarArchivos", Label: "Organizar archivos", ProcedureTitle: "Organizar archivos por extensión",
		Steps: steps, State: StateProcedural, Sources: []string{"Guía interna"},
		Operations: []Operation{
			{Text: "Listar archivos", Action: "listar_archivos", Status: OpWired},
			{Text: "Detectar extensiones", Action: "detectar_extension", Status: OpNamed},
			{Text: "Crear carpetas", Action: "crear_carpeta", Status: OpWired},
			{Text: "Mover archivos", Action: "mover_archivo", Status: OpWired},
		},
	}
	skills := NewSkillLibrary()
	skills.Install(skill)

	desc := ResolveGoal("organizar archivos", skills).Describe()

	for _, want := range []string{
		"3 de 4 operación(es)",
		"Listar archivos → reconocida, con acción conectada (Wired) — sin clasificar en seguridad, pediría confirmación igual",
		"Detectar extensiones → reconocida por nombre, sin acción conectada (Named)",
		"Crear carpetas → reconocida, con acción conectada (Wired, riesgo: safe)",
		"Mover archivos → reconocida, con acción conectada (Wired, riesgo: reversible)",
	} {
		if !strings.Contains(desc, want) {
			t.Errorf("Describe() no contiene %q — respuesta completa: %q", want, desc)
		}
	}
}

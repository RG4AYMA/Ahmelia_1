package solving

import (
	"strings"
	"testing"

	"ahmelia/gateway"
	"ahmelia/shared"
)

// buildFrom es un atajo para armar un ValidationSet con un único
// documento confiable a partir de texto plano — mismo criterio que
// docStub() en skill_update_detector_test.go, para no repetir el
// boilerplate de ValidationSet en cada test de extracción.
func buildFrom(t *testing.T, taskID, text string) *Skill {
	t.Helper()
	vs := &ValidationSet{
		Goal: taskID,
		Validations: []*SourceValidation{
			{
				Document:   &gateway.Document{URL: "https://example.com/", Source: "Fuente Test", Text: text, StatusCode: 200},
				StatusOK:   true,
				HasContent: true,
			},
		},
	}
	sb := NewSkillBuilder()
	skill := sb.Build(LearningItem{TaskID: taskID, TaskDesc: taskID}, vs)
	if skill == nil {
		t.Fatal("esperaba una Skill armada, obtuve nil")
	}
	return skill
}

// TestBuild_ProsaSinEstructuraReconocibleQuedaSinCampos confirma el
// límite honesto central del rediseño: texto narrativo sin
// frases-gatillo ni listas no INVENTA WhenToUse/Steps — los campos
// quedan vacíos y la Skill se persiste igual (el umbral de
// persistencia sigue siendo solo "hubo un documento confiable", ver
// nota de diseño en skill_builder.go).
func TestBuild_ProsaSinEstructuraReconocibleQuedaSinCampos(t *testing.T) {
	skill := buildFrom(t, "unity", strings.Repeat("Unity es un motor de videojuegos ampliamente usado en la industria. ", 5))

	if len(skill.WhenToUse) != 0 {
		t.Errorf("WhenToUse debería quedar vacío sin frases-gatillo, obtuve %v", skill.WhenToUse)
	}
	if skill.HasProcedure() {
		t.Errorf("HasProcedure debería ser false sin lista reconocible, Steps=%v", skill.Steps)
	}
	if skill.State != "" {
		t.Errorf("State debería quedar en \"\" (sin clasificación) cuando no se extrajo nada, obtuve %q", skill.State)
	}
}

// TestBuild_ExtraeWhenToUse confirma que una frase-gatillo real de
// condición de uso se extrae y clasifica la Skill como Structured.
func TestBuild_ExtraeWhenToUse(t *testing.T) {
	text := "Usa esto cuando necesites levantar muchos objetos que aparecen y desaparecen seguido."
	skill := buildFrom(t, "object-pooling", text)

	if len(skill.WhenToUse) != 1 {
		t.Fatalf("esperaba 1 condición de uso extraída, obtuve %v", skill.WhenToUse)
	}
	if !strings.Contains(skill.WhenToUse[0], "cuando necesites") {
		t.Errorf("WhenToUse[0] = %q, no conserva la frase original", skill.WhenToUse[0])
	}
	if skill.State != StateStructured {
		t.Errorf("State = %q, esperaba %q (hay WhenToUse pero no Steps)", skill.State, StateStructured)
	}
	if skill.HasProcedure() {
		t.Error("HasProcedure debería ser false sin lista de pasos")
	}
}

// TestBuild_ExtraePasosDeListaNumerada confirma el camino feliz de
// Steps: un documento con una lista numerada real produce Steps
// pobladas y State=Procedural.
func TestBuild_ExtraePasosDeListaNumerada(t *testing.T) {
	text := "Para instalar Unity, segui estos pasos:\n" +
		"1. Descarga el instalador desde unity.com\n" +
		"2. Ejecuta el instalador y segui las instrucciones\n" +
		"3. Abri Unity Hub y crea un proyecto nuevo\n"
	skill := buildFrom(t, "instalar-unity", text)

	if len(skill.Steps) != 3 {
		t.Fatalf("esperaba 3 pasos extraídos, obtuve %v", skill.Steps)
	}
	if skill.Steps[0] != "Descarga el instalador desde unity.com" {
		t.Errorf("Steps[0] = %q, no coincide con el texto del primer paso (sin el número)", skill.Steps[0])
	}
	if !skill.HasProcedure() {
		t.Error("HasProcedure debería ser true con 3 pasos extraídos")
	}
	if skill.State != StateProcedural {
		t.Errorf("State = %q, esperaba %q", skill.State, StateProcedural)
	}
}

// TestBuild_UnaSolaLineaConFormaDePasoNoCuentaComoProcedimiento
// confirma el umbral documentado: una única línea que matchee el
// patrón de paso (ej. una referencia numerada aislada, no una
// secuencia) no es evidencia de un procedimiento real — se exigen al
// menos 2 líneas con esa forma.
func TestBuild_UnaSolaLineaConFormaDePasoNoCuentaComoProcedimiento(t *testing.T) {
	text := "Documentación general sobre el framework.\n" +
		"1. Este es el único punto mencionado con forma de lista en todo el texto.\n" +
		"Más texto narrativo después, sin ninguna otra línea numerada."
	skill := buildFrom(t, "framework-x", text)

	if skill.HasProcedure() {
		t.Errorf("una sola línea con forma de paso no debería contar como procedimiento, Steps=%v", skill.Steps)
	}
}

// TestBuild_ExtraePitfalls confirma que una advertencia explícita se
// extrae como Pitfall.
func TestBuild_ExtraePitfalls(t *testing.T) {
	text := "No uses Object Pooling para pocos objetos, no vale la pena la complejidad extra."
	skill := buildFrom(t, "object-pooling", text)

	if len(skill.Pitfalls) != 1 {
		t.Fatalf("esperaba 1 pitfall extraído, obtuve %v", skill.Pitfalls)
	}
	if !strings.Contains(skill.Pitfalls[0], "No uses Object Pooling") {
		t.Errorf("Pitfalls[0] = %q, no conserva la advertencia original", skill.Pitfalls[0])
	}
}

// TestBuild_ExtraePrerequisites confirma que un prerrequisito
// mencionado explícitamente se extrae.
func TestBuild_ExtraePrerequisites(t *testing.T) {
	text := "Necesitas tener instalado el SDK de Unity antes de continuar con este tutorial."
	skill := buildFrom(t, "unity-sdk", text)

	if len(skill.Prerequisites) != 1 {
		t.Fatalf("esperaba 1 prerrequisito extraído, obtuve %v", skill.Prerequisites)
	}
	if !strings.Contains(skill.Prerequisites[0], "SDK de Unity") {
		t.Errorf("Prerequisites[0] = %q, no conserva el prerrequisito original", skill.Prerequisites[0])
	}
}

// TestBuild_ExtraeFailureRecovery confirma que una frase de
// recuperación explícita ("si falla X, hacé Y") se extrae en
// FailureRecovery — distinto de Pitfalls, que es sobre qué NO hacer.
func TestBuild_ExtraeFailureRecovery(t *testing.T) {
	text := "Si falla el proceso, reinicia el editor y volve a intentar la operacion."
	skill := buildFrom(t, "editor-x", text)

	if len(skill.FailureRecovery) != 1 {
		t.Fatalf("esperaba 1 recuperación extraída, obtuve %v", skill.FailureRecovery)
	}
	if !strings.Contains(skill.FailureRecovery[0], "reinicia el editor") {
		t.Errorf("FailureRecovery[0] = %q, no conserva la frase original", skill.FailureRecovery[0])
	}
	if len(skill.Pitfalls) != 0 {
		t.Errorf("Pitfalls debería seguir vacío — esto es recuperación, no advertencia, obtuve %v", skill.Pitfalls)
	}
}

// TestBuild_ExtraeExpectedOutcome confirma que un criterio de éxito
// explícito se extrae.
func TestBuild_ExtraeExpectedOutcome(t *testing.T) {
	text := "Al finalizar deberias ver el objeto con el material aplicado en el visor 3D."
	skill := buildFrom(t, "aplicar-material", text)

	if len(skill.ExpectedOutcome) != 1 {
		t.Fatalf("esperaba 1 resultado esperado extraído, obtuve %v", skill.ExpectedOutcome)
	}
	if !strings.Contains(skill.ExpectedOutcome[0], "material aplicado") {
		t.Errorf("ExpectedOutcome[0] = %q, no conserva la frase original", skill.ExpectedOutcome[0])
	}
}

// TestBuild_SinFrasesDeResultadoQuedaVacio confirma que, como con el
// resto de los campos heurísticos, la ausencia de una frase-gatillo
// explícita deja ExpectedOutcome/FailureRecovery vacíos — no se
// inventa un criterio de éxito ni una recuperación que el texto no
// menciona.
func TestBuild_SinFrasesDeResultadoQuedaVacio(t *testing.T) {
	text := strings.Repeat("Blender es un programa de modelado 3D de código abierto. ", 5)
	skill := buildFrom(t, "blender-intro", text)

	if len(skill.ExpectedOutcome) != 0 {
		t.Errorf("ExpectedOutcome debería quedar vacío sin frase-gatillo, obtuve %v", skill.ExpectedOutcome)
	}
	if len(skill.FailureRecovery) != 0 {
		t.Errorf("FailureRecovery debería quedar vacío sin frase-gatillo, obtuve %v", skill.FailureRecovery)
	}
}

// TestBuild_ProblemEsElTaskDescDelLearningItem confirma la decisión
// documentada: sin NLP real, Problem es honestamente el mismo
// TaskDesc que Label — no se fabrica una reformulación.
func TestBuild_ProblemEsElTaskDescDelLearningItem(t *testing.T) {
	vs := &ValidationSet{
		Goal: "unity",
		Validations: []*SourceValidation{
			{Document: &gateway.Document{Source: "Fuente Test", Text: "contenido de sobra sobre unity para pasar el umbral.", StatusCode: 200}, StatusOK: true, HasContent: true},
		},
	}
	sb := NewSkillBuilder()
	skill := sb.Build(LearningItem{TaskID: "unity", TaskDesc: "Implementar la lógica de juego en Unity"}, vs)
	if skill == nil {
		t.Fatal("esperaba una Skill armada")
	}
	if skill.Problem != skill.Label || skill.Problem != "Implementar la lógica de juego en Unity" {
		t.Errorf("Problem = %q, Label = %q — esperaba que ambos fueran el TaskDesc tal cual", skill.Problem, skill.Label)
	}
}

// TestBuild_StepsYWhenToUseJuntosDanProcedural confirma que cuando el
// documento trae AMBAS señales, State prioriza Procedural (más
// estructura extraída gana sobre Structured).
func TestBuild_StepsYWhenToUseJuntosDanProcedural(t *testing.T) {
	text := "Usa esto cuando necesites optimizar el spawneo de enemigos.\n" +
		"1. Crea un pool de objetos inactivos\n" +
		"2. Reutiliza instancias en vez de instanciar de nuevo\n"
	skill := buildFrom(t, "object-pooling", text)

	if skill.State != StateProcedural {
		t.Errorf("State = %q, esperaba %q cuando hay Steps y WhenToUse a la vez", skill.State, StateProcedural)
	}
	if len(skill.WhenToUse) != 1 {
		t.Errorf("WhenToUse debería seguir poblado aunque State sea Procedural, obtuve %v", skill.WhenToUse)
	}
}

// TestBuild_ExtraeProcedureTitleDeLineaConDosPuntos confirma la
// respuesta heurística a la crítica de "Label a nivel de dominio":
// cuando la línea justo antes de la lista termina en ":", se toma
// como el nombre de la tarea concreta que esos Steps resuelven.
func TestBuild_ExtraeProcedureTitleDeLineaConDosPuntos(t *testing.T) {
	text := "Para instalar Unity, segui estos pasos:\n" +
		"1. Descarga el instalador desde unity.com\n" +
		"2. Ejecuta el instalador y segui las instrucciones\n"
	skill := buildFrom(t, "instalar-unity", text)

	if skill.ProcedureTitle != "Para instalar Unity, segui estos pasos" {
		t.Errorf("ProcedureTitle = %q, no coincide con la línea introductoria (sin el prefijo de fuente ni los dos puntos)", skill.ProcedureTitle)
	}
	if skill.EffectiveLabel() != skill.ProcedureTitle {
		t.Errorf("EffectiveLabel() = %q, debería preferir ProcedureTitle sobre Label cuando existe", skill.EffectiveLabel())
	}
}

// TestBuild_ProcedureTitleQuedaVacioSinDosPuntos confirma que NO se
// adivina un título de la línea anterior si no termina en ":" — fiel
// a la fuente en vez de asumir que cualquier línea previa es el
// título.
func TestBuild_ProcedureTitleQuedaVacioSinDosPuntos(t *testing.T) {
	text := "Instalación de Unity\n" +
		"1. Descarga el instalador desde unity.com\n" +
		"2. Ejecuta el instalador y segui las instrucciones\n"
	skill := buildFrom(t, "instalar-unity", text)

	if skill.ProcedureTitle != "" {
		t.Errorf("ProcedureTitle debería quedar vacío sin ':' al final de la línea previa, obtuve %q", skill.ProcedureTitle)
	}
	if skill.EffectiveLabel() != skill.Label {
		t.Errorf("EffectiveLabel() debería caer a Label cuando no hay ProcedureTitle, obtuve %q", skill.EffectiveLabel())
	}
}

// TestBuild_ProcedureTitleQuedaVacioSinSteps confirma que sin Steps
// reales, ProcedureTitle ni siquiera se intenta extraer — no hay
// "tarea concreta" que nombrar si no hay procedimiento detectado.
func TestBuild_ProcedureTitleQuedaVacioSinSteps(t *testing.T) {
	text := "Introducción general:\n" + strings.Repeat("Unity es un motor de videojuegos. ", 5)
	skill := buildFrom(t, "unity", text)

	if skill.ProcedureTitle != "" {
		t.Errorf("ProcedureTitle debería quedar vacío sin Steps, obtuve %q", skill.ProcedureTitle)
	}
}

// TestBuild_TablaDeContenidosNoSeConfundeConProcedimiento (Integration
// Fix) reproduce el hallazgo real contra docs.python.org: una tabla
// de contenidos con numeración jerárquica ("2.1.1. Argument Passing")
// NO debe convertirse en Steps, aunque tenga forma de lista con más
// de 2 líneas.
func TestBuild_TablaDeContenidosNoSeConfundeConProcedimiento(t *testing.T) {
	text := "1. Whetting Your Appetite\n" +
		"2. Using the Python Interpreter\n" +
		"2.1. Invoking the Interpreter\n" +
		"2.1.1. Argument Passing\n" +
		"3. An Informal Introduction to Python\n"
	skill := buildFrom(t, "python", text)

	if skill.HasProcedure() {
		t.Errorf("una tabla de contenidos con numeración jerárquica no debería contar como procedimiento, Steps=%v", skill.Steps)
	}
}

// TestBuild_ListaDeNavegacionSinVerbosNiContextoNoCuentaComoProcedimiento
// (Integration Fix) reproduce el otro hallazgo real: enlaces de
// navegación del sitio ("Report a bug", "Improve this page") tienen
// forma de lista plana (bullets, no numeración jerárquica) pero no
// arrancan con ningún verbo imperativo y no tienen una línea de
// contexto terminada en ":" antes — sin esas dos evidencias, no
// deberían contar como procedimiento.
func TestBuild_ListaDeNavegacionSinVerbosNiContextoNoCuentaComoProcedimiento(t *testing.T) {
	text := "Documentación del proyecto.\n" +
		"- Report a bug\n" +
		"- Improve this page\n" +
		"- Python Tutorial\n"
	skill := buildFrom(t, "python", text)

	if skill.HasProcedure() {
		t.Errorf("una lista de navegación sin verbos ni contexto no debería contar como procedimiento, Steps=%v", skill.Steps)
	}
}

// TestBuild_ListaConContextoDosPuntosCuentaComoProcedimientoAunqueNoTengaVerbos
// (Integration Fix) confirma la primera evidencia aceptada: una línea
// introductoria terminada en ":" alcanza por sí sola, aunque los
// ítems no arranquen con un verbo imperativo.
func TestBuild_ListaConContextoDosPuntosCuentaComoProcedimientoAunqueNoTengaVerbos(t *testing.T) {
	text := "Para este tutorial vas a necesitar lo siguiente:\n" +
		"- El instalador ya descargado\n" +
		"- Una conexión a internet estable\n"
	skill := buildFrom(t, "requisitos", text)

	if !skill.HasProcedure() {
		t.Error("una lista con línea de contexto terminada en ':' debería contar como procedimiento, aunque los ítems no tengan verbo imperativo")
	}
}

// TestBuild_ListaConMitadDeVerbosCuentaComoProcedimientoSinContexto
// (Integration Fix) confirma la segunda evidencia aceptada: sin línea
// de contexto, la MITAD o más de los ítems arrancando con verbo
// imperativo alcanza.
func TestBuild_ListaConMitadDeVerbosCuentaComoProcedimientoSinContexto(t *testing.T) {
	text := "Documentación general del programa.\n" +
		"- Descarga el instalador desde el sitio oficial\n" +
		"- Ejecuta el archivo descargado\n" +
		"- El proceso puede tardar unos minutos\n" +
		"- Reinicia la aplicación al terminar\n"
	skill := buildFrom(t, "instalacion", text)

	if !skill.HasProcedure() {
		t.Error("con la mitad o más de los ítems arrancando con verbo imperativo, debería contar como procedimiento aunque no haya línea de contexto")
	}
}

// TestBuild_ProcedureTitleReconstruyeOracionCortadaPorSaltoDeLinea
// (Integration Fix) confirma que un <br> a mitad de oración, que parte
// la frase en DOS líneas físicas, se reconstruye completa antes de
// evaluarse como título — mirar solo la última línea perdería el
// principio real de la frase. Usa contenido específico (no genérico)
// para poder diferenciar "se reconstruyó mal" de "se rechazó por
// genérico" — ver TestBuild_ProcedureTitleRechazaIntroGenerica para
// ese caso.
func TestBuild_ProcedureTitleReconstruyeOracionCortadaPorSaltoDeLinea(t *testing.T) {
	text := "Para instalar Go\ncorrectamente, segui estos pasos:\n" +
		"- Instala Go en tu maquina\n" +
		"- Escribe un programa simple\n"
	skill := buildFrom(t, "golang", text)

	if skill.ProcedureTitle != "Para instalar Go correctamente, segui estos pasos" {
		t.Errorf("ProcedureTitle = %q, esperaba la oración completa reconstruida, no solo el fragmento final", skill.ProcedureTitle)
	}
}

// TestBuild_ExtraccionNoIncluyeTagDeFuente confirma que el tag
// "[Source] " (usado solo para armar Content, ver displayText en
// Build()) no se cuela dentro de un campo estructurado extraído
// cuando un trigger matchea justo la primera línea de un documento —
// analysisText (lo que ven extractByTriggers/extractSteps) nunca
// lleva el tag, a diferencia de displayText/Content.
func TestBuild_ExtraccionNoIncluyeTagDeFuente(t *testing.T) {
	skill := buildFrom(t, "unity", "Necesitas tener instalado Unity Hub antes de continuar con este tutorial.")

	if len(skill.Prerequisites) != 1 {
		t.Fatalf("esperaba 1 prerrequisito extraído, obtuve %v", skill.Prerequisites)
	}
	if strings.Contains(skill.Prerequisites[0], "Fuente Test") || strings.Contains(skill.Prerequisites[0], "[") {
		t.Errorf("Prerequisites[0] = %q — no debería contener el tag de fuente", skill.Prerequisites[0])
	}
}

// TestBuild_ProcedureTitlePrefiereHeadingSobreIntroGenerica reproduce
// EXACTAMENTE el bug real encontrado con Go Docs en producción:
// ProcedureTitle salió "In this tutorial, you'll get a brief
// introduction to Go programming. Along the way, you will" — una
// introducción narrativa — cuando el heading real de la página
// ("Tutorial: Get started with Go") estaba disponible más arriba.
// Confirma que, con el marcador de heading (shared.HeadingMarker) que
// ahora pone el Gateway, se prefiere el heading real sobre cualquier
// heurística de puntuación.
func TestBuild_ProcedureTitlePrefiereHeadingSobreIntroGenerica(t *testing.T) {
	text := shared.HeadingMarker + "Tutorial: Get started with Go\n" +
		"In this tutorial, you will get a brief introduction to Go programming.\n" +
		"Along the way, you will:\n" +
		"- Install Go\n" +
		"- Write some simple code\n"
	skill := buildFrom(t, "golang", text)

	if skill.ProcedureTitle != "Tutorial: Get started with Go" {
		t.Errorf("ProcedureTitle = %q, esperaba el heading real, no la intro genérica", skill.ProcedureTitle)
	}
}

// TestBuild_ContentNuncaMuestraElMarcadorDeHeading confirma que
// shared.HeadingMarker (metadata interna del pipeline, nunca algo que
// la fuente haya escrito) no se filtra al campo Content que
// eventualmente se le lee al usuario.
func TestBuild_ContentNuncaMuestraElMarcadorDeHeading(t *testing.T) {
	text := shared.HeadingMarker + "Tutorial: Get started with Go\n" +
		"Para empezar, segui estos pasos:\n" +
		"- Install Go\n" +
		"- Write some simple code\n"
	skill := buildFrom(t, "golang", text)

	if strings.Contains(skill.Content, shared.HeadingMarker) {
		t.Errorf("Content contiene el marcador interno de heading — no debería filtrarse al usuario: %q", skill.Content)
	}
	if !strings.Contains(skill.Content, "Tutorial: Get started with Go") {
		t.Error("Content debería seguir teniendo el texto real del heading, solo sin el marcador")
	}
}

// reproduce el bug real encontrado en vivo con Go Docs: "Along the
// way, you will:" es sintácticamente idéntica a un título válido
// (línea completa terminada en ":", justo antes de la lista), pero es
// una introducción genérica, no el nombre de la tarea. Sin un heading
// real en el texto (ver TestBuild_ProcedureTitlePrefiereHeadingSobreIntroGenerica
// para ese caso), debe quedar vacío — fiel a "no hay título real
// disponible" en vez de devolver la frase genérica.
func TestBuild_ProcedureTitleRechazaIntroGenerica(t *testing.T) {
	text := "Along the\nway, you will:\n" +
		"- Instala Go en tu maquina\n" +
		"- Escribe un programa simple\n"
	skill := buildFrom(t, "golang", text)

	if skill.ProcedureTitle != "" {
		t.Errorf("ProcedureTitle = %q, esperaba vacío — es una introducción genérica, no el nombre de la tarea", skill.ProcedureTitle)
	}
}

// TestBuild_ProcedureTitleNoArrastraOracionesAnterioresNoRelacionadas
// (Integration Fix) confirma el límite de seguridad del fix anterior:
// si la línea justo antes de la lista YA empieza con mayúscula (se
// ve como el principio real de una oración, no la continuación de un
// <br>), no se mira más atrás — evita arrastrar oraciones previas sin
// relación cuando el HTML pone cada una en su propia línea completa.
func TestBuild_ProcedureTitleNoArrastraOracionesAnterioresNoRelacionadas(t *testing.T) {
	text := "Esta es una oracion sin relacion.\n" +
		"Instrucciones:\n" +
		"- Instala Go en tu maquina\n" +
		"- Escribe un programa simple\n"
	skill := buildFrom(t, "golang", text)

	if skill.ProcedureTitle != "Instrucciones" {
		t.Errorf("ProcedureTitle = %q, esperaba solo \"Instrucciones\" sin arrastrar la oración anterior no relacionada", skill.ProcedureTitle)
	}
}

// TestBuild_NoCruzaContextoEntreDosDocumentosDistintos (Integration
// Fix) reproduce el bug real: Discovery a veces trae más de un
// documento para el mismo objetivo (Go: go.dev + pkg.go.dev siempre).
// Antes, extractProcedureTitle corría sobre el texto YA CONCATENADO
// de ambos documentos, así que "la línea de antes" de la lista de
// pasos del SEGUNDO documento podía ser la ÚLTIMA línea del PRIMERO —
// contaminación cruzada real que produjo "way, you will" en vivo.
// Este test arma dos documentos donde el primero (ordena antes por
// Source) NO tiene ninguna lista, y el segundo sí — con su propio
// contexto real — para confirmar que ProcedureTitle sale del contexto
// correcto y no de la cola del primer documento.
func TestBuild_NoCruzaContextoEntreDosDocumentosDistintos(t *testing.T) {
	vs := &ValidationSet{
		Goal: "golang",
		Validations: []*SourceValidation{
			{
				Document: &gateway.Document{
					Source: "A Docs", // ordena ANTES que "B Docs"
					Text:   "Este es el contenido de la portada del sitio, sin ningún procedimiento. Termina con una frase cualquiera:",
				},
				StatusOK: true, HasContent: true,
			},
			{
				Document: &gateway.Document{
					Source: "B Docs",
					Text: "Para instalar Go en tu maquina:\n" +
						"- Instala Go en tu maquina\n" +
						"- Escribe un programa simple\n",
				},
				StatusOK: true, HasContent: true,
			},
		},
	}
	sb := NewSkillBuilder()
	skill := sb.Build(LearningItem{TaskID: "golang", TaskDesc: "Instalar Go"}, vs)
	if skill == nil {
		t.Fatal("esperaba una Skill armada")
	}

	if !skill.HasProcedure() {
		t.Fatal("esperaba Steps extraídos del segundo documento")
	}
	if skill.ProcedureTitle != "Para instalar Go en tu maquina" {
		t.Errorf("ProcedureTitle = %q, esperaba el contexto REAL del documento que tiene la lista, no algo cruzado del primer documento", skill.ProcedureTitle)
	}
	if strings.Contains(skill.ProcedureTitle, "portada") || strings.Contains(skill.ProcedureTitle, "cualquiera") {
		t.Errorf("ProcedureTitle = %q, se filtró contenido del primer documento — contaminación cruzada", skill.ProcedureTitle)
	}
}

// TestDescribe_SkillReflejaElEstado confirma que Describe() distingue
// narrativamente los tres niveles: sin estructura, Structured y
// Procedural.
func TestDescribe_SkillReflejaElEstado(t *testing.T) {
	sinEstructura := &Skill{Label: "X", Sources: []string{"F"}}
	if !strings.Contains(sinEstructura.Describe(), "sin procedimiento ni condición de uso reconocibles") {
		t.Errorf("Describe() sin estructura = %q", sinEstructura.Describe())
	}

	estructurada := &Skill{Label: "X", State: StateStructured, WhenToUse: []string{"a"}, Sources: []string{"F"}}
	if !strings.Contains(estructurada.Describe(), "estructurada") {
		t.Errorf("Describe() estructurada = %q", estructurada.Describe())
	}

	procedural := &Skill{Label: "X", State: StateProcedural, Steps: []string{"a", "b"}, Sources: []string{"F"}}
	if !strings.Contains(procedural.Describe(), "procedural (2 paso(s)") {
		t.Errorf("Describe() procedural = %q", procedural.Describe())
	}
}

// TestBuild_ExtraeLimitations (Integration Fix, Skill Model
// Refinement) confirma que una frase de alcance explícita se extrae
// como Limitation — distinta de Pitfalls (qué NO hacer) y de
// FailureRecovery (qué hacer SI ya falló).
func TestBuild_ExtraeLimitations(t *testing.T) {
	text := "Este instalador no funciona en Windows 7 o versiones anteriores."
	skill := buildFrom(t, "instalador", text)

	if len(skill.Limitations) != 1 {
		t.Fatalf("esperaba 1 limitación extraída, obtuve %v", skill.Limitations)
	}
	if !strings.Contains(skill.Limitations[0], "no funciona en Windows 7") {
		t.Errorf("Limitations[0] = %q, no conserva la limitación original", skill.Limitations[0])
	}
}

// TestBuild_SinLimitacionesExplicitasQuedaVacio confirma que, sin una
// frase de alcance explícita, Limitations queda vacío — no se infiere
// de la ausencia de otros campos.
func TestBuild_SinLimitacionesExplicitasQuedaVacio(t *testing.T) {
	skill := buildFrom(t, "instalador", strings.Repeat("Este instalador es sencillo de usar. ", 5))
	if len(skill.Limitations) != 0 {
		t.Errorf("sin frase de alcance explícita, Limitations debería quedar vacío, obtuve %v", skill.Limitations)
	}
}

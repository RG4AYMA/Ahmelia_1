package solving

import (
	"ahmelia/interaction"
	"strings"
	"testing"
)

// TestAnalyze_DetectaDominioSoftware confirma la ruta central: una
// petición de software cae en la plantilla correcta y trae sus
// subobjetivos típicos.
func TestAnalyze_DetectaDominioSoftware(t *testing.T) {
	a := NewAnalyzer()
	ga := a.Analyze("quiero crear una aplicacion para gestionar tareas")

	if ga.Domain != "software" {
		t.Fatalf("esperaba dominio 'software', obtuve %q", ga.Domain)
	}
	if len(ga.Subgoals) == 0 {
		t.Fatal("esperaba subobjetivos de la plantilla software, obtuve ninguno")
	}
	if strings.HasPrefix(strings.ToLower(ga.MainGoal), "quiero") {
		t.Fatalf("MainGoal debería tener la muletilla 'quiero' recortada, quedó: %q", ga.MainGoal)
	}
}

// TestAnalyze_DetectaRequisitosImplicitos confirma que palabras sueltas
// como "rápido" y "muchos usuarios" generan Requirements marcados como
// implícitos, no que el usuario los haya pedido con esas palabras
// exactas ("requisito: ...").
func TestAnalyze_DetectaRequisitosImplicitos(t *testing.T) {
	a := NewAnalyzer()
	ga := a.Analyze("necesito un sistema rapido que soporte muchos usuarios")

	if len(ga.Requirements) < 2 {
		t.Fatalf("esperaba al menos 2 requisitos implícitos (velocidad + escala), obtuve %d: %+v", len(ga.Requirements), ga.Requirements)
	}
	for _, r := range ga.Requirements {
		if !r.Implicit {
			t.Errorf("todos los requisitos detectados por keyword deberían marcarse Implicit=true, encontré uno explícito: %q", r.Description)
		}
	}
}

// TestAnalyze_AsumeCuandoNoHayDominio confirma que, sin ningún keyword
// de dominio conocido, cae en "general" y lo deja explícito como
// supuesto (no como si Ahmelia no hubiera entendido la pregunta).
func TestAnalyze_AsumeCuandoNoHayDominio(t *testing.T) {
	a := NewAnalyzer()
	ga := a.Analyze("organizame el garage este fin de semana")

	if ga.Domain != "general" {
		t.Fatalf("esperaba dominio 'general' para un pedido sin keywords de ningún template, obtuve %q", ga.Domain)
	}
	found := false
	for _, assumption := range ga.Assumptions {
		if strings.Contains(assumption, "dominio especifico") || strings.Contains(assumption, "dominio específico") {
			found = true
		}
	}
	if !found {
		t.Fatalf("esperaba un supuesto explícito sobre la falta de dominio, obtuve: %v", ga.Assumptions)
	}
}

// TestAnalyze_SoloTriggerSinContenido confirma que si el usuario
// escribe solo el trigger sin nada que analizar, Describe() lo dice
// explícito en vez de devolver un análisis vacío/engañoso.
func TestAnalyze_SoloTriggerSinContenido(t *testing.T) {
	a := NewAnalyzer()
	ga := a.Analyze("analiza este objetivo")

	desc := ga.Describe()
	if !strings.Contains(desc, "Qué objetivo") && !strings.Contains(desc, "objetivo querés") {
		t.Fatalf("esperaba que Describe() pidiera aclaración ante un trigger vacío, obtuve: %q", desc)
	}
}

// TestParseGoalAnalysisQuery_DetectaTriggers confirma los triggers
// principales sin falsos positivos sobre preguntas de otros módulos.
func TestParseGoalAnalysisQuery_DetectaTriggers(t *testing.T) {
	positivos := []string{
		"analiza este objetivo: crear un juego",
		"cuales son los subobjetivos de armar una tienda online",
	}
	for _, p := range positivos {
		if !ParseGoalAnalysisQuery(interaction.Analyze(p, interaction.Context{})) {
			t.Errorf("esperaba detectar %q como pedido de análisis de objetivo", p)
		}
	}

	negativos := []string{
		"que aprendiste",
		"como te va",
		"que hicimos hoy",
	}
	for _, n := range negativos {
		if ParseGoalAnalysisQuery(interaction.Analyze(n, interaction.Context{})) {
			t.Errorf("%q no debería disparar Goal Analyzer (es una pregunta retrospectiva de otro módulo)", n)
		}
	}
}

// TestAnalyze_NoPanicConBytesNoUTF8 reproduce el panic real del run 4
// de compuerta V6 (GLM 20/10): la linea de stdin llego con bytes
// CP1252 invalidos para UTF-8 ("?" = 0xBF, "e con acento" = 0xE9);
// strings.ToLower re-encoda cada byte invalido como U+FFFD de 3
// bytes, lower crece sobre text y el corte text[idx+len(trigger):]
// salia de rango ([20:17] sobre len=17) matando el proceso entero.
// La regresion exige que stripTrigger no explote con la forma exacta
// del input de produccion y que Analyze complete sin panic.
func TestAnalyze_NoPanicConBytesNoUTF8(t *testing.T) {
	a := NewAnalyzer()

	produccion := "\xBFQu\xE9 sabes hacer?" // 17 bytes, 2 invalidos
	if got := len(produccion); got != 17 {
		t.Fatalf("el mimico de produccion debe tener 17 bytes, obtuve %d", got)
	}
	_ = stripTrigger(produccion) // no debe hacer panic

	// Trigger + byte invalido al frente: es la forma que hacia
	// cut=24 > len(text)=22 pre-fix.
	ga := a.Analyze("\xBFanaliza este objetivo")
	if strings.TrimSpace(ga.Describe()) == "" {
		t.Fatal("esperaba que Describe() dijera algo ante un input con bytes invalidos")
	}
}

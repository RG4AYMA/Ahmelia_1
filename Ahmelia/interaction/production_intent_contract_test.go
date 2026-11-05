package interaction

import "testing"

func TestProductionClassifierDoesNotPromoteReportedEffortToCommand(t *testing.T) {
	reports := []string{
		"Llevo horas intentando terminar el formulario y nada funciona. Estoy frustrado.",
		"Estoy intentando crear una herramienta y no me sale.",
		"I have been trying to build a program, but it still does not work.",
		"No consigo terminar el proyecto y me frustra.",
	}
	for _, input := range reports {
		r := Analyze(input, Context{})
		if r.Kind == KindProduction || CanonicalActs(r).Has(ActCommand) {
			t.Errorf("reported effort was promoted to executable production: input=%q kind=%s acts=%+v", input, r.Kind, CanonicalActs(r))
		}
	}
	acts := CanonicalActs(Analyze(reports[0], Context{}))
	if !acts.Has(ActInform) && !acts.Has(ActDeclaration) {
		t.Fatalf("first-person progress report lacks an informative act: %+v", acts)
	}
}

func TestProductionClassifierKeepsExplicitRequestsExecutable(t *testing.T) {
	requests := []string{
		"Crea un formulario para el proyecto.",
		"Hola Ahmelia, crea un programa que imprima hola.",
		"Quiero que implementes una función que valide el formulario.",
		"Quiero crear una herramienta para organizar archivos.",
		"crear una herramienta para organizar archivos",
	}
	for _, input := range requests {
		r := Analyze(input, Context{})
		if r.Kind != KindProduction || !CanonicalActs(r).Has(ActCommand) {
			t.Errorf("explicit production request lost its executable route: input=%q kind=%s acts=%+v", input, r.Kind, CanonicalActs(r))
		}
	}
}

func TestProductionCapabilityContractForSpecificRequestAcrossChannels(t *testing.T) {
	input := "Crea una función Go Par que devuelva si un entero es par, añade pruebas y explícame cómo verificaste el objetivo"
	for _, channel := range []InputChannel{ChannelText, ChannelVoice} {
		t.Run(string(channel), func(t *testing.T) {
			req := Analyze(input, Context{Channel: channel})
			decision := ResolveCapabilityInteraction(req)
			if req.Kind != KindProduction || !CanonicalActs(req).Has(ActCommand) {
				t.Fatalf("request lost production/command semantics: kind=%q acts=%+v", req.Kind, CanonicalActs(req))
			}
			if !decision.Executable || decision.Request.Operation != "produce" {
				t.Fatalf("request did not produce generic capability contract: decision=%+v", decision)
			}
			if decision.Request.Parameters["raw"] != input {
				t.Fatalf("raw objective was not preserved: %+v", decision.Request.Parameters)
			}
		})
	}
}

func TestProductionClassifierUsesWholeObjectTerms(t *testing.T) {
	if isProduction("llevo horas intentando terminar el formulario y nada funciona estoy frustrado") {
		t.Fatal("the word 'funciona' must not satisfy the production object 'funcion'")
	}
}

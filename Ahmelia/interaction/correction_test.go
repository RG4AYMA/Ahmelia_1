package interaction

import "testing"

func TestAnalyzeCorrectionExplicitReplacement(t *testing.T) {
	r := Analyze("no, me refería a la interacción del personaje", Context{})
	if !r.Correction.IsCorrection {
		t.Fatal("expected explicit correction")
	}
	if r.Correction.Replacement != "la interacción del personaje" {
		t.Fatalf("unexpected replacement: %q", r.Correction.Replacement)
	}
	if r.SpeechAct != SpeechCorrect {
		t.Fatalf("expected corrective speech act, got %q", r.SpeechAct)
	}
}

func TestAnalyzeCorrectionCompoundReformulation(t *testing.T) {
	r := Analyze("No, espera. Cambia el enfoque: quiero que sea nocturno.", Context{})
	if !r.Correction.IsCorrection {
		t.Fatal("expected compound reformulation correction")
	}
	if r.Correction.Replacement != "sea nocturno" {
		t.Fatalf("unexpected replacement: %q", r.Correction.Replacement)
	}
	if r.SpeechAct != SpeechCorrect {
		t.Fatalf("expected corrective speech act, got %q", r.SpeechAct)
	}
}

func TestAnalyzeCorrectionCompoundReformulationIsGeneric(t *testing.T) {
	r := Analyze("No, cambia el formato: quiero que sea PDF.", Context{})
	if !r.Correction.IsCorrection {
		t.Fatal("expected generic compound reformulation correction")
	}
	if r.Correction.Replacement != "sea pdf" {
		t.Fatalf("unexpected replacement: %q", r.Correction.Replacement)
	}
}

func TestAnalyzeCorrectionDoesNotTreatStateAsCorrection(t *testing.T) {
	r := Analyze("no funciona otra vez", Context{})
	if r.Correction.IsCorrection {
		t.Fatal("ordinary negative state must not become a correction")
	}
}

func TestAnalyzeCorrectionDoesNotOverridePause(t *testing.T) {
	r := Analyze("no abras Chrome todavía", Context{})
	if r.Kind != KindPause {
		t.Fatalf("expected pause, got %q", r.Kind)
	}
	if r.Correction.IsCorrection {
		t.Fatal("control negation must not be treated as conversational correction")
	}
}

func TestAnalyzeCorrectionSino(t *testing.T) {
	r := Analyze("no el proyecto viejo, sino el nuevo", Context{})
	if !r.Correction.IsCorrection {
		t.Fatal("expected replacement correction")
	}
	if r.Correction.Target != "el proyecto viejo" || r.Correction.Replacement != "el nuevo" {
		t.Fatalf("unexpected correction: %#v", r.Correction)
	}
}

func TestApplyCorrectionPreservesRawButChangesSemanticObjective(t *testing.T) {
	r := Analyze("no, quería decir crear el archivo de configuración", Context{})
	r = ApplyCorrection(r, "crear el proyecto")
	if r.Raw != "no, quería decir crear el archivo de configuración" {
		t.Fatalf("raw input was mutated: %q", r.Raw)
	}
	if r.Objective.Text != "crear el archivo de configuración" {
		t.Fatalf("unexpected semantic objective: %q", r.Objective.Text)
	}
}

func TestAnalyzeCorrectionRepairsUserSelfOwnership(t *testing.T) {
	r := Analyze("pero es mi color favorito, no el tuyo", Context{Language: "es"})
	if !r.Correction.IsCorrection {
		t.Fatal("expected ownership correction")
	}
	if r.Subject != UserObservationSubjectUser {
		t.Fatalf("subject=%q want user", r.Subject)
	}
	repaired := ApplyCorrection(r, "Mi color favorito es el negro.")
	if repaired.Subject != UserObservationSubjectUser {
		t.Fatalf("repaired subject=%q want user", repaired.Subject)
	}
	if repaired.Objective.Text != "mi color favorito" {
		t.Fatalf("replacement=%q want mi color favorito", repaired.Objective.Text)
	}
}

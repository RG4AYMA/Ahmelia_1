package linguistics

// Pruebas puras de B2 (marcas explícitas) y B1 (lector Reading) — sin HTTP.

import (
	"encoding/json"
	"testing"
)

func TestParseOpenMeaning_MarksNormalize(t *testing.T) {
	raw := `{"purpose":"pedido","owner_of_data":"user","topic":"archivo",
		"meaning":"El usuario pide que el archivo no se abra.",
		"mentions":["archivo"],
		"confidence":0.95,
		"marks":{
			"negation":{"scope":"accion","target":"  abrir el archivo "},
			"condition":{"antecedent":"  ","consequent":""},
			"modality":"definitely",
			"provenance":"fantasma",
			"incompatibility":[{"a":"A","b":"","reason":"x"},{"a":"nunca estuvo","b":"fue ayer","reason":"mismo lugar/período"}]
		}}`
	m, err := ParseOpenMeaning(raw)
	if err != nil {
		t.Fatalf("parse con marks falló: %v", err)
	}
	if m.Marks.Negation == nil || m.Marks.Negation.Target != "abrir el archivo" {
		t.Fatalf("negation sin recortar o descartada: %+v", m.Marks.Negation)
	}
	if m.Marks.Condition != nil {
		t.Fatal("condición incompleta debe descartarse")
	}
	if m.Marks.Modality != ModalityUnknown {
		t.Fatalf("modality fuera de dominio debe caer en unknown, got %q", m.Marks.Modality)
	}
	if m.Marks.Provenance != "" {
		t.Fatalf("provenance fuera de dominio debe quedar vacío, got %q", m.Marks.Provenance)
	}
	if len(m.Marks.Incompatibility) != 1 || m.Marks.Incompatibility[0].A != "nunca estuvo" || m.Marks.Incompatibility[0].Reason != "mismo lugar/período" {
		t.Fatalf("las incompatibilidades incompletas deben descartarse: %+v", m.Marks.Incompatibility)
	}
}

func TestParseOpenMeaning_LegacyJSONWithoutMarks(t *testing.T) {
	raw := `{"purpose":"declaracion","owner_of_data":"user","topic":"bebida favorita",
		"meaning":"Declara su bebida favorita","mentions":["té con limón"],"confidence":0.9}`
	m, err := ParseOpenMeaning(raw)
	if err != nil {
		t.Fatalf("JSON legado sin marks debe parsear igual: %v", err)
	}
	if m.Marks.Negation != nil || m.Marks.Condition != nil || len(m.Marks.Incompatibility) != 0 || m.Marks.Modality != "" {
		t.Fatalf("JSON sin marks debe dejar cero marcas: %+v", m.Marks)
	}
}

func TestSchema_MarksOptionalNotRequired(t *testing.T) {
	var schema map[string]any
	if err := json.Unmarshal(openMeaningJSONSchema(), &schema); err != nil {
		t.Fatalf("schema inválido: %v", err)
	}
	required, _ := schema["required"].([]any)
	if len(required) != 6 {
		t.Fatalf("marks no entra a required: debe seguir en 6, got %d", len(required))
	}
	props, _ := schema["properties"].(map[string]any)
	if _, ok := props["marks"]; !ok {
		t.Fatal("properties debe declarar marks (opcional)")
	}
}

func TestDeriveReading_MapsActoOwnerAndValor(t *testing.T) {
	om := OpenMeaning{
		Purpose:     OpenPurposeDeclaration,
		OwnerOfData: OpenOwnerUser,
		Topic:       "bebida favorita",
		Meaning:     "Declara su bebida favorita",
		Mentions:    []string{"té con limón", "bebida"}, // orden del modelo intacto
		Confidence:  0.9,
		Marks:       OpenMarks{Negation: &OpenNegation{Scope: "accion", Target: "abrir"}},
	}
	r := DeriveReading(om)
	if r.Acto != "captura" {
		t.Fatalf("acto = %q", r.Acto)
	}
	if r.Owner != OpenOwnerUser {
		t.Fatalf("owner = %q", r.Owner)
	}
	if r.Valor != "té con limón" {
		t.Fatalf("valor = %q (primera mention verbatim)", r.Valor)
	}
	if len(r.Evidence) != 3 || r.Evidence[0] != "té con limón" || r.Evidence[2] != "Declara su bebida favorita" {
		t.Fatalf("mentions + meaning deben quedar en Evidence: %+v", r.Evidence)
	}
	if r.Marks.Negation == nil || r.Marks.Negation.Target != "abrir" {
		t.Fatalf("marks deben pasar por el lector intactas: %+v", r.Marks.Negation)
	}
}

func TestDeriveReading_PurposeTable(t *testing.T) {
	cases := map[OpenPurpose]string{
		OpenPurposeQuestion:    "consulta",
		OpenPurposeDeclaration: "captura",
		OpenPurposeRequest:     "solicitud",
		OpenPurposeDelivery:    "entrega",
		OpenPurposeFeedback:    "evaluacion",
		OpenPurposeSocial:      "social",
		OpenPurposeOperational: "operativa",
		OpenPurposeUnknown:     "desconocido",
	}
	for purpose, want := range cases {
		if got := deriveActo(purpose); got != want {
			t.Fatalf("acto de %q = %q, want %q", purpose, got, want)
		}
	}
}

func TestDeriveReading_EmptyMentionsNoValor(t *testing.T) {
	r := DeriveReading(OpenMeaning{Purpose: OpenPurposeQuestion})
	if r.Valor != "" || r.Acto != "consulta" {
		t.Fatalf("valor sin mentions debe ser vacío: %+v", r)
	}
}

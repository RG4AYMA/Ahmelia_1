package memory_intelligence

import (
	"ahmelia/interaction"
	"testing"

	"ahmelia/memory"
)

func TestParseRelationsQuery_Match(t *testing.T) {
	subject, ok := ParseRelationsQuery(interaction.Analyze("que esta relacionado con subir el volumen", interaction.Context{}))
	if !ok || subject != "subir el volumen" {
		t.Fatalf("esperaba ok=true subject='subir el volumen', obtuve ok=%v subject=%q", ok, subject)
	}
}

func TestParseRelationsQuery_OtraFrase(t *testing.T) {
	subject, ok := ParseRelationsQuery(interaction.Analyze("de que otra forma digo abre chrome", interaction.Context{}))
	if !ok || subject != "abre chrome" {
		t.Fatalf("esperaba ok=true subject='abre chrome', obtuve ok=%v subject=%q", ok, subject)
	}
}

func TestParseRelationsQuery_NoMatch(t *testing.T) {
	_, ok := ParseRelationsQuery(interaction.Analyze("que hicimos hoy", interaction.Context{}))
	if ok {
		t.Fatalf("esperaba ok=false para una pregunta sin trigger de relaciones")
	}
}

func TestParseRelationsQuery_TriggerSinSujeto(t *testing.T) {
	_, ok := ParseRelationsQuery(interaction.Analyze("que esta relacionado con", interaction.Context{}))
	if ok {
		t.Fatalf("esperaba ok=false cuando el trigger no tiene sujeto después")
	}
}

func TestDescribeRelations_ConectaYDescribe(t *testing.T) {
	long := &memory.LongMemory{Data: map[string]*memory.Knowledge{
		"sube el volumen": {ID: "sube el volumen", Input: "sube el volumen", Relations: []string{"mas fuerte"}},
		"mas fuerte":      {ID: "mas fuerte", Input: "más fuerte", Relations: []string{"sube el volumen"}},
	}}

	desc, ok := DescribeRelations(long, "sube el volumen")
	if !ok {
		t.Fatal("esperaba ok=true cuando el comando tiene relaciones")
	}
	if desc == "" {
		t.Fatal("esperaba una descripción no vacía")
	}
}

func TestDescribeRelations_SinRelaciones(t *testing.T) {
	long := &memory.LongMemory{Data: map[string]*memory.Knowledge{
		"abre chrome": {ID: "abre chrome", Input: "abre chrome", Relations: []string{}},
	}}

	_, ok := DescribeRelations(long, "abre chrome")
	if ok {
		t.Fatalf("esperaba ok=false cuando no hay relaciones registradas")
	}
}

func TestDescribeRelations_ComandoInexistente(t *testing.T) {
	long := &memory.LongMemory{Data: map[string]*memory.Knowledge{}}

	_, ok := DescribeRelations(long, "algo que nunca se dijo")
	if ok {
		t.Fatalf("esperaba ok=false cuando el comando no existe en memoria")
	}
}

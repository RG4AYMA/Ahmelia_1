package selfknowledge

import (
	"testing"

	"ahmelia/interaction"
	"ahmelia/personality"
)

func TestSelfAttributeStoreSeparatesOwnershipFromUserModel(t *testing.T) {
	identity := &personality.Identity{Name: "Ahmelia", Role: "companion", Purpose: "assist"}
	store := NewSelfAttributeStore(identity)

	selfReq := interaction.Analyze("¿Cuál es tu rol?", interaction.Context{Language: "es"})
	selfReq = interaction.InferSemanticRelations(selfReq)
	attr, ok := store.QuerySemantic(selfReq)
	if !ok || attr.Key != "rol" || attr.Value != "companion" {
		t.Fatalf("self semantic query failed: %+v ok=%v", attr, ok)
	}

	userReq := interaction.Analyze("¿Cuál es mi rol?", interaction.Context{Language: "es"})
	userReq = interaction.InferSemanticRelations(userReq)
	if _, ok := store.QuerySemantic(userReq); ok {
		t.Fatal("user-owned query must never read SelfAttributeStore")
	}
}

func TestSelfAttributeStoreSupportsArbitrarySelfAttributeKeys(t *testing.T) {
	store := NewSelfAttributeStore(nil)
	if !store.Upsert("color favorito", "verde", .9, interaction.UserModelSourceRef{Kind: interaction.UserSourceSystem, SourceID: "test"}) {
		t.Fatal("failed to add generic self attribute")
	}
	req := interaction.Analyze("¿Cuál es tu color favorito?", interaction.Context{Language: "es"})
	req = interaction.InferSemanticRelations(req)
	attr, ok := store.QuerySemantic(req)
	if !ok || attr.Key != "color favorito" || attr.Value != "verde" {
		t.Fatalf("generic self attribute query failed: %+v ok=%v", attr, ok)
	}
}

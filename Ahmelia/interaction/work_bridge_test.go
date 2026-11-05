package interaction

import (
	"testing"
	"time"
)

func TestWorkBridgeAttachesNewProductionWithoutActiveWork(t *testing.T) {
	b := NewWorkBridge()
	d := b.Resolve(
		SemanticRequest{Kind: KindProduction, Objective: Objective{Text: "crear una herramienta"}},
		ConversationSnapshot{ActiveThreadID: "thread-1"},
		WorkContext{}, false,
	)
	if d.Relation != WorkRelationAttach || !d.ShouldCreate {
		t.Fatalf("expected attach/create, got %+v", d)
	}
	if d.ThreadID != "thread-1" {
		t.Fatalf("expected conversation thread to survive bridge, got %q", d.ThreadID)
	}
}

func TestWorkBridgeContinuesCurrentWorkFromConversationalContinuation(t *testing.T) {
	b := NewWorkBridge()
	d := b.Resolve(
		SemanticRequest{
			Kind: KindContinuation, Continues: true,
			Objective: Objective{Text: "mejorar la interacción"},
		},
		ConversationSnapshot{ActiveThreadID: "thread-1", ActiveObjective: "mejorar la interacción"},
		WorkContext{ID: "work-7", Objective: "crear Ahmelia"},
		true,
	)
	if d.Relation != WorkRelationContinue || d.WorkID != "work-7" {
		t.Fatalf("expected current work continuation, got %+v", d)
	}
	if d.Objective != "mejorar la interacción" {
		t.Fatalf("expected current semantic objective, got %q", d.Objective)
	}
}

func TestWorkBridgePreservesWorkDuringUnrelatedConversation(t *testing.T) {
	b := NewWorkBridge()
	d := b.Resolve(
		SemanticRequest{Kind: KindQuestion, Topic: "fotosíntesis"},
		ConversationSnapshot{ActiveThreadID: "thread-2", ActiveTopic: "fotosíntesis"},
		WorkContext{ID: "work-7", Objective: "crear Ahmelia"},
		true,
	)
	if d.Relation != WorkRelationPreserve || d.WorkID != "work-7" {
		t.Fatalf("expected work preservation, got %+v", d)
	}
	if d.ShouldCreate {
		t.Fatal("unrelated conversation must not create another work session")
	}
}

func TestWorkBridgeStartsSeparateWorkForNewProductionObjective(t *testing.T) {
	b := NewWorkBridge()
	d := b.Resolve(
		SemanticRequest{Kind: KindProduction, Objective: Objective{Text: "crear otra cosa"}},
		ConversationSnapshot{ActiveThreadID: "thread-2"},
		WorkContext{ID: "work-7", Objective: "crear Ahmelia"},
		true,
	)
	if d.Relation != WorkRelationStartNew || !d.ShouldCreate {
		t.Fatalf("expected separate work start, got %+v", d)
	}
	if d.WorkID != "work-7" {
		t.Fatalf("expected existing work to remain addressable, got %q", d.WorkID)
	}
}

func TestWorkBridgeResumesExplicitRecentWork(t *testing.T) {
	b := NewWorkBridge()
	d := b.Resolve(
		SemanticRequest{
			Kind: KindConversation, Normalized: "volvamos al trabajo",
			References: []Reference{{Kind: ReferenceRecentWork, Expression: "el trabajo", Resolved: true, Value: "work-7"}},
		},
		ConversationSnapshot{ActiveThreadID: "thread-3"},
		WorkContext{ID: "work-7", Objective: "crear Ahmelia", UpdatedAt: time.Now()},
		true,
	)
	if d.Relation != WorkRelationResume || d.WorkID != "work-7" {
		t.Fatalf("expected resume, got %+v", d)
	}
	if d.Objective != "crear Ahmelia" {
		t.Fatalf("expected work objective on resume, got %q", d.Objective)
	}
}

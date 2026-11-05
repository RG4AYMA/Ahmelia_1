package production

import (
	"testing"
	"time"

	"ahmelia/memory"
	memoryintelligence "ahmelia/memory_intelligence"
)

func TestMemoryAdapterPersistsAndRecallsProductionOutcome(t *testing.T) {
	long := &memory.LongMemory{Data: map[string]*memory.Knowledge{}, Version: 3}
	log := &memoryintelligence.EventLog{}
	adapter, err := NewMemoryAdapter(long, log)
	if err != nil {
		t.Fatal(err)
	}

	err = adapter.Remember(ProductionMemoryRecord{
		ProductionID: "prod-1", GoalID: "goal-1", TaskID: "task-1",
		Action: "create_file", Target: "enemy.cs", Success: true,
		Summary: "archivo creado y verificado", Timestamp: time.Unix(10, 0),
	})
	if err != nil {
		t.Fatal(err)
	}

	got := adapter.RecallProduction("prod-1")
	if len(got) != 1 || !got[0].Success || got[0].TaskID != "task-1" {
		t.Fatalf("unexpected recall: %+v", got)
	}
	if len(log.Events) != 1 || !log.Events[0].Outcome.Success {
		t.Fatalf("event not persisted: %+v", log.Events)
	}
}

func TestMemoryAdapterRecordsFailureAsKnowledgeError(t *testing.T) {
	long := &memory.LongMemory{Data: map[string]*memory.Knowledge{}, Version: 3}
	log := &memoryintelligence.EventLog{}
	adapter, err := NewMemoryAdapter(long, log)
	if err != nil {
		t.Fatal(err)
	}

	err = adapter.Remember(ProductionMemoryRecord{
		ProductionID: "prod-2", GoalID: "goal-2", TaskID: "task-2",
		Action: "compile", Success: false, Summary: "compilación falló",
		Timestamp: time.Unix(20, 0),
	})
	if err != nil {
		t.Fatal(err)
	}

	got := adapter.RecallTask("task-2")
	if len(got) != 1 || got[0].Success {
		t.Fatalf("expected failure: %+v", got)
	}
	if long.Data["production_prod-2_task_task-2"].Type != memory.KnowError {
		t.Fatalf("expected knowledge error")
	}
}

func TestMemoryAdapterRequiresOwnedStores(t *testing.T) {
	if _, err := NewMemoryAdapter(nil, &memoryintelligence.EventLog{}); err == nil {
		t.Fatal("expected nil long memory error")
	}
	if _, err := NewMemoryAdapter(&memory.LongMemory{Data: map[string]*memory.Knowledge{}}, nil); err == nil {
		t.Fatal("expected nil event log error")
	}
}

func TestMemoryAdapterConcurrentRecall(t *testing.T) {
	long := &memory.LongMemory{Data: map[string]*memory.Knowledge{}, Version: 3}
	log := &memoryintelligence.EventLog{}
	adapter, err := NewMemoryAdapter(long, log)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Remember(ProductionMemoryRecord{ProductionID: "p", GoalID: "g", TaskID: "t", Action: "a", Success: true, Summary: "ok"}); err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{}, 8)
	for i := 0; i < 8; i++ {
		go func() { _ = adapter.RecallProduction("p"); done <- struct{}{} }()
	}
	for i := 0; i < 8; i++ {
		<-done
	}
}

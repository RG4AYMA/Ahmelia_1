package autonomy

import (
	"context"
	"testing"
	"time"

	"ahmelia/memory"
	"ahmelia/memory_intelligence"
	"ahmelia/solving"
)

func TestMemorySkillAdapter_ResolveIntegratesAllSources(t *testing.T) {
	now := time.Now()

	long := &memory.LongMemory{
		Data: map[string]*memory.Knowledge{
			"abrir-vscode": {
				ID:       "abrir-vscode",
				Input:    "abre vscode",
				Action:   "abrir_programa",
				Target:   "vscode",
				LastUsed: now,
			},
		},
	}

	skills := solving.NewSkillLibrary()
	skills.Skills["vscode"] = &solving.Skill{
		SkillID:        "vscode",
		Label:          "Abrir VSCode",
		ProcedureTitle: "Abrir VSCode",
		Steps:          []string{"localizar VSCode", "abrirlo"},
	}

	learning := memory_intelligence.NewLearningState()
	learning.ActionConfidence["abrir_programa"] = 90
	learningStore := memory_intelligence.NewLearningStore(learning)

	adapter := NewMemorySkillAdapter(long, skills, learningStore)

	got, err := adapter.Resolve(context.Background(), ActionRequest{
		Action: "abrir_programa",
		Target: "vscode",
	})
	if err != nil {
		t.Fatalf("Resolve() error: %v", err)
	}

	if !got.HasMemory || len(got.Memories) != 1 {
		t.Fatalf("se esperaba 1 memoria relevante, got=%d", len(got.Memories))
	}
	if !got.HasSkill || got.Skill == nil {
		t.Fatal("se esperaba Skill integrada")
	}
	if !got.HasLearning {
		t.Fatal("se esperaba aprendizaje integrado")
	}
	if got.Confidence != 90 {
		t.Fatalf("confianza esperada 90, got=%v", got.Confidence)
	}
	if got.Strategy != "directo" {
		t.Fatalf("estrategia esperada 'directo', got=%q", got.Strategy)
	}
}

func TestMemorySkillAdapter_NoDataDoesNotInvent(t *testing.T) {
	adapter := NewMemorySkillAdapter(
		&memory.LongMemory{Data: map[string]*memory.Knowledge{}},
		solving.NewSkillLibrary(),
		memory_intelligence.NewLearningStore(memory_intelligence.NewLearningState()),
	)

	got, err := adapter.Resolve(context.Background(), ActionRequest{
		Action: "accion_desconocida",
		Target: "objetivo_inexistente",
	})
	if err != nil {
		t.Fatalf("Resolve() error: %v", err)
	}

	if got.HasMemory || got.HasSkill || got.HasLearning {
		t.Fatal("B84 no debe inventar conocimiento ausente")
	}
	if got.Memories != nil && len(got.Memories) != 0 {
		t.Fatalf("se esperaban cero memorias, got=%d", len(got.Memories))
	}
}

func TestMemorySkillAdapter_AmbiguousSkillIsNotSelected(t *testing.T) {
	skills := solving.NewSkillLibrary()
	skills.Skills["go-1"] = &solving.Skill{
		SkillID: "go-1",
		Label:   "Go tutorial",
		Content: "aprender Go",
	}
	skills.Skills["go-2"] = &solving.Skill{
		SkillID: "go-2",
		Label:   "Go avanzado",
		Content: "aprender Go",
	}

	adapter := NewMemorySkillAdapter(nil, skills, nil)

	got, err := adapter.Resolve(context.Background(), ActionRequest{
		Action: "aprender",
		Target: "Go",
	})
	if err != nil {
		t.Fatalf("Resolve() error: %v", err)
	}
	if got.HasSkill || got.Skill != nil {
		t.Fatal("una coincidencia ambigua no debe seleccionar una Skill arbitrariamente")
	}
}

func TestMemorySkillAdapter_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	adapter := NewMemorySkillAdapter(nil, nil, nil)

	if _, err := adapter.Resolve(ctx, ActionRequest{Action: "x"}); err == nil {
		t.Fatal("se esperaba error por contexto cancelado")
	}

	if _, _, err := adapter.Snapshot(ctx, "x"); err == nil {
		t.Fatal("se esperaba error por contexto cancelado en Snapshot")
	}
}

func TestMemorySkillAdapter_LimitsMemoryResults(t *testing.T) {
	now := time.Now()
	long := &memory.LongMemory{Data: map[string]*memory.Knowledge{}}

	for i := 0; i < 10; i++ {
		long.Data[string(rune('a'+i))] = &memory.Knowledge{
			ID:       "id-" + string(rune('a'+i)),
			Input:    "abre vscode",
			Action:   "abrir_programa",
			Target:   "vscode",
			LastUsed: now.Add(time.Duration(i) * time.Second),
		}
	}

	adapter := NewMemorySkillAdapter(long, nil, nil)
	adapter.SetMaxMemories(3)

	got, err := adapter.Resolve(context.Background(), ActionRequest{
		Action: "abrir_programa",
		Target: "vscode",
	})
	if err != nil {
		t.Fatalf("Resolve() error: %v", err)
	}
	if len(got.Memories) != 3 {
		t.Fatalf("se esperaban 3 memorias por límite B84, got=%d", len(got.Memories))
	}
	if !got.Memories[0].LastUsed.After(got.Memories[1].LastUsed) {
		t.Fatal("las memorias deben quedar ordenadas de más reciente a más antigua")
	}
}

func TestMemorySkillAdapter_ConcurrentReadsAreSafe(t *testing.T) {
	long := &memory.LongMemory{
		Data: map[string]*memory.Knowledge{
			"cmd": {
				ID:       "cmd",
				Input:    "abre vscode",
				Action:   "abrir_programa",
				Target:   "vscode",
				LastUsed: time.Now(),
			},
		},
	}

	skills := solving.NewSkillLibrary()
	skills.Skills["vscode"] = &solving.Skill{
		SkillID: "vscode",
		Label:   "Abrir VSCode",
		Content: "vscode",
	}

	learning := memory_intelligence.NewLearningState()
	learning.ActionConfidence["abrir_programa"] = 80
	learningStore := memory_intelligence.NewLearningStore(learning)

	adapter := NewMemorySkillAdapter(long, skills, learningStore)

	done := make(chan struct{})
	for i := 0; i < 8; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			for j := 0; j < 50; j++ {
				_, _ = adapter.Resolve(context.Background(), ActionRequest{
					Action: "abrir_programa",
					Target: "vscode",
				})
			}
		}()
	}

	for i := 0; i < 8; i++ {
		<-done
	}
}

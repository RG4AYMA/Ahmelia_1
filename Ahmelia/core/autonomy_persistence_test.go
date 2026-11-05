package core

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"ahmelia/planning"
)

// buildSampleSnapshot arma un snapshot con datos realistas — Goals y Tasks
// creados con los constructores reales de planning (mismo criterio que el
// resto del proyecto: no fabricar structs a mano cuando existe un
// constructor real) — para que el round-trip pruebe el formato tal cual lo
// va a producir el runtime en B90.6, no una versión simplificada.
func buildSampleSnapshot() *AutonomyPersistenceSnapshot {
	goalA := planning.NewGoal("Mejorar Reasoner", "desc A", planning.PriorityHigh, "user")
	goalB := planning.NewGoal("Optimizar Memory", "desc B", planning.PriorityMedium, "ahmelia")

	taskA1 := planning.NewTask(goalA.ID, "Analizar módulos actuales", "revisar", 1, 2*time.Hour)
	taskA2 := planning.NewTask(goalA.ID, "Diseñar mejoras", "especificar", 2, 3*time.Hour)
	taskA2.Dependencies = []string{taskA1.ID}
	taskA1.Status = planning.TaskCompleted

	snap := NewAutonomyPersistenceSnapshot()
	snap.Goals = []*planning.Goal{goalA, goalB}
	snap.Tasks = []*planning.Task{taskA1, taskA2}
	snap.Plans = []PersistedPlan{
		{
			GoalID:        goalA.ID,
			PlanID:        "plan-1",
			CreatedAt:     time.Now().Round(0), // Round(0) descarta la lectura monotónica: time.Time con monotónico nunca es DeepEqual a la misma marca leída de vuelta desde JSON (que no la tiene).
			TaskIDs:       []string{taskA1.ID, taskA2.ID},
			TotalEstimate: 5 * time.Hour,
			Priority:      planning.PriorityHigh,
			Feasible:      true,
			Issues:        []string{},
		},
	}
	snap.CurrentGoalID = goalA.ID
	snap.PendingGoalIDs = []string{goalB.ID}
	snap.ReplanRegenerations = map[string]int{goalA.ID: 1}

	return snap
}

// TestAutonomyPersistenceSnapshot_RoundTrip cubre exactamente el alcance de
// B90.1 acordado: runtime state -> snapshot -> JSON -> unmarshal ->
// reconstructed state, sin tocar todavía Planner/AutonomousState/
// GoalPlanningAdapter reales.
func TestAutonomyPersistenceSnapshot_RoundTrip(t *testing.T) {
	original := buildSampleSnapshot()

	path := filepath.Join(t.TempDir(), "autonomy_state.json")
	if err := original.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := LoadAutonomyPersistenceSnapshot(path)
	if err != nil {
		t.Fatalf("LoadAutonomyPersistenceSnapshot: %v", err)
	}

	if loaded.Version != AutonomyPersistenceVersion {
		t.Fatalf("Version = %d, want %d", loaded.Version, AutonomyPersistenceVersion)
	}
	if len(loaded.Goals) != len(original.Goals) {
		t.Fatalf("len(Goals) = %d, want %d", len(loaded.Goals), len(original.Goals))
	}
	for i, g := range original.Goals {
		if loaded.Goals[i].ID != g.ID || loaded.Goals[i].Name != g.Name ||
			loaded.Goals[i].Status != g.Status || loaded.Goals[i].Priority != g.Priority {
			t.Fatalf("Goals[%d] no sobrevivió el round-trip: got %+v, want %+v", i, loaded.Goals[i], g)
		}
	}
	if len(loaded.Tasks) != len(original.Tasks) {
		t.Fatalf("len(Tasks) = %d, want %d", len(loaded.Tasks), len(original.Tasks))
	}
	for i, tsk := range original.Tasks {
		if loaded.Tasks[i].ID != tsk.ID || loaded.Tasks[i].Status != tsk.Status ||
			!reflect.DeepEqual(loaded.Tasks[i].Dependencies, tsk.Dependencies) {
			t.Fatalf("Tasks[%d] no sobrevivió el round-trip: got %+v, want %+v", i, loaded.Tasks[i], tsk)
		}
	}
	// Comparación campo a campo en vez de reflect.DeepEqual sobre todo
	// PersistedPlan: time.Time no es DeepEqual-seguro entre una instancia
	// recién creada (con posible lectura monotónica/Location interna) y
	// la misma marca reconstruida desde JSON (siempre time.UTC, sin
	// monotónico) aunque representen el mismo instante — hay que usar
	// time.Time.Equal para esa comparación específica.
	if len(loaded.Plans) != len(original.Plans) {
		t.Fatalf("len(Plans) = %d, want %d", len(loaded.Plans), len(original.Plans))
	}
	for i, p := range original.Plans {
		lp := loaded.Plans[i]
		if lp.GoalID != p.GoalID || lp.PlanID != p.PlanID ||
			!lp.CreatedAt.Equal(p.CreatedAt) ||
			!reflect.DeepEqual(lp.TaskIDs, p.TaskIDs) ||
			lp.TotalEstimate != p.TotalEstimate || lp.Priority != p.Priority ||
			lp.Feasible != p.Feasible || !reflect.DeepEqual(lp.Issues, p.Issues) {
			t.Fatalf("Plans[%d] no sobrevivió el round-trip: got %+v, want %+v", i, lp, p)
		}
	}
	if loaded.CurrentGoalID != original.CurrentGoalID {
		t.Fatalf("CurrentGoalID = %q, want %q", loaded.CurrentGoalID, original.CurrentGoalID)
	}
	if !reflect.DeepEqual(loaded.PendingGoalIDs, original.PendingGoalIDs) {
		t.Fatalf("PendingGoalIDs = %+v, want %+v", loaded.PendingGoalIDs, original.PendingGoalIDs)
	}
	if !reflect.DeepEqual(loaded.ReplanRegenerations, original.ReplanRegenerations) {
		t.Fatalf("ReplanRegenerations = %+v, want %+v", loaded.ReplanRegenerations, original.ReplanRegenerations)
	}

	// El archivo temporal de escritura atómica no debe sobrevivir un Save
	// exitoso: Save escribe path+".tmp" y lo renombra sobre path, así que
	// tras un Save exitoso el .tmp no debería existir más.
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("esperaba que %s.tmp no existiera tras un Save exitoso, stat err=%v", path, err)
	}
}

// TestLoadAutonomyPersistenceSnapshot_NoFile confirma el caso de primera
// corrida: sin memory/autonomy_state.json todavía, se arranca vacío sin
// error — mismo criterio que LoadSkillLibrary/LoadRelationship.
func TestLoadAutonomyPersistenceSnapshot_NoFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "does_not_exist.json")

	snap, err := LoadAutonomyPersistenceSnapshot(path)
	if err != nil {
		t.Fatalf("LoadAutonomyPersistenceSnapshot: %v", err)
	}
	if snap.Version != AutonomyPersistenceVersion {
		t.Fatalf("Version = %d, want %d", snap.Version, AutonomyPersistenceVersion)
	}
	if len(snap.Goals) != 0 || len(snap.Tasks) != 0 || len(snap.Plans) != 0 {
		t.Fatalf("snapshot inicial debería estar vacío, got %+v", snap)
	}
	if snap.ReplanRegenerations == nil || snap.PendingGoalIDs == nil {
		t.Fatalf("snapshot inicial no debería tener mapas/slices nil: %+v", snap)
	}
}

// TestAutonomyPersistenceSnapshot_LoadMalformedJSON confirma que un JSON
// corrupto no se traga en silencio como si fuera un snapshot vacío — mismo
// criterio de fail-safe que el resto del proyecto (ej. InvalidTransitionError
// en autonomy/state.go): un dato incorrecto debe fallar visible, no
// disfrazarse de "no hay nada guardado".
func TestAutonomyPersistenceSnapshot_LoadMalformedJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "autonomy_state.json")
	if err := os.WriteFile(path, []byte("{ esto no es json"), 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	if _, err := LoadAutonomyPersistenceSnapshot(path); err == nil {
		t.Fatal("esperaba error al cargar un JSON malformado, got nil")
	}
}

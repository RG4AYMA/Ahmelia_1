package planning

import (
	"testing"
	"time"
)

// TestPlanner_GoalsEmpty confirma que un Planner recién creado, sin ningún
// RegisterGoal todavía, devuelve una colección vacía (no nil, no un error).
func TestPlanner_GoalsEmpty(t *testing.T) {
	p := NewPlanner()
	goals := p.Goals()
	if len(goals) != 0 {
		t.Fatalf("Goals() = %v, quería vacío", goals)
	}
}

// TestPlanner_TasksEmpty — mismo criterio que TestPlanner_GoalsEmpty, para
// Tasks().
func TestPlanner_TasksEmpty(t *testing.T) {
	p := NewPlanner()
	tasks := p.Tasks()
	if len(tasks) != 0 {
		t.Fatalf("Tasks() = %v, quería vacío", tasks)
	}
}

// TestPlanner_GoalsDevuelveLosRegistrados confirma que Goals() devuelve
// exactamente los Goals que se registraron con RegisterGoal, ni más ni
// menos.
func TestPlanner_GoalsDevuelveLosRegistrados(t *testing.T) {
	p := NewPlanner()
	g1 := NewGoal("Goal A", "desc", PriorityMedium, "user")
	g2 := NewGoal("Goal B", "desc", PriorityHigh, "ahmelia")
	// planning.Goal.generateID() no tiene el contador atómico que sí tiene
	// Task.generateTaskID() (ver comentario en task.go) — dos NewGoal()
	// en el mismo segundo de reloj producen el mismo ID. Es un bug
	// preexistente fuera del alcance de B90.2 (accessors read-only); se
	// fuerza acá un ID distinto para que este test no dependa de él.
	g2.ID = g2.ID + "-2"

	p.RegisterGoal(g1)
	p.RegisterGoal(g2)

	goals := p.Goals()
	if len(goals) != 2 {
		t.Fatalf("len(Goals()) = %d, quería 2", len(goals))
	}

	seen := map[string]bool{}
	for _, g := range goals {
		seen[g.ID] = true
	}
	if !seen[g1.ID] || !seen[g2.ID] {
		t.Fatalf("Goals() = %+v, faltan g1(%s) o g2(%s)", goals, g1.ID, g2.ID)
	}
}

// TestPlanner_TasksDevuelveLasRegistradas — mismo criterio que
// TestPlanner_GoalsDevuelveLosRegistrados, para Tasks().
func TestPlanner_TasksDevuelveLasRegistradas(t *testing.T) {
	p := NewPlanner()
	g := NewGoal("Goal A", "desc", PriorityMedium, "user")
	p.RegisterGoal(g)

	t1 := NewTask(g.ID, "Tarea 1", "desc", 1, time.Hour)
	t2 := NewTask(g.ID, "Tarea 2", "desc", 2, time.Hour)
	p.RegisterTask(t1)
	p.RegisterTask(t2)

	tasks := p.Tasks()
	if len(tasks) != 2 {
		t.Fatalf("len(Tasks()) = %d, quería 2", len(tasks))
	}

	seen := map[string]bool{}
	for _, task := range tasks {
		seen[task.ID] = true
	}
	if !seen[t1.ID] || !seen[t2.ID] {
		t.Fatalf("Tasks() = %+v, faltan t1(%s) o t2(%s)", tasks, t1.ID, t2.ID)
	}
}

// TestPlanner_GoalsPreservaIdentidad es la propiedad más importante de este
// bloque: los *Goal devueltos por Goals() deben ser los MISMOS punteros que
// se registraron, no copias. Mutar el objeto devuelto debe reflejarse en el
// Goal que el Planner sigue sosteniendo internamente — si esto fallara, B90
// terminaría persistiendo/reconstruyendo un Goal que ya no es el mismo
// objeto que ve AutonomousState/GoalPlanningAdapter.
func TestPlanner_GoalsPreservaIdentidad(t *testing.T) {
	p := NewPlanner()
	g := NewGoal("Goal A", "desc", PriorityMedium, "user")
	p.RegisterGoal(g)

	got := p.Goals()
	if len(got) != 1 {
		t.Fatalf("len(Goals()) = %d, quería 1", len(got))
	}
	if got[0] != g {
		t.Fatalf("Goals()[0] no es el mismo puntero que se registró")
	}

	got[0].AddNote("mutación a través del puntero devuelto")
	if len(g.Notes) != 1 || g.Notes[0] != "mutación a través del puntero devuelto" {
		t.Fatalf("la mutación sobre el puntero devuelto no se reflejó en el Goal original: %+v", g.Notes)
	}
}

// TestPlanner_TasksPreservaIdentidad — mismo criterio que
// TestPlanner_GoalsPreservaIdentidad, para Tasks().
func TestPlanner_TasksPreservaIdentidad(t *testing.T) {
	p := NewPlanner()
	g := NewGoal("Goal A", "desc", PriorityMedium, "user")
	p.RegisterGoal(g)
	task := NewTask(g.ID, "Tarea 1", "desc", 1, time.Hour)
	p.RegisterTask(task)

	got := p.Tasks()
	if len(got) != 1 {
		t.Fatalf("len(Tasks()) = %d, quería 1", len(got))
	}
	if got[0] != task {
		t.Fatalf("Tasks()[0] no es el mismo puntero que se registró")
	}

	got[0].Start()
	if task.Status != TaskInProgress {
		t.Fatalf("la mutación sobre el puntero devuelto no se reflejó en la Task original: %+v", task.Status)
	}
}

// TestPlanner_GoalsSliceEsIndependiente confirma que el SLICE devuelto es
// nuevo (agregar/quitar elementos del slice recibido no debe poder tocar el
// mapa interno del Planner) — distinto de la identidad de los *Goal en sí,
// que TestPlanner_GoalsPreservaIdentidad ya cubre.
func TestPlanner_GoalsSliceEsIndependiente(t *testing.T) {
	p := NewPlanner()
	g1 := NewGoal("Goal A", "desc", PriorityMedium, "user")
	p.RegisterGoal(g1)

	got := p.Goals()
	got = append(got, NewGoal("Goal fantasma", "no registrado", PriorityLow, "user"))

	if len(p.Goals()) != 1 {
		t.Fatalf("mutar el slice devuelto por Goals() afectó el estado interno del Planner: len=%d", len(p.Goals()))
	}
	_ = got
}

// Nota sobre concurrencia (ver decisión de diseño de B90.2): planning.Planner
// no tiene ningún mutex hoy — RegisterGoal/RegisterTask ya escriben sobre
// goals/tasks sin sincronización. Agregar un test -race que llame a
// Goals()/Tasks() concurrentemente con RegisterGoal/RegisterTask no
// probaría una garantía de B90.2: expondría una carrera de datos
// preexistente en Planner que este bloque no introduce ni tiene alcance
// para arreglar (eso sería inventar una garantía de concurrencia nueva que
// el Planner nunca ofreció). Por eso no hay un test de acceso concurrente
// acá — go test -race sobre este paquete sigue siendo válido para detectar
// si Goals()/Tasks() en sí mismas introdujeran una carrera con el uso
// secuencial normal, que es lo que los tests de arriba ya ejercitan.

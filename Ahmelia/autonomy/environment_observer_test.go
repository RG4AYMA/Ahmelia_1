package autonomy

import (
	"context"
	"sync"
	"testing"
	"time"
)

type mockEnvironmentSnapshotInspector struct {
	mu        sync.Mutex
	snapshots []EnvironmentSnapshot
	index     int
}

func (m *mockEnvironmentSnapshotInspector) Snapshot(ctx context.Context) (EnvironmentSnapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.index >= len(m.snapshots) {
		return m.snapshots[len(m.snapshots)-1], nil
	}

	s := m.snapshots[m.index]
	m.index++

	return s, nil
}

func TestEnvironmentObserver_FirstSnapshotProducesBaseline(t *testing.T) {
	inspector := &mockEnvironmentSnapshotInspector{
		snapshots: []EnvironmentSnapshot{{
			Timestamp: time.Now(), FocusedApp: "Chrome",
			OpenApps: []string{"Chrome", "VSCode"}, Files: []string{"project/main.go"},
			Processes: []string{"ollama"}, CPUUsage: 12, RAMUsage: 34,
		}},
	}
	observer := NewEnvironmentObserver(inspector)
	obs, err := observer.Observe(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(obs) != 1 {
		t.Fatalf("primer snapshot debe producir una observación de baseline; obtuvo %d", len(obs))
	}
	if obs[0].Type != TypeSystemSignal || obs[0].Payload["event"] != "baseline_snapshot" {
		t.Fatalf("baseline inesperado: %#v", obs[0])
	}
	if obs[0].Payload["focused_app"] != "Chrome" {
		t.Fatalf("aplicación enfocada perdida: %#v", obs[0].Payload)
	}
	if got := obs[0].Payload["files"].([]string); len(got) != 1 || got[0] != "project/main.go" {
		t.Fatalf("estado inicial de archivos perdido: %#v", got)
	}
}

func TestEnvironmentObserver_EmptyInitialSnapshotStillProducesBaseline(t *testing.T) {
	observer := NewEnvironmentObserver(&mockEnvironmentSnapshotInspector{snapshots: []EnvironmentSnapshot{{Timestamp: time.Now()}}})
	obs, err := observer.Observe(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(obs) != 1 || obs[0].Payload["event"] != "baseline_snapshot" {
		t.Fatalf("un entorno vacío sigue siendo una observación válida: %#v", obs)
	}
}

func TestEnvironmentObserver_BaselineDoesNotAliasInspectorSlices(t *testing.T) {
	snapshot := EnvironmentSnapshot{Timestamp: time.Now(), OpenApps: []string{"A"}, Files: []string{"before"}, Processes: []string{"P"}}
	observer := NewEnvironmentObserver(&mockEnvironmentSnapshotInspector{snapshots: []EnvironmentSnapshot{snapshot}})
	_, err := observer.Observe(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	snapshot.OpenApps[0] = "mutated"
	snapshot.Files[0] = "mutated"
	snapshot.Processes[0] = "mutated"
	last := observer.LastSnapshot()
	if last.OpenApps[0] != "A" || last.Files[0] != "before" || last.Processes[0] != "P" {
		t.Fatalf("snapshot almacenado comparte slices mutables: %#v", last)
	}
}

func TestEnvironmentObserver_FocusedApplicationChanges(t *testing.T) {
	inspector := &mockEnvironmentSnapshotInspector{
		snapshots: []EnvironmentSnapshot{
			{Timestamp: time.Now(), FocusedApp: "Chrome"},
			{Timestamp: time.Now(), FocusedApp: "VSCode"},
		},
	}

	observer := NewEnvironmentObserver(inspector)

	_, _ = observer.Observe(context.Background())

	obs, err := observer.Observe(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if len(obs) != 1 {
		t.Fatalf("esperaba una observación")
	}

	if obs[0].Type != TypeAppFocused {
		t.Fatalf("tipo incorrecto: %v", obs[0].Type)
	}

	if obs[0].Payload["app"] != "VSCode" {
		t.Fatalf("payload incorrecto")
	}
}

func TestEnvironmentObserver_FileChangesDetected(t *testing.T) {
	inspector := &mockEnvironmentSnapshotInspector{
		snapshots: []EnvironmentSnapshot{
			{Timestamp: time.Now(), Files: []string{"a.txt"}},
			{Timestamp: time.Now(), Files: []string{"a.txt", "b.txt"}},
		},
	}

	observer := NewEnvironmentObserver(inspector)

	_, _ = observer.Observe(context.Background())

	obs, err := observer.Observe(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if len(obs) != 1 {
		t.Fatalf("esperaba una observación")
	}

	if obs[0].Type != TypeFileChanged {
		t.Fatalf("tipo incorrecto")
	}
}

func TestEnvironmentObserver_SystemResourcesChanged(t *testing.T) {
	inspector := &mockEnvironmentSnapshotInspector{
		snapshots: []EnvironmentSnapshot{
			{
				Timestamp: time.Now(),
				CPUUsage:  10,
				RAMUsage:  20,
			},
			{
				Timestamp: time.Now(),
				CPUUsage:  30,
				RAMUsage:  20,
			},
		},
	}

	observer := NewEnvironmentObserver(inspector)

	_, _ = observer.Observe(context.Background())

	obs, err := observer.Observe(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if len(obs) != 1 {
		t.Fatalf("esperaba una observación")
	}

	if obs[0].Type != TypeSystemSignal {
		t.Fatalf("tipo incorrecto")
	}
}

func TestEnvironmentObserver_ContextCancellation(t *testing.T) {
	observer := NewEnvironmentObserver(nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := observer.Observe(ctx)

	if err == nil {
		t.Fatalf("esperaba error por contexto cancelado")
	}
}

func TestEnvironmentObserver_ConcurrentReadsAreSafe(t *testing.T) {
	inspector := &mockEnvironmentSnapshotInspector{
		snapshots: []EnvironmentSnapshot{
			{Timestamp: time.Now(), FocusedApp: "Chrome"},
			{Timestamp: time.Now(), FocusedApp: "Chrome"},
		},
	}

	observer := NewEnvironmentObserver(inspector)

	var wg sync.WaitGroup

	for i := 0; i < 20; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			_, _ = observer.Observe(context.Background())
			_ = observer.LastSnapshot()
		}()
	}

	wg.Wait()
}

func TestEnvironmentObserver_BaselineVariantsPreserveDistinctState(t *testing.T) {
	cases := []struct {
		name     string
		snapshot EnvironmentSnapshot
		key      string
		want     interface{}
	}{
		{name: "only focused application", snapshot: EnvironmentSnapshot{Timestamp: time.Now(), FocusedApp: "Blender"}, key: "focused_app", want: "Blender"},
		{name: "multiple open applications", snapshot: EnvironmentSnapshot{Timestamp: time.Now(), OpenApps: []string{"Unity", "VSCode"}}, key: "open_apps", want: []string{"Unity", "VSCode"}},
		{name: "project files already exist", snapshot: EnvironmentSnapshot{Timestamp: time.Now(), Files: []string{"Assets/scene.unity", "src/main.go"}}, key: "files", want: []string{"Assets/scene.unity", "src/main.go"}},
		{name: "existing process", snapshot: EnvironmentSnapshot{Timestamp: time.Now(), Processes: []string{"ollama"}}, key: "processes", want: []string{"ollama"}},
		{name: "resource-only state", snapshot: EnvironmentSnapshot{Timestamp: time.Now(), CPUUsage: 21, RAMUsage: 63}, key: "ram_usage", want: float64(63)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			observer := NewEnvironmentObserver(&mockEnvironmentSnapshotInspector{snapshots: []EnvironmentSnapshot{tc.snapshot}})
			observations, err := observer.Observe(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if len(observations) != 1 {
				t.Fatalf("baseline ausente: %#v", observations)
			}
			got := observations[0].Payload[tc.key]
			switch want := tc.want.(type) {
			case []string:
				actual, ok := got.([]string)
				if !ok || len(actual) != len(want) {
					t.Fatalf("%s: esperado %#v, obtenido %#v", tc.key, want, got)
				}
				for i := range want {
					if actual[i] != want[i] {
						t.Fatalf("%s: esperado %#v, obtenido %#v", tc.key, want, got)
					}
				}
			default:
				if got != want {
					t.Fatalf("%s: esperado %#v, obtenido %#v", tc.key, want, got)
				}
			}
		})
	}
}

func TestEnvironmentObserver_DetectsWindowFocusTitleAndClosureTransitions(t *testing.T) {
	base := time.Now()
	inspector := &mockEnvironmentSnapshotInspector{snapshots: []EnvironmentSnapshot{
		{Timestamp: base, CoverageKnown: true, ObservedDomains: []string{"windows", "processes", "focus"}, FocusedApp: "chrome", OpenApps: []string{"chrome", "code"}, Processes: []string{"chrome#10", "code#20"}, Windows: []WindowObservation{{ProcessID: 10, ProcessName: "chrome", Title: "Home", Focused: true}, {ProcessID: 20, ProcessName: "code", Title: "main.go"}}},
		{Timestamp: base.Add(time.Second), CoverageKnown: true, ObservedDomains: []string{"windows", "processes", "focus"}, FocusedApp: "code", OpenApps: []string{"code"}, Processes: []string{"code#20"}, Windows: []WindowObservation{{ProcessID: 20, ProcessName: "code", Title: "main_test.go", Focused: true}}},
	}}
	observer := NewEnvironmentObserver(inspector)
	if _, err := observer.Observe(context.Background()); err != nil {
		t.Fatal(err)
	}
	observations, err := observer.Observe(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	events := map[string]bool{}
	for _, observation := range observations {
		if event, ok := observation.Payload["event"].(string); ok {
			events[event] = true
		}
		if observation.Type == TypeAppFocused && observation.Payload["app"] == "code" {
			events["focus_changed"] = true
		}
	}
	for _, expected := range []string{"closed", "process_stopped", "window_changed"} {
		if !events[expected] {
			t.Errorf("falta evento %q; observaciones=%#v", expected, observations)
		}
	}
	if !events["focus_changed"] {
		t.Errorf("falta cambio de foco; observaciones=%#v", observations)
	}
}

func TestEnvironmentObserver_UnavailableWindowDomainDoesNotInferClosure(t *testing.T) {
	base := time.Now()
	inspector := &mockEnvironmentSnapshotInspector{snapshots: []EnvironmentSnapshot{
		{Timestamp: base, CoverageKnown: true, ObservedDomains: []string{"windows"}, OpenApps: []string{"chrome"}},
		{Timestamp: base.Add(time.Second), CoverageKnown: true, UnavailableDomains: []string{"windows"}, OpenApps: nil},
		{Timestamp: base.Add(2 * time.Second), CoverageKnown: true, ObservedDomains: []string{"windows"}, OpenApps: nil},
		{Timestamp: base.Add(3 * time.Second), CoverageKnown: true, ObservedDomains: []string{"windows"}, OpenApps: nil},
	}}
	observer := NewEnvironmentObserver(inspector)
	_, _ = observer.Observe(context.Background())
	obs, err := observer.Observe(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range obs {
		if item.Payload["event"] == "closed" {
			t.Fatalf("fallo de observación convertido en cierre: %#v", item)
		}
	}
	_, _ = observer.Observe(context.Background()) // primera lectura válida vuelve a establecer baseline
	obs, err = observer.Observe(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range obs {
		if item.Payload["event"] == "closed" {
			t.Fatalf("cierre inferido usando baseline de antes de la pérdida: %#v", item)
		}
	}
}

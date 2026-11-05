package capability

import (
	"sync"
	"testing"
)

func testCapability(id ID, domain Domain, action string) Capability {
	return Capability{
		ID:          id,
		Domain:      domain,
		Description: "capability de prueba",
		Inputs: []Parameter{
			{Name: "input", Description: "entrada", Required: true},
		},
		Permission: PermissionAutomatic,
		Action:     action,
	}
}

func TestRegistry_RegisterAndGet(t *testing.T) {
	r := NewRegistry()
	c := testCapability("crear_carpeta", DomainLocal, "crear_carpeta")

	if err := r.Register(c); err != nil {
		t.Fatalf("Register() = %v, esperaba nil", err)
	}

	got, ok := r.Get("crear_carpeta")
	if !ok {
		t.Fatal("Get() no encontró una capability registrada")
	}
	if got.ID != c.ID || got.Domain != c.Domain || got.Action != c.Action {
		t.Fatalf("Get() = %+v, esperaba %+v", got, c)
	}
}

func TestRegistry_RejectsInvalidCapability(t *testing.T) {
	r := NewRegistry()

	err := r.Register(Capability{
		ID:         "x",
		Domain:     DomainLocal,
		Action:     "hacer",
		Permission: "inventado",
	})
	if err == nil {
		t.Fatal("Register() = nil, esperaba rechazo de Permission inválido")
	}
	if r.Count() != 0 {
		t.Fatalf("Count() = %d, esperaba 0 tras rechazar capability inválida", r.Count())
	}
}

func TestRegistry_RejectsDuplicateID(t *testing.T) {
	r := NewRegistry()
	first := testCapability("misma_id", DomainLocal, "accion_1")
	second := testCapability("misma_id", DomainUnity, "accion_2")

	if err := r.Register(first); err != nil {
		t.Fatalf("primer Register() = %v", err)
	}
	if err := r.Register(second); err == nil {
		t.Fatal("segundo Register() = nil, esperaba error por ID duplicado")
	}

	got, ok := r.Get("misma_id")
	if !ok {
		t.Fatal("la primera capability debería seguir registrada")
	}
	if got.Action != "accion_1" || got.Domain != DomainLocal {
		t.Fatalf("la capability original fue reemplazada: %+v", got)
	}
}

func TestRegistry_List_IsDeterministic(t *testing.T) {
	r := NewRegistry()

	r.MustRegister(testCapability("unity_play", DomainUnity, "reproducir"))
	r.MustRegister(testCapability("local_move", DomainLocal, "mover_archivo"))
	r.MustRegister(testCapability("blender_export", DomainBlender, "exportar"))
	r.MustRegister(testCapability("local_create", DomainLocal, "crear_carpeta"))

	got := r.List()
	want := []ID{"blender_export", "local_create", "local_move", "unity_play"}

	if len(got) != len(want) {
		t.Fatalf("List() devolvió %d elementos, esperaba %d", len(got), len(want))
	}
	for i, c := range got {
		if c.ID != want[i] {
			t.Errorf("List()[%d].ID = %q, esperaba %q", i, c.ID, want[i])
		}
	}
}

func TestRegistry_ByDomain(t *testing.T) {
	r := NewRegistry()
	r.MustRegister(testCapability("unity_play", DomainUnity, "reproducir"))
	r.MustRegister(testCapability("unity_stop", DomainUnity, "detener"))
	r.MustRegister(testCapability("local_move", DomainLocal, "mover_archivo"))

	got := r.ByDomain(DomainUnity)
	if len(got) != 2 {
		t.Fatalf("ByDomain(unity) devolvió %d, esperaba 2", len(got))
	}
	if got[0].ID != "unity_play" || got[1].ID != "unity_stop" {
		t.Fatalf("ByDomain(unity) = %+v, esperaba orden por ID", got)
	}
}

func TestRegistry_Find(t *testing.T) {
	r := NewRegistry()
	r.MustRegister(testCapability("unity_play", DomainUnity, "reproducir"))
	r.MustRegister(testCapability("unity_stop", DomainUnity, "detener"))
	r.MustRegister(testCapability("local_play", DomainLocal, "reproducir"))

	got := r.Find(DomainUnity, "reproducir")
	if len(got) != 1 {
		t.Fatalf("Find() devolvió %d, esperaba 1", len(got))
	}
	if got[0].ID != "unity_play" {
		t.Fatalf("Find() devolvió %q, esperaba unity_play", got[0].ID)
	}
}

func TestRegistry_GetReturnsCopy(t *testing.T) {
	r := NewRegistry()
	r.MustRegister(Capability{
		ID:         "copy_test",
		Domain:     DomainLocal,
		Action:     "crear_carpeta",
		Permission: PermissionAutomatic,
		Inputs: []Parameter{
			{Name: "ruta", Description: "ruta", Required: true},
		},
		Preconditions: []string{"padre existe"},
	})

	got, ok := r.Get("copy_test")
	if !ok {
		t.Fatal("Get() no encontró capability")
	}

	got.Inputs[0].Name = "mutado"
	got.Preconditions[0] = "mutado"

	again, _ := r.Get("copy_test")
	if again.Inputs[0].Name != "ruta" {
		t.Fatal("Get() expuso la slice interna de Inputs")
	}
	if again.Preconditions[0] != "padre existe" {
		t.Fatal("Get() expuso la slice interna de Preconditions")
	}
}

func TestRegistry_ConcurrentAccess(t *testing.T) {
	r := NewRegistry()

	const n = 50
	var wg sync.WaitGroup
	wg.Add(n * 2)

	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			r.Register(testCapability(ID("cap_"+string(rune('a'+i))), DomainLocal, "accion"))
		}(i)

		go func() {
			defer wg.Done()
			_ = r.List()
			_ = r.Count()
			_, _ = r.Get("missing")
			_ = r.Has("missing")
		}()
	}

	wg.Wait()

	if r.Count() != n {
		t.Fatalf("Count() = %d, esperaba %d", r.Count(), n)
	}
}

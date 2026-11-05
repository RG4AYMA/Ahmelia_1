package planning

import "testing"

func TestParseCreateProjectQuery_DetectaYExtraeNombre(t *testing.T) {
	name, ok := ParseCreateProjectQuery("quiero crear un proyecto para las gafas xr")
	if !ok {
		t.Fatal("esperaba detectar el trigger de crear proyecto")
	}
	if name != "las gafas xr" {
		t.Errorf("esperaba name='las gafas xr', obtuve %q", name)
	}
}

func TestParseCreateProjectQuery_SinTriggerNoDetecta(t *testing.T) {
	if _, ok := ParseCreateProjectQuery("como esta el clima hoy"); ok {
		t.Error("no debería detectar un trigger que no está")
	}
}

func TestParseCreateProjectQuery_TriggerSinNombreNoEsValido(t *testing.T) {
	if _, ok := ParseCreateProjectQuery("quiero planificar"); ok {
		t.Error("un trigger sin nada después no debería ser válido — no hay nombre de proyecto")
	}
}

func TestParseProjectStatusQuery_DetectaYExtraeNombre(t *testing.T) {
	name, ok := ParseProjectStatusQuery("como vamos con las gafas xr")
	if !ok {
		t.Fatal("esperaba detectar el trigger de consulta de progreso")
	}
	if name != "las gafas xr" {
		t.Errorf("esperaba name='las gafas xr', obtuve %q", name)
	}
}

func TestParseCompleteTaskQuery_DetectaYExtraeNombre(t *testing.T) {
	name, ok := ParseCompleteTaskQuery("termine una tarea de las gafas xr")
	if !ok {
		t.Fatal("esperaba detectar el trigger de tarea completada")
	}
	if name != "las gafas xr" {
		t.Errorf("esperaba name='las gafas xr', obtuve %q", name)
	}
}

func TestParsePlanningQueries_NoSePisanEntreSi(t *testing.T) {
	if _, ok := ParseProjectStatusQuery("quiero crear un proyecto para x"); ok {
		t.Error("un pedido de crear proyecto no debería disparar consulta de progreso")
	}
	if _, ok := ParseCompleteTaskQuery("como vamos con x"); ok {
		t.Error("una consulta de progreso no debería disparar tarea completada")
	}
	if _, ok := ParseCreateProjectQuery("termine una tarea de x"); ok {
		t.Error("un reporte de tarea completada no debería disparar crear proyecto")
	}
}
